//go:build !no_worker

package yandexclient

import (
	"testing"

	"github.com/netnomadd/vot-cli-go/internal/backend"
)

func TestWorkerEncodeIncludesTranslationHelp(t *testing.T) {
	c := NewWorkerClient()
	withHelp := backend.TranslateParams{
		DirectURL:  true,
		SubsURL:    "https://example.com/subs.vtt",
		VoiceStyle: backend.VoiceStyleTTS,
	}
	body, err := c.encodeTranslationRequest("https://cdn.example.com/v.mp4", 120, "en", "ru", withHelp, true)
	if err != nil {
		t.Fatal(err)
	}
	bare, err := c.encodeTranslationRequest("https://cdn.example.com/v.mp4", 120, "en", "ru", backend.TranslateParams{
		VoiceStyle: backend.VoiceStyleTTS,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !(len(body) > len(bare)) {
		t.Fatalf("expected TranslationHelp to grow payload: withHelp=%d bare=%d", len(body), len(bare))
	}
}
