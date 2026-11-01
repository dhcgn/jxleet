package preset

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/dhcgn/jxleet/internal/cjxl"
	"github.com/dhcgn/jxleet/internal/cjxl/flags"
)

// DefaultPresets are the immutable presets installed for the three entry
// points on first start.
var DefaultPresets = []Preset{
	{
		Name:        "default-gui",
		Description: "GUI defaults: JPEG lossless, PNG lossless, moderate JXL compression.",
		Version:     CurrentVersion,
		ReadOnly:    true,
		Output:      DefaultOutput(),
		Rules: []Rule{
			{Match: []string{"JXL"}, Args: []cjxl.Arg{{Key: "-d", Value: "0.3"}, {Key: "-e", Value: "8"}}},
			{Match: []string{"JPEG"}, Args: []cjxl.Arg{{Key: "--lossless_jpeg", Value: "1"}}},
			{Match: []string{"PNG"}, Args: []cjxl.Arg{{Key: "-d", Value: "0"}, {Key: "-e", Value: "9"}}},
			{Match: []string{"*"}, Args: []cjxl.Arg{{Key: "-d", Value: "0.5"}, {Key: "-e", Value: "7"}}},
		},
	},
	{
		Name:        "default-cli",
		Description: "CLI defaults: lossy d 0.3, effort 8, originals replaced after verification.",
		Version:     CurrentVersion,
		ReadOnly:    true,
		Output:      Output{Policy: PolicyReplace, OnCollision: CollisionSkip, EmbedSettings: true},
		Rules: []Rule{
			{
				Match: []string{"*"},
				Args: []cjxl.Arg{
					{Key: "--lossless_jpeg", Value: "0"},
					{Key: "-d", Value: "0.3"},
					{Key: "-e", Value: "8"},
				},
			},
		},
	},
	{
		Name:        "default-explorer-context",
		Description: "Explorer defaults: JPEG lossless, PNG lossless, moderate JXL compression.",
		Version:     CurrentVersion,
		ReadOnly:    true,
		Output:      DefaultOutput(),
		Rules: []Rule{
			{Match: []string{"JXL"}, Args: []cjxl.Arg{{Key: "-d", Value: "0.3"}, {Key: "-e", Value: "8"}}},
			{Match: []string{"JPEG"}, Args: []cjxl.Arg{{Key: "--lossless_jpeg", Value: "1"}}},
			{Match: []string{"PNG"}, Args: []cjxl.Arg{{Key: "-d", Value: "0"}, {Key: "-e", Value: "9"}}},
			{Match: []string{"*"}, Args: []cjxl.Arg{{Key: "-d", Value: "0.5"}, {Key: "-e", Value: "7"}}},
		},
	},
}

// EnsureDefaults writes every built-in preset whose file is missing or differs
// from what this build ships, and returns whether it wrote at least one file.
// The defaults are read-only, so a differing file is either from an older build
// or a hand edit; both are replaced. User presets are untouched.
func EnsureDefaults(store *Store) (bool, error) {
	if err := os.MkdirAll(store.Dir, 0o755); err != nil {
		return false, err
	}
	wrote := false
	for _, p := range DefaultPresets {
		want, err := defaultYAML(p)
		if err != nil {
			return wrote, err
		}
		path := store.pathFor(p.Name)
		if have, err := os.ReadFile(path); err == nil && bytes.Equal(have, want) {
			continue
		}
		if err := os.WriteFile(path, want, 0o644); err != nil {
			return wrote, err
		}
		wrote = true
	}
	_ = store.EnsureSchema()
	return wrote, nil
}

// defaultYAML renders a built-in preset followed by a commented reference of
// every cjxl flag. The "# " sits at the args column, so an editor's uncomment
// leaves the line correctly indented under the last rule's args.
func defaultYAML(p Preset) ([]byte, error) {
	body, err := Marshal(p)
	if err != nil {
		return nil, err
	}
	set := flags.Default()
	var b bytes.Buffer
	b.WriteString(schemaModeline + "\n")
	b.WriteString("# Built-in preset (read-only). jxleet rewrites this file on start; edits are lost.\n")
	b.WriteString("# Duplicate it in the Presets view to get a writable copy that keeps these comments.\n")
	b.Write(body)
	fmt.Fprintf(&b, "\n# --- All cjxl %s flags (uncomment to use) ---\n", set.Version)
	b.WriteString("# An uncommented line joins the rule directly above; copy it under any rule's args.\n")
	b.WriteString("# Valueless flags take `true`; replace UPPERCASE / a|b placeholders with a value.\n")

	type line struct{ entry, note string }
	var lines []line
	width, section := 0, ""
	for _, f := range set.Flags {
		switch f.Long {
		case "version", "help", "disable_output": // never produce a result file
			continue
		}
		if f.Section != section {
			section = f.Section
			lines = append(lines, line{note: section})
		}
		value := "true"
		if f.TakesValue {
			value = f.ValueSpec
		}
		entry := fmt.Sprintf("%s: %s", f.Canonical(), value)
		note := f.Description
		if len(f.Tokens()) > 1 {
			note = f.Tokens()[0] + "  " + note
		}
		lines = append(lines, line{entry, note})
		width = max(width, len(entry))
	}
	indent := argsIndent(body)
	for _, l := range lines {
		if l.entry == "" {
			fmt.Fprintf(&b, "#\n# %s\n", l.note)
			continue
		}
		fmt.Fprintf(&b, "%s# %-*s  # %s\n", indent, width, l.entry, l.note)
	}
	return b.Bytes(), nil
}

// argsIndent returns the leading whitespace of the last args entry in body, so
// the commented flags line up with it.
func argsIndent(body []byte) string {
	rows := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	last := rows[len(rows)-1]
	return last[:len(last)-len(strings.TrimLeft(last, " "))]
}
