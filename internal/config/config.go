package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config represents user-level configuration for vot-cli-go.
// Fields are intentionally minimal and map directly to what
// Yandex direct backend needs.
type Config struct {
	UserAgent     string `json:"user_agent"`
	YandexHMACKey string `json:"yandex_hmac_key"`
	YandexToken   string `json:"yandex_token"`
}

// DefaultPath returns the OS-specific default path to config.json.
// Example (Linux/macOS): $XDG_CONFIG_HOME/vot-cli/config.json
// Example (Windows): %APPDATA%\\vot-cli\\config.json
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return ""
	}
	return filepath.Join(dir, "vot-cli", "config.json")
}

// Load loads configuration from an explicit path (if provided) or
// from the default path. It returns the parsed config and the
// actual path it tried to read. When the default config file is
// missing, it returns an empty config and no error.
func Load(explicitPath string) (*Config, string, error) {
	if explicitPath != "" {
		data, err := os.ReadFile(explicitPath)
		if err != nil {
			return nil, explicitPath, err
		}
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, explicitPath, err
		}
		return &cfg, explicitPath, nil
	}

	path := DefaultPath()
	if path == "" {
		// No reasonable default path on this OS; behave as if no config.
		return &Config{}, "", nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Missing default config is not an error.
			return &Config{}, path, nil
		}
		return nil, path, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, path, err
	}
	return &cfg, path, nil
}
