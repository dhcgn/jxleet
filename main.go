// jxleet: a Windows front end for libjxl's cjxl. It decides which files to
// hand over, assembles the cjxl arguments, runs the process, verifies the
// result and reports — it encodes nothing itself (see README.md).
package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/dhcgn/jxleet/internal/app"
	"github.com/dhcgn/jxleet/internal/cli"
	"github.com/dhcgn/jxleet/internal/config"
	"github.com/dhcgn/jxleet/internal/ipc"
	"github.com/dhcgn/jxleet/internal/preset"
	"github.com/dhcgn/jxleet/internal/shellext"
	"github.com/dhcgn/jxleet/internal/toolchain"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/updater"
	githubprovider "github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend. See https://pkg.go.dev/embed for details.
//
//go:embed all:frontend/dist
var assets embed.FS

// version is stamped at release time via -ldflags "-X main.version=v1.2.3";
// local and CI builds report "dev".
var version = "dev"

func init() {
	// Files handed over from secondary invocations are delivered to the frontend
	// as this event; registering it gives the binding generator a typed API.
	application.RegisterEvent[[]string]("files")
	application.RegisterEvent[app.ProgressUpdate]("progress")
	application.RegisterEvent[app.FileUpdate]("conversion-file")
	application.RegisterEvent[app.ConversionSummary]("conversion-done")
	application.RegisterEvent[string]("conversion-error")
	application.RegisterEvent[string]("preset")
	application.RegisterEvent[app.ToolchainProgress]("toolchain-progress")
	application.RegisterEvent[app.CollisionPrompt]("collision-prompt")
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
		fmt.Println("jxleet", version)
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
	presetStore := preset.NewStore(paths.PresetsDir)
	if _, err := preset.EnsureDefaults(presetStore); err != nil {
		log.Fatal(err)
	}
	if err := presetStore.EnsureSchema(); err != nil {
		log.Printf("preset: could not write schema: %v", err)
	}
	_, configFileErr := os.Stat(paths.ConfigFile)
	legacyDefault := false
	if legacy, err := presetStore.Load(config.LegacyDefaultPresetName); err == nil {
		legacyDefault = legacy.ReadOnly
	}
	if ensureDefaultBindings(&cfg, legacyDefault) || errors.Is(configFileErr, os.ErrNotExist) {
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
	svc := app.New(paths, cfg, toolchain.NewManager(paths.BinDir), version, app.Callbacks{
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
				CanChooseDirectories:    false,
				AllowsMultipleSelection: true,
				AllowsOtherFileTypes:    true,
			}).PromptForMultipleSelection()
		},
		OpenFolders: func() ([]string, error) {
			if wailsApp == nil {
				return nil, fmt.Errorf("application is not initialized")
			}
			return wailsApp.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
				Title:                   "Select folders",
				CanChooseFiles:          false,
				CanChooseDirectories:    true,
				AllowsMultipleSelection: true,
			}).PromptForMultipleSelection()
		},
		// App updates are notify-only: these run only for the startup banner
		// (silent Check, no window) and the user-triggered update window.
		// wailsApp is assigned below before any binding can fire.
		CheckAppUpdate: func(ctx context.Context) (app.Update, error) {
			if wailsApp == nil {
				return app.Update{}, fmt.Errorf("application is not initialized")
			}
			rel, err := wailsApp.Updater.Check(ctx)
			if err != nil || rel == nil {
				return app.Update{}, err
			}
			latest := strings.TrimPrefix(rel.Version, "v")
			update := app.Update{Latest: "v" + latest, Available: true}
			if url, ok := rel.Metadata["github.release.htmlURL"].(string); ok {
				update.URL = url
			}
			return update, nil
		},
		InstallAppUpdate: func(ctx context.Context) error {
			if wailsApp == nil {
				return fmt.Errorf("application is not initialized")
			}
			return wailsApp.Updater.CheckAndInstall(ctx)
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

	window := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "jxleet " + version,
		// Window sized to the golden ratio (1000 / 618 ≈ 1.618).
		Width:            1000,
		Height:           618,
		MinWidth:         420,
		Hidden:           true,
		EnableFileDrop:   true,
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})

	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		if event.Context() != nil {
			svc.AddPaths(event.Context().DroppedFiles())
		}
	})
	window.OnWindowEvent(events.Windows.WebViewNavigationCompleted, func(_ *application.WindowEvent) {
		window.Show()
	})

	// App updates are notify-only: the updater is initialized without a
	// CheckInterval, so nothing is ever checked or downloaded automatically.
	// Dev builds skip this entirely and never report updates.
	if version != "" && version != "dev" {
		if err := initAppUpdater(wailsApp, version); err != nil {
			log.Printf("updater: %v (app update checks disabled)", err)
		}
	}

	// Coalesce handovers from later invocations into this running instance.
	if server != nil {
		go server.Serve(func(m ipc.Message) {
			svc.ReceivePaths(m.Paths, m.Preset)
		})
		defer func() { _ = server.Close() }()
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

// initAppUpdater configures the Wails self-updater against this project's
// GitHub releases. The release workflow already publishes SHA256SUMS next to
// the zip, so downloads are checksum-verified with no release-format changes.
// The default asset matcher picks jxleet_<version>_windows_amd64.zip by
// platform/arch substring, and Prerelease stays false so beta tags never
// disturb stable users. The updater expects the version without the leading
// "v" that release tags carry.
// Ed25519 signing (updater.Config.PublicKey) is deliberately deferred: it
// needs a signing step in release.yml first.
func initAppUpdater(wailsApp *application.App, version string) error {
	gh, err := githubprovider.New(githubprovider.Config{
		Repository:    "dhcgn/jxleet",
		ChecksumAsset: "SHA256SUMS",
	})
	if err != nil {
		return fmt.Errorf("updater: github provider: %w", err)
	}
	if err := wailsApp.Updater.Init(updater.Config{
		CurrentVersion: strings.TrimPrefix(version, "v"),
		Providers:      []updater.Provider{gh},
	}); err != nil {
		return fmt.Errorf("updater: init: %w", err)
	}
	return nil
}

func ensureDefaultBindings(cfg *config.Config, migrateLegacy bool) bool {
	changed := false
	if cfg.Bindings == nil {
		cfg.Bindings = map[config.EntryPoint]string{}
	}
	for _, entryPoint := range []config.EntryPoint{config.EntryGUI, config.EntryCLI, config.EntryContextMenu} {
		if cfg.Bindings[entryPoint] == "" || (migrateLegacy && cfg.Bindings[entryPoint] == config.LegacyDefaultPresetName) {
			cfg.Bindings[entryPoint] = config.DefaultPresetFor(entryPoint)
			changed = true
		}
	}
	return changed
}
