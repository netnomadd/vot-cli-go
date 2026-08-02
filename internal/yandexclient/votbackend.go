package yandexclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/netnomadd/vot-cli-go/internal/backend"
)

const (
	defaultVOTSchema = "https"
	defaultVOTHost   = "vot.toil.cc/v1"
)

var (
	customMediaExtRe = regexp.MustCompile(`(?i)\.(m3u8|m4a|m4v|mpd)(?:$|[?#])`)
	epicGamesCDNRe   = regexp.MustCompile(`(?i)^https://cdn\.qstv\.on\.epicgames\.com`)
)

// VOTBackendClient talks to FOSWLY vot-backend (JSON API) for custom media links.
type VOTBackendClient struct {
	httpClient *http.Client
	schema     string
	host       string // host + optional path prefix, e.g. vot.toil.cc/v1
	userAgent  string
}

// NewVOTBackendClient creates a client pointed at the public vot-backend by default.
func NewVOTBackendClient() *VOTBackendClient {
	return &VOTBackendClient{
		httpClient: &http.Client{Timeout: defaultTimeout},
		schema:     defaultVOTSchema,
		host:       defaultVOTHost,
		userAgent:  "vot-cli-go",
	}
}

// DefaultVOTHostURL returns the default vot-backend base URL.
func DefaultVOTHostURL() string {
	return defaultVOTSchema + "://" + defaultVOTHost
}

// NormalizeVOTHost parses a vot-backend base URL. Unlike worker URLs, a path
// prefix (e.g. /v1) is allowed and preserved.
func NormalizeVOTHost(raw string) (schema, hostWithPath string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultVOTSchema, defaultVOTHost, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("parse vot-host URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", "", fmt.Errorf("vot-host URL must include scheme and host")
	}
	if parsed.RawQuery != "" {
		return "", "", fmt.Errorf("vot-host URL must not include a query string")
	}
	if parsed.Fragment != "" {
		return "", "", fmt.Errorf("vot-host URL must not include a fragment")
	}
	path := strings.TrimSuffix(parsed.Path, "/")
	hostWithPath = strings.ToLower(parsed.Host)
	if path != "" && path != "/" {
		hostWithPath += path
	}
	return strings.ToLower(parsed.Scheme), hostWithPath, nil
}

// SetVOTHost overrides the default vot-backend endpoint.
func (c *VOTBackendClient) SetVOTHost(raw string) error {
	schema, host, err := NormalizeVOTHost(raw)
	if err != nil {
		return err
	}
	c.schema = schema
	c.host = host
	return nil
}

// SetUserAgent overrides the User-Agent sent to vot-backend.
func (c *VOTBackendClient) SetUserAgent(ua string) {
	if ua != "" {
		c.userAgent = ua
	}
}

// SetHTTPProxy configures an HTTP(S) proxy for vot-backend requests.
func (c *VOTBackendClient) SetHTTPProxy(proxyURL string) error {
	return applyHTTPProxy(c.httpClient, proxyURL)
}

// IsCustomLink reports whether url should be routed through vot-backend
// (HLS/DASH/audio containers and known CDN exceptions), matching vot.js.
func IsCustomLink(rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return false
	}
	if epicGamesCDNRe.MatchString(rawURL) {
		return true
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return customMediaExtRe.MatchString(rawURL)
	}
	return customMediaExtRe.MatchString(u.Path) || customMediaExtRe.MatchString(u.RawPath)
}

type votTranslationRequest struct {
	Provider string `json:"provider"`
	Service  string `json:"service"`
	VideoID  string `json:"video_id"`
	FromLang string `json:"from_lang"`
	ToLang   string `json:"to_lang"`
	RawVideo string `json:"raw_video"`
}

type votTranslationResponse struct {
	ID            int    `json:"id"`
	Status        string `json:"status"`
	Provider      string `json:"provider"`
	TranslatedURL string `json:"translated_url"`
	Message       string `json:"message"`
	RemainingTime int    `json:"remaining_time"`
}

type votSubtitleItem struct {
	Lang        string `json:"lang"`
	LangFrom    string `json:"lang_from"`
	SubtitleURL string `json:"subtitle_url"`
}

// TranslateVideo implements backend.Client for custom media via vot-backend.
func (c *VOTBackendClient) TranslateVideo(ctx context.Context, p backend.TranslateParams) (backend.TranslateResult, error) {
	provider := "yandex"
	if p.VoiceStyle == backend.VoiceStyleLive {
		provider = "yandex_lively"
	}
	fromLang := p.RequestLang
	if fromLang == "" {
		fromLang = "auto"
	}
	toLang := p.ResponseLang
	if toLang == "" {
		toLang = "ru"
	}

	cfg := buildPollConfig(p)
	videoURL := p.URL
	service, videoID := votServiceAndID(videoURL)

	for attempt := 1; cfg.unlimitedAttempts || attempt <= cfg.attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return backend.TranslateResult{}, err
		}

		body := votTranslationRequest{
			Provider: provider,
			Service:  service,
			VideoID:  videoID,
			FromLang: fromLang,
			ToLang:   toLang,
			RawVideo: videoURL,
		}
		var resp votTranslationResponse
		if err := c.postJSON(ctx, "/video-translation/translate", body, &resp); err != nil {
			return backend.TranslateResult{}, err
		}

		switch strings.ToLower(resp.Status) {
		case "success":
			if resp.TranslatedURL == "" {
				return backend.TranslateResult{}, fmt.Errorf("vot-backend: success without translated_url")
			}
			return backend.TranslateResult{
				AudioURL:      resp.TranslatedURL,
				IsLivelyVoice: provider == "yandex_lively",
			}, nil
		case "waiting", "parted":
			if p.Debug {
				fmt.Fprintf(os.Stderr, "[debug] [vot-backend] poll attempt %d status=%s remaining=%d message=%s\n",
					attempt, resp.Status, resp.RemainingTime, resp.Message)
			}
			delay := nextPollDelay(cfg.interval, int32(resp.RemainingTime), cfg.fixedInterval)
			if delay < 5*time.Second {
				delay = 5 * time.Second
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return backend.TranslateResult{}, ctx.Err()
			case <-timer.C:
			}
			continue
		case "failed":
			msg := resp.Message
			if msg == "" {
				msg = "translation failed"
			}
			return backend.TranslateResult{}, fmt.Errorf("vot-backend: %s", msg)
		default:
			return backend.TranslateResult{}, fmt.Errorf("vot-backend: unknown status %q", resp.Status)
		}
	}
	return backend.TranslateResult{}, fmt.Errorf("vot-backend: translation not ready after polling")
}

// TranslateStream is not supported via vot-backend in this CLI.
func (c *VOTBackendClient) TranslateStream(context.Context, backend.StreamParams) (backend.StreamResult, error) {
	return backend.StreamResult{}, fmt.Errorf("vot-backend: stream translation is not supported")
}

// GetSubtitles requests subtitles for a custom media URL via vot-backend.
func (c *VOTBackendClient) GetSubtitles(ctx context.Context, videoURL string) (GetSubtitlesResult, error) {
	service, videoID := votServiceAndID(videoURL)
	body := map[string]string{
		"provider": "yandex",
		"service":  service,
		"video_id": videoID,
	}
	var items []votSubtitleItem
	if err := c.postJSON(ctx, "/video-subtitles/get-subtitles", body, &items); err != nil {
		return GetSubtitlesResult{}, err
	}

	tracks := make([]SubtitleTrack, 0, len(items))
	for _, item := range items {
		if item.LangFrom == "" {
			tracks = append(tracks, SubtitleTrack{
				Language: item.Lang,
				URL:      item.SubtitleURL,
			})
			continue
		}
		// Pair translated track with its original language/source when possible.
		var origURL string
		for _, other := range items {
			if other.Lang == item.LangFrom && other.LangFrom == "" {
				origURL = other.SubtitleURL
				break
			}
		}
		tracks = append(tracks, SubtitleTrack{
			Language:           item.LangFrom,
			URL:                origURL,
			TranslatedLanguage: item.Lang,
			TranslatedURL:      item.SubtitleURL,
		})
	}
	return GetSubtitlesResult{Waiting: false, Subtitles: tracks}, nil
}

func votServiceAndID(videoURL string) (service, videoID string) {
	if epicGamesCDNRe.MatchString(videoURL) {
		return "epicgames", videoURL
	}
	return "custom", videoURL
}

func (c *VOTBackendClient) postJSON(ctx context.Context, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	endpoint := c.schema + "://" + c.host + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vot-backend request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("vot-backend read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("vot-backend: unexpected status %s: %s", resp.Status, msg)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("vot-backend decode response: %w", err)
	}
	return nil
}
