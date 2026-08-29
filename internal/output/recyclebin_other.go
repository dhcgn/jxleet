//go:build !windows

package output

import "errors"

// MoveToRecycleBin is only implemented on Windows; jxleet targets Windows.
func MoveToRecycleBin(path string) error {
	return errors.New("output: recycle bin is only supported on Windows")
}

// RecycleBinAvailable always reports false off Windows.
func RecycleBinAvailable(path string) bool {
	return false
}
