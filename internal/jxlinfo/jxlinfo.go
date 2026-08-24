// Package jxlinfo runs the libjxl metadata inspection tool.
package jxlinfo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dhcgn/jxleet/internal/process"
)

// Runner executes a specific jxlinfo binary.
type Runner struct {
	Binary string
}

// NewRunner returns a Runner for the given jxlinfo binary path.
func NewRunner(binary string) *Runner {
	return &Runner{Binary: binary}
}

// Inspect returns the verbose metadata emitted for a JPEG XL file.
func (r *Runner) Inspect(ctx context.Context, path string) (string, error) {
	if strings.TrimSpace(r.Binary) == "" {
		return "", errors.New("jxlinfo: binary path is empty")
	}
	cmd := process.CommandContext(ctx, r.Binary, "-v", path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("jxlinfo: inspect failed: %s", message)
	}
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		output = strings.TrimSpace(stderr.String())
	}
	if output == "" {
		return "", errors.New("jxlinfo: inspect produced no output")
	}
	return output, nil
}
