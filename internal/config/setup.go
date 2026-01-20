package config

import (
	"os"
	"strings"
)

type ConfigSource string

const (
	ConfigSourceNone ConfigSource = "none"
	ConfigSourceFile ConfigSource = "file"
	ConfigSourceEnv  ConfigSource = "env"
)

var setupMode *bool

const (
	envForceSetupUI     = "FORCE_SETUP_UI"
	envForceEmptyConfig = "FORCE_EMPTY_CONFIG"
)

// ConfigSourceType reports where the config comes from.
func ConfigSourceType() ConfigSource {
	if ForceEmptyConfigEnabled() {
		return ConfigSourceNone
	}
	if HasEnvConfigSource() {
		return ConfigSourceEnv
	}
	if _, err := os.Stat(ConfigFile); err == nil {
		return ConfigSourceFile
	}
	return ConfigSourceNone
}

// NeedsSetup reports whether the setup wizard should be shown.
func NeedsSetup() bool {
	if forceSetupEnabled() {
		return true
	}
	if ForceEmptyConfigEnabled() {
		return true
	}
	return ConfigSourceType() == ConfigSourceNone
}

// ConfigReadOnly reports whether config is provided by env and should be treated read-only.
func ConfigReadOnly() bool {
	return ConfigSourceType() == ConfigSourceEnv
}

// SetSetupMode locks runtime setup mode for this process.
func SetSetupMode(value bool) {
	setupMode = &value
}

// IsSetupMode reports whether the current process is running in setup mode.
func IsSetupMode() bool {
	if setupMode != nil {
		return *setupMode
	}
	return NeedsSetup()
}

func forceSetupEnabled() bool {
	return isTruthyEnv(envForceSetupUI)
}

// ForceEmptyConfigEnabled reports whether config loading should ignore files/env config.
func ForceEmptyConfigEnabled() bool {
	return isTruthyEnv(envForceEmptyConfig)
}

func isTruthyEnv(key string) bool {
	value := strings.TrimSpace(os.Getenv(key))
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
