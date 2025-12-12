package yandexclient

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/netnomadd/vot-cli-go/internal/backend"
	"github.com/netnomadd/vot-cli-go/internal/yandexproto"
)

// DirectClient implements backend.Client and talks directly to
// api.browser.yandex.ru using protobuf messages.
type DirectClient struct {
	httpClient *http.Client

	// base config
	schema           string
	host             string
	userAgent        string
	hmacKey          string
	componentVersion string
	apiToken         string

	// session cache per module (e.g. "video-translation")
	sessions map[string]*clientSession
}

// SetUserAgent overrides the default User-Agent if non-empty.
func (c *DirectClient) SetUserAgent(ua string) {
	if ua != "" {
		c.userAgent = ua
	}
}

// SetHMACKey overrides the default HMAC key if non-empty.
func (c *DirectClient) SetHMACKey(key string) {
	if key != "" {
		c.hmacKey = key
	}
}

// SetAPIToken sets OAuth token used for Lively Voice, if non-empty.
func (c *DirectClient) SetAPIToken(token string) {
	if token != "" {
		c.apiToken = token
	}
}

type clientSession struct {
	secretKey string
	expires   int32
	uuid      string
	timestamp int64 // unix seconds
}

const (
	defaultSchema           = "https"
	defaultHost             = "api.browser.yandex.ru"
	defaultUserAgent        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 YaBrowser/25.4.0.0 Safari/537.36"
	defaultComponentVersion = "25.6.0.2259"
	defaultHMACKey          = "bt8xH3VOlb4mqf0nqAibnDOoiPlXsisf"
	defaultTimeout          = 30 * time.Second
)

// NewDirectClient creates a DirectClient with sane defaults.
// In future we will inject config (user agent, hmac key, token, host) from CLI config.
func NewDirectClient() *DirectClient {
	return &DirectClient{
		httpClient: &http.Client{Timeout: defaultTimeout},
		schema:     defaultSchema,
		host:       defaultHost,
		userAgent:  defaultUserAgent,
		hmacKey:    defaultHMACKey,
		// componentVersion must match the one used to form Sec-* tokens
		componentVersion: defaultComponentVersion,
		// apiToken can be filled later from config/env if needed for Lively Voice
		apiToken: "",
		sessions: make(map[string]*clientSession),
	}
}

// TranslateVideo implements backend.Client.TranslateVideo (minimal version).
// It does a single request to /video-translation/translate and treats only
// responses that contain a non-empty URL as success.
func (c *DirectClient) TranslateVideo(ctx context.Context, p backend.TranslateParams) (backend.TranslateResult, error) {
	// For now we don't distinguish direct/custom links or resend audio, we
	// mirror translateVideoYAImpl's happy-path behaviour.

	// Ensure session
	sess, err := c.getSession(ctx, "video-translation")
	if err != nil {
		return backend.TranslateResult{}, err
	}

	// Build request body. If caller provided a known duration (e.g. via
	// yt-dlp), prefer it over the JS default (343s).
	const defaultDuration = 343.0
	dur := defaultDuration
	if p.DurationSec > 0 {
		dur = p.DurationSec
	}

	interval := time.Duration(p.PollIntervalSec) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}

	attempts := p.PollAttempts
	if attempts <= 0 {
		attempts = 10
	}

	reqLang := p.RequestLang
	respLang := p.ResponseLang

	path := "/video-translation/translate"

	for attempt := 0; attempt < attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return backend.TranslateResult{}, err
		}

		body, err := c.encodeTranslationRequest(p.URL, dur, reqLang, respLang, p)
		if err != nil {
			return backend.TranslateResult{}, err
		}

		vtransHeaders := c.secYaHeaders("Vtrans", sess, body, path)

		// If Lively Voice requested and token is set, add OAuth header
		if p.VoiceStyle == backend.VoiceStyleLive && c.apiToken != "" {
			vtransHeaders["Authorization"] = "OAuth " + c.apiToken
		}

		resBytes, err := c.doProtoRequest(ctx, path, body, vtransHeaders)
		if err != nil {
			return backend.TranslateResult{}, err
		}

		var resp yandexproto.VideoTranslationResponse
		if err := proto.Unmarshal(resBytes, &resp); err != nil {
			return backend.TranslateResult{}, err
		}

		status := resp.GetStatus()
		remaining := resp.GetRemainingTime()
		if p.Debug {
			fmt.Fprintf(os.Stderr, "[debug] [direct] poll attempt %d/%d status=%d remaining=%d\n", attempt+1, attempts, status, remaining)
		}

		url := resp.GetUrl()
		if url != "" {
			return backend.TranslateResult{
				AudioURL:      url,
				Duration:      resp.GetDuration(),
				DetectedLang:  resp.GetLanguage(),
				IsLivelyVoice: resp.GetIsLivelyVoice(),
			}, nil
		}

		msg := resp.GetMessage()

		// Use Yandex status/remainingTime to decide whether to poll again.
		retry := false
		switch status {
		case 2, 3, 6: // WAITING / LONG_WAITING / AUDIO_REQUESTED
			retry = true
		case 7: // SESSION_REQUIRED
			if msg == "" {
				msg = "yandex: this video requires an authenticated Yandex session (SESSION_REQUIRED)"
			} else {
				msg = fmt.Sprintf("yandex: this video requires an authenticated Yandex session (SESSION_REQUIRED): %s", msg)
			}
		default:
			if msg == "" {
				msg = fmt.Sprintf("yandex: translation not ready or failed (status=%d, empty url)", status)
			} else {
				msg = fmt.Sprintf("yandex: translation not ready or failed (status=%d): %s", status, msg)
			}
		}

		if !retry || attempt == attempts-1 {
			if retry {
				if remaining > 0 {
					msg = fmt.Sprintf("yandex: translation still in progress after %d attempts (remaining ~%d seconds)", attempts, remaining)
				} else if msg == "" {
					msg = fmt.Sprintf("yandex: translation still in progress after %d attempts", attempts)
				}
			}
			return backend.TranslateResult{}, errors.New(msg)
		}

		// Wait before next polling attempt.
		select {
		case <-ctx.Done():
			return backend.TranslateResult{}, ctx.Err()
		case <-time.After(interval):
		}
	}

	return backend.TranslateResult{}, errors.New("yandex: translation not ready after polling")
}

// TranslateStream implements backend.Client.TranslateStream.
// It polls /stream-translation/translate-stream until STREAMING or error.
func (c *DirectClient) TranslateStream(ctx context.Context, p backend.StreamParams) (backend.StreamResult, error) {
	sess, err := c.getSession(ctx, "video-translation")
	if err != nil {
		return backend.StreamResult{}, err
	}

	const (
		maxAttempts      = 20
		pollDelaySeconds = 3
	)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return backend.StreamResult{}, err
		}

		msg := &yandexproto.StreamTranslationRequest{
			Url:              p.URL,
			Language:         p.RequestLang,
			ResponseLanguage: p.ResponseLang,
			Unknown0:         1,
			Unknown1:         0,
		}
		body, err := proto.Marshal(msg)
		if err != nil {
			return backend.StreamResult{}, err
		}

		path := "/stream-translation/translate-stream"
		headers := c.secYaHeaders("Vtrans", sess, body, path)

		resBytes, err := c.doProtoRequest(ctx, path, body, headers)
		if err != nil {
			return backend.StreamResult{}, err
		}

		var resp yandexproto.StreamTranslationResponse
		if err := proto.Unmarshal(resBytes, &resp); err != nil {
			return backend.StreamResult{}, err
		}

		interval := resp.GetInterval()
		switch interval {
		case yandexproto.StreamInterval_STREAMING:
			info := resp.GetTranslatedInfo()
			if info == nil || info.GetUrl() == "" {
				return backend.StreamResult{}, errors.New("yandex: empty stream url in STREAMING response")
			}
			return backend.StreamResult{StreamURL: info.GetUrl()}, nil
		case yandexproto.StreamInterval_NO_CONNECTION:
			return backend.StreamResult{}, errors.New("yandex: no connection for stream translation")
		case yandexproto.StreamInterval_TRANSLATING:
			// wait and retry
			select {
			case <-ctx.Done():
				return backend.StreamResult{}, ctx.Err()
			case <-time.After(pollDelaySeconds * time.Second):
			}
		default:
			return backend.StreamResult{}, errors.New("yandex: unknown stream interval")
		}
	}

	return backend.StreamResult{}, errors.New("yandex: stream translation not ready after polling")
}

// encodeTranslationRequest maps high-level params to protobuf request.
func (c *DirectClient) encodeTranslationRequest(url string, duration float64, reqLang, respLang string, p backend.TranslateParams) ([]byte, error) {
	msg := &yandexproto.VideoTranslationRequest{
		Url:              url,
		FirstRequest:     true,
		Duration:         duration,
		Unknown0:         1,
		Language:         reqLang,
		ForceSourceLang:  reqLang != "", // treat explicit source lang as forced
		Unknown1:         0,
		WasStream:        p.WasStream,
		ResponseLanguage: respLang,
		Unknown2:         1,
		Unknown3:         2,
		BypassCache:      p.BypassCache,
		UseLivelyVoice:   p.VoiceStyle == backend.VoiceStyleLive,
		VideoTitle:       p.VideoTitle,
	}

	// translationHelp (subs / video url) пока не заполняем; добавим позже при обсуждении.

	return proto.Marshal(msg)
}

// getSession returns a cached or newly created session for the given module.
func (c *DirectClient) getSession(ctx context.Context, module string) (*clientSession, error) {
	now := time.Now().Unix()
	if s, ok := c.sessions[module]; ok {
		if s.timestamp+int64(s.expires) > now {
			return s, nil
		}
	}

	sess, err := c.createSession(ctx, module)
	if err != nil {
		return nil, err
	}
	c.sessions[module] = sess
	return sess, nil
}

func (c *DirectClient) createSession(ctx context.Context, module string) (*clientSession, error) {
	uuid := randomUUID32()
	bodyMsg := &yandexproto.YandexSessionRequest{
		Uuid:   uuid,
		Module: module,
	}
	body, err := proto.Marshal(bodyMsg)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"Vtrans-Signature": c.signature(body),
	}

	resBytes, err := c.doProtoRequest(ctx, "/session/create", body, headers)
	if err != nil {
		return nil, err
	}

	var resp yandexproto.YandexSessionResponse
	if err := proto.Unmarshal(resBytes, &resp); err != nil {
		return nil, err
	}

	return &clientSession{
		secretKey: resp.GetSecretKey(),
		expires:   resp.GetExpires(),
		uuid:      uuid,
		timestamp: time.Now().Unix(),
	}, nil
}

// doProtoRequest sends a protobuf POST request to Yandex API and returns raw body bytes.
func (c *DirectClient) doProtoRequest(ctx context.Context, path string, body []byte, extraHeaders map[string]string) ([]byte, error) {
	url := c.schema + "://" + c.host + path

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/x-protobuf")
	req.Header.Set("Accept-Language", "en")
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Cache-Control", "no-cache")

	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("yandex: unexpected status " + resp.Status)
	}

	return io.ReadAll(resp.Body)
}

// signature computes HMAC-SHA256 over data with configured hmac key.
func (c *DirectClient) signature(data []byte) string {
	h := hmac.New(sha256.New, []byte(c.hmacKey))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// secYaHeaders builds Sec-* headers for Vtrans / Vsubs based on session and body.
func (c *DirectClient) secYaHeaders(secType string, sess *clientSession, body []byte, path string) map[string]string {
	token := sess.uuid + ":" + path + ":" + c.componentVersion
	tokenSign := c.signature([]byte(token))

	sign := c.signature(body)

	// Example for Vtrans:
	//
	//	Vtrans-Signature: <sign>
	//	Sec-Vtrans-Sk: <secretKey>
	//	Sec-Vtrans-Token: <tokenSign>:<token>
	return map[string]string{
		secType + "-Signature":      sign,
		"Sec-" + secType + "-Sk":    sess.secretKey,
		"Sec-" + secType + "-Token": tokenSign + ":" + token,
	}
}

// randomUUID32 generates 32 hex digits UUID (same format as getUUID in JS).
func randomUUID32() string {
	const hexDigits = "0123456789ABCDEF"
	b := make([]byte, 32)
	for i := 0; i < 32; i++ {
		// it's not cryptographically secure; it's enough for session UUID
		b[i] = hexDigits[time.Now().UnixNano()%16]
	}
	return string(b)
}
