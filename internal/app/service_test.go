package app

import (
	"os"
	"path/filepath"
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
