package preset

import "github.com/dhcgn/jxleet/internal/cjxl"

// EnsureDefault creates the built-in read-only preset when it is absent. It
// returns true when the file was created.
func EnsureDefault(store *Store) (bool, error) {
	if store.Exists(DefaultName) {
		return false, nil
	}
	p := Preset{
		Name:        DefaultName,
		Description: "Safe defaults: JPEGs remain reconstructable; other inputs use d 1.0.",
		Version:     CurrentVersion,
		ReadOnly:    true,
		Output:      DefaultOutput(),
		Rules: []Rule{
			{
				Match: []string{"JPEG"},
				Args: []cjxl.Arg{
					{Key: "--lossless_jpeg", Value: "1"},
					{Key: "-e", Value: "7"},
				},
			},
			{
				Match: []string{"*"},
				Args: []cjxl.Arg{
					{Key: "-d", Value: "1.0"},
					{Key: "-e", Value: "7"},
				},
			},
		},
	}
	if err := store.Save(p); err != nil {
		return false, err
	}
	return true, nil
}
