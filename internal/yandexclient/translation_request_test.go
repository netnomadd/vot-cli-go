package yandexclient

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/netnomadd/vot-cli-go/internal/backend"
	"github.com/netnomadd/vot-cli-go/internal/yandexproto"
)

func TestEncodeTranslationRequestFirstRequestFlag(t *testing.T) {
	c := NewDirectClient()
	p := backend.TranslateParams{ResponseLang: "ru", VoiceStyle: backend.VoiceStyleLive}

	firstBody, err := c.encodeTranslationRequest("https://example.com/watch", 120, "en", "ru", p, true)
	if err != nil {
		t.Fatalf("encode first request: %v", err)
	}
	secondBody, err := c.encodeTranslationRequest("https://example.com/watch", 120, "en", "ru", p, false)
	if err != nil {
		t.Fatalf("encode follow-up request: %v", err)
	}

	var first, second yandexproto.VideoTranslationRequest
	if err := proto.Unmarshal(firstBody, &first); err != nil {
		t.Fatalf("unmarshal first request: %v", err)
	}
	if err := proto.Unmarshal(secondBody, &second); err != nil {
		t.Fatalf("unmarshal follow-up request: %v", err)
	}

	if !first.GetFirstRequest() {
		t.Fatal("expected firstRequest=true on initial poll")
	}
	if second.GetFirstRequest() {
		t.Fatal("expected firstRequest=false on follow-up poll")
	}
}

func TestEncodeTranslationRequestAlwaysSupportsTrueFirstRequest(t *testing.T) {
	c := NewDirectClient()
	p := backend.TranslateParams{ResponseLang: "ru", VoiceStyle: backend.VoiceStyleLive}

	body, err := c.encodeTranslationRequest("https://example.com/watch", 120, "en", "ru", p, true)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}

	var req yandexproto.VideoTranslationRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	if !req.GetFirstRequest() {
		t.Fatal("expected firstRequest=true")
	}
}

func TestYoutubeAudioWorkaroundURL(t *testing.T) {
	t.Run("watch url normalized to youtu.be", func(t *testing.T) {
		got, ok := youtubeAudioWorkaroundURL("https://www.youtube.com/watch?v=abc123&pp=foo")
		if !ok {
			t.Fatal("expected workaround url for youtube watch")
		}
		if got != "https://youtu.be/abc123" {
			t.Fatalf("got %q, want %q", got, "https://youtu.be/abc123")
		}
	})

	t.Run("short url stays short", func(t *testing.T) {
		got, ok := youtubeAudioWorkaroundURL("https://youtu.be/xyz789")
		if !ok {
			t.Fatal("expected workaround url for youtu.be")
		}
		if got != "https://youtu.be/xyz789" {
			t.Fatalf("got %q, want %q", got, "https://youtu.be/xyz789")
		}
	})

	t.Run("non youtube unsupported", func(t *testing.T) {
		if _, ok := youtubeAudioWorkaroundURL("https://example.com/video"); ok {
			t.Fatal("expected non-youtube URL to be unsupported")
		}
	})
}

func TestBuildTranslationHelp(t *testing.T) {
	p := backend.TranslateParams{
		DirectURL: true,
		SubsURL:   "https://example.com/subs.vtt",
	}
	help := buildTranslationHelp("https://cdn.example.com/video.mp4", p)
	if len(help) != 2 {
		t.Fatalf("expected 2 translationHelp items, got %d", len(help))
	}
	if help[0].GetTarget() != "video_file_url" || help[0].GetTargetUrl() != "https://cdn.example.com/video.mp4" {
		t.Fatalf("unexpected first help item: %+v", help[0])
	}
	if help[1].GetTarget() != "subtitles_file_url" || help[1].GetTargetUrl() != "https://example.com/subs.vtt" {
		t.Fatalf("unexpected second help item: %+v", help[1])
	}
}

func TestRandomUUID32(t *testing.T) {
	uuid := randomUUID32()
	if len(uuid) != 32 {
		t.Fatalf("uuid length = %d, want 32", len(uuid))
	}
	for i := 0; i < len(uuid); i++ {
		ch := uuid[i]
		isDigit := ch >= '0' && ch <= '9'
		isHexUpper := ch >= 'A' && ch <= 'F'
		if !isDigit && !isHexUpper {
			t.Fatalf("uuid contains non-hex character %q at %d", ch, i)
		}
	}
}
