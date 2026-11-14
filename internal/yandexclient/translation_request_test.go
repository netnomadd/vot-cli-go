package yandexclient

import (
	"strings"
	"testing"
	"time"

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

func TestLivelyVoiceEnabled(t *testing.T) {
	if !livelyVoiceEnabled(backend.VoiceStyleLive, "en", "ru", "token") {
		t.Fatal("en→ru with a token should request lively voice")
	}
	if livelyVoiceEnabled(backend.VoiceStyleLive, "EN", "RU", "token") == false {
		t.Fatal("language check should be case-insensitive")
	}
	cases := []struct {
		style backend.VoiceStyle
		req   string
		resp  string
		token string
	}{
		{backend.VoiceStyleLive, "en", "ru", ""},
		{backend.VoiceStyleLive, "auto", "ru", "token"},
		{backend.VoiceStyleLive, "", "ru", "token"},
		{backend.VoiceStyleLive, "en", "en", "token"},
		{backend.VoiceStyleTTS, "en", "ru", "token"},
	}
	for _, tc := range cases {
		if livelyVoiceEnabled(tc.style, tc.req, tc.resp, tc.token) {
			t.Fatalf("lively voice enabled for %+v", tc)
		}
	}
}

func TestEncodeTranslationRequestLivelyVoiceRequiresToken(t *testing.T) {
	c := NewDirectClient()
	p := backend.TranslateParams{ResponseLang: "ru", VoiceStyle: backend.VoiceStyleLive, RequestLang: "en"}
	body, err := c.encodeTranslationRequest("https://youtu.be/abc", 120, "en", "ru", p, true)
	if err != nil {
		t.Fatal(err)
	}
	var req yandexproto.VideoTranslationRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	if req.GetUseLivelyVoice() {
		t.Fatal("lively voice must stay off without an OAuth token")
	}
	c.SetAPIToken("token")
	body, err = c.encodeTranslationRequest("https://youtu.be/abc", 120, "en", "ru", p, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := proto.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	if !req.GetUseLivelyVoice() {
		t.Fatal("en→ru with a token should set useLivelyVoice")
	}
}

func TestYtDLPAudioFormat(t *testing.T) {
	if got := ytDLPAudioFormat("en"); got != "ba[language^=en]/ba" {
		t.Fatalf("en format = %q", got)
	}
	if got := ytDLPAudioFormat(" EN "); got != "ba[language^=en]/ba" {
		t.Fatalf("trimmed format = %q", got)
	}
	for _, lang := range []string{"", "auto", "AUTO", "en/ru"} {
		if got := ytDLPAudioFormat(lang); got != "ba" {
			t.Fatalf("format(%q) = %q, want ba", lang, got)
		}
	}
}

func TestUserAgentMatchesComponent(t *testing.T) {
	if !UserAgentMatchesComponent(defaultUserAgent) {
		t.Fatal("built-in user agent should match the component version")
	}
	old := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 YaBrowser/24.4.0.0 Safari/537.36"
	if UserAgentMatchesComponent(old) {
		t.Fatal("YaBrowser 24.4 must not match the current component")
	}
	hints := browserClientHints()
	if !strings.Contains(hints["sec-ch-ua"], "YaBrowser") || !strings.Contains(hints["sec-ch-ua-full-version-list"], defaultComponentVersion) {
		t.Fatalf("sec-ch-ua hints = %#v", hints)
	}
}

func TestEmptyAudioFileID(t *testing.T) {
	got := emptyAudioFileID("https://youtu.be/RYO-RGXs00c")
	want := "fallback-empty-audio:video-translation:RYO-RGXs00c"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestChooseChunkSizeRespectsYandexMinimum(t *testing.T) {
	if got := chooseChunkSize(10_575_016); got != yandexMinChunkSize {
		t.Fatalf("10MB chunk = %d, want %d", got, yandexMinChunkSize)
	}
	if got := chooseChunkSize(30 * 1024 * 1024); got < yandexMinChunkSize {
		t.Fatalf("chunk %d is below yandex minimum %d", got, yandexMinChunkSize)
	}
}

func TestValidateAudioUploadResponse(t *testing.T) {
	done, err := proto.Marshal(&yandexproto.VideoTranslationAudioResponse{Status: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateAudioUploadResponse(done, true); err != nil {
		t.Fatalf("complete upload: %v", err)
	}
	waiting, err := proto.Marshal(&yandexproto.VideoTranslationAudioResponse{Status: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateAudioUploadResponse(waiting, false); err != nil {
		t.Fatalf("intermediate chunk: %v", err)
	}
	if err := validateAudioUploadResponse(waiting, true); err == nil {
		t.Fatal("expected last chunk with status 1 to fail")
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

func TestNextPollDelay(t *testing.T) {
	base := 30 * time.Second

	tests := []struct {
		name      string
		remaining int32
		fixed     bool
		want      time.Duration
	}{
		{name: "auto no remaining uses base", remaining: 0, fixed: false, want: 30 * time.Second},
		{name: "auto negative remaining uses base", remaining: -1, fixed: false, want: 30 * time.Second},
		{name: "auto short remaining equals remaining", remaining: 1, fixed: false, want: 1 * time.Second},
		{name: "auto remaining under base uses remaining", remaining: 12, fixed: false, want: 12 * time.Second},
		{name: "auto remaining equals base", remaining: 30, fixed: false, want: 30 * time.Second},
		{name: "auto remaining above base uses remaining", remaining: 45, fixed: false, want: 45 * time.Second},
		{name: "auto very large remaining equals remaining", remaining: 300, fixed: false, want: 300 * time.Second},
		{name: "fixed mode always uses base", remaining: 12, fixed: true, want: 30 * time.Second},
		{name: "fixed mode ignores remaining", remaining: 45, fixed: true, want: 30 * time.Second},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nextPollDelay(base, tc.remaining, tc.fixed)
			if got != tc.want {
				t.Fatalf("nextPollDelay(%s, %d, fixed=%v) = %s, want %s", base, tc.remaining, tc.fixed, got, tc.want)
			}
		})
	}
}
