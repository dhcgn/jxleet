package preset

import "github.com/dhcgn/jxleet/internal/cjxl"

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
		Output:      Output{Policy: PolicyReplace, OnCollision: CollisionSkip},
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

// EnsureDefaults creates any missing built-in presets and returns whether it
// created at least one file. Existing files and user presets are untouched.
func EnsureDefaults(store *Store) (bool, error) {
	created := false
	for _, p := range DefaultPresets {
		if store.Exists(p.Name) {
			continue
		}
		if err := store.Save(p); err != nil {
			return created, err
		}
		created = true
	}
	return created, nil
}
