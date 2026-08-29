//go:build windows

// Package shellext registers jxleet's per-user Windows Explorer context menu.
package shellext

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const menuName = "jxleet"

var menuRoots = []string{
	`Software\Classes\*\shell\` + menuName,
	`Software\Classes\Directory\shell\` + menuName,
	`Software\Classes\Directory\Background\shell\` + menuName,
}

// Register installs the menu for the current user without administrator
// privileges. The preset name is displayed in the menu and passed to jxleet.
func Register(executable, preset string) error {
	if strings.TrimSpace(preset) == "" {
		return errors.New("shellext: context-menu preset is required")
	}
	executable, err := filepath.Abs(executable)
	if err != nil {
		return fmt.Errorf("shellext: resolve executable: %w", err)
	}
	if _, err := os.Stat(executable); err != nil {
		return fmt.Errorf("shellext: executable is not available: %w", err)
	}

	for _, root := range menuRoots {
		key, _, err := registry.CreateKey(registry.CURRENT_USER, root, registry.SET_VALUE)
		if err != nil {
			return fmt.Errorf("shellext: create %s: %w", root, err)
		}
		err = key.SetStringValue("MUIVerb", "To JXL - "+preset)
		if err == nil {
			err = key.SetStringValue("Icon", executable)
		}
		if closeErr := key.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return fmt.Errorf("shellext: configure %s: %w", root, err)
		}

		command := quoteWindowsArg(executable) + ` --preset=` + quoteWindowsArg(preset)
		if strings.Contains(root, `\Background\`) {
			command += ` "%V"`
		} else {
			command += ` "%1"`
		}
		commandKey, _, err := registry.CreateKey(registry.CURRENT_USER, root+`\command`, registry.SET_VALUE)
		if err != nil {
			return fmt.Errorf("shellext: create command %s: %w", root, err)
		}
		err = commandKey.SetStringValue("", command)
		if closeErr := commandKey.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return fmt.Errorf("shellext: configure command %s: %w", root, err)
		}
	}
	return nil
}

// Unregister removes all jxleet context-menu entries for the current user.
func Unregister() error {
	for _, root := range menuRoots {
		if err := deleteTree(root); err != nil {
			return fmt.Errorf("shellext: remove %s: %w", root, err)
		}
	}
	return nil
}

// Registered reports whether the primary file entry exists.
func Registered() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, menuRoots[0], registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = key.Close() }()
	_, _, err = key.GetStringValue("MUIVerb")
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func deleteTree(path string) error {
	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		path,
		registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS,
	)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	children, err := key.ReadSubKeyNames(-1)
	_ = key.Close()
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := deleteTree(path + `\` + child); err != nil {
			return err
		}
	}
	return registry.DeleteKey(registry.CURRENT_USER, path)
}

func quoteWindowsArg(value string) string {
	var quoted strings.Builder
	quoted.WriteByte('"')
	backslashes := 0
	for _, r := range value {
		if r == '\\' {
			backslashes++
			continue
		}
		if r == '"' {
			quoted.WriteString(strings.Repeat(`\`, backslashes*2+1))
			quoted.WriteByte('"')
			backslashes = 0
			continue
		}
		if backslashes > 0 {
			quoted.WriteString(strings.Repeat(`\`, backslashes))
			backslashes = 0
		}
		quoted.WriteRune(r)
	}
	if backslashes > 0 {
		quoted.WriteString(strings.Repeat(`\`, backslashes*2))
	}
	quoted.WriteByte('"')
	return quoted.String()
}
