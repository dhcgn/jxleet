// Package config resolves jxleet's on-disk locations and the persisted
// application configuration, including the three entry-point preset bindings.
//
// Layout (see README "Where things are stored"):
//
//	%APPDATA%\jxleet\config.yaml       settings and the three entry-point bindings
//	%APPDATA%\jxleet\presets\          one YAML file per preset
//	%APPDATA%\jxleet\history.jsonl     one JSON line per successful conversion
//	%LOCALAPPDATA%\jxleet\bin\         the managed libjxl binaries
//	%LOCALAPPDATA%\jxleet\logs\        run logs
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const appDir = "jxleet"

// portableDataDir is the single folder next to a portable executable holding
// everything jxleet persists (config, presets, history, binaries, logs).
const portableDataDir = "jxleet-files"

// jl:tech.mode.portable=Portable mode: the executable name contains "portable", all files live in jxleet-files next to it, and app self-updates are off (replace the binary instead).

// Paths holds the resolved absolute locations jxleet reads from and writes to.
type Paths struct {
	// ConfigDir is %APPDATA%\jxleet.
	ConfigDir string
	// ConfigFile is %APPDATA%\jxleet\config.yaml.
	ConfigFile string
	// PresetsDir is %APPDATA%\jxleet\presets.
	PresetsDir string
	// HistoryFile is %APPDATA%\jxleet\history.jsonl.
	HistoryFile string
	// BinDir is %LOCALAPPDATA%\jxleet\bin.
	BinDir string
	// LogsDir is %LOCALAPPDATA%\jxleet\logs.
	LogsDir string
}

// IsPortableExe reports whether exePath selects portable mode: the file name
// contains "portable" (case-insensitive), e.g. jxleet-portable.exe.
func IsPortableExe(exePath string) bool {
	return strings.Contains(strings.ToLower(filepath.Base(exePath)), "portable")
}

// ResolvePortablePaths roots every location under exeDir/jxleet-files, so a
// portable copy keeps config, presets, history, binaries and logs next to the
// executable instead of %APPDATA%/%LOCALAPPDATA% (ref:jl:tech.mode.portable).
func ResolvePortablePaths(exeDir string) Paths {
	root := filepath.Join(exeDir, portableDataDir)
	return Paths{
		ConfigDir:   root,
		ConfigFile:  filepath.Join(root, "config.yaml"),
		PresetsDir:  filepath.Join(root, "presets"),
		HistoryFile: filepath.Join(root, "history.jsonl"),
		BinDir:      filepath.Join(root, "bin"),
		LogsDir:     filepath.Join(root, "logs"),
	}
}

// ResolvePaths computes the standard jxleet paths from the environment.
//
// It relies on %APPDATA% and %LOCALAPPDATA%, which are always present on a
// normal Windows session. It does not create any directories; call EnsureDirs
// for that.
func ResolvePaths() (Paths, error) {
	appData, err := envDir("APPDATA")
	if err != nil {
		return Paths{}, err
	}
	localAppData, err := envDir("LOCALAPPDATA")
	if err != nil {
		return Paths{}, err
	}

	configDir := filepath.Join(appData, appDir)
	localDir := filepath.Join(localAppData, appDir)

	return Paths{
		ConfigDir:   configDir,
		ConfigFile:  filepath.Join(configDir, "config.yaml"),
		PresetsDir:  filepath.Join(configDir, "presets"),
		HistoryFile: filepath.Join(configDir, "history.jsonl"),
		BinDir:      filepath.Join(localDir, "bin"),
		LogsDir:     filepath.Join(localDir, "logs"),
	}, nil
}

// EnsureDirs creates the config, presets, bin and logs directories if missing.
func (p Paths) EnsureDirs() error {
	for _, dir := range []string{p.ConfigDir, p.PresetsDir, p.BinDir, p.LogsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}

func envDir(name string) (string, error) {
	v := os.Getenv(name)
	if v == "" {
		return "", fmt.Errorf("environment variable %%%s%% is not set", name)
	}
	return v, nil
}
