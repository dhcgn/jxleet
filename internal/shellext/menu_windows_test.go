//go:build windows

package shellext

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQuoteWindowsArg(t *testing.T) {
	if got := quoteWindowsArg(`C:\Program Files\jxleet\jxleet.exe`); got != `"C:\Program Files\jxleet\jxleet.exe"` {
		t.Errorf("quoted path = %q", got)
	}
	if got := quoteWindowsArg(`a"b`); got != `"a\"b"` {
		t.Errorf("quoted quote = %q", got)
	}
}

func TestRegisterUnregisterContextMenu(t *testing.T) {
	if os.Getenv("JXLEET_TEST_REGISTRY") == "" {
		t.Skip("set JXLEET_TEST_REGISTRY=1 to modify the current user's Explorer registry")
	}
	executable := filepath.Join(t.TempDir(), "jxleet.exe")
	if err := os.WriteFile(executable, []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Register(executable, "archive-lossless"); err != nil {
		t.Fatal(err)
	}
	registered, err := Registered()
	if err != nil || !registered {
		t.Fatalf("registered=%v err=%v", registered, err)
	}
	if err := Unregister(); err != nil {
		t.Fatal(err)
	}
	registered, err = Registered()
	if err != nil || registered {
		t.Fatalf("after unregister registered=%v err=%v", registered, err)
	}
}
