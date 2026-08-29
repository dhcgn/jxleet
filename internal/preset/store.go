package preset

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store manages the preset YAML files in a directory.
type Store struct {
	Dir string
}

// NewStore returns a Store rooted at dir (typically %APPDATA%\jxleet\presets).
func NewStore(dir string) *Store {
	return &Store{Dir: dir}
}

type entry struct {
	path   string
	preset Preset
}

// readAll loads every *.yaml preset in the directory, migrating each. Files that
// fail to parse are skipped silently so one bad file does not hide the rest;
// callers that need strictness can Load by name.
func (s *Store) readAll() ([]entry, error) {
	matches, err := filepath.Glob(filepath.Join(s.Dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	entries := make([]entry, 0, len(matches))
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		p, err := Unmarshal(data)
		if err != nil {
			continue
		}
		p, err = migrate(p)
		if err != nil {
			continue
		}
		entries = append(entries, entry{path: path, preset: p})
	}
	return entries, nil
}

// List returns all valid presets, sorted by name.
func (s *Store) List() ([]Preset, error) {
	entries, err := s.readAll()
	if err != nil {
		return nil, err
	}
	presets := make([]Preset, 0, len(entries))
	for _, e := range entries {
		presets = append(presets, e.preset)
	}
	return presets, nil
}

// Names returns the names of all valid presets.
func (s *Store) Names() ([]string, error) {
	presets, err := s.List()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(presets))
	for _, p := range presets {
		names = append(names, p.Name)
	}
	return names, nil
}

// Load returns the preset with the given name.
func (s *Store) Load(name string) (Preset, error) {
	entries, err := s.readAll()
	if err != nil {
		return Preset{}, err
	}
	for _, e := range entries {
		if e.preset.Name == name {
			return e.preset, nil
		}
	}
	return Preset{}, fmt.Errorf("preset %q not found", name)
}

// Exists reports whether a preset with the given name is stored.
func (s *Store) Exists(name string) bool {
	_, err := s.Load(name)
	return err == nil
}

// Save validates and writes a preset. It reuses the existing file for the
// preset's name if one exists, otherwise derives a filename from the name.
func (s *Store) Save(p Preset) error {
	if p.Version == 0 {
		p.Version = CurrentVersion
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	if existing, err := s.Load(p.Name); err == nil && existing.ReadOnly {
		return fmt.Errorf("preset %q is read-only", p.Name)
	}
	path := s.pathFor(p.Name)
	data, err := Marshal(p)
	if err != nil {
		return err
	}
	// Prepend the schema modeline so editors validate the file against the
	// committed JSON schema (written by EnsureSchema).
	data = append([]byte(schemaModeline+"\n"), data...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	// Best-effort: keep the schema file available next to the presets.
	_ = s.EnsureSchema()
	return nil
}

// PathFor exposes the backing file of a preset (existing or derived).
func (s *Store) PathFor(name string) string {
	return s.pathFor(name)
}

// CreateTemplate writes the commented starter preset produced by TemplateYAML.
// The active part is parsed and validated before anything hits disk.
func (s *Store) CreateTemplate(name, description string) error {
	if s.Exists(name) {
		return fmt.Errorf("preset %q already exists", name)
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	data := TemplateYAML(name, description)
	p, err := Unmarshal(data)
	if err != nil {
		return err
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if err := os.WriteFile(s.pathFor(name), data, 0o644); err != nil {
		return err
	}
	_ = s.EnsureSchema()
	return nil
}

// pathFor returns the existing file for name, or a derived path when none exists.
func (s *Store) pathFor(name string) string {
	if entries, err := s.readAll(); err == nil {
		for _, e := range entries {
			if e.preset.Name == name {
				return e.path
			}
		}
	}
	return filepath.Join(s.Dir, safeFilename(name)+".yaml")
}

// Delete removes the preset with the given name.
func (s *Store) Delete(name string) error {
	entries, err := s.readAll()
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.preset.Name == name {
			if e.preset.ReadOnly {
				return fmt.Errorf("preset %q is read-only", name)
			}
			return os.Remove(e.path)
		}
	}
	return fmt.Errorf("preset %q not found", name)
}

// Rename changes a preset's name, moving its file.
func (s *Store) Rename(oldName, newName string) error {
	if newName == "" {
		return fmt.Errorf("preset: new name is required")
	}
	if s.Exists(newName) {
		return fmt.Errorf("preset %q already exists", newName)
	}
	p, err := s.Load(oldName)
	if err != nil {
		return err
	}
	if p.ReadOnly {
		return fmt.Errorf("preset %q is read-only", oldName)
	}
	if err := s.Delete(oldName); err != nil {
		return err
	}
	p.Name = newName
	return s.Save(p)
}

// Duplicate copies a preset under a new name.
func (s *Store) Duplicate(name, newName string) error {
	if s.Exists(newName) {
		return fmt.Errorf("preset %q already exists", newName)
	}
	p, err := s.Load(name)
	if err != nil {
		return err
	}
	p.ReadOnly = false
	p.Name = newName
	return s.Save(p)
}

// Export writes a preset to an arbitrary file path.
func (s *Store) Export(name, destPath string) error {
	p, err := s.Load(name)
	if err != nil {
		return err
	}
	data, err := Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(destPath, data, 0o644)
}

// Import reads a preset from an arbitrary file and stores it. The imported
// preset never adopts the source output policy — it is reset to the safe default
// (see README "Import from a file, without adopting the output policy"). Name
// collisions are handled per onCollision. The stored name is returned.
func (s *Store) Import(srcPath string, onCollision Collision) (string, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return "", err
	}
	p, err := Unmarshal(data)
	if err != nil {
		return "", err
	}
	p, err = migrate(p)
	if err != nil {
		return "", err
	}
	p.ReadOnly = false
	p.Output = DefaultOutput()
	if err := p.Validate(); err != nil {
		return "", err
	}

	if s.Exists(p.Name) {
		switch onCollision {
		case CollisionSkip:
			return "", fmt.Errorf("preset %q already exists; import skipped", p.Name)
		case CollisionOverwrite:
			// fall through and overwrite
		case CollisionNumber, "":
			p.Name = s.uniqueName(p.Name)
		default:
			return "", fmt.Errorf("preset: unknown collision mode %q", onCollision)
		}
	}
	if err := s.Save(p); err != nil {
		return "", err
	}
	return p.Name, nil
}

// uniqueName appends -2, -3, ... until the name is free.
func (s *Store) uniqueName(base string) string {
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !s.Exists(candidate) {
			return candidate
		}
	}
}

// safeFilename maps a preset name to a filesystem-safe base name.
func safeFilename(name string) string {
	repl := func(r rune) rune {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			return '_'
		}
		if r < 0x20 {
			return '_'
		}
		return r
	}
	s := strings.TrimSpace(strings.Map(repl, name))
	s = strings.Trim(s, " .")
	if s == "" {
		return "preset"
	}
	return s
}
