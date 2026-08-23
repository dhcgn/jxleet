// Package toolchain manages the libjxl command-line tools used by jxleet.
// Releases come from the official libjxl GitHub repository and are installed
// as immutable, versioned tool directories with an atomically replaced pointer
// to the active version.
package toolchain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dhcgn/jxleet/internal/cjxl/flags"
	"github.com/dhcgn/jxleet/internal/process"
	"github.com/spf13/pathologize"
)

const (
	ownerRepo       = "libjxl/libjxl"
	windowsAsset    = "jxl-x64-windows-static.zip"
	currentFileName = "current.txt"
)

var (
	// ErrNotInstalled means no active toolchain has been selected yet.
	ErrNotInstalled = errors.New("toolchain: no active installation")
	versionPattern  = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
)

// Release is the selected official libjxl release and its Windows x64 static
// asset.
type Release struct {
	TagName     string    `json:"tag_name"`
	Version     string    `json:"-"`
	PublishedAt time.Time `json:"published_at"`
	Asset       Asset     `json:"-"`
}

// Asset is a GitHub release asset with its integrity metadata.
type Asset struct {
	Name        string
	DownloadURL string
	Digest      string
	Size        int64
}

// Installation identifies the active command-line tools.
type Installation struct {
	Version        string
	Root           string
	CJXLPath       string
	DJXLPath       string
	JXLInfoPath    string
	CJXLVersion    string
	DJXLVersion    string
	JXLInfoVersion string
}

// FlagStatus describes whether the Expert flag surface is safe to expose.
type FlagStatus struct {
	Locked           bool
	GeneratedVersion string
	InstalledVersion string
	Added            []string
	Removed          []string
}

// Status is the complete toolchain state shown by the Tools view.
type Status struct {
	Installed       *Installation
	Latest          *Release
	UpdateAvailable bool
	NeedsInstall    bool
	Flags           FlagStatus
}

// Manager owns the local toolchain directory and the GitHub API client.
type Manager struct {
	BinDir string
	Client *GitHubClient
}

// NewManager constructs a manager rooted at binDir.
func NewManager(binDir string) *Manager {
	return &Manager{
		BinDir: binDir,
		Client: NewGitHubClient(),
	}
}

// NewManagerWithClient is useful for tests and for callers that need a custom
// HTTP transport or GitHub API endpoint.
func NewManagerWithClient(binDir string, client *GitHubClient) *Manager {
	if client == nil {
		client = NewGitHubClient()
	}
	return &Manager{BinDir: binDir, Client: client}
}

// VersionsDir is the directory containing immutable versioned installations.
func (m *Manager) VersionsDir() string {
	return filepath.Join(m.BinDir, "versions")
}

// CurrentPath is the active-version pointer file.
func (m *Manager) CurrentPath() string {
	return filepath.Join(m.BinDir, currentFileName)
}

// ToolsDir returns the normalized directory layout for a version.
func (m *Manager) ToolsDir(version string) string {
	return filepath.FromSlash(pathologize.Join(m.VersionsDir(), version, "bin"))
}

// Latest fetches the latest official libjxl release metadata.
func (m *Manager) Latest(ctx context.Context) (Release, error) {
	if m.Client == nil {
		return Release{}, errors.New("toolchain: GitHub client is nil")
	}
	return m.Client.Latest(ctx)
}

// Installed reads the active pointer and verifies all three tools. A missing
// pointer is a normal first-run state and returns ErrNotInstalled.
func (m *Manager) Installed(ctx context.Context) (Installation, error) {
	version, err := readCurrentVersion(m.CurrentPath())
	if err != nil {
		if errors.Is(err, ErrNotInstalled) {
			return Installation{}, ErrNotInstalled
		}
		return Installation{}, fmt.Errorf("toolchain: read active version: %w", err)
	}
	if version, err = normalizeVersion(version); err != nil {
		return Installation{}, fmt.Errorf("toolchain: invalid active version: %w", err)
	}
	return m.inspectInstallation(ctx, version)
}

// Status computes the local install state, latest-release notification, and
// Expert flag lock/diff. Network errors are returned to the caller rather than
// silently converted into a false update state.
func (m *Manager) Status(ctx context.Context) (Status, error) {
	var status Status

	installed, err := m.Installed(ctx)
	if errors.Is(err, ErrNotInstalled) {
		status.NeedsInstall = true
		status.Flags = lockedFlags("")
	} else if err != nil {
		return status, err
	} else {
		status.Installed = &installed
		status.Flags, err = m.flagStatus(ctx, installed)
		if err != nil {
			return status, err
		}
	}

	latest, err := m.Latest(ctx)
	if err != nil {
		return status, err
	}
	status.Latest = &latest
	if status.Installed != nil {
		cmp, err := CompareVersions(latest.Version, status.Installed.Version)
		if err != nil {
			return status, err
		}
		status.UpdateAvailable = cmp > 0
	}
	return status, nil
}

// InstallLatest downloads and installs the current latest release. Updates are
// intentionally user-triggered; this method is never called automatically.
func (m *Manager) InstallLatest(ctx context.Context) (Installation, error) {
	release, err := m.Latest(ctx)
	if err != nil {
		return Installation{}, err
	}
	return m.InstallRelease(ctx, release)
}

func (m *Manager) inspectInstallation(ctx context.Context, version string) (Installation, error) {
	root := m.ToolsDir(version)
	installation := Installation{
		Version:        version,
		Root:           filepath.Dir(root),
		CJXLPath:       filepath.Join(root, "cjxl.exe"),
		DJXLPath:       filepath.Join(root, "djxl.exe"),
		JXLInfoPath:    filepath.Join(root, "jxlinfo.exe"),
		JXLInfoVersion: version, // jxlinfo has no --version flag; it is bundled with the release.
	}
	for _, path := range []string{installation.CJXLPath, installation.DJXLPath, installation.JXLInfoPath} {
		if _, err := os.Stat(path); err != nil {
			return Installation{}, fmt.Errorf("toolchain: missing %s: %w", filepath.Base(path), err)
		}
	}
	var err error
	installation.CJXLVersion, err = probeVersion(ctx, installation.CJXLPath)
	if err != nil {
		return Installation{}, err
	}
	installation.DJXLVersion, err = probeVersion(ctx, installation.DJXLPath)
	if err != nil {
		return Installation{}, err
	}
	if installation.CJXLVersion != version || installation.DJXLVersion != version {
		return Installation{}, fmt.Errorf(
			"toolchain: active directory %s contains cjxl %s and djxl %s",
			version, installation.CJXLVersion, installation.DJXLVersion,
		)
	}
	return installation, nil
}

func (m *Manager) flagStatus(ctx context.Context, installation Installation) (FlagStatus, error) {
	status := FlagStatus{
		GeneratedVersion: flags.GeneratedVersion,
		InstalledVersion: installation.Version,
		Locked:           installation.Version != flags.GeneratedVersion,
	}
	help, err := commandOutput(ctx, installation.CJXLPath, "--help", "-v", "-v", "-v", "-v")
	if err != nil {
		return status, fmt.Errorf("toolchain: read cjxl flags: %w", err)
	}
	current, err := flags.Parse(strings.NewReader(help))
	if err != nil {
		return status, fmt.Errorf("toolchain: parse installed cjxl flags: %w", err)
	}
	added, removed := flags.Diff(flags.NewSet(installation.Version, flags.Default().Flags), flags.NewSet(installation.Version, current))
	status.Added = added
	status.Removed = removed
	return status, nil
}

func lockedFlags(installedVersion string) FlagStatus {
	return FlagStatus{
		Locked:           true,
		GeneratedVersion: flags.GeneratedVersion,
		InstalledVersion: installedVersion,
	}
}

func probeVersion(ctx context.Context, binary string) (string, error) {
	out, err := commandOutput(ctx, binary, "--version")
	if err != nil {
		return "", fmt.Errorf("toolchain: probe %s: %w", filepath.Base(binary), err)
	}
	match := versionPattern.FindStringSubmatch(extractVersion(out))
	if match == nil {
		return "", fmt.Errorf("toolchain: could not parse version from %s output", filepath.Base(binary))
	}
	return match[0], nil
}

func commandOutput(ctx context.Context, binary string, args ...string) (string, error) {
	cmd := osCommandContext(ctx, binary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func extractVersion(output string) string {
	for _, field := range strings.Fields(output) {
		if strings.HasPrefix(field, "v") && versionPattern.MatchString(strings.TrimPrefix(field, "v")) {
			return strings.TrimPrefix(field, "v")
		}
	}
	return ""
}

func normalizeVersion(tag string) (string, error) {
	version := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if !versionPattern.MatchString(version) {
		return "", fmt.Errorf("invalid semantic version %q", tag)
	}
	return version, nil
}

// CompareVersions compares two semantic versions supported by libjxl tags.
func CompareVersions(a, b string) (int, error) {
	aa, err := normalizeVersion(a)
	if err != nil {
		return 0, err
	}
	bb, err := normalizeVersion(b)
	if err != nil {
		return 0, err
	}
	ap, apre := splitVersion(aa)
	bp, bpre := splitVersion(bb)
	for i := range ap {
		if ap[i] != bp[i] {
			if ap[i] < bp[i] {
				return -1, nil
			}
			return 1, nil
		}
	}
	switch {
	case apre == "" && bpre != "":
		return 1, nil
	case apre != "" && bpre == "":
		return -1, nil
	}
	return comparePrerelease(apre, bpre), nil
}

func splitVersion(version string) ([3]int, string) {
	base, _, _ := strings.Cut(version, "+")
	base, prerelease, _ := strings.Cut(base, "-")
	parts := strings.Split(base, ".")
	var result [3]int
	for i := range result {
		result[i], _ = strconv.Atoi(parts[i])
	}
	return result, prerelease
}

func comparePrerelease(a, b string) int {
	if a == b {
		return 0
	}
	aa := strings.Split(a, ".")
	bb := strings.Split(b, ".")
	for i := 0; i < len(aa) && i < len(bb); i++ {
		an, aErr := strconv.Atoi(aa[i])
		bn, bErr := strconv.Atoi(bb[i])
		switch {
		case aErr == nil && bErr == nil && an != bn:
			if an < bn {
				return -1
			}
			return 1
		case aErr == nil && bErr != nil:
			return -1
		case aErr != nil && bErr == nil:
			return 1
		case aa[i] != bb[i]:
			if aa[i] < bb[i] {
				return -1
			}
			return 1
		}
	}
	if len(aa) < len(bb) {
		return -1
	}
	return 1
}

// osCommandContext is a variable to keep command execution replaceable in
// package tests without adding an abstraction to the production path.
var osCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return process.CommandContext(ctx, name, args...)
}
