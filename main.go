package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/dhcgn/jxleet/internal/app"
	"github.com/dhcgn/jxleet/internal/config"
	"github.com/dhcgn/jxleet/internal/ipc"
	"github.com/dhcgn/jxleet/internal/toolchain"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend. See https://pkg.go.dev/embed for details.
//
//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// Files handed over from secondary invocations are delivered to the frontend
	// as this event; registering it gives the binding generator a typed API.
	application.RegisterEvent[[]string]("files")
	application.RegisterEvent[app.ProgressUpdate]("progress")
	application.RegisterEvent[app.FileUpdate]("conversion-file")
	application.RegisterEvent[app.ConversionSummary]("conversion-done")
	application.RegisterEvent[string]("conversion-error")
}

func main() {
	paths, err := config.ResolvePaths()
	if err != nil {
		log.Fatal(err)
	}
	if err := paths.EnsureDirs(); err != nil {
		log.Fatal(err)
	}

	// Load persisted config (entry-point bindings). Missing file -> defaults.
	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		log.Fatal(err)
	}

	inputs, presetOverride := parseArgs(os.Args[1:])

	// Single-instance: become the owner, or hand our paths to the running
	// instance and exit immediately (so callers like Lightroom are not blocked).
	server, handedOver, err := ipc.Acquire(ipc.Message{Paths: inputs, Preset: presetOverride}, 500*time.Millisecond)
	if err != nil {
		// Could neither own nor reach an owner; continue standalone without IPC.
		log.Printf("ipc: %v (continuing without single-instance handover)", err)
	}
	if handedOver {
		return
	}

	var wailsApp *application.App
	svc := app.New(paths, cfg, toolchain.NewManager(paths.BinDir), app.Callbacks{
		Emit: func(name string, data any) {
			if wailsApp != nil {
				wailsApp.Event.Emit(name, data)
			}
		},
		OpenFiles: func() ([]string, error) {
			if wailsApp == nil {
				return nil, fmt.Errorf("application is not initialized")
			}
			return wailsApp.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
				Title:                   "Select images or folders",
				CanChooseFiles:          true,
				CanChooseDirectories:    true,
				AllowsMultipleSelection: true,
				AllowsOtherFileTypes:    true,
			}).PromptForMultipleSelection()
		},
	})
	svc.AddPaths(inputs)

	wailsApp = application.New(application.Options{
		Name:        "jxleet",
		Description: "JPEG-XL-Expert-Encoding-Tool — a comfortable way to use cjxl on Windows.",
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "jxleet",
		// Window sized to the golden ratio (1000 / 618 ≈ 1.618).
		Width:            1000,
		Height:           618,
		MinWidth:         420,
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})

	// Coalesce handovers from later invocations into this running instance.
	if server != nil {
		go server.Serve(func(m ipc.Message) {
			svc.AddPaths(m.Paths)
		})
		defer server.Close()
	}

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}

// parseArgs performs the minimal argument parsing needed at startup: everything
// that is not a flag is treated as an input path, and --preset[=name] is
// captured as an override. Full CLI handling lives in a later phase.
func parseArgs(args []string) (paths []string, presetOverride string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--preset" && i+1 < len(args):
			presetOverride = args[i+1]
			i++
		case strings.HasPrefix(a, "--preset="):
			presetOverride = strings.TrimPrefix(a, "--preset=")
		case strings.HasPrefix(a, "-"):
			// Ignore unrecognised flags for now (handled in the CLI phase).
		default:
			paths = append(paths, a)
		}
	}
	return paths, presetOverride
}
