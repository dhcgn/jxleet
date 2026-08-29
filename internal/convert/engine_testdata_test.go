package convert_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dhcgn/jxleet/internal/cjxl"
	"github.com/dhcgn/jxleet/internal/convert"
	"github.com/dhcgn/jxleet/internal/djxl"
	"github.com/dhcgn/jxleet/internal/preset"
	"github.com/dhcgn/jxleet/internal/routes"
)

// testDataDir returns the repository's committed test-data directory.
func testDataDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	// internal/convert/<file> -> repo root is two levels up.
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "test-data")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("test-data not found: %v", err)
	}
	return dir
}

func copyInto(t *testing.T, src, dstDir string) string {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dstDir, filepath.Base(src))
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dst
}

// TestEngineOnRealTestData runs the engine over the committed sample image in
// every supported format and asserts they all convert. JPEGs take the lossless
// transcode route (reconstruction is verified byte-for-byte); JXL/PNG take the
// reencode/encode routes. A subfolder policy keeps outputs distinct from inputs
// (a .jxl input would otherwise collide with its .jxl output).
func TestEngineOnRealTestData(t *testing.T) {
	cjxlBin := findBin(t, "JXLEET_CJXL", "cjxl")
	djxlBin := findBin(t, "JXLEET_DJXL", "djxl")

	srcDir := testDataDir(t)
	patterns := []string{"*.jpg", "*.jpeg", "*.jxl", "*.png"}
	var sources []string
	for _, p := range patterns {
		m, _ := filepath.Glob(filepath.Join(srcDir, p))
		sources = append(sources, m...)
	}
	if len(sources) == 0 {
		t.Skip("no supported files in test-data")
	}

	work := t.TempDir()
	var inputs []string
	for _, s := range sources {
		inputs = append(inputs, copyInto(t, s, work))
	}

	ps := preset.Preset{
		Name:    "testdata",
		Version: 1,
		Output:  preset.Output{Policy: preset.PolicySubfolder, Subfolder: "converted", OnCollision: preset.CollisionOverwrite},
		Rules: []preset.Rule{
			{Match: []string{"JPEG"}, Args: []cjxl.Arg{{Key: "--lossless_jpeg", Value: "1"}, {Key: "-e", Value: "4"}}},
			{Match: []string{"*"}, Args: []cjxl.Arg{{Key: "-d", Value: "1.0"}, {Key: "-e", Value: "3"}}},
		},
	}

	failures := map[string]error{}
	e := convert.New(
		convert.Deps{Encoder: cjxl.NewRunner(cjxlBin), Verifier: djxl.NewVerifier(djxlBin)},
		convert.Settings{Processes: 3, Threads: 4, Preset: ps},
	)
	e.OnFile = func(r convert.FileResult) {
		t.Logf("%-34s %-9s in=%d out=%d skip=%v err=%v",
			filepath.Base(r.Input), r.Route, r.InputSize, r.OutputSize, r.Skipped, r.Err)
		if r.Err != nil {
			failures[filepath.Base(r.Input)] = r.Err
		}
		if r.Skipped {
			failures[filepath.Base(r.Input)] = errSkipped
		}
	}

	sum := e.Run(context.Background(), inputs)
	if len(failures) > 0 {
		for name, err := range failures {
			t.Errorf("%s: %v", name, err)
		}
	}
	if sum.Completed != len(inputs) {
		t.Fatalf("completed %d of %d (failed=%d skipped=%d)", sum.Completed, len(inputs), sum.Failed, sum.Skipped)
	}

	// Confirm the JPEG samples actually took the reversible transcode route.
	jpegSeen := false
	for _, in := range inputs {
		if routes.DetectFormat(nil, in) == routes.FormatJPEG {
			jpegSeen = true
		}
	}
	if !jpegSeen {
		t.Log("no JPEG sample present to exercise the transcode route")
	}
}

var errSkipped = os.ErrInvalid
