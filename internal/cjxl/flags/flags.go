// Package flags models the command-line flag surface of the installed cjxl
// binary. The set is produced by parsing `cjxl --help -v -v -v -v` (see Parse)
// and is used to drive the Expert-mode UI and to validate preset arguments
// before a run.
//
// The committed flags_generated.go is produced by the generator in ./gen from a
// captured help sample; run `go generate ./...` with cjxl on PATH to refresh it
// on a libjxl version bump.
package flags

import (
	"fmt"
	"sort"
)

// Flag describes a single cjxl option, possibly available under both a short
// (-d) and a long (--distance) spelling.
type Flag struct {
	// Short is the one-letter form without its dash, e.g. "d". Empty if none.
	Short string `json:"short,omitempty"`
	// Long is the long form without its dashes, e.g. "distance". Empty if none.
	Long string `json:"long,omitempty"`
	// TakesValue is true when the flag expects a value (e.g. -d DISTANCE,
	// --group_order=0|1). Valueless flags (e.g. --progressive_ac) are false.
	TakesValue bool `json:"takesValue"`
	// ValueSpec is the raw placeholder or choice list, e.g. "DISTANCE", "0|1",
	// "-1..41", "key=value". Empty when TakesValue is false.
	ValueSpec string `json:"valueSpec,omitempty"`
	// Section is the help heading the flag appeared under, e.g. "Basic options".
	Section string `json:"section,omitempty"`
	// Description is the first line of the flag's help text.
	Description string `json:"description,omitempty"`
}

// Canonical returns the preferred spelling as it appears on a command line,
// preferring the long form.
func (f Flag) Canonical() string {
	if f.Long != "" {
		return "--" + f.Long
	}
	if f.Short != "" {
		return "-" + f.Short
	}
	return ""
}

// Tokens returns every command-line spelling of the flag ("-d", "--distance").
func (f Flag) Tokens() []string {
	var t []string
	if f.Short != "" {
		t = append(t, "-"+f.Short)
	}
	if f.Long != "" {
		t = append(t, "--"+f.Long)
	}
	return t
}

// Set is the collection of flags for a specific cjxl version, indexed by token.
type Set struct {
	Version string `json:"version"`
	Flags   []Flag `json:"flags"`

	byToken map[string]Flag
}

// NewSet builds a Set and its token index.
func NewSet(version string, flags []Flag) *Set {
	s := &Set{Version: version, Flags: flags, byToken: make(map[string]Flag, len(flags)*2)}
	for _, f := range flags {
		for _, tok := range f.Tokens() {
			s.byToken[tok] = f
		}
	}
	return s
}

// Lookup finds a flag by any of its tokens ("-d" or "--distance").
func (s *Set) Lookup(token string) (Flag, bool) {
	f, ok := s.byToken[token]
	return f, ok
}

// Validate reports an error if key is not a known cjxl flag token. Preset
// argument keys are validated with this before a run so an unknown flag stops
// the run rather than being passed to cjxl.
func (s *Set) Validate(key string) error {
	if _, ok := s.byToken[key]; ok {
		return nil
	}
	return fmt.Errorf("unknown cjxl flag %q (not present in cjxl %s)", key, s.Version)
}

// TokenNames returns every known token, sorted, for diagnostics and diffing.
func (s *Set) TokenNames() []string {
	names := make([]string, 0, len(s.byToken))
	for tok := range s.byToken {
		names = append(names, tok)
	}
	sort.Strings(names)
	return names
}

// Diff compares two sets and returns the tokens added and removed going from
// old to new. Used to show the flag diff on a libjxl version bump.
func Diff(old, updated *Set) (added, removed []string) {
	oldTokens := map[string]bool{}
	for _, t := range old.TokenNames() {
		oldTokens[t] = true
	}
	newTokens := map[string]bool{}
	for _, t := range updated.TokenNames() {
		newTokens[t] = true
	}
	for t := range newTokens {
		if !oldTokens[t] {
			added = append(added, t)
		}
	}
	for t := range oldTokens {
		if !newTokens[t] {
			removed = append(removed, t)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

// Default returns the Set generated from the committed help snapshot.
func Default() *Set {
	return NewSet(GeneratedVersion, generatedFlags())
}
