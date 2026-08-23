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

const (
	DefaultGUIPresetName             = "default-gui"
	DefaultCLIPresetName             = "default-cli"
	DefaultExplorerContextPresetName = "default-explorer-context"
	LegacyDefaultPresetName          = "Default"
)

// Default returns a Config with a read-only preset bound to each entry point.
// Users can replace these bindings with their own presets later.
func Default() Config {
	return Config{
		Version: 1,
		Bindings: map[EntryPoint]string{
			EntryGUI:         DefaultGUIPresetName,
			EntryCLI:         DefaultCLIPresetName,
			EntryContextMenu: DefaultExplorerContextPresetName,
		},
	}
}

// DefaultPresetFor returns the built-in preset name for an entry point.
func DefaultPresetFor(entryPoint EntryPoint) string {
	switch entryPoint {
	case EntryGUI:
		return DefaultGUIPresetName
	case EntryCLI:
		return DefaultCLIPresetName
	case EntryContextMenu:
		return DefaultExplorerContextPresetName
	default:
		return ""
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
