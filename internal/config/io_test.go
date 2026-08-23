package config

import (
	"path/filepath"
	"testing"
)

func TestLoadMissingReturnsDefault(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.UnboundEntryPoints()) != 3 {
		t.Errorf("fresh config should have 3 unbound entry points, got %v", c.UnboundEntryPoints())
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	c := Default()
	c.Bindings[EntryGUI] = "archive-lossless"
	c.Bindings[EntryCLI] = "web-d15-e7"
	c.Bindings[EntryContextMenu] = "archive-lossless"

	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if name, ok := got.Binding(EntryGUI); !ok || name != "archive-lossless" {
		t.Errorf("gui binding = %q ok=%v", name, ok)
	}
	if len(got.UnboundEntryPoints()) != 0 {
		t.Errorf("all entry points should be bound, unbound = %v", got.UnboundEntryPoints())
	}
}

func TestBindingEmptyIsUnbound(t *testing.T) {
	c := Default()
	c.Bindings[EntryGUI] = ""
	if _, ok := c.Binding(EntryGUI); ok {
		t.Error("empty binding should be treated as unbound")
	}
}
