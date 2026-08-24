package preset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplateYAMLIsValidStarterPreset(t *testing.T) {
	p, err := Unmarshal(TemplateYAML("demo", "Try it"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "demo" || p.Description != "Try it" {
		t.Errorf("identity = %+v", p)
	}
	if p.Version != CurrentVersion {
		t.Errorf("version = %d", p.Version)
	}
	if p.Output.Policy != PolicyAlongside || p.Output.OnCollision != CollisionSkip {
		t.Errorf("output = %+v", p.Output)
	}
	if len(p.Rules) != 1 || len(p.Rules[0].Match) != 1 || p.Rules[0].Match[0] != "*" {
		t.Fatalf("rules = %+v", p.Rules)
	}
	if distance, ok := EffectiveDistance(p.Rules[0].Args); !ok || distance != 0.5 {
		t.Errorf("distance = %v, %v", distance, ok)
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreCreateTemplate(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := store.CreateTemplate("demo", ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "demo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), schemaModeline) {
		t.Error("template lacks the schema modeline")
	}
	if !strings.Contains(string(data), "--- Examples") {
		t.Error("template lacks the commented examples")
	}
	if err := store.CreateTemplate("demo", ""); err == nil {
		t.Error("duplicate template creation should fail")
	}
	loaded, err := store.Load("demo")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "demo" {
		t.Errorf("loaded name = %q", loaded.Name)
	}
}
