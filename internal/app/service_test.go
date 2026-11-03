package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dhcgn/jxleet/internal/cjxl"
	"github.com/dhcgn/jxleet/internal/config"
	"github.com/dhcgn/jxleet/internal/convert"
	"github.com/dhcgn/jxleet/internal/preset"
	"github.com/dhcgn/jxleet/internal/routes"
)

func testService(t *testing.T) *Service {
	t.Helper()
	root := t.TempDir()
	paths := config.Paths{
		ConfigDir:  filepath.Join(root, "config"),
		ConfigFile: filepath.Join(root, "config", "config.yaml"),
		PresetsDir: filepath.Join(root, "config", "presets"),
		BinDir:     filepath.Join(root, "local", "bin"),
		LogsDir:    filepath.Join(root, "local", "logs"),
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	return New(paths, config.Default(), nil, "dev", Callbacks{})
}

func saveTestPreset(t *testing.T, service *Service) {
	t.Helper()
	err := preset.NewStore(service.paths.PresetsDir).Save(preset.Preset{
		Name:    "test",
		Version: preset.CurrentVersion,
		Output:  preset.DefaultOutput(),
		Rules: []preset.Rule{{
			Match: []string{"JPEG"},
			Args:  []cjxl.Arg{{Key: "-q", Value: "90"}, {Key: "-e", Value: "7"}},
		}, {
			Match: []string{"*"},
			Args:  []cjxl.Arg{{Key: "-d", Value: "1.0"}, {Key: "-e", Value: "7"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Issue 75: a rule nobody edited must run exactly as stored, not with the
// fallback rule's values the GUI controls start from.
func TestEffectivePresetKeepsUntouchedRulesVerbatim(t *testing.T) {
	service := testService(t)
	store := preset.NewStore(service.paths.PresetsDir)
	if _, err := preset.EnsureDefaults(store); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(preset.Preset{
		Name:    "issue-75",
		Version: preset.CurrentVersion,
		Output:  preset.Output{Policy: preset.PolicySubfolder, Subfolder: "jxl", EmbedSettings: true},
		Rules: []preset.Rule{
			{Match: []string{"JXL"}, Args: []cjxl.Arg{{Key: "-d", Value: "0.7"}, {Key: "-e", Value: "3"}}},
			{Match: []string{"JPEG"}, Args: []cjxl.Arg{{Key: "--lossless_jpeg", Value: "0"}, {Key: "-d", Value: "2.90"}, {Key: "-e", Value: "1"}}},
			{Match: []string{"PNG"}, Args: []cjxl.Arg{{Key: "-d", Value: "0"}, {Key: "-e", Value: "9"}}},
			{Match: []string{"*"}, Args: []cjxl.Arg{{Key: "-d", Value: "0.9"}, {Key: "-e", Value: "2"}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"issue-75", "default-gui"} {
		t.Run(name, func(t *testing.T) {
			stored, err := store.Load(name)
			if err != nil {
				t.Fatal(err)
			}
			p, err := service.effectivePreset(ConversionOptions{Preset: name, OutputPolicy: string(stored.Output.Policy)})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(p.Rules, stored.Rules) {
				t.Errorf("untouched rules changed:\n got %+v\nwant %+v", p.Rules, stored.Rules)
			}
		})
	}
}

func TestEffectivePresetOverrideTouchesOnlyItsRule(t *testing.T) {
	service := testService(t)
	saveTestPreset(t, service)

	p, err := service.effectivePreset(ConversionOptions{
		Preset:       "test",
		Processes:    2,
		Threads:      4,
		OutputPolicy: string(preset.PolicySubfolder),
		Rules: []RuleOverride{{
			Index:       1,
			JPEGMode:    "reencode",
			Distance:    1.5,
			UseDistance: true,
			Effort:      3,
			UseEffort:   true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Output.Policy != preset.PolicySubfolder || p.Output.Subfolder != "jxl" {
		t.Fatalf("output override = %+v", p.Output)
	}
	want := []cjxl.Arg{{Key: "--distance", Value: "1.5"}, {Key: "--effort", Value: "3"}, {Key: "--lossless_jpeg", Value: "0"}}
	if !reflect.DeepEqual(p.Rules[1].Args, want) {
		t.Errorf("edited rule = %+v, want %+v", p.Rules[1].Args, want)
	}

	stored, err := preset.NewStore(service.paths.PresetsDir).Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Rules[0], stored.Rules[0]) {
		t.Errorf("unedited rule changed: %+v", p.Rules[0].Args)
	}
	if stored.Rules[1].Args[0].Key != "-d" || stored.Rules[1].Args[0].Value != "1.0" {
		t.Error("GUI overrides must not modify the stored preset")
	}
}

func TestEffectivePresetRejectsUnknownRuleIndex(t *testing.T) {
	service := testService(t)
	saveTestPreset(t, service)
	for _, index := range []int{-1, 2} {
		_, err := service.effectivePreset(ConversionOptions{Preset: "test", Rules: []RuleOverride{{Index: index, UseEffort: true, Effort: 3}}})
		if err == nil {
			t.Errorf("override for rule %d should be rejected", index)
		}
	}
}

func TestSetBindingPersists(t *testing.T) {
	service := testService(t)
	saveTestPreset(t, service)
	if err := service.SetBinding("gui", "test"); err != nil {
		t.Fatal(err)
	}
	if got := service.GetBindings(); got.GUI != "test" {
		t.Fatalf("GUI binding = %q", got.GUI)
	}
	loaded, err := config.Load(service.paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := loaded.Binding(config.EntryGUI); !ok || got != "test" {
		t.Fatalf("persisted binding = %q, ok=%v", got, ok)
	}
}

func TestEffectivePresetOmitsDistanceForJPEGTranscode(t *testing.T) {
	service := testService(t)
	saveTestPreset(t, service)
	p, err := service.effectivePreset(ConversionOptions{
		Preset: "test",
		Rules: []RuleOverride{{
			Index:       0,
			JPEGMode:    "transcode",
			Distance:    1.5,
			UseDistance: true,
			UseEffort:   true,
			Effort:      7,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range p.Rules[0].Args {
		if arg.Key == "-d" || arg.Key == "--distance" || arg.Key == "-q" || arg.Key == "--quality" {
			t.Errorf("transcode rule contains quality setting: %+v", arg)
		}
		if arg.Key == "--lossless_jpeg" && arg.Value != "1" {
			t.Errorf("JPEG mode = %s", arg.Value)
		}
	}
}

func TestExpandPathsUsesDirectFilesAndDeduplicates(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(root, "a.png")
	b := filepath.Join(nested, "b.png")
	for _, path := range []string{a, b} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := expandPaths([]string{root, a})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("expanded paths = %v", paths)
	}
	if paths[0] != a {
		t.Fatalf("expanded paths = %v, want only %s", paths, a)
	}
}

// A vanished path must not drop its batch (nor freeze every later preview):
// it stays in the list so the engine can fail it on its own row.
func TestExpandPathsKeepsMissingFiles(t *testing.T) {
	root := t.TempDir()
	present := filepath.Join(root, "a.png")
	if err := os.WriteFile(present, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "gone.d0.75-e7-cjxl0.12.0.jxl")
	paths, err := expandPaths([]string{missing, present})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != missing || paths[1] != present {
		t.Fatalf("expanded paths = %v", paths)
	}
}

func TestPreviewPathsFlagsMissingFile(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "gone.jxl")
	previews, err := (&Service{}).PreviewPaths([]string{missing}, ConversionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(previews) != 1 || !previews[0].Skip || previews[0].Reason != "file not found" {
		t.Fatalf("previews = %+v", previews)
	}
}

func TestExplorerArgs(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "a.jxl")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(root, "gone.jxl")
	orphan := filepath.Join(root, "no-such-dir", "gone.jxl")

	if args, isGone, err := explorerArgs(file); err != nil || isGone || len(args) != 2 || args[0] != "/select," || args[1] != file {
		t.Errorf("file: args=%v gone=%v err=%v, want select", args, isGone, err)
	}
	if args, isGone, err := explorerArgs(root); err != nil || isGone || len(args) != 1 || args[0] != root {
		t.Errorf("folder: args=%v gone=%v err=%v, want open", args, isGone, err)
	}
	if args, isGone, err := explorerArgs(gone); err != nil || !isGone || len(args) != 1 || args[0] != root {
		t.Errorf("vanished file: args=%v gone=%v err=%v, want parent folder", args, isGone, err)
	}
	if _, _, err := explorerArgs(orphan); err == nil {
		t.Error("vanished file and folder: want error")
	}
}

func TestOpenConvertedFileMissingSaysSo(t *testing.T) {
	err := (&Service{}).OpenConvertedFile(filepath.Join(t.TempDir(), "gone.jxl"))
	if err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Errorf("error = %v, want a no-longer-exists message", err)
	}
}

func TestPresetSummaryValues(t *testing.T) {
	uniform := preset.Preset{
		Rules: []preset.Rule{{
			Match: []string{"*"},
			Args: []cjxl.Arg{
				{Key: "--distance", Value: "1.0"},
				{Key: "--effort", Value: "7"},
				{Key: "--lossless_jpeg", Value: "1"},
			},
		}},
	}
	if got := summarizeCoreValue(uniform); got != "d 1.0" {
		t.Errorf("uniform core value = %q", got)
	}
	if got := summarizeEffort(uniform); got != "7" {
		t.Errorf("uniform effort = %q", got)
	}
	if got := summarizeJPEGMode(uniform); got != "lossless" {
		t.Errorf("uniform JPEG mode = %q", got)
	}

	mixed := preset.Preset{
		Rules: []preset.Rule{
			{Match: []string{"JPEG"}, Args: []cjxl.Arg{{Key: "-e", Value: "7"}, {Key: "-j", Value: "1"}}},
			{Match: []string{"PNG"}, Args: []cjxl.Arg{{Key: "-q", Value: "90"}, {Key: "-e", Value: "9"}}},
			{Match: []string{"*"}, Args: []cjxl.Arg{{Key: "-d", Value: "2.0"}, {Key: "-e", Value: "7"}, {Key: "-j", Value: "0"}}},
		},
	}
	if got := summarizeCoreValue(mixed); got != "Mixed" {
		t.Errorf("mixed core value = %q", got)
	}
	if got := summarizeEffort(mixed); got != "Mixed" {
		t.Errorf("mixed effort = %q", got)
	}
	if got := summarizeJPEGMode(mixed); got != "Mixed" {
		t.Errorf("mixed JPEG mode = %q", got)
	}

	defaults := preset.Preset{
		Rules: []preset.Rule{{Match: []string{"JPEG"}, Args: []cjxl.Arg{{Key: "-j", Value: "1"}}}},
	}
	if got := summarizeCoreValue(defaults); got != "default" {
		t.Errorf("default core value = %q", got)
	}
	if got := summarizeEffort(defaults); got != "default" {
		t.Errorf("default effort = %q", got)
	}
}

func TestPresetRuleSummaries(t *testing.T) {
	p := preset.Preset{
		Rules: []preset.Rule{
			{Match: []string{"JPEG"}, Args: []cjxl.Arg{{Key: "--lossless_jpeg", Value: "1"}}},
			{Match: []string{"PNG"}, Args: []cjxl.Arg{{Key: "-d", Value: "0"}, {Key: "-e", Value: "9"}}},
			{Match: []string{"*"}, Args: []cjxl.Arg{{Key: "-q", Value: "90"}, {Key: "-e", Value: "7"}, {Key: "-j", Value: "0"}}},
		},
	}
	rules := summarizeRules(p)
	if len(rules) != 3 {
		t.Fatalf("got %d rule summaries", len(rules))
	}
	if rules[0].CoreValue != "n/a" || rules[0].JPEGMode != "lossless" {
		t.Errorf("JPEG rule summary = %+v", rules[0])
	}
	if rules[1].CoreValue != "d 0" || rules[1].Effort != "9" || rules[1].JPEGMode != "n/a" {
		t.Errorf("PNG rule summary = %+v", rules[1])
	}
	if rules[2].CoreValue != "q 90" || rules[2].Effort != "7" || rules[2].JPEGMode != "lossy" {
		t.Errorf("fallback rule summary = %+v", rules[2])
	}
}

func TestSavePresetOutput(t *testing.T) {
	service := testService(t)
	saveTestPreset(t, service)
	if err := service.SavePresetOutput("test", string(preset.PolicyReplace), string(preset.CollisionOverwrite), true, true); err != nil {
		t.Fatal(err)
	}
	updated, err := preset.NewStore(service.paths.PresetsDir).Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Output.Policy != preset.PolicyReplace || updated.Output.OnCollision != preset.CollisionOverwrite {
		t.Fatalf("output = %+v", updated.Output)
	}
	if !updated.Output.EmbedSettings {
		t.Errorf("embed_settings = false, want true")
	}
	if !updated.Output.JXLInfoSidecar {
		t.Errorf("jxlinfo_sidecar = false, want true")
	}

	defaultStore := preset.NewStore(filepath.Join(t.TempDir(), "presets"))
	if _, err := preset.EnsureDefaults(defaultStore); err != nil {
		t.Fatal(err)
	}
	service.paths.PresetsDir = defaultStore.Dir
	if err := service.SavePresetOutput("default-gui", string(preset.PolicyReplace), string(preset.CollisionSkip), false, false); err == nil {
		t.Error("read-only default output change should fail")
	}
}

func TestPreviewPathsRetainsFilesWithoutPreset(t *testing.T) {
	service := testService(t)
	input := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(input, []byte("not a real PNG"), 0o644); err != nil {
		t.Fatal(err)
	}

	preview, err := service.PreviewPaths([]string{input}, ConversionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview) != 1 {
		t.Fatalf("preview length = %d", len(preview))
	}
	if preview[0].Path != input || preview[0].Format != "PNG" || preview[0].Reason != "select a preset to classify" {
		t.Fatalf("preview = %+v", preview[0])
	}
}

func TestListCJXLFlagsIncludesGeneratedHelp(t *testing.T) {
	service := testService(t)
	flags := service.ListCJXLFlags()
	if len(flags) < 10 {
		t.Fatalf("got only %d cjxl flags", len(flags))
	}
	var foundDistance, foundProgressive bool
	for _, flag := range flags {
		if flag.Key == "--distance" && flag.Description != "" {
			foundDistance = true
		}
		if flag.Key == "--progressive" && !flag.TakesValue {
			foundProgressive = true
		}
	}
	if !foundDistance || !foundProgressive {
		t.Fatalf("generated flag set missing expected entries: distance=%v progressive=%v", foundDistance, foundProgressive)
	}
}

func TestEffectivePresetAppliesExpertFlagOverrides(t *testing.T) {
	service := testService(t)
	saveTestPreset(t, service)
	p, err := service.effectivePreset(ConversionOptions{
		Preset: "test",
		Rules: []RuleOverride{{
			Index:    1,
			UseFlags: true,
			Flags: []FlagOverride{
				{Key: "--progressive", Valueless: true},
				{Key: "--modular", Value: "1"},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var progressive, modular bool
	for _, arg := range p.Rules[1].Args {
		switch arg.Key {
		case "--progressive":
			progressive = arg.Valueless
		case "--modular":
			modular = arg.Value == "1"
		}
	}
	if !progressive || !modular {
		t.Fatalf("expert overrides missing from edited rule: %+v", p.Rules[1].Args)
	}
	if hasExpertArguments(preset.Preset{Rules: p.Rules[:1]}) {
		t.Errorf("expert overrides leaked into another rule: %+v", p.Rules[0].Args)
	}
}

func TestPreviewCommandsIncludesResolvedPresetFlags(t *testing.T) {
	service := testService(t)
	err := preset.NewStore(service.paths.PresetsDir).Save(preset.Preset{
		Name:    "preview",
		Version: preset.CurrentVersion,
		Output:  preset.DefaultOutput(),
		Rules: []preset.Rule{{
			Match: []string{"*"},
			Args: []cjxl.Arg{
				{Key: "--distance", Value: "1.0"},
				{Key: "--effort", Value: "7"},
				{Key: "--progressive", Valueless: true},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	previews, err := service.PreviewCommands(ConversionOptions{
		Preset:  "preview",
		Threads: 4,
		Rules: []RuleOverride{{
			JPEGMode:    "reencode",
			Distance:    1.5,
			UseDistance: true,
			Effort:      3,
			UseEffort:   true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(previews) != 1 {
		t.Fatalf("preview count = %d", len(previews))
	}
	for _, want := range []string{"--distance=1.5", "--effort=3", "--progressive", "--num_threads=4"} {
		if !strings.Contains(previews[0].Command, want) {
			t.Errorf("command %q does not contain %q", previews[0].Command, want)
		}
	}
}

func TestEffectivePresetRejectsInvalidExpertFlagShape(t *testing.T) {
	service := testService(t)
	saveTestPreset(t, service)
	tests := []FlagOverride{
		{Key: "--progressive", Value: "1"},
		{Key: "--distance", Valueless: true},
		{Key: "--distance"},
	}
	for _, override := range tests {
		t.Run(override.Key, func(t *testing.T) {
			_, err := service.effectivePreset(ConversionOptions{
				Preset: "test",
				Rules:  []RuleOverride{{UseFlags: true, Flags: []FlagOverride{override}}},
			})
			if err == nil {
				t.Fatalf("override %+v should be rejected", override)
			}
		})
	}
}

func TestEffectivePresetResetsPresetExpertFlags(t *testing.T) {
	service := testService(t)
	presetName := "expert"
	err := preset.NewStore(service.paths.PresetsDir).Save(preset.Preset{
		Name:    presetName,
		Version: preset.CurrentVersion,
		Output:  preset.DefaultOutput(),
		Rules: []preset.Rule{{
			Match: []string{"*"},
			Args: []cjxl.Arg{
				{Key: "--distance", Value: "1.0"},
				{Key: "--effort", Value: "7"},
				{Key: "--progressive", Valueless: true},
				{Key: "--modular", Value: "1"},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := service.effectivePreset(ConversionOptions{
		Preset: presetName,
		Rules:  []RuleOverride{{UseFlags: true, Flags: []FlagOverride{{Key: "--container", Value: "1"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range resolved.Rules[0].Args {
		if arg.Key == "--progressive" || arg.Key == "--modular" {
			t.Fatalf("preset Expert flag survived reset: %+v", arg)
		}
	}
	foundContainer := false
	for _, arg := range resolved.Rules[0].Args {
		if arg.Key == "--container" && arg.Value == "1" {
			foundContainer = true
		}
	}
	if !foundContainer {
		t.Fatalf("new Expert override missing: %+v", resolved.Rules[0].Args)
	}
}

func TestHasExpertArguments(t *testing.T) {
	if hasExpertArguments(preset.Preset{
		Rules: []preset.Rule{{Args: []cjxl.Arg{{Key: "--distance", Value: "1.0"}, {Key: "--effort", Value: "7"}}}},
	}) {
		t.Error("core arguments should not require the Expert flag surface")
	}
	if !hasExpertArguments(preset.Preset{
		Rules: []preset.Rule{{Args: []cjxl.Arg{{Key: "--progressive", Valueless: true}}}},
	}) {
		t.Error("advanced arguments should require the Expert flag surface")
	}
}

func TestInspectJXLValidatesPathBeforeToolchain(t *testing.T) {
	service := testService(t)
	if _, err := service.InspectJXL(filepath.Join(t.TempDir(), "image.png")); err == nil {
		t.Error("non-JXL metadata inspection should fail")
	}
}

func TestSummarizeFileSettings(t *testing.T) {
	if got, flags := summarizeFileSettings(routes.RouteTranscode, nil); got != "lossless (reversible)" || flags {
		t.Errorf("transcode = %q flags=%v", got, flags)
	}
	got, flags := summarizeFileSettings(routes.RouteEncode, []cjxl.Arg{
		{Key: "--distance", Value: "0.3"},
		{Key: "--effort", Value: "7"},
		{Key: "--num_threads", Value: "8"},
	})
	if got != "D 0.3 · E 7" || flags {
		t.Errorf("encode core = %q flags=%v", got, flags)
	}
	got, flags = summarizeFileSettings(routes.RouteReencode, []cjxl.Arg{
		{Key: "--quality", Value: "90"},
		{Key: "--effort", Value: "9"},
		{Key: "--progressive", Valueless: true},
	})
	if got != "Q 90 · E 9" || !flags {
		t.Errorf("reencode with extra flag = %q flags=%v", got, flags)
	}
}

func TestGetPresetCore(t *testing.T) {
	service := testService(t)
	if err := preset.NewStore(service.paths.PresetsDir).Save(preset.Preset{
		Name:    "core",
		Version: preset.CurrentVersion,
		Output:  preset.Output{Policy: preset.PolicySubfolder, Subfolder: "jxl"},
		Rules: []preset.Rule{{
			Match: []string{"JPEG"},
			Args:  []cjxl.Arg{{Key: "-j", Value: "0"}},
		}, {
			Match: []string{"*"},
			Args:  []cjxl.Arg{{Key: "-q", Value: "90"}, {Key: "-e", Value: "8"}, {Key: "--progressive", Valueless: true}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	core, err := service.GetPresetCore("core")
	if err != nil {
		t.Fatal(err)
	}
	if core.Policy != "subfolder" || len(core.Rules) != 2 || core.Selected != 1 || core.JPEGRule != 0 {
		t.Fatalf("core = %+v", core)
	}
	jpegRule, fallback := core.Rules[0], core.Rules[1]
	if jpegRule.LosslessJPEG || !jpegRule.JPEGOnly || jpegRule.Distance != 1 || jpegRule.Effort != 7 || len(jpegRule.Flags) != 0 {
		t.Errorf("JPEG rule core = %+v", jpegRule)
	}
	if fallback.Effort != 8 || fallback.JPEGOnly {
		t.Errorf("fallback rule core = %+v", fallback)
	}
	if want := cjxl.DistanceFromQuality(90); fallback.Distance != want {
		t.Errorf("distance = %v, want %v (from -q 90)", fallback.Distance, want)
	}
	if len(fallback.Flags) != 1 || fallback.Flags[0].Key != "--progressive" {
		t.Errorf("flags = %+v", fallback.Flags)
	}
}

// A hand-edited preset with mistakes still loads, and reports every problem so
// the GUI can show them before anything is dropped.
func TestPresetProblemsReportedOnLoad(t *testing.T) {
	service := testService(t)
	yaml := "name: broken\nversion: 1\noutput:\n  policy: subfolder\nrules:\n" +
		"  - match: [JPEG, JPG]\n    args:\n      -d: 2.9\n" +
		"  - match: ['*']\n    args:\n      --bogus: 1\n"
	if err := os.WriteFile(filepath.Join(service.paths.PresetsDir, "broken.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	core, err := service.GetPresetCore("broken")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"output.subfolder is required", `rule 1: unknown format "JPG"`, `rule 2: unknown cjxl flag "--bogus"`}
	if len(core.Problems) != len(want) {
		t.Fatalf("problems = %q, want %d", core.Problems, len(want))
	}
	for i, w := range want {
		if !strings.Contains(core.Problems[i], w) || strings.HasPrefix(core.Problems[i], "preset:") {
			t.Errorf("problem %d = %q, want it to contain %q without the preset: prefix", i, core.Problems[i], w)
		}
	}
	summaries, err := service.ListPresets()
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || len(summaries[0].Problems) != len(want) {
		t.Errorf("library summary problems = %+v", summaries)
	}
}

func TestProgressUpdateStartsAtTen(t *testing.T) {
	if got := progressUpdate(convert.Progress{}); got.Percent != 0 {
		t.Errorf("empty percent = %v, want 0", got.Percent)
	}
	if got := progressUpdate(convert.Progress{Total: 4}); got.Percent != 10 {
		t.Errorf("started percent = %v, want 10", got.Percent)
	}
	if got := progressUpdate(convert.Progress{Total: 4, Completed: 2}); got.Percent != 55 {
		t.Errorf("half percent = %v, want 55", got.Percent)
	}
	if got := progressUpdate(convert.Progress{Total: 4, Completed: 4}); got.Percent != 100 {
		t.Errorf("done percent = %v, want 100", got.Percent)
	}
}

func TestFileUpdateCarriesSettings(t *testing.T) {
	update := fileUpdate(7, convert.FileResult{
		Input: "a.png",
		Route: routes.RouteEncode,
		Args:  []cjxl.Arg{{Key: "-d", Value: "0.5"}, {Key: "-e", Value: "9"}},
	})
	if update.Seq != 7 {
		t.Errorf("seq = %d, want 7", update.Seq)
	}
	if update.Settings != "D 0.5 \u00b7 E 9" {
		t.Errorf("settings = %q, want D/E chip", update.Settings)
	}
}

func TestFileUpdateCarriesDuration(t *testing.T) {
	update := fileUpdate(1, convert.FileResult{Input: "a.png", Duration: 1500 * 1000000})
	if update.DurationSeconds != 1.5 {
		t.Errorf("duration = %v, want 1.5", update.DurationSeconds)
	}
}
