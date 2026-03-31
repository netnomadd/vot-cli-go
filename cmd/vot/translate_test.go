package main

import (
	"testing"

	"github.com/netnomadd/vot-cli-go/internal/config"
)

func TestSourceRuleHelpersApplyConfigAndRewrite(t *testing.T) {
	cfg := &config.Config{
		SourceRules: []config.SourceRuleConfig{
			{
				Patterns: []string{
					`(?i)^https?://example\.com/watch/`,
					`(?i)^https?://alt\.example\.com/watch/`,
					`[`,
				},
				UseYtDLP:                boolPtr(true),
				YtDLPUseDirectURL:       boolPtr(false),
				YtDLPCookies:            stringPtr("/tmp/cookies.txt"),
				YtDLPCookiesFromBrowser: stringPtr("firefox"),
				RequestLang:             stringPtr("de"),
				Backend:                 stringPtr("worker"),
				VoiceStyle:              stringPtr("tts"),
				Rewrite: []config.RewriteRuleConfig{
					{
						Pattern: `(?i)^https?://alt\.example\.com/`,
						Replace: "https://example.com/",
					},
					{
						Pattern: `[`,
						Replace: "ignored",
					},
				},
			},
		},
	}

	rules := buildSourceRulesFromConfig(cfg)
	url := "https://alt.example.com/watch/123"

	useYtDLP, useDirect := applySourceRules(url, rules, false, true, false, false)
	if !useYtDLP || useDirect {
		t.Fatalf("applySourceRules() = (%v, %v), want (true, false)", useYtDLP, useDirect)
	}

	reqLang, backend := applySourceLangAndBackend(url, rules, "", "", false, false)
	if reqLang != "de" || backend != "worker" {
		t.Fatalf("applySourceLangAndBackend() = (%q, %q), want (%q, %q)", reqLang, backend, "de", "worker")
	}

	voiceStyle := applySourceVoiceStyle(url, rules, "live", false)
	if voiceStyle != "tts" {
		t.Fatalf("applySourceVoiceStyle() = %q, want %q", voiceStyle, "tts")
	}

	cookies, cookiesFromBrowser := applySourceYtDLPCookies(url, rules, "", "", false, false)
	if cookies != "/tmp/cookies.txt" || cookiesFromBrowser != "firefox" {
		t.Fatalf("applySourceYtDLPCookies() = (%q, %q), want (%q, %q)", cookies, cookiesFromBrowser, "/tmp/cookies.txt", "firefox")
	}

	rewritten := applySourceURLRewrites(url, rules)
	if rewritten != "https://example.com/watch/123" {
		t.Fatalf("applySourceURLRewrites() = %q, want %q", rewritten, "https://example.com/watch/123")
	}

	summaries := matchedSourceRuleSummaries(url, rules)
	if len(summaries) == 0 {
		t.Fatalf("matchedSourceRuleSummaries() returned no matches")
	}
}

func TestSourceRuleHelpersRespectExplicitFlags(t *testing.T) {
	cfg := &config.Config{
		SourceRules: []config.SourceRuleConfig{
			{
				Pattern:                 `(?i)^https?://example\.com/`,
				UseYtDLP:                boolPtr(true),
				YtDLPUseDirectURL:       boolPtr(true),
				YtDLPCookies:            stringPtr("/tmp/cookies.txt"),
				YtDLPCookiesFromBrowser: stringPtr("firefox"),
				RequestLang:             stringPtr("de"),
				Backend:                 stringPtr("worker"),
				VoiceStyle:              stringPtr("tts"),
			},
		},
	}

	rules := buildSourceRulesFromConfig(cfg)
	url := "https://example.com/video"

	useYtDLP, useDirect := applySourceRules(url, rules, false, false, true, true)
	if useYtDLP || useDirect {
		t.Fatalf("explicit CLI flags should win, got (%v, %v)", useYtDLP, useDirect)
	}

	reqLang, backend := applySourceLangAndBackend(url, rules, "en", "direct", true, true)
	if reqLang != "en" || backend != "direct" {
		t.Fatalf("explicit CLI request/backend should win, got (%q, %q)", reqLang, backend)
	}

	voiceStyle := applySourceVoiceStyle(url, rules, "live", true)
	if voiceStyle != "live" {
		t.Fatalf("explicit CLI voice style should win, got %q", voiceStyle)
	}

	cookies, cookiesFromBrowser := applySourceYtDLPCookies(url, rules, "base.txt", "chrome", true, true)
	if cookies != "base.txt" || cookiesFromBrowser != "chrome" {
		t.Fatalf("explicit CLI cookies should win, got (%q, %q)", cookies, cookiesFromBrowser)
	}
}
