package config

// EntryPoint identifies one of the three ways jxleet is invoked. Each entry
// point has its own preset binding; jxleet refuses to run an entry point until
// its binding is set (see README "Three ways to run it").
type EntryPoint string

const (
	EntryGUI         EntryPoint = "gui"
	EntryCLI         EntryPoint = "cli"
	EntryContextMenu EntryPoint = "contextmenu"
)

// Config is the persisted application configuration stored at ConfigFile.
type Config struct {
	// Version is the config schema version, for future migrations.
	Version int `yaml:"version"`

	// Bindings maps each entry point to the name of the preset it uses. A
	// missing or empty value means the entry point is unbound and must not run.
	Bindings map[EntryPoint]string `yaml:"bindings"`

	// RouteColors overrides the default per-route colours used across the UI.
	// Empty values fall back to the built-in defaults.
	RouteColors map[string]string `yaml:"route_colors,omitempty"`
}

const DefaultPresetName = "Default"

// Default returns a Config with the read-only Default preset bound to every
// entry point. Users can replace these bindings with their own presets later.
func Default() Config {
	return Config{
		Version: 1,
		Bindings: map[EntryPoint]string{
			EntryGUI:         DefaultPresetName,
			EntryCLI:         DefaultPresetName,
			EntryContextMenu: DefaultPresetName,
		},
	}
}

// Binding returns the preset name bound to ep and whether a non-empty binding
// exists.
func (c Config) Binding(ep EntryPoint) (string, bool) {
	name, ok := c.Bindings[ep]
	return name, ok && name != ""
}

// UnboundEntryPoints lists the entry points that still lack a preset binding.
func (c Config) UnboundEntryPoints() []EntryPoint {
	var missing []EntryPoint
	for _, ep := range []EntryPoint{EntryGUI, EntryCLI, EntryContextMenu} {
		if _, ok := c.Binding(ep); !ok {
			missing = append(missing, ep)
		}
	}
	return missing
}
