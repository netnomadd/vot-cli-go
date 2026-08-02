package yandexclient

import "testing"

func TestPickSubtitleURL(t *testing.T) {
	tracks := []SubtitleTrack{
		{Language: "en", URL: "http://orig.en", TranslatedLanguage: "ru", TranslatedURL: "http://tr.ru"},
		{Language: "de", URL: "http://orig.de"},
	}
	got, ok := PickSubtitleURL(tracks, "ru")
	if !ok || got != "http://tr.ru" {
		t.Fatalf("prefer translated: got %q ok=%v", got, ok)
	}
	got, ok = PickSubtitleURL(tracks, "de")
	if !ok || got != "http://orig.de" {
		t.Fatalf("fallback original: got %q ok=%v", got, ok)
	}
	if _, ok := PickSubtitleURL(tracks, "kk"); ok {
		t.Fatal("expected miss for kk")
	}
}
