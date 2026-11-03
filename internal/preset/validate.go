package preset

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dhcgn/jxleet/internal/cjxl/flags"
	"github.com/dhcgn/jxleet/internal/routes"
)

// Validate checks a preset's structure and reports every problem found, joined
// into one error (one line each). Rules are numbered from 1, as a reader of
// the YAML file counts them. It does not check flag names against cjxl; use
// ValidateArgs for that before a run.
func (p Preset) Validate() error {
	var errs []error
	if p.Name == "" {
		errs = append(errs, errors.New("preset: name is required"))
	}
	switch p.Output.Policy {
	case PolicyAlongside, PolicySubfolder, PolicyReplace:
	case "":
		errs = append(errs, errors.New("preset: output.policy is required"))
	default:
		errs = append(errs, fmt.Errorf("preset: unknown output.policy %q", p.Output.Policy))
	}
	if p.Output.Policy == PolicySubfolder && p.Output.Subfolder == "" {
		errs = append(errs, errors.New("preset: output.subfolder is required for the subfolder policy"))
	}
	switch p.Output.OnCollision {
	case CollisionSkip, CollisionNumber, CollisionOverwrite, "":
	default:
		errs = append(errs, fmt.Errorf("preset: unknown output.on_collision %q", p.Output.OnCollision))
	}
	if len(p.Rules) == 0 {
		errs = append(errs, errors.New("preset: at least one rule is required"))
	}
	for i, r := range p.Rules {
		if len(r.Match) == 0 {
			errs = append(errs, fmt.Errorf("preset: rule %d has no match filters", i+1))
		}
		for _, m := range r.Match {
			if m == "*" || routes.Known(routes.Format(strings.ToUpper(m))) {
				continue
			}
			msg := fmt.Sprintf("preset: rule %d: unknown format %q", i+1, m)
			if strings.EqualFold(m, "JPG") {
				msg += "; use JPEG, which covers .jpg and .jpeg files"
			}
			errs = append(errs, errors.New(msg))
		}
	}
	return errors.Join(errs...)
}

// ValidateArgs checks every flag key used by the preset against the installed
// cjxl flag set and reports every unknown one, so the run is refused rather
// than passed to cjxl (see README "Validation").
func (p Preset) ValidateArgs(set *flags.Set) error {
	var errs []error
	for i, r := range p.Rules {
		for _, a := range r.Args {
			if err := set.Validate(a.Key); err != nil {
				errs = append(errs, fmt.Errorf("preset: rule %d: %w", i+1, err))
			}
		}
	}
	return errors.Join(errs...)
}
