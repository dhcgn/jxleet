//go:build !windows

package shellext

import "errors"

var errWindowsOnly = errors.New("shellext: Explorer context menus are only supported on Windows")

func Register(executable, preset string) error {
	return errWindowsOnly
}

func Unregister() error {
	return errWindowsOnly
}

func Registered() (bool, error) {
	return false, errWindowsOnly
}
