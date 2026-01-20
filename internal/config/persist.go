package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// WriteConfigFile writes the config to disk with an atomic rename.
func WriteConfigFile(cfg *Config) error {
	if cfg == nil {
		return nil
	}

	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(ConfigFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpPath := ConfigFile + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, ConfigFile)
}
