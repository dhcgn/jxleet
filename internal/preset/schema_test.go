package preset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureSchemaWritesFile(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(s.Dir, SchemaFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\"jxleet preset\"") {
		t.Fatalf("schema missing title: %s", string(data))
	}
}

func TestSavePrependsSchemaModeline(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.Save(newSample("web")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(s.Dir, "web.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), schemaModeline) {
		t.Fatalf("preset missing schema modeline: %s", string(data))
	}
	// The schema file must be written alongside the preset.
	if _, err := os.Stat(filepath.Join(s.Dir, SchemaFileName)); err != nil {
		t.Fatalf("schema file not written: %v", err)
	}
	// Round-trip must still parse despite the leading comment.
	loaded, err := s.Load("web")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "web" {
		t.Fatalf("round-trip name = %q", loaded.Name)
	}
}
