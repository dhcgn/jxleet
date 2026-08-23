// Package app contains the Wails service(s) exposed to the frontend. Methods on
// exported service types become the strongly-typed JS/TS binding surface.
package app

import (
	"context"
	"errors"
	"sync"

	"github.com/dhcgn/jxleet/internal/config"
	"github.com/dhcgn/jxleet/internal/toolchain"
)

// Service is the root object bound to the frontend.
type Service struct {
	paths config.Paths
	cfg   config.Config
	tools *toolchain.Manager

	mu      sync.Mutex
	pending []string
}

// New constructs the root service with resolved paths and loaded config.
func New(paths config.Paths, cfg config.Config, tools *toolchain.Manager) *Service {
	return &Service{paths: paths, cfg: cfg, tools: tools}
}

// Status is a small snapshot the frontend can render on start.
type Status struct {
	// UnboundEntryPoints lists entry points that still need a preset binding.
	// While non-empty, jxleet must say so rather than run.
	UnboundEntryPoints []string `json:"unboundEntryPoints"`
	// Ready is true when all three entry points are bound.
	Ready bool `json:"ready"`
}

// GetStatus reports whether the app is ready to run conversions.
func (s *Service) GetStatus() Status {
	missing := s.cfg.UnboundEntryPoints()
	names := make([]string, 0, len(missing))
	for _, ep := range missing {
		names = append(names, string(ep))
	}
	return Status{
		UnboundEntryPoints: names,
		Ready:              len(names) == 0,
	}
}

// AddPaths queues paths handed over from this or another process invocation.
// The frontend also receives a "files" event for live updates; TakePending lets
// it drain anything queued before it started listening.
func (s *Service) AddPaths(paths []string) {
	if len(paths) == 0 {
		return
	}
	s.mu.Lock()
	s.pending = append(s.pending, paths...)
	s.mu.Unlock()
}

// TakePending returns and clears the queued paths.
func (s *Service) TakePending() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.pending
	s.pending = nil
	return p
}

// ToolchainStatus is the compact toolchain state rendered by the Tools view.
// Updates are notify-only: querying status never downloads or installs anything.
type ToolchainStatus struct {
	InstalledVersion string   `json:"installedVersion"`
	CJXLVersion      string   `json:"cjxlVersion"`
	DJXLVersion      string   `json:"djxlVersion"`
	JXLInfoVersion   string   `json:"jxlinfoVersion"`
	LatestVersion    string   `json:"latestVersion"`
	UpdateAvailable  bool     `json:"updateAvailable"`
	NeedsInstall     bool     `json:"needsInstall"`
	FlagsLocked      bool     `json:"flagsLocked"`
	FlagBaseVersion  string   `json:"flagBaseVersion"`
	FlagToolVersion  string   `json:"flagToolVersion"`
	AddedFlags       []string `json:"addedFlags"`
	RemovedFlags     []string `json:"removedFlags"`
}

// GetToolchainStatus reports installed versions, the latest release, and the
// Expert flag lock/diff. Network or local toolchain errors are returned to the
// caller for display; they are not hidden as an empty status.
func (s *Service) GetToolchainStatus() (ToolchainStatus, error) {
	if s.tools == nil {
		return ToolchainStatus{}, errors.New("toolchain service is not configured")
	}
	status, err := s.tools.Status(context.Background())
	if err != nil {
		return ToolchainStatus{}, err
	}
	view := ToolchainStatus{
		UpdateAvailable: status.UpdateAvailable,
		NeedsInstall:    status.NeedsInstall,
		FlagsLocked:     status.Flags.Locked,
		FlagBaseVersion: status.Flags.GeneratedVersion,
		FlagToolVersion: status.Flags.InstalledVersion,
		AddedFlags:      status.Flags.Added,
		RemovedFlags:    status.Flags.Removed,
	}
	if status.Installed != nil {
		view.InstalledVersion = status.Installed.Version
		view.CJXLVersion = status.Installed.CJXLVersion
		view.DJXLVersion = status.Installed.DJXLVersion
		view.JXLInfoVersion = status.Installed.JXLInfoVersion
	}
	if status.Latest != nil {
		view.LatestVersion = status.Latest.Version
	}
	return view, nil
}

// InstallLatestToolchain performs the user-requested libjxl download and
// atomic installation. It is deliberately not called by GetToolchainStatus.
func (s *Service) InstallLatestToolchain() (string, error) {
	if s.tools == nil {
		return "", errors.New("toolchain service is not configured")
	}
	installed, err := s.tools.InstallLatest(context.Background())
	if err != nil {
		return "", err
	}
	return installed.Version, nil
}
