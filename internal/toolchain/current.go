package toolchain

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func writeCurrentVersion(path, version string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("toolchain: create pointer directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".current-*.tmp")
	if err != nil {
		return fmt.Errorf("toolchain: create pointer temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.WriteString(version + "\n"); err != nil {
		return fmt.Errorf("toolchain: write active version: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("toolchain: flush active version: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("toolchain: close active version: %w", err)
	}
	if err := atomicReplaceFile(tmpPath, path); err != nil {
		return fmt.Errorf("toolchain: atomically activate %s: %w", version, err)
	}
	cleanup = false
	return nil
}

func readCurrentVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotInstalled
	}
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(data))
	if version == "" {
		return "", errors.New("toolchain: active version pointer is empty")
	}
	return version, nil
}
