//go:build windows

package output

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SHFILEOPSTRUCTW mirrors the Win32 struct used by SHFileOperationW.
type shFileOpStructW struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

const (
	foDelete           = 0x0003
	fofAllowUndo       = 0x0040
	fofNoConfirmation  = 0x0010
	fofSilent          = 0x0004
	fofNoErrorUI       = 0x0400
	fofNoConfirmMkdir  = 0x0200
	fofWantNukeWarning = 0x4000
)

var (
	modShell32           = syscall.NewLazyDLL("shell32.dll")
	procSHFileOperationW = modShell32.NewProc("SHFileOperationW")
)

// MoveToRecycleBin sends a file to the Windows recycle bin. It returns an error
// rather than deleting outright if the operation cannot be performed; callers
// must first confirm RecycleBinAvailable for the file's volume.
func MoveToRecycleBin(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	// pFrom must be a list terminated by a double null.
	from, err := doubleNullUTF16(abs)
	if err != nil {
		return err
	}
	op := shFileOpStructW{
		wFunc:  foDelete,
		pFrom:  &from[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI | fofNoConfirmMkdir,
	}
	ret, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	if ret != 0 {
		return fmt.Errorf("output: SHFileOperation failed (code %d) for %s", ret, abs)
	}
	if op.fAnyOperationsAborted != 0 {
		return fmt.Errorf("output: recycle operation aborted for %s", abs)
	}
	return nil
}

// RecycleBinAvailable reports whether the volume holding path supports the
// recycle bin. Network shares and optical media do not, and jxleet refuses to
// replace there rather than deleting permanently.
func RecycleBinAvailable(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	root := filepath.VolumeName(abs) + `\`
	rootPtr, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return false
	}
	switch windows.GetDriveType(rootPtr) {
	case windows.DRIVE_FIXED, windows.DRIVE_REMOVABLE, windows.DRIVE_RAMDISK:
		return true
	default: // DRIVE_REMOTE, DRIVE_CDROM, DRIVE_UNKNOWN, DRIVE_NO_ROOT_DIR
		return false
	}
}

// doubleNullUTF16 encodes s as UTF-16 terminated by two null code units.
func doubleNullUTF16(s string) ([]uint16, error) {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return nil, err
	}
	// u already ends in one null; append another for the list terminator.
	u = append(u, 0)
	return u, nil
}
