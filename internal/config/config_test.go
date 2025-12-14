package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExplicitPathOK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := []byte(`{"user_agent":"ua","yandex_hmac_key":"hmac","yandex_token":"token","default_response_lang":"en","use_yt_dlp":true,"yt_dlp_use_direct_url":true,
		"source_rules":[{"pattern":"(?i)^https?://example.com/","use_yt_dlp":true,"yt_dlp_use_direct_url":false,"request_lang":"de","backend":"worker","voice_style":"tts"}]}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, gotPath, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if gotPath != path {
		t.Fatalf("Load path = %q, want %q", gotPath, path)
	}
	if cfg == nil {
		t.Fatalf("Load returned nil config")
	}
	if cfg.UserAgent != "ua" || cfg.YandexHMACKey != "hmac" || cfg.YandexToken != "token" || cfg.DefaultResponseLang != "en" {
		t.Fatalf("unexpected config values: %+v", cfg)
	}
	if !cfg.UseYtDLP || !cfg.YtDLPUseDirectURL {
		t.Fatalf("yt-dlp flags not loaded correctly: %+v", cfg)
	}
	if len(cfg.SourceRules) != 1 {
		t.Fatalf("expected 1 source rule, got %d", len(cfg.SourceRules))
	}
	rule := cfg.SourceRules[0]
	if rule.Pattern == "" || rule.UseYtDLP == nil || rule.YtDLPUseDirectURL == nil || rule.RequestLang == nil || rule.Backend == nil || rule.VoiceStyle == nil {
		t.Fatalf("source rule not loaded correctly: %+v", rule)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	if _, _, err := Load(path); err == nil {
		t.Fatalf("expected error for invalid JSON, got nil")
	}
}

func TestLoadMissingDefaultReturnsEmpty(t *testing.T) {
	oldXDG, hadXDG := os.LookupEnv("XDG_CONFIG_HOME")
	if hadXDG {
		defer os.Setenv("XDG_CONFIG_HOME", oldXDG)
	} else {
		defer os.Unsetenv("XDG_CONFIG_HOME")
	}

	tdir := t.TempDir()
	if err := os.Setenv("XDG_CONFIG_HOME", tdir); err != nil {
		t.Fatalf("failed to set XDG_CONFIG_HOME: %v", err)
	}

	cfg, path, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg == nil {
		t.Fatalf("Load returned nil config")
	}

	wantPath := filepath.Join(tdir, "vot-cli", "config.json")
	if path != wantPath {
		t.Fatalf("Load default path = %q, want %q", path, wantPath)
	}

	// When default config is missing, we expect an empty struct (all fields zero).
	if cfg.UserAgent != "" || cfg.YandexHMACKey != "" || cfg.YandexToken != "" || cfg.DefaultResponseLang != "" || cfg.UseYtDLP || cfg.YtDLPUseDirectURL || len(cfg.SourceRules) != 0 {
		t.Fatalf("expected empty config for missing default file, got %+v", cfg)
	}
}
