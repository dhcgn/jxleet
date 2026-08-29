package preset

import (
	"errors"
	"fmt"

	"github.com/dhcgn/jxleet/internal/cjxl/flags"
)

// Validate checks a preset's structure. It does not check flag names against
// cjxl; use ValidateArgs for that before a run.
func (p Preset) Validate() error {
	if p.Name == "" {
		return errors.New("preset: name is required")
	}
	switch p.Output.Policy {
	case PolicyAlongside, PolicySubfolder, PolicyReplace:
	case "":
		return errors.New("preset: output.policy is required")
	default:
		return fmt.Errorf("preset: unknown output.policy %q", p.Output.Policy)
	}
	if p.Output.Policy == PolicySubfolder && p.Output.Subfolder == "" {
		return errors.New("preset: output.subfolder is required for the subfolder policy")
	}
	switch p.Output.OnCollision {
	case CollisionSkip, CollisionNumber, CollisionOverwrite, "":
	default:
		return fmt.Errorf("preset: unknown output.on_collision %q", p.Output.OnCollision)
	}
	if len(p.Rules) == 0 {
		return errors.New("preset: at least one rule is required")
	}
	for i, r := range p.Rules {
		if len(r.Match) == 0 {
			return fmt.Errorf("preset: rule %d has no match filters", i)
		}
	}
	return nil
}

// ValidateArgs checks every flag key used by the preset against the installed
// cjxl flag set. An unknown flag returns an error so the run is refused rather
// than passed to cjxl (see README "Validation").
func (p Preset) ValidateArgs(set *flags.Set) error {
	for _, r := range p.Rules {
		for _, a := range r.Args {
			if err := set.Validate(a.Key); err != nil {
				return err
			}
		}
	}
	return nil
}
