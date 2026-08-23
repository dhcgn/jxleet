package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads the config from path. A missing file is not an error: it returns
// Default so a fresh install starts with no bindings set.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Version == 0 {
		c.Version = 1
	}
	if c.Bindings == nil {
		c.Bindings = map[EntryPoint]string{}
	}
	return c, nil
}

// Save writes the config to path, creating parent directories as needed.
func Save(path string, c Config) error {
	if c.Version == 0 {
		c.Version = 1
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
