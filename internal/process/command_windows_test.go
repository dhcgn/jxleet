//go:build windows

package process

import (
	"context"
	"testing"
)

func TestCommandContextHidesWindow(t *testing.T) {
	cmd := CommandContext(context.Background(), "example.exe")
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow {
		t.Fatalf("SysProcAttr = %#v, want HideWindow", cmd.SysProcAttr)
	}
}
