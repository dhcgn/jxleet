// Package cli parses jxleet's small command-line surface. Path arguments are
// intentionally accepted in any mix of files and folders; the GUI/engine owns
// recursive expansion and conversion.
package cli

import (
	"errors"
	"fmt"
	"strings"
)

// Args is the result of parsing one invocation.
type Args struct {
	Paths                 []string
	Preset                string
	Help                  bool
	Version               bool
	RegisterContextMenu   bool
	UnregisterContextMenu bool
}

// Parse parses flags and paths without stopping at the first positional
// argument, which lets callers mix files, folders, and --preset.
func Parse(input []string) (Args, error) {
	var result Args
	for i := 0; i < len(input); i++ {
		arg := input[i]
		switch {
		case arg == "--help" || arg == "-h":
			result.Help = true
		case arg == "--version":
			result.Version = true
		case arg == "--register-context-menu":
			result.RegisterContextMenu = true
		case arg == "--unregister-context-menu":
			result.UnregisterContextMenu = true
		case arg == "--preset":
			if i+1 >= len(input) || strings.TrimSpace(input[i+1]) == "" {
				return Args{}, errors.New("--preset requires a name")
			}
			result.Preset = input[i+1]
			i++
		case strings.HasPrefix(arg, "--preset="):
			result.Preset = strings.TrimPrefix(arg, "--preset=")
			if strings.TrimSpace(result.Preset) == "" {
				return Args{}, errors.New("--preset requires a name")
			}
		case strings.HasPrefix(arg, "-"):
			return Args{}, fmt.Errorf("unknown option %q", arg)
		default:
			result.Paths = append(result.Paths, arg)
		}
	}
	if result.Help && (result.Version || len(result.Paths) > 0 || result.Preset != "" || result.RegisterContextMenu || result.UnregisterContextMenu) {
		return Args{}, errors.New("--help cannot be combined with other arguments")
	}
	if result.Version && (len(result.Paths) > 0 || result.Preset != "" || result.RegisterContextMenu || result.UnregisterContextMenu) {
		return Args{}, errors.New("--version cannot be combined with other arguments")
	}
	if result.RegisterContextMenu && result.UnregisterContextMenu {
		return Args{}, errors.New("context-menu register and unregister are mutually exclusive")
	}
	return result, nil
}

// Usage is the concise CLI help text printed to stdout.
func Usage() string {
	return `Usage: jxleet [OPTIONS] [PATH...]

Convert files or folders with the CLI preset binding.

Options:
  --preset NAME                 override the CLI preset for this invocation
  --register-context-menu      register the per-user Explorer menu
  --unregister-context-menu    remove the per-user Explorer menu
  -h, --help                   show this help
  --version                    show the application version
`
}
