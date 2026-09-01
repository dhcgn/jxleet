package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dhcgn/jxleet/internal/cjxl"
	"github.com/dhcgn/jxleet/internal/config"
	"github.com/dhcgn/jxleet/internal/preset"
	"github.com/dhcgn/jxleet/internal/toolchain"
)

func TestStartConversionWithRealToolchain(t *testing.T) {
	archivePath := os.Getenv("JXLEET_TOOLCHAIN_ZIP")
	if archivePath == "" {
		t.Skip("set JXLEET_TOOLCHAIN_ZIP to run the Wails service conversion integration test")
	}
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archive)

	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer download.Close()

	root := t.TempDir()
	paths := config.Paths{
		ConfigDir:  filepath.Join(root, "config"),
		ConfigFile: filepath.Join(root, "config", "config.yaml"),
		PresetsDir: filepath.Join(root, "config", "presets"),
		BinDir:     filepath.Join(root, "local", "bin"),
		LogsDir:    filepath.Join(root, "local", "logs"),
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	manager := toolchain.NewManager(paths.BinDir)
	manager.Client = toolchain.NewGitHubClient()
	release := toolchain.Release{
		TagName: "v0.12.0",
		Version: "0.12.0",
		Asset: toolchain.Asset{
			Name:        "jxl-x64-windows-static.zip",
			DownloadURL: download.URL,
			Digest:      "sha256:" + hex.EncodeToString(sum[:]),
			Size:        int64(len(archive)),
		},
	}
	if _, err := manager.InstallRelease(context.Background(), release); err != nil {
		t.Fatal(err)
	}

	if err := preset.NewStore(paths.PresetsDir).Save(preset.Preset{
		Name:    "test",
		Version: preset.CurrentVersion,
		Output:  preset.DefaultOutput(),
		Rules: []preset.Rule{
			{Match: []string{"JPEG"}, Args: []cjxl.Arg{{Key: "--lossless_jpeg", Value: "1"}, {Key: "-e", Value: "3"}}},
			{Match: []string{"*"}, Args: []cjxl.Arg{{Key: "-d", Value: "0"}, {Key: "-e", Value: "3"}}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	done := make(chan ConversionSummary, 1)
	service := New(paths, config.Default(), manager, "dev", Callbacks{
		Emit: func(name string, data any) {
			if name == "conversion-done" {
				done <- data.(ConversionSummary)
			}
		},
	})
	input := filepath.Join(root, "input.png")
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 20), uint8(y * 20), 100, 255})
		}
	}
	file, err := os.Create(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, img); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	jpegInput := filepath.Join(root, "input-jpeg.jpg")
	jpegFile, err := os.Create(jpegInput)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(jpegFile, img, &jpeg.Options{Quality: 90}); err != nil {
		_ = jpegFile.Close()
		t.Fatal(err)
	}
	if err := jpegFile.Close(); err != nil {
		t.Fatal(err)
	}

	err = service.StartConversion([]string{input, jpegInput}, ConversionOptions{
		Preset:       "test",
		Processes:    1,
		Threads:      2,
		JPEGMode:     "transcode",
		Distance:     0,
		UseDistance:  true,
		Effort:       3,
		UseEffort:    true,
		OutputPolicy: string(preset.PolicyAlongside),
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case result := <-done:
		if result.Completed != 2 || result.Failed != 0 {
			t.Fatalf("conversion summary = %+v", result)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("conversion did not finish")
	}
	if _, err := os.Stat(filepath.Join(root, "input.jxl")); err != nil {
		t.Fatalf("expected output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "input-jpeg.jxl")); err != nil {
		t.Fatalf("expected JPEG output: %v", err)
	}
}
