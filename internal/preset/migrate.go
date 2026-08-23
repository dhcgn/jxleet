package preset

import "fmt"

// migrate upgrades a loaded preset to CurrentVersion, applying any per-version
// transformations. It is the single place new schema migrations are added.
//
// Rules:
//   - version 0 (unset) is treated as version 1 for presets written before the
//     field existed;
//   - a version newer than this build is rejected, since we cannot know how to
//     read it.
func migrate(p Preset) (Preset, error) {
	v := p.Version
	if v == 0 {
		v = 1
	}
	if v > CurrentVersion {
		return Preset{}, fmt.Errorf("preset %q is version %d but this build understands up to %d; please update jxleet", p.Name, v, CurrentVersion)
	}

	// Future migrations chain here, e.g.:
	//   if v == 1 { p = migrateV1toV2(p); v = 2 }

	p.Version = CurrentVersion
	return p, nil
}
