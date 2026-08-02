package yandexclient

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/netnomadd/vot-cli-go/internal/yandexproto"
)

// SubtitleTrack is one Yandex subtitle entry (original and/or translated).
type SubtitleTrack struct {
	Language           string
	URL                string
	TranslatedLanguage string
	TranslatedURL      string
}

// GetSubtitlesResult is the decoded /video-subtitles/get-subtitles response.
type GetSubtitlesResult struct {
	Waiting   bool
	Subtitles []SubtitleTrack
}

// GetSubtitles requests Yandex subtitles for a video URL via the direct API.
// requestLang is the source language hint (empty = let Yandex decide).
func (c *DirectClient) GetSubtitles(ctx context.Context, videoURL, requestLang string) (GetSubtitlesResult, error) {
	if videoURL == "" {
		return GetSubtitlesResult{}, fmt.Errorf("video URL is required")
	}

	sess, err := c.getSession(ctx, "video-translation")
	if err != nil {
		return GetSubtitlesResult{}, fmt.Errorf("create session: %w", err)
	}

	body, err := proto.Marshal(&yandexproto.SubtitlesRequest{
		Url:      videoURL,
		Language: requestLang,
	})
	if err != nil {
		return GetSubtitlesResult{}, err
	}

	path := "/video-subtitles/get-subtitles"
	headers := c.secYaHeaders("Vsubs", sess, body, path)
	resBytes, err := c.doProtoRequest(ctx, path, body, headers)
	if err != nil {
		return GetSubtitlesResult{}, err
	}

	var resp yandexproto.SubtitlesResponse
	if err := proto.Unmarshal(resBytes, &resp); err != nil {
		return GetSubtitlesResult{}, fmt.Errorf("decode subtitles response: %w", err)
	}

	tracks := make([]SubtitleTrack, 0, len(resp.GetSubtitles()))
	for _, s := range resp.GetSubtitles() {
		tracks = append(tracks, SubtitleTrack{
			Language:           s.GetLanguage(),
			URL:                s.GetUrl(),
			TranslatedLanguage: s.GetTranslatedLanguage(),
			TranslatedURL:      s.GetTranslatedUrl(),
		})
	}

	return GetSubtitlesResult{
		Waiting:   resp.GetWaiting(),
		Subtitles: tracks,
	}, nil
}

// PickSubtitleURL selects a subtitle download URL for responseLang.
// Prefers translated tracks, then original-language tracks.
func PickSubtitleURL(tracks []SubtitleTrack, responseLang string) (string, bool) {
	if responseLang == "" {
		responseLang = "ru"
	}
	for _, t := range tracks {
		if t.TranslatedLanguage == responseLang && t.TranslatedURL != "" {
			return t.TranslatedURL, true
		}
	}
	for _, t := range tracks {
		if t.Language == responseLang && t.URL != "" {
			return t.URL, true
		}
	}
	return "", false
}
