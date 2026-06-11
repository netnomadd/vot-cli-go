package backend

import (
	"context"
)

// VoiceStyle represents desired voice style for translation.
// "live" enables Lively Voice, "tts" uses classic voices.
type VoiceStyle string

const (
	VoiceStyleLive VoiceStyle = "live"
	VoiceStyleTTS  VoiceStyle = "tts"
)

// TranslateParams contains high-level parameters for a VOT video translation request.
type TranslateParams struct {
	URL          string
	OriginalURL  string
	RequestLang  string
	ResponseLang string
	DirectURL    bool
	SubsURL      string
	VoiceStyle   VoiceStyle
	BypassCache  bool
	WasStream    bool
	VideoTitle   string

	// Optional known duration of the video in seconds. If zero or negative,
	// backends fall back to their internal default (e.g. 343s).
	DurationSec float64

	// Optional yt-dlp settings used by backend-side recovery flows
	// (for example AUDIO_REQUESTED real-audio upload).
	UseYtDLP                bool
	YtDLPCookies            string
	YtDLPCookiesFromBrowser string
	// If true, AUDIO_REQUESTED handler attempts real-audio upload first.
	// If false, it follows vot.js-like behaviour (fake/empty audio payload only).
	AudioRequestedRealUpload bool

	// Polling configuration (seconds between attempts, number of attempts).
	// PollAttempts==0 means unlimited attempts until context timeout/cancel.
	// If PollIntervalSec is zero or negative, backends fall back to defaults.
	// FixedPollInterval=true means always sleep PollIntervalSec between attempts.
	// FixedPollInterval=false means prefer API remainingTime when available.
	PollIntervalSec    int
	FixedPollInterval  bool
	PollAttempts       int

	// Debug enables verbose backend-level logging (wired from CLI --debug).
	Debug bool
}

// TranslateResult contains essential data returned from a video translation.
type TranslateResult struct {
	AudioURL      string
	Duration      float64
	DetectedLang  string
	IsLivelyVoice bool
}

// StreamParams contains parameters for a stream (m3u8) translation request.
type StreamParams struct {
	URL          string
	RequestLang  string
	ResponseLang string
}

// StreamResult contains data returned from a successful stream translation.
type StreamResult struct {
	StreamURL string
}

// Client is a generic interface for a VOT backend implementation.
type Client interface {
	TranslateVideo(ctx context.Context, p TranslateParams) (TranslateResult, error)
	TranslateStream(ctx context.Context, p StreamParams) (StreamResult, error)
}
