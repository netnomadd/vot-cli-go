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
	RequestLang  string
	ResponseLang string
	DirectURL    bool
	SubsURL      string
	VoiceStyle   VoiceStyle
	BypassCache  bool
	WasStream    bool
	VideoTitle   string

	// Polling configuration (seconds between attempts, number of attempts).
	// If zero or negative, backends fall back to their defaults.
	PollIntervalSec int
	PollAttempts    int

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
