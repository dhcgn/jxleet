//go:build windows

package output

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecycleBinAvailableOnTempDir(t *testing.T) {
	// t.TempDir lives on the runner's system drive (fixed), which supports the
	// recycle bin. This has no side effects.
	if !RecycleBinAvailable(filepath.Join(t.TempDir(), "x.txt")) {
		t.Error("expected the recycle bin to be available on the temp volume")
	}
}

func TestMoveToRecycleBin(t *testing.T) {
	if os.Getenv("JXLEET_TEST_RECYCLE") == "" {
		t.Skip("set JXLEET_TEST_RECYCLE=1 to run the recycle-bin side-effect test")
	}
	path := filepath.Join(t.TempDir(), "to-recycle.txt")
	if err := os.WriteFile(path, []byte("bye"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MoveToRecycleBin(path); err != nil {
		t.Fatalf("MoveToRecycleBin: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should have been moved to the recycle bin")
	}
}
