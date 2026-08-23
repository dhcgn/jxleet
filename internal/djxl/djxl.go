// Package djxl runs the djxl decoder. jxleet uses it only to verify results:
// decoding a freshly written .jxl proves it is readable, and on the transcode
// route decoding back to JPEG proves the original can be reconstructed byte for
// byte before the original is ever moved to the recycle bin.
package djxl

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"regexp"
	"time"

	"github.com/dhcgn/jxleet/internal/process"
)

// Runner executes a specific djxl binary.
type Runner struct {
	Binary string
}

// NewRunner returns a Runner for the given djxl binary path.
func NewRunner(binary string) *Runner {
	return &Runner{Binary: binary}
}

// Result captures the outcome of one djxl invocation.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
	Err      error
}

// Success reports whether djxl exited cleanly.
func (r Result) Success() bool {
	return r.Err == nil && r.ExitCode == 0
}

// Decode decodes input (.jxl) to output; djxl selects the output format from the
// output file's extension.
func (r *Runner) Decode(ctx context.Context, input, output string) Result {
	start := time.Now()
	cmd := process.CommandContext(ctx, r.Binary, input, output)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	res := Result{Stdout: stdout.String(), Stderr: stderr.String(), Duration: time.Since(start), Err: err}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
	} else if err != nil {
		res.ExitCode = -1
	}
	return res
}

var versionRe = regexp.MustCompile(`v(\d+\.\d+\.\d+)`)

// Version runs `djxl --version` and returns the parsed semantic version.
func (r *Runner) Version(ctx context.Context) (string, error) {
	out, err := process.CommandContext(ctx, r.Binary, "--version").CombinedOutput()
	if err != nil {
		return "", err
	}
	if m := versionRe.FindSubmatch(out); m != nil {
		return string(m[1]), nil
	}
	return "", errors.New("djxl: could not parse version")
}

// Verifier decodes .jxl files to prove they are valid. It satisfies the
// output.Verifier interface.
type Verifier struct {
	Runner *Runner
}

// NewVerifier returns a Verifier backed by the given djxl binary.
func NewVerifier(binary string) Verifier {
	return Verifier{Runner: NewRunner(binary)}
}

// Readable decodes jxlPath to a throwaway PNG in the same directory and reports
// an error if decoding fails or produces no output.
func (v Verifier) Readable(ctx context.Context, jxlPath string) error {
	tmp := jxlPath + ".verify.png"
	defer os.Remove(tmp)
	res := v.Runner.Decode(ctx, jxlPath, tmp)
	if !res.Success() {
		return decodeError("readability", res)
	}
	if fi, err := os.Stat(tmp); err != nil || fi.Size() == 0 {
		return errors.New("djxl: verification decode produced no output")
	}
	return nil
}

// Reconstructs decodes jxlPath back to JPEG and reports an error unless the
// result is byte-identical to originalJPEG. Used on the transcode route.
func (v Verifier) Reconstructs(ctx context.Context, jxlPath, originalJPEG string) error {
	tmp := jxlPath + ".verify.jpg"
	defer os.Remove(tmp)
	res := v.Runner.Decode(ctx, jxlPath, tmp)
	if !res.Success() {
		return decodeError("reconstruction", res)
	}
	same, err := filesEqual(tmp, originalJPEG)
	if err != nil {
		return err
	}
	if !same {
		return errors.New("djxl: reconstructed JPEG is not byte-identical to the original")
	}
	return nil
}

func decodeError(what string, res Result) error {
	msg := res.Stderr
	if msg == "" {
		msg = res.Stdout
	}
	return errors.New("djxl: " + what + " check failed: " + msg)
}

// filesEqual streams two files and reports whether their contents are identical.
func filesEqual(a, b string) (bool, error) {
	fa, err := os.Open(a)
	if err != nil {
		return false, err
	}
	defer fa.Close()
	fb, err := os.Open(b)
	if err != nil {
		return false, err
	}
	defer fb.Close()

	if ia, err := fa.Stat(); err == nil {
		if ib, err := fb.Stat(); err == nil && ia.Size() != ib.Size() {
			return false, nil
		}
	}

	const chunk = 64 * 1024
	ba := make([]byte, chunk)
	bb := make([]byte, chunk)
	for {
		na, ea := io.ReadFull(fa, ba)
		nb, eb := io.ReadFull(fb, bb)
		if na != nb || !bytes.Equal(ba[:na], bb[:nb]) {
			return false, nil
		}
		if ea == io.EOF || ea == io.ErrUnexpectedEOF {
			// Both must end together.
			return eb == io.EOF || eb == io.ErrUnexpectedEOF, nil
		}
		if ea != nil {
			return false, ea
		}
	}
}
