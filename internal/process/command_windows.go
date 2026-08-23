//go:build windows

// Package process creates child processes with the Windows-specific behavior
// required by the desktop application.
package process

import (
	"context"
	"os/exec"
	"syscall"
)

// CommandContext starts a child process without creating a visible console
// window.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}
