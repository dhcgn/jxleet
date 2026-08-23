package preset

import (
	"strconv"
	"strings"

	"github.com/dhcgn/jxleet/internal/cjxl"
	"github.com/dhcgn/jxleet/internal/routes"
)

// Match returns the first rule whose filters match the format, honouring the
// first-match-wins order. A "*" entry matches any format. The bool is false when
// no rule matches (the file is skipped and reported).
func (p Preset) Match(format routes.Format) (Rule, bool) {
	for _, r := range p.Rules {
		for _, m := range r.Match {
			if m == "*" || strings.EqualFold(m, string(format)) {
				return r, true
			}
		}
	}
	return Rule{}, false
}

// Route resolves the route and the cjxl argument list for a file of the given
// format under this preset. The bool is false when no rule matches.
func (p Preset) Route(format routes.Format) (routes.Route, []cjxl.Arg, bool) {
	rule, ok := p.Match(format)
	if !ok {
		return routes.RouteSkip, nil, false
	}
	lossless := EffectiveLosslessJPEG(rule.Args)
	return routes.Determine(format, lossless), rule.Args, true
}

// EffectiveLosslessJPEG reports the value of --lossless_jpeg / -j for a rule's
// args, defaulting to true (cjxl's default of 1) when unspecified. This decides
// whether a JPEG takes the transcode or the reencode route.
func EffectiveLosslessJPEG(args []cjxl.Arg) bool {
	for _, a := range args {
		if a.Key == "-j" || a.Key == "--lossless_jpeg" {
			return a.Value != "0"
		}
	}
	return true
}

// EffectiveDistance returns the value of -d / --distance for a rule's args, and
// whether it was specified. Used to decide reversibility on the encode route.
func EffectiveDistance(args []cjxl.Arg) (float64, bool) {
	for _, a := range args {
		if a.Key == "-d" || a.Key == "--distance" {
			if v, err := strconv.ParseFloat(a.Value, 64); err == nil {
				return v, true
			}
		}
		if a.Key == "-q" || a.Key == "--quality" {
			if v, err := strconv.ParseFloat(a.Value, 64); err == nil {
				return cjxl.DistanceFromQuality(v), true
			}
		}
	}
	return 0, false
}
