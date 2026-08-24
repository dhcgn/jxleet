// Template support for the "New preset" action: a hand-authored, commented
// starter file. Comments would not survive a YAML marshal, so the app never
// rewrites presets it did not save itself (see Store.SavePresetOutput note in
// internal/app).
package preset

import (
	"fmt"
	"strconv"
	"strings"
)

// TemplateYAML returns the starter preset for "New preset": a minimal active
// rule plus commented examples for everything else.
func TemplateYAML(name, description string) []byte {
	var b strings.Builder
	b.WriteString(schemaModeline + "\n")
	b.WriteString("# jxleet preset. Edit this file, then press Reload in the Presets view.\n")
	b.WriteString("# preset.schema.json next to this file validates it live in YAML-aware editors.\n")
	fmt.Fprintf(&b, "name: %s\n", name)
	if strings.TrimSpace(description) != "" {
		fmt.Fprintf(&b, "description: %s\n", strconv.Quote(strings.TrimSpace(description)))
	}
	b.WriteString(`version: 1
output:
  policy: alongside        # alongside | subfolder | replace
  # subfolder: jxl         # where results go when policy: subfolder
  on_collision: skip       # skip | number | overwrite
rules:
  - match: ['*']           # catch-all — first matching rule wins
    args:
      -d: 0.5              # visual distance: 0 = lossless, 1.0 = visually lossless, up to 25
      -e: 7                # effort: 1 = fastest .. 10 = smallest

# --- Examples (uncomment to use) --------------------------------------------
#
# Lossless JPEG transcode (byte-reversible, ignores -d):
#   - match: [JPEG]
#     args:
#       --lossless_jpeg: 1
#
# Quality instead of distance (same setting, scale 0..100):
#   - match: [PNG]
#     args:
#       -q: 90             # roughly d 0.3
#       -e: 9
#
# Replace the original file (moved to the recycle bin after verification):
# output:
#   policy: replace
#   on_collision: skip
#
# Extra cjxl flags simply append under args, for example:
#       --progressive: true
`)
	return []byte(b.String())
}
