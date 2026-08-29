package toolchain

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultGitHubAPI = "https://api.github.com"

// GitHubClient retrieves libjxl release metadata. BaseURL and HTTPClient are
// replaceable so tests can use an httptest server without network access.
type GitHubClient struct {
	BaseURL    string
	HTTPClient *http.Client
	UserAgent  string
}

// HTTPClientOrDefault returns the configured client, falling back to
// http.DefaultClient.
func (c *GitHubClient) HTTPClientOrDefault() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

// NewGitHubClient returns a client for the public GitHub API.
func NewGitHubClient() *GitHubClient {
	return &GitHubClient{
		BaseURL:    defaultGitHubAPI,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		UserAgent:  "jxleet",
	}
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	PublishedAt time.Time     `json:"published_at"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
	Size               int64  `json:"size"`
}

// Latest retrieves the latest release and selects the official Windows x64
// static zip. A release without a SHA-256 asset digest is rejected.
func (c *GitHubClient) Latest(ctx context.Context) (Release, error) {
	if c == nil {
		return Release{}, fmt.Errorf("toolchain: GitHub client is nil")
	}
	base := strings.TrimRight(c.BaseURL, "/")
	endpoint, err := url.JoinPath(base, "/repos/"+ownerRepo+"/releases/latest")
	if err != nil {
		return Release{}, fmt.Errorf("toolchain: build GitHub API URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Release{}, fmt.Errorf("toolchain: create GitHub request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	resp, err := c.HTTPClientOrDefault().Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("toolchain: fetch latest libjxl release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Release{}, fmt.Errorf("toolchain: GitHub API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var raw githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Release{}, fmt.Errorf("toolchain: decode GitHub release: %w", err)
	}
	version, err := normalizeVersion(raw.TagName)
	if err != nil {
		return Release{}, fmt.Errorf("toolchain: latest release tag: %w", err)
	}
	var selected *githubAsset
	for i := range raw.Assets {
		if raw.Assets[i].Name == windowsAsset {
			selected = &raw.Assets[i]
			break
		}
	}
	if selected == nil {
		return Release{}, fmt.Errorf("toolchain: release %s has no %s asset", raw.TagName, windowsAsset)
	}
	if err := validateDigest(selected.Digest); err != nil {
		return Release{}, err
	}
	if selected.BrowserDownloadURL == "" {
		return Release{}, fmt.Errorf("toolchain: release %s asset has no download URL", windowsAsset)
	}
	if selected.Size <= 0 {
		return Release{}, fmt.Errorf("toolchain: release %s asset has invalid size %d", windowsAsset, selected.Size)
	}
	return Release{
		TagName:     raw.TagName,
		Version:     version,
		PublishedAt: raw.PublishedAt,
		Asset: Asset{
			Name:        selected.Name,
			DownloadURL: selected.BrowserDownloadURL,
			Digest:      selected.Digest,
			Size:        selected.Size,
		},
	}, nil
}

func validateDigest(digest string) error {
	if !strings.HasPrefix(digest, "sha256:") {
		return fmt.Errorf("toolchain: asset digest is not sha256: %q", digest)
	}
	hexDigest := strings.TrimPrefix(digest, "sha256:")
	if len(hexDigest) != 64 {
		return fmt.Errorf("toolchain: invalid sha256 digest length %d", len(hexDigest))
	}
	for _, r := range hexDigest {
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
		if !isHex {
			return fmt.Errorf("toolchain: invalid sha256 digest %q", digest)
		}
	}
	return nil
}
