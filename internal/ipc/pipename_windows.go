//go:build windows

// Package ipc implements jxleet's single-instance behaviour over a Windows
// named pipe. The first process to start owns the pipe; later invocations hand
// their paths to the owner and exit within milliseconds, so a tool like
// Lightroom that launches many processes in parallel is never left waiting and
// the user sees one window and one progress bar (see README "The command line").
package ipc

import (
	"fmt"

	"golang.org/x/sys/windows"
)

const pipePrefix = `\\.\pipe\jxleet-`

// PipeName returns the per-user pipe name, keyed by the current user's SID so
// separate users on the same machine never share an instance.
func PipeName() (string, error) {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("ipc: get token user: %w", err)
	}
	sid := user.User.Sid.String()
	return pipePrefix + sid, nil
}
