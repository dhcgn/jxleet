// Package preset loads, validates and stores jxleet presets. A preset pairs file
// filters with cjxl arguments; the arguments are passed to cjxl verbatim (see
// README "Presets"). Presets are one YAML file each under
// %APPDATA%\jxleet\presets\.
package preset

import "github.com/dhcgn/jxleet/internal/cjxl"

// CurrentVersion is the preset schema version written by this build.
const CurrentVersion = 1

// Policy selects where a converted file is written.
type Policy string

const (
	PolicyAlongside Policy = "alongside" // next to the original (default)
	PolicySubfolder Policy = "subfolder" // into ./<subfolder>/
	PolicyReplace   Policy = "replace"   // original goes to the recycle bin
)

// Collision selects what happens when the output path already exists.
type Collision string

const (
	CollisionSkip      Collision = "skip"
	CollisionNumber    Collision = "number"
	CollisionOverwrite Collision = "overwrite"
)

// Output describes where results go and how name collisions are handled.
type Output struct {
	Policy      Policy    `yaml:"policy"`
	Subfolder   string    `yaml:"subfolder,omitempty"`
	OnCollision Collision `yaml:"on_collision,omitempty"`
}

// Rule pairs a set of format filters with an ordered list of cjxl arguments.
// Match entries are format names (see routes.Format) or "*". Args preserve the
// order written in the YAML file so the command preview is deterministic.
type Rule struct {
	Match []string
	Args  []cjxl.Arg
}

// Preset is a named, versioned set of output settings and match rules.
type Preset struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Version     int    `yaml:"version"`
	ReadOnly    bool   `yaml:"read_only,omitempty"`
	Output      Output `yaml:"output"`
	Rules       []Rule `yaml:"rules"`
}

// DefaultOutput is the safe default applied to new and imported presets:
// results are written alongside the original and colliding files are skipped.
func DefaultOutput() Output {
	return Output{Policy: PolicyAlongside, OnCollision: CollisionSkip}
}
