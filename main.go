package main

import (
	"embed"
	"log"

	"github.com/dhcgn/jxleet/internal/app"
	"github.com/dhcgn/jxleet/internal/config"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend. See https://pkg.go.dev/embed for details.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	paths, err := config.ResolvePaths()
	if err != nil {
		log.Fatal(err)
	}
	if err := paths.EnsureDirs(); err != nil {
		log.Fatal(err)
	}

	// TODO(config): load config.yaml from paths.ConfigFile. Defaults for now.
	cfg := config.Default()

	svc := app.New(paths, cfg)

	wailsApp := application.New(application.Options{
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

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
