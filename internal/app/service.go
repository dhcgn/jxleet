// Package app contains the Wails service(s) exposed to the frontend. Methods on
// exported service types become the strongly-typed JS/TS binding surface.
package app

import (
	"sync"

	"github.com/dhcgn/jxleet/internal/config"
)

// Service is the root object bound to the frontend.
type Service struct {
	paths config.Paths
	cfg   config.Config

	mu      sync.Mutex
	pending []string
}

// New constructs the root service with resolved paths and loaded config.
func New(paths config.Paths, cfg config.Config) *Service {
	return &Service{paths: paths, cfg: cfg}
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
