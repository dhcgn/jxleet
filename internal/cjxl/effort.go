package cjxl

// Effort maps to cjxl's -e / --effort, trading encoding time for size.
// The level codenames are libjxl's own; 7 (squirrel) is the cjxl default.
// See README "Distance, quality and effort".

const (
	MinEffort     = 1
	MaxEffort     = 10
	DefaultEffort = 7
	// ExpertEffort (11) requires cjxl's --allow_expert_options.
	ExpertEffort = 11
)

// effortNames holds the libjxl codename for each effort level 1..10.
var effortNames = map[int]string{
	1:  "lightning",
	2:  "thunder",
	3:  "falcon",
	4:  "cheetah",
	5:  "hare",
	6:  "wombat",
	7:  "squirrel",
	8:  "kitten",
	9:  "tortoise",
	10: "glacier",
}

// EffortName returns the codename for an effort level, or "" if out of range.
func EffortName(level int) string {
	return effortNames[level]
}

// ValidEffort reports whether level is within the standard 1..10 range.
func ValidEffort(level int) bool {
	return level >= MinEffort && level <= MaxEffort
}
