package app

import (
	"context"
	"errors"
	"fmt"
	neturl "net/url"
	"time"

	"github.com/dhcgn/jxleet/internal/process"
)

// appRepo is this project's own GitHub repository, checked once per start so
// the GUI can warn about a newer jxleet release. Notify-only, like the libjxl
// toolchain: nothing is ever downloaded automatically.
const appRepo = "dhcgn/jxleet"

// Update is the app-release banner state. Available is true only when the
// latest stable GitHub release is newer than the running build.
type Update struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	URL       string `json:"url"`
	Available bool   `json:"available"`
}

// GetAppUpdate reports whether a newer jxleet release exists on GitHub, via
// the silent update check wired in Callbacks (the Wails updater's Check — no
// window ever opens here). All failure modes — offline, rate limit,
// unconfigured updater, dev build — report "nothing available" instead of an
// error, so a missing network never shows in the GUI.
func (s *Service) GetAppUpdate() Update {
	update := Update{
		Current: s.appVersion,
		URL:     "https://github.com/" + appRepo + "/releases/latest",
	}
	if s.appVersion == "" || s.appVersion == "dev" || s.cb.CheckAppUpdate == nil {
		return update
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	checked, err := s.cb.CheckAppUpdate(ctx)
	if err != nil || !checked.Available {
		return update
	}
	update.Latest = checked.Latest
	if checked.URL != "" {
		update.URL = checked.URL
	}
	update.Available = true
	return update
}

// CheckForAppUpdate opens the Wails update window and runs the full
// check → download → verify → install flow. It is only ever called from the
// banner's Update button or the manual check action: no download starts
// without the user asking. The window itself stays open for the up-to-date
// and error states, so the caller needs nothing back.
func (s *Service) CheckForAppUpdate() error {
	if s.cb.InstallAppUpdate == nil {
		return errors.New("app updates are not available in this build")
	}
	return s.cb.InstallAppUpdate(context.Background())
}

// OpenURL opens an https link in the system browser, nothing else.
func (s *Service) OpenURL(raw string) error {
	u, err := neturl.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("only https links can be opened: %q", raw)
	}
	cmd := process.CommandContext(context.Background(), "cmd", "/c", "start", "", raw)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open link: %w", err)
	}
	return cmd.Process.Release()
}
