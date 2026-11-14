package yandexclient

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
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

// ComponentVersion is the YaBrowser build embedded in Sec-*-Token.
func ComponentVersion() string {
	return defaultComponentVersion
}

// UserAgentMatchesComponent reports whether ua names the same YaBrowser
// major.minor as ComponentVersion. A config UA from an older snapshot must
// not be sent together with the current component version.
func UserAgentMatchesComponent(ua string) bool {
	const marker = "YaBrowser/"
	i := strings.Index(ua, marker)
	if i < 0 {
		return false
	}
	rest := ua[i+len(marker):]
	parts := strings.Split(defaultComponentVersion, ".")
	if len(parts) < 2 {
		return false
	}
	return strings.HasPrefix(rest, parts[0]+"."+parts[1]+".")
}

func browserClientHints() map[string]string {
	short := defaultComponentVersion
	if len(short) > 5 {
		short = short[:5]
	}
	return map[string]string{
		"sec-ch-ua":                   fmt.Sprintf(`"Chromium";v="152", "Not?A_Brand";v="24", "YaBrowser";v="%s", "Yowser";v="2.5"`, short),
		"sec-ch-ua-full-version-list": fmt.Sprintf(`"Chromium";v="152.0.7977.%s", "Not?A_Brand";v="24.0.0.0", "YaBrowser";v="%s", "Yowser";v="2.5"`, defaultChromiumRevision, defaultComponentVersion),
		"Sec-Fetch-Mode":              "no-cors",
	}
}

func applyBrowserClientHints(h http.Header) {
	for k, v := range browserClientHints() {
		h.Set(k, v)
	}
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

// SetHTTPProxy configures an HTTP(S) proxy for Yandex API requests.
// An empty value restores the default transport (env proxies still apply).
func (c *DirectClient) SetHTTPProxy(proxyURL string) error {
	return applyHTTPProxy(c.httpClient, proxyURL)
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
	// Pair from vot.js 3.2.2. The UA, componentVersion inside Sec-*-Token, and
	// sec-ch-ua hints must describe the same YaBrowser build.
	defaultUserAgent        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 YaBrowser/26.10.0.0 Safari/537.36"
	defaultComponentVersion = "26.10.0.1854"
	defaultChromiumRevision = "1854"
	defaultHMACKey          = "bt8xH3VOlb4mqf0nqAibnDOoiPlXsisf"
	defaultTimeout = 30 * time.Second
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
	return translateVideoWithPolling(ctx, p, translateVideoPollOptions{
		backendName:   "direct",
		getSession:    c.getSession,
		encodeRequest: c.encodeTranslationRequest,
		secHeaders:    c.secYaHeaders,
		sendTranslate: c.doProtoRequest,
		beforeTranslate: func(headers map[string]string, p backend.TranslateParams) {
			if p.VoiceStyle == backend.VoiceStyleLive && c.apiToken != "" {
				headers["Authorization"] = "OAuth " + c.apiToken
			}
		},
		applyAudioFix: func(ctx context.Context, sess *clientSession, videoURL, translationID, fileID string, audio []byte, chunked bool, chunkLen int) error {
			// fail-audio-js is the empty-audio workaround from vot.js. A real
			// upload must not be marked failed before the bytes are sent.
			if len(audio) == 0 {
				if err := c.requestVtransFailAudio(ctx, videoURL); err != nil {
					return fmt.Errorf("at fail-audio-js: %w", err)
				}
			}
			if err := c.requestVtransAudio(ctx, sess, videoURL, translationID, fileID, audio, chunked, chunkLen); err != nil {
				return fmt.Errorf("at audio upload: %w", err)
			}
			return nil
		},
		resolveAudioData: resolveAudioDataForUpload,
	})
}

func youtubeAudioWorkaroundURL(rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed == nil {
		return "", false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "youtu.be" {
		id := strings.Trim(parsed.Path, "/")
		return "https://youtu.be/" + id, id != ""
	}
	if host == "youtube.com" || host == "www.youtube.com" || host == "m.youtube.com" {
		id := strings.TrimSpace(parsed.Query().Get("v"))
		if id == "" {
			return "", false
		}
		return "https://youtu.be/" + id, true
	}
	return "", false
}

func (c *DirectClient) requestVtransFailAudio(ctx context.Context, videoURL string) error {
	payload := map[string]string{"video_url": videoURL}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resBytes, err := c.doJSONRequest(ctx, http.MethodPut, "/video-translation/fail-audio-js", data)
	if err != nil {
		return err
	}

	var resp struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(resBytes, &resp); err != nil {
		return err
	}
	// Some sessions return 0 when fail-audio was already marked before.
	// Treat it as non-fatal so we can continue with audio upload fallback.
	if resp.Status != 1 && resp.Status != 0 {
		return fmt.Errorf("unexpected fail-audio-js status: %d", resp.Status)
	}
	return nil
}

func (c *DirectClient) requestVtransAudio(ctx context.Context, sess *clientSession, videoURL, translationID, fileID string, audio []byte, chunked bool, chunkLen int) error {
	const maxUploadAttempts = 3

	if chunked && len(audio) > 0 {
		if chunkLen <= 0 {
			chunkLen = chooseChunkSize(len(audio))
		}
		totalParts := (len(audio) + chunkLen - 1) / chunkLen
		for part := 0; part < totalParts; part++ {
			start := part * chunkLen
			end := start + chunkLen
			if end > len(audio) {
				end = len(audio)
			}

			bodyMsg := &yandexproto.VideoTranslationAudioRequest{
				TranslationId: translationID,
				Url:           videoURL,
				PartialAudioInfo: &yandexproto.ChunkAudioObject{
					AudioBuffer: &yandexproto.PartialAudioBufferObject{
						ChunkId:   int32(part),
						AudioFile: audio[start:end],
					},
					AudioPartsLength: int32(totalParts),
					FileId:           fileID,
					Version:          1,
				},
			}
			body, err := proto.Marshal(bodyMsg)
			if err != nil {
				return err
			}
			path := "/video-translation/audio"
			headers := c.secYaHeaders("Vtrans", sess, body, path)
			last := part == totalParts-1
			var uploadErr error
			for uploadAttempt := 1; uploadAttempt <= maxUploadAttempts; uploadAttempt++ {
				resBody, err := c.doProtoRequestWithMethod(ctx, http.MethodPut, path, body, headers)
				if err != nil {
					uploadErr = err
					if uploadAttempt < maxUploadAttempts && isTransientAudioUploadError(err) {
						backoff := time.Duration(uploadAttempt) * time.Second
						select {
						case <-ctx.Done():
							return ctx.Err()
						case <-time.After(backoff):
						}
						continue
					}
					break
				}
				if err := validateAudioUploadResponse(resBody, last); err != nil {
					uploadErr = err
					break
				}
				uploadErr = nil
				break
			}
			if uploadErr != nil {
				return fmt.Errorf("chunk %d/%d upload failed: %w", part+1, totalParts, uploadErr)
			}
		}
		return nil
	}

	bodyMsg := &yandexproto.VideoTranslationAudioRequest{
		TranslationId: translationID,
		Url:           videoURL,
		AudioInfo: &yandexproto.AudioBufferObject{
			AudioFile: audio,
			FileId:    fileID,
		},
	}
	body, err := proto.Marshal(bodyMsg)
	if err != nil {
		return err
	}

	path := "/video-translation/audio"
	headers := c.secYaHeaders("Vtrans", sess, body, path)
	var uploadErr error
	for uploadAttempt := 1; uploadAttempt <= maxUploadAttempts; uploadAttempt++ {
		resBody, err := c.doProtoRequestWithMethod(ctx, http.MethodPut, path, body, headers)
		if err != nil {
			uploadErr = err
			if uploadAttempt < maxUploadAttempts && isTransientAudioUploadError(err) {
				backoff := time.Duration(uploadAttempt) * time.Second
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
				}
				continue
			}
			break
		}
		if len(audio) > 0 {
			if err := validateAudioUploadResponse(resBody, true); err != nil {
				uploadErr = err
				break
			}
		}
		uploadErr = nil
		break
	}
	return uploadErr
}

func resolveAudioDataForUpload(ctx context.Context, p backend.TranslateParams, _ string) (*audioUploadPayload, bool) {
	if !p.AudioRequestedRealUpload || !p.UseYtDLP {
		return nil, false
	}
	audioFixURL, ok := youtubeAudioWorkaroundURL(p.URL)
	if !ok {
		return nil, false
	}
	candidateURL := p.URL
	if p.OriginalURL != "" {
		candidateURL = p.OriginalURL
	}

	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return nil, false
	}
	directURL, err := resolveDirectURLWithYtDLP(candidateURL, p.YtDLPCookies, p.YtDLPCookiesFromBrowser, p.HTTPProxy)
	if err != nil {
		if p.Debug {
			fmt.Fprintf(os.Stderr, "[debug] [direct] AUDIO_REQUESTED: yt-dlp direct URL resolution failed: %v\n", err)
		}
		return nil, false
	}

	audioBytes, err := downloadAudioWithYtDLP(ctx, candidateURL, p.YtDLPCookies, p.YtDLPCookiesFromBrowser, p.HTTPProxy)
	if err != nil {
		if p.Debug {
			fmt.Fprintf(os.Stderr, "[debug] [direct] AUDIO_REQUESTED: yt-dlp audio download failed, using empty payload fallback: %v\n", err)
		}
		return nil, false
	}

	chunkLen := chooseChunkSize(len(audioBytes))
	fileID := buildAudioFileID(selectAudioDownloadType(directURL), yandexMinChunkSize, len(audioBytes))
	return &audioUploadPayload{
		videoURL: audioFixURL,
		fileID:   fileID,
		audio:    audioBytes,
		chunked:  len(audioBytes) > chunkLen,
		chunkLen: chunkLen,
	}, true
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
func (c *DirectClient) encodeTranslationRequest(url string, duration float64, reqLang, respLang string, p backend.TranslateParams, firstRequest bool) ([]byte, error) {
	msg := &yandexproto.VideoTranslationRequest{
		Url:              url,
		FirstRequest:     firstRequest,
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
		TranslationHelp:  buildTranslationHelp(url, p),
	}

	return proto.Marshal(msg)
}

func buildTranslationHelp(videoURL string, p backend.TranslateParams) []*yandexproto.VideoTranslationHelpObject {
	help := make([]*yandexproto.VideoTranslationHelpObject, 0, 2)

	if p.DirectURL && videoURL != "" {
		help = append(help, &yandexproto.VideoTranslationHelpObject{
			Target:    "video_file_url",
			TargetUrl: videoURL,
		})
	}
	if p.SubsURL != "" {
		help = append(help, &yandexproto.VideoTranslationHelpObject{
			Target:    "subtitles_file_url",
			TargetUrl: p.SubsURL,
		})
	}

	return help
}

func nextPollDelay(base time.Duration, remainingSec int32, fixedInterval bool) time.Duration {
	if fixedInterval {
		return base
	}
	// In auto mode rely on backend hint when available.
	if remainingSec <= 0 {
		return base
	}
	return time.Duration(remainingSec) * time.Second
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
	return c.doProtoRequestWithMethod(ctx, http.MethodPost, path, body, extraHeaders)
}

func (c *DirectClient) doProtoRequestWithMethod(ctx context.Context, method, path string, body []byte, extraHeaders map[string]string) ([]byte, error) {
	url := c.schema + "://" + c.host + path

	requestCtx := ctx
	// Audio upload can take much longer than regular API requests, especially
	// in chunked mode with large payloads.
	if method == http.MethodPut && path == "/video-translation/audio" {
		if _, ok := requestCtx.Deadline(); !ok {
			var cancel context.CancelFunc
			requestCtx, cancel = context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
		}
	}

	req, err := http.NewRequestWithContext(requestCtx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/x-protobuf")
	req.Header.Set("Accept-Language", "en")
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Cache-Control", "no-cache")
	applyBrowserClientHints(req.Header)

	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	client := c.httpClient
	if method == http.MethodPut && path == "/video-translation/audio" {
		// Use request context for deadline control instead of the shared 30s
		// client timeout; otherwise large chunk uploads fail prematurely.
		clientCopy := *c.httpClient
		clientCopy.Timeout = 0
		client = &clientCopy
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("yandex: unexpected status %s (HTTP 403 Forbidden). The API refused the request – this may mean your config (token/HMAC/user-agent) is invalid, the video/site is not supported, or access is temporarily blocked.", resp.Status)
		}
		return nil, fmt.Errorf("yandex: unexpected status %s from API", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

func (c *DirectClient) doJSONRequest(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	url := c.schema + "://" + c.host + path
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en")
	req.Header.Set("Content-Type", "application/json")
	applyBrowserClientHints(req.Header)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yandex: unexpected status %s from API", resp.Status)
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
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err == nil {
		return strings.ToUpper(hex.EncodeToString(raw))
	}

	// Fallback path should be practically unreachable, but keeps behaviour
	// deterministic if random source is unavailable.
	const hexDigits = "0123456789ABCDEF"
	out := make([]byte, 32)
	for i := 0; i < 32; i++ {
		out[i] = hexDigits[time.Now().UnixNano()%16]
	}
	return string(out)
}
