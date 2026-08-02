package langs

import (
	"fmt"
	"strings"
)

// Request language codes accepted by Yandex VOT / vot.js (including auto).
var availableRequestLangs = []string{
	"auto", "ru", "en", "zh", "ko", "lt", "lv", "ar", "fr", "it", "es", "de", "ja",
}

// Response language codes for working TTS voices (vot.js availableTTS).
var availableResponseLangs = []string{
	"ru", "en", "kk",
}

// AvailableRequestLangs returns a copy of supported source language codes.
func AvailableRequestLangs() []string {
	out := make([]string, len(availableRequestLangs))
	copy(out, availableRequestLangs)
	return out
}

// AvailableResponseLangs returns a copy of supported target language codes.
func AvailableResponseLangs() []string {
	out := make([]string, len(availableResponseLangs))
	copy(out, availableResponseLangs)
	return out
}

func contains(list []string, code string) bool {
	for _, item := range list {
		if item == code {
			return true
		}
	}
	return false
}

// NormalizeLang lowercases and trims a language code.
func NormalizeLang(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

// ValidateRequestLang checks a source language. Empty means auto-detect and is OK.
func ValidateRequestLang(code string) error {
	code = NormalizeLang(code)
	if code == "" {
		return nil
	}
	if !contains(availableRequestLangs, code) {
		return fmt.Errorf("unsupported --request-lang %q (allowed: %s)", code, strings.Join(availableRequestLangs, ", "))
	}
	return nil
}

// ValidateResponseLang checks a target language (required, non-empty).
func ValidateResponseLang(code string) error {
	code = NormalizeLang(code)
	if code == "" {
		return fmt.Errorf("empty --response-lang")
	}
	if !contains(availableResponseLangs, code) {
		return fmt.Errorf("unsupported --response-lang %q (allowed: %s)", code, strings.Join(availableResponseLangs, ", "))
	}
	return nil
}
