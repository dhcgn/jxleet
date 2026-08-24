// Package app contains the Wails service(s) exposed to the frontend. Methods on
// exported service types become the strongly-typed JS/TS binding surface.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dhcgn/jxleet/internal/cjxl"
	"github.com/dhcgn/jxleet/internal/cjxl/flags"
	"github.com/dhcgn/jxleet/internal/config"
	"github.com/dhcgn/jxleet/internal/convert"
	"github.com/dhcgn/jxleet/internal/djxl"
	"github.com/dhcgn/jxleet/internal/jxlinfo"
	"github.com/dhcgn/jxleet/internal/output"
	"github.com/dhcgn/jxleet/internal/preset"
	"github.com/dhcgn/jxleet/internal/routes"
	"github.com/dhcgn/jxleet/internal/shellext"
	"github.com/dhcgn/jxleet/internal/toolchain"
)

// Callbacks connect the Wails adapter to native dialogs and events without
// importing Wails into the application domain.
type Callbacks struct {
	Emit        func(name string, data any)
	OpenFiles   func() ([]string, error)
	OpenFolders func() ([]string, error)
}

// Service is the root object bound to the frontend.
type Service struct {
	paths config.Paths
	cfg   config.Config
	tools *toolchain.Manager
	cb    Callbacks

	mu            sync.Mutex
	pending       []string
	pendingPreset string
	engine        *convert.Engine
	activePreset  string
}

// New constructs the root service with resolved paths, loaded config, and
// native callbacks.
func New(paths config.Paths, cfg config.Config, tools *toolchain.Manager, cb Callbacks) *Service {
	return &Service{paths: paths, cfg: cfg, tools: tools, cb: cb}
}

// Status is a small snapshot the frontend can render on start.
type Status struct {
	UnboundEntryPoints []string `json:"unboundEntryPoints"`
	Ready              bool     `json:"ready"`
}

// GetStatus reports whether the app is ready to run conversions.
func (s *Service) GetStatus() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	missing := s.cfg.UnboundEntryPoints()
	names := make([]string, 0, len(missing))
	for _, ep := range missing {
		names = append(names, string(ep))
	}
	return Status{UnboundEntryPoints: names, Ready: len(names) == 0}
}

// AddPaths queues paths handed over from this or another process invocation.
// When a conversion is already running, the paths are added to that engine so
// coalesced invocations share one run.
func (s *Service) AddPaths(paths []string) {
	if len(paths) == 0 {
		return
	}
	s.mu.Lock()
	engine := s.engine
	if engine == nil {
		s.pending = append(s.pending, paths...)
	}
	s.mu.Unlock()
	if engine != nil {
		expanded, err := expandPaths(paths)
		if err != nil {
			s.emit("conversion-error", err.Error())
			return
		}
		engine.Add(expanded)
	}
	s.emit("files", paths)
}

// ReceivePaths accepts an external invocation, retaining its explicit preset
// override until the frontend starts the run.
func (s *Service) ReceivePaths(paths []string, presetName string) {
	s.mu.Lock()
	engine := s.engine
	activePreset := s.activePreset
	if engine == nil && presetName != "" {
		s.pendingPreset = presetName
	}
	s.mu.Unlock()
	if engine != nil && presetName != "" && activePreset != "" && presetName != activePreset {
		s.emit("conversion-error", fmt.Sprintf("an invocation selected preset %q while %q is already running", presetName, activePreset))
		return
	}
	s.AddPaths(paths)
	if presetName != "" {
		s.emit("preset", presetName)
	}
}

// TakePending returns and clears paths received before the frontend subscribed.
func (s *Service) TakePending() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.pending
	s.pending = nil
	return p
}

// TakePendingPreset returns an external preset override received before the
// frontend subscribed.
func (s *Service) TakePendingPreset() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	presetName := s.pendingPreset
	s.pendingPreset = ""
	return presetName
}

// GetActivePreset reports the preset currently used by an asynchronous run.
func (s *Service) GetActivePreset() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activePreset
}

// OpenFiles opens the native multi-file picker.
func (s *Service) OpenFiles() ([]string, error) {
	if s.cb.OpenFiles == nil {
		return nil, errors.New("file dialog is not configured")
	}
	paths, err := s.cb.OpenFiles()
	if err != nil {
		return nil, err
	}
	s.AddPaths(paths)
	return paths, nil
}

// OpenFolders opens the native multi-folder picker.
func (s *Service) OpenFolders() ([]string, error) {
	if s.cb.OpenFolders == nil {
		return nil, errors.New("folder dialog is not configured")
	}
	paths, err := s.cb.OpenFolders()
	if err != nil {
		return nil, err
	}
	s.AddPaths(paths)
	return paths, nil
}

// ListCJXLFlags returns the generated cjxl flag surface and its help text.
func (s *Service) ListCJXLFlags() []FlagInfo {
	definitions := flags.Default().Flags
	result := make([]FlagInfo, 0, len(definitions))
	for _, definition := range definitions {
		result = append(result, FlagInfo{
			Key:         definition.Canonical(),
			Short:       definition.Short,
			Long:        definition.Long,
			TakesValue:  definition.TakesValue,
			ValueSpec:   definition.ValueSpec,
			Section:     definition.Section,
			Description: definition.Description,
		})
	}
	return result
}

// PreviewCommands returns the resolved command preview for every preset rule.
func (s *Service) PreviewCommands(options ConversionOptions) ([]CommandPreview, error) {
	p, err := s.effectivePreset(options)
	if err != nil {
		return nil, err
	}
	result := make([]CommandPreview, 0, len(p.Rules))
	for _, rule := range p.Rules {
		args := append([]cjxl.Arg(nil), rule.Args...)
		args = addPreviewThreads(args, options.Threads)
		tokens := append([]string{"cjxl"}, cjxl.Args(args)...)
		tokens = append(tokens, "input", "output.jxl")
		result = append(result, CommandPreview{
			Matches: append([]string(nil), rule.Match...),
			Command: strings.Join(tokens, " "),
		})
	}
	return result, nil
}

// Bindings contains the three explicit entry-point preset bindings.
type Bindings struct {
	GUI         string `json:"gui"`
	CLI         string `json:"cli"`
	ContextMenu string `json:"contextMenu"`
}

// GetBindings returns the configured preset for each entry point.
func (s *Service) GetBindings() Bindings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Bindings{
		GUI:         s.cfg.Bindings[config.EntryGUI],
		CLI:         s.cfg.Bindings[config.EntryCLI],
		ContextMenu: s.cfg.Bindings[config.EntryContextMenu],
	}
}

// SetBinding validates and persists one entry-point binding.
func (s *Service) SetBinding(entryPoint, presetName string) error {
	ep := config.EntryPoint(strings.ToLower(strings.TrimSpace(entryPoint)))
	if ep != config.EntryGUI && ep != config.EntryCLI && ep != config.EntryContextMenu {
		return fmt.Errorf("unknown entry point %q", entryPoint)
	}
	store := preset.NewStore(s.paths.PresetsDir)
	if _, err := store.Load(presetName); err != nil {
		return err
	}

	s.mu.Lock()
	next := s.cfg
	next.Bindings = cloneBindings(s.cfg.Bindings)
	next.Bindings[ep] = presetName
	if err := config.Save(s.paths.ConfigFile, next); err != nil {
		s.mu.Unlock()
		return err
	}
	s.cfg = next
	s.mu.Unlock()
	return nil
}

// PresetSummary is the compact data shown by the Presets view.
type PresetSummary struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Policy      string              `json:"policy"`
	ReadOnly    bool                `json:"readOnly"`
	CoreValue   string              `json:"coreValue"`
	Effort      string              `json:"effort"`
	JPEGMode    string              `json:"jpegMode"`
	Rules       []PresetRuleSummary `json:"rules"`
}

// PresetRuleSummary is one file-filter rule shown in the selected-preset details.
type PresetRuleSummary struct {
	Matches   []string `json:"matches"`
	CoreValue string   `json:"coreValue"`
	Effort    string   `json:"effort"`
	JPEGMode  string   `json:"jpegMode"`
}

// ListPresets returns valid stored presets.
func (s *Service) ListPresets() ([]PresetSummary, error) {
	items, err := preset.NewStore(s.paths.PresetsDir).List()
	if err != nil {
		return nil, err
	}
	result := make([]PresetSummary, 0, len(items))
	for _, p := range items {
		result = append(result, PresetSummary{
			Name:        p.Name,
			Description: p.Description,
			Policy:      string(p.Output.Policy),
			ReadOnly:    p.ReadOnly,
			CoreValue:   summarizeCoreValue(p),
			Effort:      summarizeEffort(p),
			JPEGMode:    summarizeJPEGMode(p),
			Rules:       summarizeRules(p),
		})
	}
	return result, nil
}

// SavePresetOutputPolicy persists a policy change for a writable preset.
func (s *Service) SavePresetOutputPolicy(name, policy string) error {
	p, err := preset.NewStore(s.paths.PresetsDir).Load(name)
	if err != nil {
		return err
	}
	if p.ReadOnly {
		return fmt.Errorf("preset %q is read-only", name)
	}
	selected := preset.Policy(policy)
	switch selected {
	case preset.PolicyAlongside, preset.PolicySubfolder, preset.PolicyReplace:
	default:
		return fmt.Errorf("unknown output policy %q", policy)
	}
	p.Output.Policy = selected
	if selected == preset.PolicySubfolder && p.Output.Subfolder == "" {
		p.Output.Subfolder = "jxl"
	}
	return preset.NewStore(s.paths.PresetsDir).Save(p)
}

// CreatePreset creates a usable basic preset.
func (s *Service) CreatePreset(name, description string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("preset name is required")
	}
	return preset.NewStore(s.paths.PresetsDir).Save(preset.Preset{
		Name:        name,
		Description: strings.TrimSpace(description),
		Version:     preset.CurrentVersion,
		Output:      preset.DefaultOutput(),
		Rules: []preset.Rule{{
			Match: []string{"*"},
			Args:  []cjxl.Arg{{Key: "-d", Value: "1.0"}, {Key: "-e", Value: "7"}},
		}},
	})
}

// DeletePreset deletes a named preset.
func (s *Service) DeletePreset(name string) error {
	return preset.NewStore(s.paths.PresetsDir).Delete(name)
}

// DuplicatePreset duplicates a named preset.
func (s *Service) DuplicatePreset(name, newName string) error {
	return preset.NewStore(s.paths.PresetsDir).Duplicate(name, newName)
}

// RenamePreset renames a named preset.
func (s *Service) RenamePreset(name, newName string) error {
	return preset.NewStore(s.paths.PresetsDir).Rename(name, newName)
}

// RegisterContextMenu installs the per-user Explorer entries using the current
// context-menu preset binding.
func (s *Service) RegisterContextMenu() error {
	binding := s.GetBindings().ContextMenu
	if binding == "" {
		return errors.New("bind a context-menu preset first")
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve jxleet executable: %w", err)
	}
	return shellext.Register(executable, binding)
}

// UnregisterContextMenu removes the per-user Explorer entries.
func (s *Service) UnregisterContextMenu() error {
	return shellext.Unregister()
}

// ContextMenuRegistered reports whether the primary Explorer entry exists.
func (s *Service) ContextMenuRegistered() (bool, error) {
	return shellext.Registered()
}

// OpenStorageLocation opens one of jxleet's storage directories in Explorer.
func (s *Service) OpenStorageLocation(location string) error {
	var path string
	switch location {
	case "config":
		path = s.paths.ConfigDir
	case "presets":
		path = s.paths.PresetsDir
	case "bin":
		path = s.paths.BinDir
	case "logs":
		path = s.paths.LogsDir
	default:
		return fmt.Errorf("unknown storage location %q", location)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create storage location: %w", err)
	}
	cmd := exec.Command("explorer.exe", path)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open storage location: %w", err)
	}
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("release Explorer process: %w", err)
	}
	return nil
}

// ConversionOptions are the temporary GUI settings applied to a preset for one
// run. The stored preset is never modified by these overrides.
type ConversionOptions struct {
	Preset       string         `json:"preset"`
	Processes    int            `json:"processes"`
	Threads      int            `json:"threads"`
	JPEGMode     string         `json:"jpegMode"`
	Distance     float64        `json:"distance"`
	UseDistance  bool           `json:"useDistance"`
	Effort       int            `json:"effort"`
	UseEffort    bool           `json:"useEffort"`
	OutputPolicy string         `json:"outputPolicy"`
	ExpertFlags  []FlagOverride `json:"expertFlags"`
	ResetExpert  bool           `json:"resetExpert"`
}

// FlagInfo describes one generated cjxl flag for the Expert UI.
type FlagInfo struct {
	Key         string `json:"key"`
	Short       string `json:"short,omitempty"`
	Long        string `json:"long,omitempty"`
	TakesValue  bool   `json:"takesValue"`
	ValueSpec   string `json:"valueSpec,omitempty"`
	Section     string `json:"section,omitempty"`
	Description string `json:"description,omitempty"`
}

// FlagOverride is a temporary global cjxl flag override for one conversion.
type FlagOverride struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Valueless bool   `json:"valueless"`
}

// CommandPreview is the resolved cjxl command for one preset rule.
type CommandPreview struct {
	Matches []string `json:"matches"`
	Command string   `json:"command"`
}

// FilePreview is one item in the Basic view before conversion starts.
type FilePreview struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Format string `json:"format"`
	Route  string `json:"route"`
	Size   int64  `json:"size"`
	Skip   bool   `json:"skip"`
	Reason string `json:"reason"`
}

// PreviewPaths classifies files using the same preset/options as StartConversion.
func (s *Service) PreviewPaths(paths []string, options ConversionOptions) ([]FilePreview, error) {
	inputs, err := expandPaths(paths)
	if err != nil {
		return nil, err
	}
	var selected *preset.Preset
	if strings.TrimSpace(options.Preset) != "" {
		p, err := s.effectivePreset(options)
		if err != nil {
			return nil, err
		}
		selected = &p
	}
	result := make([]FilePreview, 0, len(inputs))
	for _, path := range inputs {
		item := FilePreview{Path: path, Name: filepath.Base(path)}
		if info, statErr := os.Stat(path); statErr == nil {
			item.Size = info.Size()
		}
		format := convert.DetectFormat(path)
		item.Format = string(format)
		if format == routes.FormatUnknown {
			item.Skip = true
			item.Reason = "unsupported format"
			result = append(result, item)
			continue
		}
		if selected == nil {
			item.Reason = "select a preset to classify"
			result = append(result, item)
			continue
		}
		route, _, ok := selected.Route(format)
		if !ok {
			item.Skip = true
			item.Reason = "no matching rule"
			result = append(result, item)
			continue
		}
		item.Route = route.String()
		result = append(result, item)
	}
	return result, nil
}

// InspectJXL returns verbose metadata for one existing JPEG XL file.
func (s *Service) InspectJXL(path string) (string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", fmt.Errorf("resolve metadata path: %w", err)
	}
	if strings.ToLower(filepath.Ext(absolute)) != ".jxl" {
		return "", fmt.Errorf("metadata inspection requires a .jxl file")
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("stat metadata path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("metadata path is not a regular file")
	}
	if s.tools == nil {
		return "", errors.New("toolchain service is not configured")
	}
	installed, err := s.tools.Installed(context.Background())
	if err != nil {
		return "", fmt.Errorf("toolchain is not ready: %w", err)
	}
	return jxlinfo.NewRunner(installed.JXLInfoPath).Inspect(context.Background(), absolute)
}

// FileUpdate is emitted when one file finishes.
type FileUpdate struct {
	Input      string `json:"input"`
	Output     string `json:"output"`
	Format     string `json:"format"`
	Route      string `json:"route"`
	InputSize  int64  `json:"inputSize"`
	OutputSize int64  `json:"outputSize"`
	Skipped    bool   `json:"skipped"`
	SkipReason string `json:"skipReason"`
	Cancelled  bool   `json:"cancelled"`
	Error      string `json:"error"`
}

// ProgressUpdate is emitted while a conversion is running.
type ProgressUpdate struct {
	Total      int     `json:"total"`
	Completed  int     `json:"completed"`
	Failed     int     `json:"failed"`
	Skipped    int     `json:"skipped"`
	InFlight   int     `json:"inFlight"`
	BytesTotal int64   `json:"bytesTotal"`
	BytesDone  int64   `json:"bytesDone"`
	Throughput float64 `json:"throughput"`
	ETASeconds int64   `json:"etaSeconds"`
	Coalesced  int     `json:"coalesced"`
	Paused     bool    `json:"paused"`
	Percent    float64 `json:"percent"`
}

// ConversionSummary is emitted when a run finishes.
type ConversionSummary struct {
	Total     int   `json:"total"`
	Completed int   `json:"completed"`
	Failed    int   `json:"failed"`
	Skipped   int   `json:"skipped"`
	Cancelled bool  `json:"cancelled"`
	BytesIn   int64 `json:"bytesIn"`
	BytesOut  int64 `json:"bytesOut"`
}

// StartConversion starts an asynchronous conversion; progress arrives through
// Wails events so the UI remains responsive.
func (s *Service) StartConversion(paths []string, options ConversionOptions) error {
	p, err := s.effectivePreset(options)
	if err != nil {
		return err
	}
	inputs, err := expandPaths(paths)
	if err != nil {
		return err
	}
	if len(inputs) == 0 {
		return errors.New("no input files were selected")
	}
	if s.tools == nil {
		return errors.New("toolchain is not configured")
	}
	installed, err := s.tools.Installed(context.Background())
	if err != nil {
		return fmt.Errorf("toolchain is not ready: %w", err)
	}
	if hasExpertArguments(p) {
		flagStatus, err := s.tools.CheckFlags(context.Background(), installed)
		if err != nil {
			return fmt.Errorf("read installed cjxl flags: %w", err)
		}
		if flagStatus.Locked || len(flagStatus.Added) > 0 || len(flagStatus.Removed) > 0 {
			return fmt.Errorf("Expert flags are locked for cjxl %s; generated flags target %s", installed.Version, flags.GeneratedVersion)
		}
	}

	s.mu.Lock()
	if s.engine != nil {
		engine := s.engine
		s.mu.Unlock()
		engine.Add(inputs)
		return nil
	}

	engine := convert.New(
		convert.Deps{
			Encoder:  cjxl.NewRunner(installed.CJXLPath),
			Verifier: djxl.NewVerifier(installed.DJXLPath),
		},
		convert.Settings{
			Processes: options.Processes,
			Threads:   options.Threads,
			Preset:    p,
			Deletion:  output.DeletionByRoute{},
		},
	)
	engine.OnProgress = func(progress convert.Progress) {
		s.emit("progress", progressUpdate(progress))
	}
	engine.OnFile = func(result convert.FileResult) {
		s.emit("conversion-file", fileUpdate(result))
	}
	s.engine = engine
	s.activePreset = p.Name
	s.mu.Unlock()

	go func() {
		engine.Start(context.Background())
		engine.Add(inputs)
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
	return nil
}

// PauseConversion pauses dispatching new files.
func (s *Service) PauseConversion() error {
	engine := s.currentEngine()
	if engine == nil {
		return errors.New("no conversion is running")
	}
	engine.Pause()
	return nil
}

// ResumeConversion resumes a paused conversion.
func (s *Service) ResumeConversion() error {
	engine := s.currentEngine()
	if engine == nil {
		return errors.New("no conversion is running")
	}
	engine.Resume()
	return nil
}

// CancelConversion cancels in-flight processes and queued work.
func (s *Service) CancelConversion() error {
	engine := s.currentEngine()
	if engine == nil {
		return errors.New("no conversion is running")
	}
	engine.Cancel()
	return nil
}

// GetProgress returns the current progress snapshot, or an empty snapshot when
// no conversion is running.
func (s *Service) GetProgress() ProgressUpdate {
	engine := s.currentEngine()
	if engine == nil {
		return ProgressUpdate{}
	}
	return progressUpdate(engine.Progress())
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
// Expert flag lock/diff. Errors are returned for display, never hidden.
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
// atomic installation. It is deliberately not called by status queries.
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

func (s *Service) effectivePreset(options ConversionOptions) (preset.Preset, error) {
	name := strings.TrimSpace(options.Preset)
	if name == "" {
		s.mu.Lock()
		name = s.cfg.Bindings[config.EntryGUI]
		s.mu.Unlock()
	}
	if name == "" {
		return preset.Preset{}, errors.New("the graphical-interface preset is not bound")
	}
	p, err := preset.NewStore(s.paths.PresetsDir).Load(name)
	if err != nil {
		return preset.Preset{}, err
	}
	if options.JPEGMode != "" && options.JPEGMode != "transcode" && options.JPEGMode != "reencode" {
		return preset.Preset{}, fmt.Errorf("unknown JPEG mode %q", options.JPEGMode)
	}
	if options.Processes < 0 || options.Threads < 0 {
		return preset.Preset{}, errors.New("processes and threads cannot be negative")
	}
	if options.Processes == 0 {
		options.Processes = 1
	}
	if options.UseDistance && (options.Distance < 0 || options.Distance > 25) {
		return preset.Preset{}, errors.New("distance must be between 0 and 25")
	}
	if options.UseEffort && (options.Effort < 1 || options.Effort > 10) {
		return preset.Preset{}, errors.New("effort must be between 1 and 10")
	}
	if err := validateExpertFlags(options.ExpertFlags); err != nil {
		return preset.Preset{}, err
	}
	if options.OutputPolicy != "" {
		policy := preset.Policy(options.OutputPolicy)
		if policy != preset.PolicyAlongside && policy != preset.PolicySubfolder && policy != preset.PolicyReplace {
			return preset.Preset{}, fmt.Errorf("unknown output policy %q", options.OutputPolicy)
		}
		p.Output.Policy = policy
		if policy == preset.PolicySubfolder && p.Output.Subfolder == "" {
			p.Output.Subfolder = "jxl"
		}
	}
	for i := range p.Rules {
		args := append([]cjxl.Arg(nil), p.Rules[i].Args...)
		if options.ResetExpert {
			args = clearExpertFlags(args)
		}
		args = applyExpertOverrides(args, options.ExpertFlags)
		if options.JPEGMode != "" && ruleMatchesJPEG(p.Rules[i]) {
			value := "1"
			if options.JPEGMode == "reencode" {
				value = "0"
			}
			args = setArg(args, []string{"-j", "--lossless_jpeg"}, cjxl.Arg{Key: "--lossless_jpeg", Value: value})
		}
		transcodeJPEG := ruleMatchesJPEG(p.Rules[i]) && preset.EffectiveLosslessJPEG(args)
		if transcodeJPEG {
			args = removeArgs(args, "-d", "--distance", "-q", "--quality")
		} else if options.UseDistance {
			args = removeArgs(args, "-q", "--quality")
			args = setArg(args, []string{"-d", "--distance"}, cjxl.Arg{Key: "--distance", Value: formatFloat(options.Distance)})
		}
		if options.UseEffort {
			args = setArg(args, []string{"-e", "--effort"}, cjxl.Arg{Key: "--effort", Value: fmt.Sprintf("%d", options.Effort)})
		}
		p.Rules[i].Args = args
	}
	if err := p.Validate(); err != nil {
		return preset.Preset{}, err
	}
	if err := p.ValidateArgs(flags.Default()); err != nil {
		return preset.Preset{}, err
	}
	return p, nil
}

func validateExpertFlags(overrides []FlagOverride) error {
	set := flags.Default()
	for _, override := range overrides {
		key := strings.TrimSpace(override.Key)
		if key == "" {
			return errors.New("Expert flag key is required")
		}
		definition, ok := set.Lookup(key)
		if !ok {
			return fmt.Errorf("unknown cjxl flag %q (not present in cjxl %s)", key, set.Version)
		}
		if definition.TakesValue && override.Valueless {
			return fmt.Errorf("Expert flag %q requires a value", key)
		}
		if !definition.TakesValue && !override.Valueless {
			return fmt.Errorf("Expert flag %q does not take a value", key)
		}
		if definition.TakesValue && override.Value == "" {
			return fmt.Errorf("Expert flag %q requires a value", key)
		}
		if !definition.TakesValue && override.Value != "" {
			return fmt.Errorf("Expert flag %q is valueless", key)
		}
	}
	return nil
}

func applyExpertOverrides(args []cjxl.Arg, overrides []FlagOverride) []cjxl.Arg {
	set := flags.Default()
	for _, override := range overrides {
		key := strings.TrimSpace(override.Key)
		if key == "" {
			continue
		}
		arg := cjxl.Arg{Key: key, Value: override.Value, Valueless: override.Valueless}
		aliases := []string{key}
		if definition, ok := set.Lookup(key); ok {
			aliases = definition.Tokens()
		}
		args = removeArgs(args, aliases...)
		args = append(args, arg)
	}
	return args
}

func clearExpertFlags(args []cjxl.Arg) []cjxl.Arg {
	set := flags.Default()
	result := args[:0]
	for _, arg := range args {
		if _, known := set.Lookup(arg.Key); known && !isCoreFlag(arg.Key) {
			continue
		}
		result = append(result, arg)
	}
	return result
}

func isCoreFlag(key string) bool {
	switch key {
	case "-d", "--distance", "-q", "--quality", "-e", "--effort", "-j", "--lossless_jpeg", "--num_threads":
		return true
	default:
		return false
	}
}

func hasExpertArguments(p preset.Preset) bool {
	for _, rule := range p.Rules {
		for _, arg := range rule.Args {
			if !isCoreFlag(arg.Key) {
				return true
			}
		}
	}
	return false
}

func addPreviewThreads(args []cjxl.Arg, threads int) []cjxl.Arg {
	if threads <= 0 {
		return args
	}
	for _, arg := range args {
		if arg.Key == "--num_threads" {
			return args
		}
	}
	return append(args, cjxl.Arg{Key: "--num_threads", Value: strconv.Itoa(threads)})
}

func summarizeCoreValue(p preset.Preset) string {
	var values []string
	for _, rule := range p.Rules {
		if onlyJPEGTranscode(rule) {
			continue
		}
		value := "default"
		for _, arg := range rule.Args {
			switch arg.Key {
			case "-d", "--distance":
				value = "d " + arg.Value
			case "-q", "--quality":
				value = "q " + arg.Value
			default:
				continue
			}
			break
		}
		values = append(values, value)
	}
	return summarizeValues(values, "default")
}

func summarizeEffort(p preset.Preset) string {
	values := make([]string, 0, len(p.Rules))
	for _, rule := range p.Rules {
		value := "default"
		for _, arg := range rule.Args {
			if arg.Key == "-e" || arg.Key == "--effort" {
				value = arg.Value
				break
			}
		}
		values = append(values, value)
	}
	return summarizeValues(values, "default")
}

func summarizeJPEGMode(p preset.Preset) string {
	var values []string
	for _, rule := range p.Rules {
		if !ruleMatchesJPEG(rule) {
			continue
		}
		if preset.EffectiveLosslessJPEG(rule.Args) {
			values = append(values, "lossless")
		} else {
			values = append(values, "lossy")
		}
	}
	return summarizeValues(values, "default")
}

func summarizeRules(p preset.Preset) []PresetRuleSummary {
	result := make([]PresetRuleSummary, 0, len(p.Rules))
	for _, rule := range p.Rules {
		jpegMode := "n/a"
		if ruleMatchesJPEG(rule) {
			if preset.EffectiveLosslessJPEG(rule.Args) {
				jpegMode = "lossless"
			} else {
				jpegMode = "lossy"
			}
		}
		coreValue := "default"
		if onlyJPEGTranscode(rule) {
			coreValue = "n/a"
		} else {
			for _, arg := range rule.Args {
				switch arg.Key {
				case "-d", "--distance":
					coreValue = "d " + arg.Value
				case "-q", "--quality":
					coreValue = "q " + arg.Value
				default:
					continue
				}
				break
			}
		}
		effortValue := "default"
		for _, arg := range rule.Args {
			if arg.Key == "-e" || arg.Key == "--effort" {
				effortValue = arg.Value
				break
			}
		}
		result = append(result, PresetRuleSummary{
			Matches:   append([]string(nil), rule.Match...),
			CoreValue: coreValue,
			Effort:    effortValue,
			JPEGMode:  jpegMode,
		})
	}
	return result
}

func onlyJPEGTranscode(rule preset.Rule) bool {
	if len(rule.Match) == 0 || !preset.EffectiveLosslessJPEG(rule.Args) {
		return false
	}
	for _, match := range rule.Match {
		if !strings.EqualFold(match, "JPEG") {
			return false
		}
	}
	return true
}

func summarizeValues(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	if len(seen) == 1 {
		return values[0]
	}
	return "Mixed"
}

func ruleMatchesJPEG(rule preset.Rule) bool {
	for _, match := range rule.Match {
		if match == "*" || strings.EqualFold(match, "JPEG") {
			return true
		}
	}
	return false
}

func setArg(args []cjxl.Arg, keys []string, replacement cjxl.Arg) []cjxl.Arg {
	for i := range args {
		for _, key := range keys {
			if args[i].Key == key {
				args[i] = replacement
				return args
			}
		}
	}
	return append(args, replacement)
}

func removeArgs(args []cjxl.Arg, keys ...string) []cjxl.Arg {
	result := args[:0]
	for _, arg := range args {
		remove := false
		for _, key := range keys {
			if arg.Key == key {
				remove = true
				break
			}
		}
		if !remove {
			result = append(result, arg)
		}
	}
	return result
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func cloneBindings(bindings map[config.EntryPoint]string) map[config.EntryPoint]string {
	result := make(map[config.EntryPoint]string, len(bindings)+1)
	for key, value := range bindings {
		result[key] = value
	}
	return result
}

func (s *Service) currentEngine() *convert.Engine {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.engine
}

func (s *Service) emit(name string, data any) {
	if s.cb.Emit != nil {
		s.cb.Emit(name, data)
	}
}

func expandPaths(inputs []string) ([]string, error) {
	seen := make(map[string]struct{})
	var result []string
	for _, input := range inputs {
		if strings.TrimSpace(input) == "" {
			continue
		}
		absolute, err := filepath.Abs(input)
		if err != nil {
			return nil, fmt.Errorf("resolve %q: %w", input, err)
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return nil, fmt.Errorf("stat %q: %w", input, err)
		}
		if !info.IsDir() {
			addPath(absolute, seen, &result)
			continue
		}
		entries, err := os.ReadDir(absolute)
		if err != nil {
			return nil, fmt.Errorf("read directory %q: %w", input, err)
		}
		for _, entry := range entries {
			if !entry.Type().IsRegular() {
				continue
			}
			addPath(filepath.Join(absolute, entry.Name()), seen, &result)
		}
	}
	return result, nil
}

func addPath(path string, seen map[string]struct{}, result *[]string) {
	key := strings.ToLower(filepath.Clean(path))
	if _, exists := seen[key]; exists {
		return
	}
	seen[key] = struct{}{}
	*result = append(*result, path)
}

func progressUpdate(progress convert.Progress) ProgressUpdate {
	processed := progress.Completed + progress.Failed + progress.Skipped
	percent := 0.0
	if progress.Total > 0 {
		percent = float64(processed) * 100 / float64(progress.Total)
	}
	return ProgressUpdate{
		Total:      progress.Total,
		Completed:  progress.Completed,
		Failed:     progress.Failed,
		Skipped:    progress.Skipped,
		InFlight:   progress.InFlight,
		BytesTotal: progress.BytesTotal,
		BytesDone:  progress.BytesDone,
		Throughput: progress.Throughput,
		ETASeconds: int64(progress.ETA.Seconds()),
		Coalesced:  progress.Coalesced,
		Paused:     progress.Paused,
		Percent:    percent,
	}
}

func fileUpdate(result convert.FileResult) FileUpdate {
	update := FileUpdate{
		Input:      result.Input,
		Output:     result.Output,
		Format:     string(result.Format),
		Route:      result.Route.String(),
		InputSize:  result.InputSize,
		OutputSize: result.OutputSize,
		Skipped:    result.Skipped,
		SkipReason: result.SkipReason,
		Cancelled:  result.Cancelled,
	}
	if result.Err != nil {
		update.Error = result.Err.Error()
	}
	return update
}
