package convert_test

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dhcgn/jxleet/internal/cjxl"
	"github.com/dhcgn/jxleet/internal/convert"
	"github.com/dhcgn/jxleet/internal/djxl"
	"github.com/dhcgn/jxleet/internal/preset"
)

func findBin(t *testing.T, env, name string) string {
	t.Helper()
	if p := os.Getenv(env); p != "" {
		return p
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	t.Skipf("%s not found (set %s)", name, env)
	return ""
}

func writePNG(t *testing.T, path string, seed int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 24, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			img.Set(x, y, color.RGBA{uint8(x*10 + seed), uint8(y * 10), uint8(seed * 2), 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestEngineEndToEnd(t *testing.T) {
	cjxlBin := findBin(t, "JXLEET_CJXL", "cjxl")
	djxlBin := findBin(t, "JXLEET_DJXL", "djxl")

	dir := t.TempDir()
	var inputs []string
	for i := 0; i < 5; i++ {
		p := filepath.Join(dir, string(rune('a'+i))+".png")
		writePNG(t, p, i)
		inputs = append(inputs, p)
	}

	ps := preset.Preset{
		Name:    "e2e",
		Version: 1,
		Output:  preset.Output{Policy: preset.PolicyAlongside, OnCollision: preset.CollisionOverwrite},
		Rules:   []preset.Rule{{Match: []string{"*"}, Args: []cjxl.Arg{{Key: "-d", Value: "0"}, {Key: "-e", Value: "3"}}}},
	}

	e := convert.New(
		convert.Deps{Encoder: cjxl.NewRunner(cjxlBin), Verifier: djxl.NewVerifier(djxlBin)},
		convert.Settings{Processes: 2, Threads: 2, Preset: ps},
	)
	sum := e.Run(context.Background(), inputs)
	if sum.Completed != 5 || sum.Failed != 0 {
		t.Fatalf("summary = %+v", sum)
	}
	for _, in := range inputs {
		out := in[:len(in)-len(".png")] + ".jxl"
		fi, err := os.Stat(out)
		if err != nil || fi.Size() == 0 {
			t.Errorf("bad output for %s: %v", in, err)
		}
	}
}
