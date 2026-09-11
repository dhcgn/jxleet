package output

import (
	"fmt"
	"strings"

	"github.com/dhcgn/jxleet/internal/cjxl"
	"github.com/dhcgn/jxleet/internal/preset"
	"github.com/dhcgn/jxleet/internal/routes"
)

// defaultDistanceForSuffix is cjxl's default distance, used when a rule sets no
// -d/-q (matching the GetPresetCore fallback of 1.0).
const defaultDistanceForSuffix = 1.0

// SettingsSuffix renders the filename infix for embedded encoding settings,
// e.g. "d1.00-e7-cjxl0.11.1". Distance always uses two decimals so suffixed
// files sort lexically in numeric order. Empty version omits the cjxl part.
func SettingsSuffix(distance float64, effort int, version string) string {
	suffix := fmt.Sprintf("d%.2f", distance)
	if effort > 0 {
		suffix += fmt.Sprintf("-e%d", effort)
	}
	if version = strings.TrimSpace(strings.TrimPrefix(version, "v")); version != "" {
		suffix += "-cjxl" + version
	}
	return suffix
}

// SuffixFor resolves the suffix for one file from its route and cjxl args.
// Transcode ignores distance and effort, so it always reports d0.00 with the
// rule's effort (or the cjxl default when unset).
func SuffixFor(route routes.Route, args []cjxl.Arg, version string) string {
	distance := defaultDistanceForSuffix
	if route == routes.RouteTranscode {
		distance = 0
	} else if d, ok := preset.EffectiveDistance(args); ok {
		distance = d
	}
	effort := cjxl.DefaultEffort
	if e, ok := preset.EffectiveEffort(args); ok {
		effort = e
	}
	return SettingsSuffix(distance, effort, version)
}
