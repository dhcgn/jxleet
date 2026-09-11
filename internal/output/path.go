// Package output computes where converted files go and safely finalises them,
// including the recycle-bin-backed replace flow (see README "Output policies").
package output

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhcgn/jxleet/internal/preset"
)

// DefaultSubfolder is used by the subfolder policy when none is configured.
const DefaultSubfolder = "jxl"

// Plan describes the destination for one input file: the final .jxl path, a
// temp path in the same directory to encode into first, and whether the file
// should be skipped because of a name collision.
type Plan struct {
	Input    string
	Final    string
	TempPath string
	Policy   preset.Policy
	Skip     bool
	// InPlace is true when Final resolves to the same path as Input (reencoding
	// a .jxl onto itself under the replace policy).
	InPlace bool
}

// Prepare computes the output Plan for input under the given output settings. It
// resolves name collisions per the policy but does not touch the filesystem
// beyond stat checks.
func Prepare(input string, out preset.Output) (Plan, error) {
	return PrepareWithSuffix(input, out, "")
}

// PrepareWithSuffix is Prepare with a settings suffix (see SettingsSuffix)
// inserted before the .jxl extension, e.g. photo.d1.00-e7-cjxl0.11.1.jxl. An
// empty suffix behaves exactly like Prepare.
func PrepareWithSuffix(input string, out preset.Output, suffix string) (Plan, error) {
	absIn, err := filepath.Abs(input)
	if err != nil {
		return Plan{}, err
	}
	dir := filepath.Dir(absIn)
	base := jxlName(absIn, suffix)

	finalDir := dir
	if out.Policy == preset.PolicySubfolder {
		sub := out.Subfolder
		if sub == "" {
			sub = DefaultSubfolder
		}
		finalDir = filepath.Join(dir, sub)
		if err := os.MkdirAll(finalDir, 0o755); err != nil {
			return Plan{}, err
		}
	}
	final := filepath.Join(finalDir, base)

	plan := Plan{Input: absIn, Final: final, Policy: out.Policy}
	plan.InPlace = out.Policy == preset.PolicyReplace && samePath(final, absIn)

	// Collision resolution: an in-place replace target is not a collision.
	if !plan.InPlace && fileExists(final) {
		switch out.OnCollision {
		case preset.CollisionOverwrite:
			// keep final as-is
		case preset.CollisionNumber:
			final, err = nextNumbered(final)
			if err != nil {
				return Plan{}, err
			}
			plan.Final = final
		default: // skip and empty both skip, the safe default
			plan.Skip = true
			return plan, nil
		}
	}

	tmp, err := tempPath(final)
	if err != nil {
		return Plan{}, err
	}
	plan.TempPath = tmp
	return plan, nil
}

// SidecarPath returns the jxlinfo sidecar path for a converted file:
// the final .jxl path with ".jxlinfo.txt" appended.
func SidecarPath(final string) string {
	return final + ".jxlinfo.txt"
}

// jxlName returns the .jxl output filename for an input path, inserting the
// settings suffix (without a leading dot) before the extension when set.
func jxlName(input, suffix string) string {
	b := filepath.Base(input)
	ext := filepath.Ext(b)
	stem := strings.TrimSuffix(b, ext)
	if suffix != "" {
		stem += "." + suffix
	}
	return stem + ".jxl"
}

// nextNumbered finds "name (1).jxl", "name (2).jxl", ... that does not exist.
func nextNumbered(path string) (string, error) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 1; i < 100000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if !fileExists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("output: could not find a free numbered name for %s", path)
}

// tempPath returns a unique sibling temp path so the encoder writes on the same
// volume as the final file, keeping the later rename atomic.
func tempPath(final string) (string, error) {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return final + "." + hex.EncodeToString(buf[:]) + ".jxleet-tmp", nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// samePath compares two paths for equality, case-insensitively (Windows paths
// are case-insensitive).
func samePath(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return strings.EqualFold(a, b)
	}
	return strings.EqualFold(filepath.Clean(aa), filepath.Clean(bb))
}
