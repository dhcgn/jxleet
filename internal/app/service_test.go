package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dhcgn/jxleet/internal/cjxl"
	"github.com/dhcgn/jxleet/internal/config"
	"github.com/dhcgn/jxleet/internal/preset"
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
	return New(paths, config.Default(), nil, Callbacks{})
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

func TestEffectivePresetAppliesGUIOverridesWithoutSaving(t *testing.T) {
	service := testService(t)
	saveTestPreset(t, service)

	p, err := service.effectivePreset(ConversionOptions{
		Preset:       "test",
		Processes:    2,
		Threads:      4,
		JPEGMode:     "reencode",
		Distance:     1.5,
		UseDistance:  true,
		Effort:       3,
		UseEffort:    true,
		OutputPolicy: string(preset.PolicySubfolder),
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Output.Policy != preset.PolicySubfolder || p.Output.Subfolder != "jxl" {
		t.Fatalf("output override = %+v", p.Output)
	}
	for i, rule := range p.Rules {
		if i == 0 {
			for _, arg := range rule.Args {
				if arg.Key == "--lossless_jpeg" && arg.Value != "0" {
					t.Errorf("JPEG mode = %s", arg.Value)
				}
			}
			continue
		}
		for _, arg := range rule.Args {
			if arg.Key == "-q" || arg.Key == "--quality" {
				t.Error("quality flag should be replaced by distance")
			}
			if arg.Key == "--distance" && arg.Value != "1.5" {
				t.Errorf("distance = %s", arg.Value)
			}
			if arg.Key == "--effort" && arg.Value != "3" {
				t.Errorf("effort = %s", arg.Value)
			}
		}
	}

	stored, err := preset.NewStore(service.paths.PresetsDir).Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Rules[0].Args[0].Key != "-q" {
		t.Error("GUI overrides must not modify the stored preset")
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
		Preset:      "test",
		JPEGMode:    "transcode",
		Distance:    1.5,
		UseDistance: true,
		UseEffort:   true,
		Effort:      7,
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

func TestSavePresetOutputPolicy(t *testing.T) {
	service := testService(t)
	saveTestPreset(t, service)
	if err := service.SavePresetOutputPolicy("test", string(preset.PolicyReplace)); err != nil {
		t.Fatal(err)
	}
	updated, err := preset.NewStore(service.paths.PresetsDir).Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Output.Policy != preset.PolicyReplace {
		t.Fatalf("policy = %q", updated.Output.Policy)
	}

	defaultStore := preset.NewStore(filepath.Join(t.TempDir(), "presets"))
	if _, err := preset.EnsureDefaults(defaultStore); err != nil {
		t.Fatal(err)
	}
	service.paths.PresetsDir = defaultStore.Dir
	if err := service.SavePresetOutputPolicy("default-gui", string(preset.PolicyReplace)); err == nil {
		t.Error("read-only default policy change should fail")
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
		ExpertFlags: []FlagOverride{
			{Key: "--progressive", Valueless: true},
			{Key: "--modular", Value: "1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range p.Rules {
		var progressive, modular bool
		for _, arg := range rule.Args {
			switch arg.Key {
			case "--progressive":
				progressive = arg.Valueless
			case "--modular":
				modular = arg.Value == "1"
			}
		}
		if !progressive || !modular {
			t.Fatalf("expert overrides missing from rule: %+v", rule.Args)
		}
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
		Preset:      "preview",
		Threads:     4,
		JPEGMode:    "reencode",
		Distance:    1.5,
		UseDistance: true,
		Effort:      3,
		UseEffort:   true,
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
				Preset:      "test",
				ExpertFlags: []FlagOverride{override},
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
		Preset:      presetName,
		ResetExpert: true,
		ExpertFlags: []FlagOverride{{Key: "--container", Value: "1"}},
		UseDistance: false,
		UseEffort:   false,
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
