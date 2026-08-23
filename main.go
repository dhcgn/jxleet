package main

import (
	"embed"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/dhcgn/jxleet/internal/app"
	"github.com/dhcgn/jxleet/internal/cli"
	"github.com/dhcgn/jxleet/internal/config"
	"github.com/dhcgn/jxleet/internal/ipc"
	"github.com/dhcgn/jxleet/internal/preset"
	"github.com/dhcgn/jxleet/internal/shellext"
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
	application.RegisterEvent[string]("preset")
}

func main() {
	arguments, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "jxleet:", err)
		fmt.Fprintln(os.Stderr, cli.Usage())
		os.Exit(2)
	}
	if arguments.Help {
		fmt.Print(cli.Usage())
		return
	}
	if arguments.Version {
		fmt.Println("jxleet 0.0.1")
		return
	}

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
	if _, err := preset.EnsureDefault(preset.NewStore(paths.PresetsDir)); err != nil {
		log.Fatal(err)
	}
	_, configFileErr := os.Stat(paths.ConfigFile)
	if ensureDefaultBindings(&cfg) || errors.Is(configFileErr, os.ErrNotExist) {
		if err := config.Save(paths.ConfigFile, cfg); err != nil {
			log.Fatal(err)
		}
	}

	if arguments.RegisterContextMenu || arguments.UnregisterContextMenu {
		if arguments.UnregisterContextMenu {
			if err := shellext.Unregister(); err != nil {
				log.Fatal(err)
			}
			return
		}
		presetName := cfg.Bindings[config.EntryContextMenu]
		if presetName == "" {
			log.Fatal("context-menu preset is not bound")
		}
		if _, err := preset.NewStore(paths.PresetsDir).Load(presetName); err != nil {
			log.Fatal(err)
		}
		executable, err := os.Executable()
		if err != nil {
			log.Fatal(err)
		}
		if err := shellext.Register(executable, presetName); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Single-instance: become the owner, or hand our paths to the running
	// instance and exit immediately (so callers like Lightroom are not blocked).
	server, handedOver, err := ipc.Acquire(ipc.Message{Paths: arguments.Paths, Preset: arguments.Preset}, 500*time.Millisecond)
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
			svc.ReceivePaths(m.Paths, m.Preset)
		})
		defer server.Close()
	}

	if len(arguments.Paths) > 0 {
		presetName := arguments.Preset
		if presetName == "" {
			presetName = cfg.Bindings[config.EntryCLI]
		}
		if err := svc.StartConversion(arguments.Paths, app.ConversionOptions{Preset: presetName}); err != nil {
			log.Fatal(err)
		}
	}

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}

func ensureDefaultBindings(cfg *config.Config) bool {
	changed := false
	if cfg.Bindings == nil {
		cfg.Bindings = map[config.EntryPoint]string{}
	}
	for _, entryPoint := range []config.EntryPoint{config.EntryGUI, config.EntryCLI, config.EntryContextMenu} {
		if cfg.Bindings[entryPoint] == "" {
			cfg.Bindings[entryPoint] = config.DefaultPresetName
			changed = true
		}
	}
	return changed
}
