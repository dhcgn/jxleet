package output

import (
	"github.com/dhcgn/jxleet/internal/preset"
	"github.com/dhcgn/jxleet/internal/routes"
)

// Deletion is the per-route rule for what happens to the original under the
// replace policy.
type Deletion string

// The available deletion rules.
const (
	DeletionRecycle Deletion = "recycle" // move original to the recycle bin (default)
	DeletionKeep    Deletion = "keep"    // keep the original; do not replace it
)

// DeletionByRoute maps a route to its deletion rule. A missing entry means the
// default, DeletionRecycle.
type DeletionByRoute map[routes.Route]Deletion

// Recycle reports whether the original should be recycled for the given route.
func (d DeletionByRoute) Recycle(route routes.Route) bool {
	if v, ok := d[route]; ok {
		return v != DeletionKeep
	}
	return true
}

// EffectiveOutput applies the per-route deletion rule to an output setting.
// Under the replace policy, a "keep" rule downgrades the policy to alongside so
// the original is preserved rather than recycled.
func EffectiveOutput(out preset.Output, route routes.Route, del DeletionByRoute) preset.Output {
	if out.Policy == preset.PolicyReplace && !del.Recycle(route) {
		out.Policy = preset.PolicyAlongside
	}
	return out
}
