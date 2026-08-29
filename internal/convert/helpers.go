package convert

import (
	"errors"
	"os"

	"github.com/dhcgn/jxleet/internal/cjxl"
	"github.com/dhcgn/jxleet/internal/routes"
)

// detectFormat reads the file header and classifies the input, falling back to
// the extension. Unreadable files are reported as unknown (and thus skipped).
func detectFormat(path string) routes.Format {
	f, err := os.Open(path)
	if err != nil {
		return routes.DetectFormat(nil, path)
	}
	defer func() { _ = f.Close() }()
	var header [512]byte
	n, _ := f.Read(header[:])
	return routes.DetectFormat(header[:n], path)
}

// DetectFormat identifies one input path using the same content-first logic as
// the conversion engine. It is used by the GUI preview before a run starts.
func DetectFormat(path string) routes.Format {
	return detectFormat(path)
}

// encodeError turns a failed cjxl result into an error carrying its stderr.
func encodeError(res cjxl.Result) error {
	msg := res.Stderr
	if msg == "" {
		msg = res.Stdout
	}
	if msg == "" && res.Err != nil {
		msg = res.Err.Error()
	}
	return errors.New("cjxl: encode failed: " + msg)
}
