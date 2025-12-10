package config

import (
	"os"
)

// Config is a minimal placeholder for future configuration loading.
// We'll extend this as we add real config parsing.
type Config struct {
	UserAgent string
}

// Load returns default config for now; later we'll implement YAML/TOML loading
// with XDG/OS-specific paths and CLI/env overrides.
func Load(path string) (*Config, error) {
	_ = path
	return &Config{UserAgent: "vot-cli-go/0.1"}, nil
}

// LangFromEnv returns preferred language from environment as a small helper.
func LangFromEnv() string {
	if v := os.Getenv("VOT_LANG"); v != "" {
		return v
	}
	return ""
}
