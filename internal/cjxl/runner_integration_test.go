package cjxl

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// findCJXL locates a cjxl binary for integration tests: the JXLEET_CJXL
// environment variable takes precedence, otherwise PATH. Tests skip when none is
// found (e.g. on CI without the libjxl toolchain installed).
func findCJXL(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("JXLEET_CJXL"); p != "" {
		return p
	}
	if p, err := exec.LookPath("cjxl"); err == nil {
		return p
	}
	t.Skip("cjxl not found (set JXLEET_CJXL or add cjxl to PATH to run integration tests)")
	return ""
}

// writePPM writes a minimal valid P6 PPM the encoder accepts, so tests need no
// committed binary fixtures.
func writePPM(t *testing.T, path string) {
	t.Helper()
	header := []byte("P6\n2 2\n255\n")
	pixels := make([]byte, 2*2*3)
	for i := range pixels {
		pixels[i] = 128
	}
	if err := os.WriteFile(path, append(header, pixels...), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerVersion(t *testing.T) {
	r := NewRunner(findCJXL(t))
	ver, err := r.Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ver == "" {
		t.Error("empty version")
	}
	t.Logf("cjxl version %s", ver)
}

func TestRunnerEncode(t *testing.T) {
	r := NewRunner(findCJXL(t))
	dir := t.TempDir()
	in := filepath.Join(dir, "in.ppm")
	out := filepath.Join(dir, "out.jxl")
	writePPM(t, in)

	res := r.Run(context.Background(), []Arg{{Key: "-d", Value: "0"}, {Key: "-e", Value: "3"}}, in, out)
	if !res.Success() {
		t.Fatalf("encode failed: exit=%d err=%v stderr=%s", res.ExitCode, res.Err, res.Stderr)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("output not written: %v", err)
	}
	if info.Size() == 0 {
		t.Error("output file is empty")
	}
}

func TestRunnerCancel(t *testing.T) {
	r := NewRunner(findCJXL(t))
	dir := t.TempDir()
	in := filepath.Join(dir, "in.ppm")
	out := filepath.Join(dir, "out.jxl")
	writePPM(t, in)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before running
	res := r.Run(ctx, []Arg{{Key: "-e", Value: "3"}}, in, out)
	if res.Success() {
		t.Error("expected failure when context is already cancelled")
	}
}
