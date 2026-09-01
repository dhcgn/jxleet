package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/dhcgn/jxleet/internal/process"
	"github.com/dhcgn/jxleet/internal/toolchain"
)

// appRepo is this project's own GitHub repository, checked once per start so
// the GUI can warn about a newer jxleet release. Notify-only, like the libjxl
// toolchain: nothing is ever downloaded automatically.
const appRepo = "dhcgn/jxleet"

// AppUpdate is the app-release banner state. Available is true only when the
// latest stable GitHub release is newer than the running build.
type AppUpdate struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	URL       string `json:"url"`
	Available bool   `json:"available"`
}

type appReleaseResponse struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// latestAppRelease queries the GitHub API for the newest release of the app
// itself; /releases/latest excludes drafts and pre-releases.
func latestAppRelease(ctx context.Context, client *http.Client, apiBase string) (version, htmlURL string, err error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(apiBase, "/")+"/"+appRepo+"/releases/latest", nil)
	if err != nil {
		return "", "", fmt.Errorf("app update: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "jxleet")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("app update: query GitHub: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("app update: GitHub API returned %s", resp.Status)
	}
	var body appReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", "", fmt.Errorf("app update: decode release: %w", err)
	}
	if body.TagName == "" {
		return "", "", fmt.Errorf("app update: release has no tag")
	}
	if body.HTMLURL == "" {
		body.HTMLURL = "https://github.com/" + appRepo + "/releases/latest"
	}
	return body.TagName, body.HTMLURL, nil
}

// GetAppUpdate reports whether a newer jxleet release exists on GitHub. All
// failure modes — offline, rate limit, unparseable tag, dev build — report
// "nothing available" instead of an error, so a missing network never shows
// in the GUI.
func (s *Service) GetAppUpdate() AppUpdate {
	update := AppUpdate{
		Current: s.appVersion,
		URL:     "https://github.com/" + appRepo + "/releases/latest",
	}
	if s.appVersion == "" || s.appVersion == "dev" {
		return update
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	latest, url, err := latestAppRelease(ctx, nil, "https://api.github.com")
	if err != nil {
		return update
	}
	cmp, err := toolchain.CompareVersions(latest, s.appVersion)
	if err != nil {
		return update
	}
	update.Latest = latest
	update.URL = url
	update.Available = cmp > 0
	return update
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
