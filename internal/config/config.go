package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config represents user-level configuration for vot-cli-go.
// Fields are intentionally minimal and map directly to what
// Yandex-related backends and helpers need.
// SourceRuleConfig describes a single heuristic rule for specific URL patterns
// that can tweak how yt-dlp is used for that source and optionally adjust
// backend / language defaults. It is configured in config.json to avoid
// hard-coding site-specific behaviour in the binary.
//
// All fields are optional except Pattern/Patterns; boolean and string fields
// are pointers so that "unset" differs from an explicit value.
// Example JSON entry:
// {
//   "patterns": [
//     "(?i)^https?://www\\.zdf\\.de/play/",
//     "(?i)^https?://zdf\\.example/alternate/"
//   ],
//   "use_yt_dlp": true,
//   "yt_dlp_use_direct_url": true,
//   "request_lang": "de",
//   "backend": "worker",
//   "voice_style": "tts",
//   "rewrite": [
//     { "pattern": "(?i)^https?://zdf\\.example/(.*)", "replace": "https://www.zdf.de/play/$1" }
//   ]
// }
//
// Rules from config are applied after built-in defaults, so they can override
// behaviour for matching sites.
type SourceRuleConfig struct {
	// Pattern is a single regex; Patterns allows specifying multiple regexes
	// within one logical rule. Both are supported for backwards compatibility.
	Pattern  string   `json:"pattern"`
	Patterns []string `json:"patterns,omitempty"`

	UseYtDLP                *bool   `json:"use_yt_dlp,omitempty"`
	YtDLPUseDirectURL       *bool   `json:"yt_dlp_use_direct_url,omitempty"`
	YtDLPCookies            *string `json:"yt_dlp_cookies,omitempty"`
	YtDLPCookiesFromBrowser *string `json:"yt_dlp_cookies_from_browser,omitempty"`
	RequestLang             *string `json:"request_lang,omitempty"`
	Backend                 *string `json:"backend,omitempty"`
	VoiceStyle              *string `json:"voice_style,omitempty"`

	// Rewrite optionally contains one or more rewrite rules that can transform
	// incoming URLs before they are classified / passed to yt-dlp or backends.
	// Patterns in Rewrite are regular expressions; Replace is the replacement
	// string in Go's regexp.ReplaceAllString semantics.
	Rewrite []RewriteRuleConfig `json:"rewrite,omitempty"`
}

// RewriteRuleConfig describes a single URL rewrite rule used inside a
// SourceRuleConfig.
type RewriteRuleConfig struct {
	Pattern string `json:"pattern"`
	Replace string `json:"replace"`
}

// Config represents user-level configuration for vot-cli-go.
// Fields are intentionally minimal and map directly to what
// Yandex-related backends and helpers need.
type Config struct {
	UserAgent     string `json:"user_agent"`
	YandexHMACKey string `json:"yandex_hmac_key"`
	YandexToken   string `json:"yandex_token"`

	// Optional default target language for translations when --response-lang
	// не указан явно. Если пусто, по умолчанию используется "ru".
	DefaultResponseLang string `json:"default_response_lang"`

	// Optional integration with yt-dlp (if installed in the system).
	// These flags only take effect in features that explicitly support yt-dlp.
	UseYtDLP              bool   `json:"use_yt_dlp"`
	YtDLPUseDirectURL     bool   `json:"yt_dlp_use_direct_url"`
	YtDLPCookies          string `json:"yt_dlp_cookies"`
	YtDLPCookiesFromBrowser string `json:"yt_dlp_cookies_from_browser"`

	// Optional per-source rules that can tweak yt-dlp usage and direct URL
	// handling depending on the URL pattern. When empty, only built-in rules
	// are used.
	SourceRules []SourceRuleConfig `json:"source_rules,omitempty"`
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
