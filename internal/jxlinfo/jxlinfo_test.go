package jxlinfo

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInspectRejectsEmptyBinary(t *testing.T) {
	_, err := NewRunner("").Inspect(context.Background(), "image.jxl")
	if err == nil || !strings.Contains(err.Error(), "binary path is empty") {
		t.Fatalf("error = %v", err)
	}
}

func TestInspectReportsCommandFailure(t *testing.T) {
	_, err := NewRunner("jxlinfo-does-not-exist").Inspect(context.Background(), "image.jxl")
	if err == nil || !strings.Contains(err.Error(), "jxlinfo: inspect failed") {
		t.Fatalf("error = %v", err)
	}
}

func TestInspectFixture(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("bundled fixture tool is Windows-only")
	}
	binary := filepath.Join("..", "..", "test-bins", "jxlinfo.exe")
	input := filepath.Join("..", "..", "test-data", "jxl", "from_lr_png_8bit_loseless.jxl")
	if _, err := os.Stat(binary); err != nil {
		t.Skipf("bundled jxlinfo is unavailable: %v", err)
	}
	if _, err := os.Stat(input); err != nil {
		t.Skipf("metadata fixture is unavailable: %v", err)
	}
	output, err := NewRunner(binary).Inspect(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "JPEG XL") {
		t.Fatalf("unexpected jxlinfo output: %s", output)
	}
}
