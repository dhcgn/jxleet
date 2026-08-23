package cjxl

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"regexp"
	"time"

	"github.com/dhcgn/jxleet/internal/process"
)

// Runner executes a specific cjxl binary.
type Runner struct {
	// Binary is the path to cjxl.exe.
	Binary string
}

// NewRunner returns a Runner for the given cjxl binary path.
func NewRunner(binary string) *Runner {
	return &Runner{Binary: binary}
}

// Result captures the outcome of one cjxl invocation.
type Result struct {
	Args     []string
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
	Err      error
}

// Success reports whether cjxl exited cleanly.
func (r Result) Success() bool {
	return r.Err == nil && r.ExitCode == 0
}

// Run encodes input to output with the given options, honouring ctx for
// cancellation. The returned Result is always populated; check Result.Success.
func (r *Runner) Run(ctx context.Context, args []Arg, input, output string) Result {
	argv := Args(args)
	argv = append(argv, input, output)
	return r.exec(ctx, argv)
}

// exec runs the binary with raw arguments and captures its streams.
func (r *Runner) exec(ctx context.Context, argv []string) Result {
	start := time.Now()
	cmd := process.CommandContext(ctx, r.Binary, argv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := Result{
		Args:     argv,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(start),
		Err:      err,
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
	} else if err == nil {
		res.ExitCode = 0
	} else {
		res.ExitCode = -1
	}
	return res
}

var versionRe = regexp.MustCompile(`v(\d+\.\d+\.\d+)`)

// Version runs `cjxl --version` and returns the parsed semantic version, e.g.
// "0.12.0". It returns an error if the binary cannot be run.
func (r *Runner) Version(ctx context.Context) (string, error) {
	cmd := process.CommandContext(ctx, r.Binary, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	if m := versionRe.FindSubmatch(out); m != nil {
		return string(m[1]), nil
	}
	return "", errors.New("cjxl: could not parse version from output")
}
