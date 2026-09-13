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
// jl:tech.libjxl.cjxl=libjxl's encoder binary; the sole writer of every .jxl byte jxleet produces.
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
	PID      int // OS pid of the child, 0 if it never started
	Err      error
}

// Success reports whether cjxl exited cleanly.
func (r Result) Success() bool {
	return r.Err == nil && r.ExitCode == 0
}

// Run encodes input to output with the given options, honouring ctx for
// cancellation. The returned Result is always populated; check Result.Success.
// Options come from the preset rule (ref:jl:domain.preset.args-verbatim).
// jl:tech.tool.encode=One cjxl invocation per file from the rule's verbatim args; cancellable per process.
func (r *Runner) Run(ctx context.Context, args []Arg, input, output string) Result {
	argv := Args(args)
	argv = append(argv, input, output)
	return r.exec(ctx, argv, nil)
}

// RunWithStart behaves like Run but reports the OS PID via onStart once the
// child has started, so callers can watch or cancel that specific process.
// jl:tech.tool.resources=Per-process CPU usage and working-set RAM of a running cjxl child, sampled on a fixed cadence while its row shows processing.
func (r *Runner) RunWithStart(ctx context.Context, args []Arg, input, output string, onStart func(pid int)) Result {
	argv := Args(args)
	argv = append(argv, input, output)
	return r.exec(ctx, argv, onStart)
}

// exec runs the binary with raw arguments and captures its streams.
func (r *Runner) exec(ctx context.Context, argv []string, onStart func(pid int)) Result {
	start := time.Now()
	cmd := process.CommandContext(ctx, r.Binary, argv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return Result{Args: argv, Stdout: stdout.String(), Stderr: stderr.String(), Duration: time.Since(start), ExitCode: -1, Err: err}
	}
	res := Result{Args: argv}
	if cmd.Process != nil {
		res.PID = cmd.Process.Pid
		if onStart != nil {
			onStart(res.PID)
		}
	}

	err := cmd.Wait()
	res.Stdout = stdout.String()
	res.Stderr = stderr.String()
	res.Duration = time.Since(start)
	res.Err = err
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
