package djxl_test

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dhcgn/jxleet/internal/cjxl"
	"github.com/dhcgn/jxleet/internal/djxl"
)

func findBin(t *testing.T, env, name string) string {
	t.Helper()
	if p := os.Getenv(env); p != "" {
		return p
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	t.Skipf("%s not found (set %s or add %s to PATH)", name, env, name)
	return ""
}

func writeJPEG(t *testing.T, path string, seed int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{uint8(x*8 + seed), uint8(y * 8), uint8(seed), 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
}

func TestVerifierReconstructsTranscode(t *testing.T) {
	cjxlBin := findBin(t, "JXLEET_CJXL", "cjxl")
	djxlBin := findBin(t, "JXLEET_DJXL", "djxl")

	dir := t.TempDir()
	orig := filepath.Join(dir, "orig.jpg")
	out := filepath.Join(dir, "out.jxl")
	writeJPEG(t, orig, 0)

	// Lossless transcode (the reversible route).
	res := cjxl.NewRunner(cjxlBin).Run(context.Background(),
		[]cjxl.Arg{{Key: "--lossless_jpeg", Value: "1"}}, orig, out)
	if !res.Success() {
		t.Fatalf("transcode failed: %s", res.Stderr)
	}

	v := djxl.NewVerifier(djxlBin)
	if err := v.Readable(context.Background(), out); err != nil {
		t.Fatalf("readable check failed: %v", err)
	}
	if err := v.Reconstructs(context.Background(), out, orig); err != nil {
		t.Fatalf("reconstruction should be byte-identical: %v", err)
	}

	// A different original must not be reported as a match.
	other := filepath.Join(dir, "other.jpg")
	writeJPEG(t, other, 99)
	if err := v.Reconstructs(context.Background(), out, other); err == nil {
		t.Fatal("reconstruction against a different original should fail")
	}
}

func TestVerifierReadableEncode(t *testing.T) {
	cjxlBin := findBin(t, "JXLEET_CJXL", "cjxl")
	djxlBin := findBin(t, "JXLEET_DJXL", "djxl")

	dir := t.TempDir()
	in := filepath.Join(dir, "in.ppm")
	out := filepath.Join(dir, "out.jxl")
	header := []byte("P6\n4 4\n255\n")
	pix := make([]byte, 4*4*3)
	for i := range pix {
		pix[i] = byte(i * 3)
	}
	if err := os.WriteFile(in, append(header, pix...), 0o644); err != nil {
		t.Fatal(err)
	}

	res := cjxl.NewRunner(cjxlBin).Run(context.Background(),
		[]cjxl.Arg{{Key: "-d", Value: "0"}, {Key: "-e", Value: "3"}}, in, out)
	if !res.Success() {
		t.Fatalf("encode failed: %s", res.Stderr)
	}
	if err := djxl.NewVerifier(djxlBin).Readable(context.Background(), out); err != nil {
		t.Fatalf("readable check failed: %v", err)
	}
}
