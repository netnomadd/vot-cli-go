//go:build !no_worker

package yandexclient

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// WorkerClient implements backend.Client and talks to the VOT worker backend
// (e.g. vot-worker.toil.cc), which in turn proxies requests to Yandex.
//
// The protobuf payloads and headers are the same as in DirectClient, but
// wrapped into a JSON structure understood by the worker service.
type WorkerClient struct {
	httpClient *http.Client

	// worker endpoint
	schema string
	host   string

	// yandex crypto/config reused for header generation
	userAgent        string
	hmacKey          string
	componentVersion string

	// session cache per module (e.g. "video-translation")
	sessions map[string]*clientSession
}

// NewWorkerClient creates a WorkerClient with sane defaults.
func NewWorkerClient() *WorkerClient {
	return &WorkerClient{
		httpClient:       &http.Client{Timeout: defaultTimeout},
		schema:           defaultWorkerSchema,
		host:             defaultWorkerHost,
		userAgent:        defaultUserAgent,
		hmacKey:          defaultHMACKey,
		componentVersion: defaultComponentVersion,
		sessions:         make(map[string]*clientSession),
	}
}

func WorkerBackendAvailable() bool {
	return true
}

// SetWorkerURL overrides the default worker endpoint if non-empty.
func (c *WorkerClient) SetWorkerURL(raw string) error {
	schema, host, err := NormalizeWorkerURL(raw)
	if err != nil {
		return err
	}
	c.schema = schema
	c.host = host
	return nil
}

// SetUserAgent overrides the default User-Agent if non-empty.
func (c *WorkerClient) SetUserAgent(ua string) {
	if ua != "" {
		c.userAgent = ua
	}
}

// SetHMACKey overrides the default HMAC key if non-empty.
func (c *WorkerClient) SetHMACKey(key string) {
	if key != "" {
		c.hmacKey = key
	}
}

// SetAPIToken is accepted for API parity with DirectClient but is not
// currently used by the worker backend (OAuth happens server-side).
func (c *WorkerClient) SetAPIToken(token string) {
	_ = token
}

// TranslateVideo mirrors DirectClient.TranslateVideo but uses the worker
// transport instead of talking to Yandex directly.
func (c *WorkerClient) TranslateVideo(ctx context.Context, p backend.TranslateParams) (backend.TranslateResult, error) {
	// Ensure session (per worker)
	sess, err := c.getSession(ctx, "video-translation")
	if err != nil {
		return backend.TranslateResult{}, err
	}

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

	path := "/video-translation/translate"

	for attempt := 0; attempt < attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return backend.TranslateResult{}, err
		}

		body, err := c.encodeTranslationRequest(p.URL, dur, p.RequestLang, p.ResponseLang, p)
		if err != nil {
			return backend.TranslateResult{}, err
		}

		vtransHeaders := c.secYaHeaders("Vtrans", sess, body, path)

		resBytes, err := c.doWorkerProtoRequest(ctx, path, body, vtransHeaders)
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
			fmt.Fprintf(os.Stderr, "[debug] [worker] poll attempt %d/%d status=%d remaining=%d\n", attempt+1, attempts, status, remaining)
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

		retry := false
		switch status {
		case 2, 3, 6: // WAITING / LONG_WAITING / AUDIO_REQUESTED
			retry = true
		case 7: // SESSION_REQUIRED – requires authenticated Yandex account / browser session
			if msg == "" {
				msg = "yandex: this video requires an authenticated Yandex session (worker, SESSION_REQUIRED). This usually means the video is restricted to logged-in Yandex Browser users; the CLI cannot bypass this."
			} else {
				msg = fmt.Sprintf("yandex: this video requires an authenticated Yandex session (worker, SESSION_REQUIRED): %s", msg)
			}
		default:
			base := ""
			if msg == "" {
				base = fmt.Sprintf("yandex: translation not ready or failed via worker (status=%d, empty url)", status)
			} else {
				base = fmt.Sprintf("yandex: translation not ready or failed via worker (status=%d): %s", status, msg)
			}
			if p.DirectURL {
				base += " (unsupported direct URL: this media link may not be accepted by the worker backend; try passing the original page URL or disabling direct-url / yt-dlp direct mode)"
			}
			msg = base
		}

		if !retry || attempt == attempts-1 {
			if retry {
				if remaining > 0 {
					msg = fmt.Sprintf("yandex: translation still in progress via worker after %d attempts (remaining ~%d seconds)", attempts, remaining)
				} else if msg == "" {
					msg = fmt.Sprintf("yandex: translation still in progress via worker after %d attempts", attempts)
				}
			}
			return backend.TranslateResult{}, errors.New(msg)
		}

		select {
		case <-ctx.Done():
			return backend.TranslateResult{}, ctx.Err()
		case <-time.After(interval):
		}
	}

	return backend.TranslateResult{}, errors.New("yandex: translation not ready after polling (worker)")
}

// TranslateStream mirrors DirectClient.TranslateStream but uses the worker
// transport.
func (c *WorkerClient) TranslateStream(ctx context.Context, p backend.StreamParams) (backend.StreamResult, error) {
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

		resBytes, err := c.doWorkerProtoRequest(ctx, path, body, headers)
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
				return backend.StreamResult{}, errors.New("yandex: empty stream url in STREAMING response (worker)")
			}
			return backend.StreamResult{StreamURL: info.GetUrl()}, nil
		case yandexproto.StreamInterval_NO_CONNECTION:
			return backend.StreamResult{}, errors.New("yandex: no connection for stream translation (worker)")
		case yandexproto.StreamInterval_TRANSLATING:
			select {
			case <-ctx.Done():
				return backend.StreamResult{}, ctx.Err()
			case <-time.After(pollDelaySeconds * time.Second):
			}
		default:
			return backend.StreamResult{}, errors.New("yandex: unknown stream interval (worker)")
		}
	}

	return backend.StreamResult{}, errors.New("yandex: stream translation not ready after polling (worker)")
}

// encodeTranslationRequest is a copy of DirectClient.encodeTranslationRequest
// but bound to WorkerClient fields.
func (c *WorkerClient) encodeTranslationRequest(url string, duration float64, reqLang, respLang string, p backend.TranslateParams) ([]byte, error) {
	msg := &yandexproto.VideoTranslationRequest{
		Url:              url,
		FirstRequest:     true,
		Duration:         duration,
		Unknown0:         1,
		Language:         reqLang,
		ForceSourceLang:  reqLang != "",
		Unknown1:         0,
		WasStream:        p.WasStream,
		ResponseLanguage: respLang,
		Unknown2:         1,
		Unknown3:         2,
		BypassCache:      p.BypassCache,
		UseLivelyVoice:   p.VoiceStyle == backend.VoiceStyleLive,
		VideoTitle:       p.VideoTitle,
	}
	return proto.Marshal(msg)
}

// getSession returns a cached or newly created session for the given module
// using the worker transport.
func (c *WorkerClient) getSession(ctx context.Context, module string) (*clientSession, error) {
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

func (c *WorkerClient) createSession(ctx context.Context, module string) (*clientSession, error) {
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

	resBytes, err := c.doWorkerProtoRequest(ctx, "/session/create", body, headers)
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

// doWorkerProtoRequest sends a protobuf payload to the worker backend, wrapped
// into a JSON envelope with headers and body bytes, and returns raw response
// bytes (protobuf-encoded).
func (c *WorkerClient) doWorkerProtoRequest(ctx context.Context, path string, body []byte, extraHeaders map[string]string) ([]byte, error) {
	url := c.schema + "://" + c.host + path

	headers := map[string]string{
		"User-Agent":      c.userAgent,
		"Accept":          "application/x-protobuf",
		"Accept-Language": "en",
		"Content-Type":    "application/x-protobuf",
		"Pragma":          "no-cache",
		"Cache-Control":   "no-cache",
	}
	for k, v := range extraHeaders {
		headers[k] = v
	}

	// Convert body bytes to []int so that JSON matches Array.from(body) shape.
	ints := make([]int, len(body))
	for i, b := range body {
		ints[i] = int(b)
	}

	payload := struct {
		Headers map[string]string `json:"headers"`
		Body    []int             `json:"body"`
	}{
		Headers: headers,
		Body:    ints,
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("worker: unexpected status %s (HTTP 403 Forbidden). The backend refused the request – this may mean your config (token/HMAC/user-agent) is invalid, the video/site is not supported, or access is temporarily blocked.", resp.Status)
		}
		return nil, fmt.Errorf("worker: unexpected status %s from backend", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

// signature computes HMAC-SHA256 over data with configured hmac key.
func (c *WorkerClient) signature(data []byte) string {
	h := hmac.New(sha256.New, []byte(c.hmacKey))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// secYaHeaders builds Sec-* headers for Vtrans/Vsubs based on session and body
// for the worker transport (same format as direct Yandex API).
func (c *WorkerClient) secYaHeaders(secType string, sess *clientSession, body []byte, path string) map[string]string {
	token := sess.uuid + ":" + path + ":" + c.componentVersion
	tokenSign := c.signature([]byte(token))

	sign := c.signature(body)

	return map[string]string{
		secType + "-Signature":      sign,
		"Sec-" + secType + "-Sk":    sess.secretKey,
		"Sec-" + secType + "-Token": tokenSign + ":" + token,
	}
}
