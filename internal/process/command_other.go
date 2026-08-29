//go:build !windows

// Package process creates child processes for non-Windows builds.
package process

import (
	"context"
	"os/exec"
)

// CommandContext matches exec.CommandContext off Windows.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}
