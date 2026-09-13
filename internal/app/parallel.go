package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dhcgn/jxleet/internal/cjxl"
	"github.com/dhcgn/jxleet/internal/cjxl/flags"
	"github.com/dhcgn/jxleet/internal/convert"
	"github.com/dhcgn/jxleet/internal/djxl"
	"github.com/dhcgn/jxleet/internal/jxlinfo"
	"github.com/dhcgn/jxleet/internal/output"
	"github.com/dhcgn/jxleet/internal/preset"
	"github.com/dhcgn/jxleet/internal/toolchain"
)

// MaxAutoProcesses caps core-derived parallelism: each cjxl child holds an
// image in RAM, so unbounded process counts risk exhaustion on many-core
// machines. Explicit user counts pass through uncapped.
const MaxAutoProcesses = 16

// AutoProcesses resolves a requested process count: positive values pass
// through untouched, anything else means automatic — all cores minus one for
// the UI/OS, at least one, capped at MaxAutoProcesses.
func AutoProcesses(requested int) int {
	if requested > 0 {
		return requested
	}
	return max(1, min(runtime.NumCPU()-1, MaxAutoProcesses))
}

// GetCPUCount reports logical processor cores for the parallelism display.
func (s *Service) GetCPUCount() int {
	return runtime.NumCPU()
}

// QueueItemInput is one staged queue item: a file plus its frozen run options.
type QueueItemInput struct {
	Path    string            `json:"path"`
	Options ConversionOptions `json:"options"`
}

// StartQueueRun starts one asynchronous run over staged items, each encoded
// with its own frozen options on a shared worker pool sized by AutoProcesses.
// Progress arrives through Wails events so the UI remains responsive. Unlike
// StartConversion it never coalesces into a running engine — the frontend only
// calls it when idle, and a live engine is refused rather than joined, so
// frozen per-item settings cannot leak into another run.
func (s *Service) StartQueueRun(items []QueueItemInput) error {
	if len(items) == 0 {
		return errors.New("no queued files to convert")
	}
	s.mu.Lock()
	if s.engine != nil {
		s.mu.Unlock()
		return errors.New("a conversion is already running")
	}
	s.mu.Unlock()

	if s.tools == nil {
		return errors.New("toolchain is not configured")
	}
	installed, err := s.tools.Installed(context.Background())
	if err != nil {
		return fmt.Errorf("toolchain is not ready: %w", err)
	}

	staged := make([]convert.WorkItem, 0, len(items))
	var defaultPreset preset.Preset
	needsFlagCheck := false
	processes := 0
	for i, item := range items {
		p, err := s.effectivePreset(item.Options)
		if err != nil {
			return err
		}
		absolute, err := filepath.Abs(strings.TrimSpace(item.Path))
		if err != nil {
			return fmt.Errorf("resolve %q: %w", item.Path, err)
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return fmt.Errorf("stat %q: %w", item.Path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("queue item %q is not a regular file", item.Path)
		}
		staged = append(staged, convert.WorkItem{Path: absolute, Preset: p, Threads: item.Options.Threads})
		if i == 0 {
			defaultPreset = p
		}
		if hasExpertArguments(p) {
			needsFlagCheck = true
		}
		processes = max(processes, AutoProcesses(item.Options.Processes))
	}
	if needsFlagCheck {
		if err := s.checkExpertFlags(installed); err != nil {
			return err
		}
	}

	engine := convert.New(
		convert.Deps{
			Encoder:   cjxl.NewRunner(installed.CJXLPath),       // ref:jl:tech.tool.encode
			Verifier:  djxl.NewVerifier(installed.DJXLPath),     // ref:jl:domain.output.verify
			Inspector: jxlinfo.NewRunner(installed.JXLInfoPath), // ref:jl:tech.tool.inspect
		},
		convert.Settings{
			Processes:   processes,
			Preset:      defaultPreset, // fallback for coalesced arrivals
			Deletion:    output.DeletionByRoute{},
			CJXLVersion: installed.Version,
		},
	)
	s.mu.Lock()
	s.wireEngine(engine, func(result convert.FileResult) string { return result.PresetName })
	s.engine = engine
	s.activePreset = "" // mixed run: no single preset to match arrivals against
	s.mu.Unlock()

	s.launchEngine(engine, func() { engine.AddItems(staged) })
	return nil
}

// checkExpertFlags refuses runs with expert arguments when the installed cjxl
// drifted from the generated flag surface.
func (s *Service) checkExpertFlags(installed toolchain.Installation) error {
	flagStatus, err := s.tools.CheckFlags(context.Background(), installed)
	if err != nil {
		return fmt.Errorf("read installed cjxl flags: %w", err)
	}
	if flagStatus.Locked || len(flagStatus.Added) > 0 || len(flagStatus.Removed) > 0 {
		return fmt.Errorf("expert flags are locked for cjxl %s; generated flags target %s", installed.Version, flags.GeneratedVersion)
	}
	return nil
}

// wireEngine attaches the Wails event wiring shared by conversion runs.
// presetOf selects the preset name recorded to history per finished file.
func (s *Service) wireEngine(engine *convert.Engine, presetOf func(convert.FileResult) string) {
	engine.OnProgress = func(progress convert.Progress) {
		s.emit("progress", progressUpdate(progress))
	}
	engine.OnFileStart = func(started convert.FileStarted) {
		s.emit("conversion-file-start", FileStartUpdate{
			Input:     started.Input,
			PID:       started.PID,
			StartedAt: started.StartedAt.Unix(),
		})
	}
	engine.OnFile = func(result convert.FileResult) {
		s.mu.Lock()
		s.seq++
		seq := s.seq
		s.mu.Unlock()
		s.emit("conversion-file", fileUpdate(seq, result))
		s.recordHistory(presetOf(result), result)
	}
	engine.CollisionHandler = s.askCollision
}

// launchEngine runs the engine asynchronously: Start, add work, then close
// the input after a short coalescing window so late arrivals join the run.
func (s *Service) launchEngine(engine *convert.Engine, add func()) {
	go func() {
		engine.Start(context.Background())
		add()
		timer := time.NewTimer(400 * time.Millisecond)
		<-timer.C
		engine.CloseInput()
		summary := engine.Wait()

		s.mu.Lock()
		if s.engine == engine {
			s.engine = nil
			s.activePreset = ""
		}
		s.mu.Unlock()
		s.emit("conversion-done", ConversionSummary{
			Total:     summary.Total,
			Completed: summary.Completed,
			Failed:    summary.Failed,
			Skipped:   summary.Skipped,
			Cancelled: summary.Cancelled,
			BytesIn:   summary.BytesIn,
			BytesOut:  summary.BytesOut,
		})
	}()
}
