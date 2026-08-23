package preset

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dhcgn/jxleet/internal/cjxl"
)

func newSample(name string) Preset {
	return Preset{
		Name:    name,
		Version: CurrentVersion,
		Output:  Output{Policy: PolicyReplace, OnCollision: CollisionOverwrite},
		Rules: []Rule{
			{Match: []string{"*"}, Args: []cjxl.Arg{{Key: "-d", Value: "1.0"}, {Key: "-e", Value: "7"}}},
		},
	}
}

func TestStoreCRUD(t *testing.T) {
	s := NewStore(t.TempDir())

	if err := s.Save(newSample("web")); err != nil {
		t.Fatal(err)
	}
	if !s.Exists("web") {
		t.Fatal("saved preset should exist")
	}

	got, err := s.Load("web")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "web" || len(got.Rules) != 1 {
		t.Errorf("unexpected loaded preset: %+v", got)
	}

	if err := s.Duplicate("web", "web-copy"); err != nil {
		t.Fatal(err)
	}
	if !s.Exists("web-copy") {
		t.Error("duplicate should exist")
	}
	if err := s.Duplicate("web", "web-copy"); err == nil {
		t.Error("duplicating onto an existing name should fail")
	}

	if err := s.Rename("web-copy", "web-renamed"); err != nil {
		t.Fatal(err)
	}
	if s.Exists("web-copy") || !s.Exists("web-renamed") {
		t.Error("rename did not move the preset")
	}

	if err := s.Delete("web-renamed"); err != nil {
		t.Fatal(err)
	}
	if s.Exists("web-renamed") {
		t.Error("deleted preset should be gone")
	}

	names, err := s.Names()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "web" {
		t.Errorf("names = %v, want [web]", names)
	}
}

func TestStoreExportImportResetsPolicy(t *testing.T) {
	src := NewStore(t.TempDir())
	if err := src.Save(newSample("archive")); err != nil {
		t.Fatal(err)
	}
	exportPath := filepath.Join(t.TempDir(), "archive.yaml")
	if err := src.Export("archive", exportPath); err != nil {
		t.Fatal(err)
	}

	dst := NewStore(t.TempDir())
	name, err := dst.Import(exportPath, CollisionNumber)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := dst.Load(name)
	if err != nil {
		t.Fatal(err)
	}
	// The source used policy=replace; import must reset to the safe default.
	if imported.Output.Policy != PolicyAlongside {
		t.Errorf("imported policy = %q, want alongside", imported.Output.Policy)
	}
	if imported.Output.OnCollision != CollisionSkip {
		t.Errorf("imported on_collision = %q, want skip", imported.Output.OnCollision)
	}
}

func TestStoreImportCollision(t *testing.T) {
	dst := NewStore(t.TempDir())
	if err := dst.Save(newSample("dup")); err != nil {
		t.Fatal(err)
	}
	exportPath := filepath.Join(t.TempDir(), "dup.yaml")
	if err := dst.Export("dup", exportPath); err != nil {
		t.Fatal(err)
	}

	// Number: creates dup-2.
	name, err := dst.Import(exportPath, CollisionNumber)
	if err != nil {
		t.Fatal(err)
	}
	if name != "dup-2" {
		t.Errorf("numbered import name = %q, want dup-2", name)
	}

	// Skip: refuses.
	if _, err := dst.Import(exportPath, CollisionSkip); err == nil {
		t.Error("skip collision should return an error")
	}

	// Overwrite: keeps the same name.
	name, err = dst.Import(exportPath, CollisionOverwrite)
	if err != nil {
		t.Fatal(err)
	}
	if name != "dup" {
		t.Errorf("overwrite import name = %q, want dup", name)
	}
}

func TestSafeFilename(t *testing.T) {
	cases := map[string]string{
		"web-d15-e7": "web-d15-e7",
		"my/preset":  "my_preset",
		`bad:name?*`: "bad_name__",
		"   ":        "preset",
	}
	for in, want := range cases {
		if got := safeFilename(in); got != want {
			t.Errorf("safeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStoreWritesReadableYAML(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Save(newSample("web")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "web.yaml"))
	if err != nil {
		t.Fatalf("expected web.yaml: %v", err)
	}
	if len(data) == 0 {
		t.Error("written preset is empty")
	}
}

func TestDefaultPresetIsReadOnly(t *testing.T) {
	store := NewStore(t.TempDir())
	created, err := EnsureDefaults(store)
	if err != nil || !created {
		t.Fatalf("EnsureDefaults created=%v err=%v", created, err)
	}
	for _, name := range []string{"default-gui", "default-cli", "default-explorer-context"} {
		p, err := store.Load(name)
		if err != nil {
			t.Fatal(err)
		}
		if !p.ReadOnly {
			t.Errorf("%s must be read-only", name)
		}
	}
	p, err := store.Load("default-gui")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(p); err == nil {
		t.Error("saving the default preset should fail")
	}
	if err := store.Delete("default-gui"); err == nil {
		t.Error("deleting the default preset should fail")
	}
	if err := store.Rename("default-gui", "custom"); err == nil {
		t.Error("renaming the default preset should fail")
	}
	if err := store.Duplicate("default-gui", "custom"); err != nil {
		t.Fatalf("duplicating the default preset: %v", err)
	}
	custom, err := store.Load("custom")
	if err != nil {
		t.Fatal(err)
	}
	if custom.ReadOnly {
		t.Error("duplicating a read-only preset should create a writable copy")
	}
	if created, err := EnsureDefaults(store); err != nil || created {
		t.Errorf("EnsureDefaults on existing defaults created=%v err=%v", created, err)
	}
}

func TestDefaultPresetRules(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := EnsureDefaults(store); err != nil {
		t.Fatal(err)
	}
	gui, err := store.Load("default-gui")
	if err != nil {
		t.Fatal(err)
	}
	if gui.Output.Policy != PolicyAlongside || len(gui.Rules) != 4 {
		t.Fatalf("GUI defaults = %+v", gui)
	}
	if got := gui.Rules[0].Args[0]; got.Key != "-d" || got.Value != "0.3" {
		t.Errorf("JXL rule = %+v", got)
	}
	if got := gui.Rules[0].Args[1]; got.Key != "-e" || got.Value != "8" {
		t.Errorf("JXL effort = %+v", got)
	}
	if got := gui.Rules[1].Args[0]; got.Key != "--lossless_jpeg" || got.Value != "1" {
		t.Errorf("JPEG rule = %+v", got)
	}
	if got := gui.Rules[2].Args[0]; got.Key != "-d" || got.Value != "0" {
		t.Errorf("PNG distance = %+v", got)
	}
	if got := gui.Rules[2].Args[1]; got.Key != "-e" || got.Value != "9" {
		t.Errorf("PNG effort = %+v", got)
	}
	if got := gui.Rules[3].Args[0]; got.Key != "-d" || got.Value != "0.5" {
		t.Errorf("fallback distance = %+v", got)
	}
	if got := gui.Rules[3].Args[1]; got.Key != "-e" || got.Value != "7" {
		t.Errorf("fallback effort = %+v", got)
	}

	cliPreset, err := store.Load("default-cli")
	if err != nil {
		t.Fatal(err)
	}
	if cliPreset.Output.Policy != PolicyReplace || len(cliPreset.Rules) != 1 {
		t.Fatalf("CLI defaults = %+v", cliPreset)
	}
	args := cliPreset.Rules[0].Args
	if len(args) != 3 || args[0].Key != "--lossless_jpeg" || args[0].Value != "0" ||
		args[1].Key != "-d" || args[1].Value != "0.3" ||
		args[2].Key != "-e" || args[2].Value != "8" {
		t.Errorf("CLI rule = %+v", args)
	}
}
