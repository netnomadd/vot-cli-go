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

// TranslateParams contains high-level parameters for a translation request.
type TranslateParams struct {
	URL          string
	RequestLang  string
	ResponseLang string
	DirectURL    bool
	SubsURL      string
	UseM3U8      bool
	VoiceStyle   VoiceStyle
	BypassCache  bool
	WasStream    bool
	VideoTitle   string
}

// TranslateResult contains essential data returned from backend.
type TranslateResult struct {
	AudioURL      string
	Duration      float64
	DetectedLang  string
	IsLivelyVoice bool
}

// Client is a generic interface for a VOT backend implementation.
type Client interface {
	TranslateVideo(ctx context.Context, p TranslateParams) (TranslateResult, error)
}
