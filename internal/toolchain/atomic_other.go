//go:build !windows

package toolchain

import "os"

func atomicReplaceFile(from, to string) error {
	return os.Rename(from, to)
}
