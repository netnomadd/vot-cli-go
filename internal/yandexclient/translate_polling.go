package yandexclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/netnomadd/vot-cli-go/internal/backend"
	"github.com/netnomadd/vot-cli-go/internal/yandexproto"
)

const defaultVideoDurationSec = 343.0

type pollConfig struct {
	interval          time.Duration
	fixedInterval     bool
	attempts          int
	unlimitedAttempts bool
}

type translateVideoPollOptions struct {
	backendName      string
	getSession       func(ctx context.Context, module string) (*clientSession, error)
	encodeRequest    func(url string, duration float64, reqLang, respLang string, p backend.TranslateParams, firstRequest bool) ([]byte, error)
	secHeaders       func(secType string, sess *clientSession, body []byte, path string) map[string]string
	sendTranslate    func(ctx context.Context, path string, body []byte, headers map[string]string) ([]byte, error)
	beforeTranslate  func(headers map[string]string, p backend.TranslateParams)
	applyAudioFix    func(ctx context.Context, sess *clientSession, videoURL, translationID, fileID string, audio []byte, chunked bool, chunkLen int) error
	resolveAudioData func(ctx context.Context, p backend.TranslateParams, translationID string) (*audioUploadPayload, bool)
}

type audioUploadPayload struct {
	videoURL string
	fileID   string
	audio    []byte
	chunked  bool
	chunkLen int
}

func buildPollConfig(p backend.TranslateParams) pollConfig {
	interval := time.Duration(p.PollIntervalSec) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}

	attempts := p.PollAttempts
	unlimited := attempts == 0
	if attempts < 0 {
		attempts = 10
	}

	return pollConfig{
		interval:          interval,
		fixedInterval:     p.FixedPollInterval,
		attempts:          attempts,
		unlimitedAttempts: unlimited,
	}
}

func translateVideoWithPolling(ctx context.Context, p backend.TranslateParams, opts translateVideoPollOptions) (backend.TranslateResult, error) {
	sess, err := opts.getSession(ctx, "video-translation")
	if err != nil {
		return backend.TranslateResult{}, err
	}

	dur := defaultVideoDurationSec
	if p.DurationSec > 0 {
		dur = p.DurationSec
	}
	retriedWithDefaultDuration := false

	cfg := buildPollConfig(p)
	path := "/video-translation/translate"
	triedAudioRequestedFix := false

	for attempt := 0; cfg.unlimitedAttempts || attempt < cfg.attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return backend.TranslateResult{}, err
		}

		firstRequest := true
		body, err := opts.encodeRequest(p.URL, dur, p.RequestLang, p.ResponseLang, p, firstRequest)
		if err != nil {
			return backend.TranslateResult{}, err
		}

		vtransHeaders := opts.secHeaders("Vtrans", sess, body, path)
		if opts.beforeTranslate != nil {
			opts.beforeTranslate(vtransHeaders, p)
		}

		resBytes, err := opts.sendTranslate(ctx, path, body, vtransHeaders)
		if err != nil {
			if strings.Contains(err.Error(), "400 Bad Request") && p.DurationSec > 0 && !retriedWithDefaultDuration {
				retriedWithDefaultDuration = true
				dur = defaultVideoDurationSec
				if p.Debug {
					fmt.Fprintf(os.Stderr, "[debug] [%s] got 400 with duration=%.1fs; retrying with fallback duration=%.1fs\n", opts.backendName, p.DurationSec, dur)
				}
				attempt--
				continue
			}
			return backend.TranslateResult{}, err
		}

		var resp yandexproto.VideoTranslationResponse
		if err := proto.Unmarshal(resBytes, &resp); err != nil {
			return backend.TranslateResult{}, err
		}

		status := resp.GetStatus()
		remaining := resp.GetRemainingTime()
		translationID := resp.GetTranslationId()
		shouldRetry := resp.GetShouldRetry()
		if p.Debug {
			total := "unlimited"
			if !cfg.unlimitedAttempts {
				total = fmt.Sprintf("%d", cfg.attempts)
			}
			fmt.Fprintf(os.Stderr, "[debug] [%s] poll attempt %d/%s first_request=%v status=%d remaining=%d translation_id=%s should_retry=%d\n",
				opts.backendName, attempt+1, total, firstRequest, status, remaining, translationID, shouldRetry)
		}

		if audioURL := resp.GetUrl(); audioURL != "" {
			return backend.TranslateResult{
				AudioURL:      audioURL,
				Duration:      resp.GetDuration(),
				DetectedLang:  resp.GetLanguage(),
				IsLivelyVoice: resp.GetIsLivelyVoice(),
			}, nil
		}

		msg := resp.GetMessage()
		retry := false
		switch status {
		case 2, 3, 6: // WAITING / LONG_WAITING / AUDIO_REQUESTED
			audioFixURL, canApplyAudioFix := youtubeAudioWorkaroundURL(p.URL)
			if status == 6 && !triedAudioRequestedFix && canApplyAudioFix {
				if translationID == "" {
					msg = fmt.Sprintf("yandex: AUDIO_REQUESTED returned empty translationId%s", backendMsgSuffix(opts.backendName))
					break
				}

				payload := &audioUploadPayload{
					videoURL: audioFixURL,
					fileID:   defaultAudioFileID,
					audio:    []byte{},
				}
				if opts.resolveAudioData != nil {
					if p.Debug {
						fmt.Fprintf(os.Stderr, "[debug] [%s] AUDIO_REQUESTED: preparing real audio payload...\n", opts.backendName)
					}
					start := time.Now()
					if resolved, ok := opts.resolveAudioData(ctx, p, translationID); ok && resolved != nil {
						payload = resolved
					}
					if p.Debug {
						fmt.Fprintf(os.Stderr, "[debug] [%s] AUDIO_REQUESTED: payload preparation took %s\n", opts.backendName, time.Since(start).Round(time.Millisecond))
					}
				}

				if p.Debug {
					mode := "full"
					if payload.chunked {
						mode = "chunked"
					}
					fmt.Fprintf(os.Stderr, "[debug] [%s] AUDIO_REQUESTED: sending fail-audio/audio for translation_id=%s video_url=%s mode=%s file_id=%s audio_bytes=%d\n",
						opts.backendName, translationID, payload.videoURL, mode, payload.fileID, len(payload.audio))
				}
				if err := opts.applyAudioFix(ctx, sess, payload.videoURL, translationID, payload.fileID, payload.audio, payload.chunked, payload.chunkLen); err != nil {
					if payload.chunked || len(payload.audio) > 0 {
						if p.Debug {
							fmt.Fprintf(os.Stderr, "[debug] [%s] AUDIO_REQUESTED: real audio upload failed (%v), falling back to fake audio payload\n", opts.backendName, err)
						}
						fallback := &audioUploadPayload{
							videoURL: audioFixURL,
							fileID:   defaultAudioFileID,
							audio:    []byte{},
						}
						if err2 := opts.applyAudioFix(ctx, sess, fallback.videoURL, translationID, fallback.fileID, fallback.audio, false, 0); err2 == nil {
							triedAudioRequestedFix = true
							attempt--
							continue
						} else {
							return backend.TranslateResult{}, fmt.Errorf("yandex: AUDIO_REQUESTED fix%s failed: real upload error=%v; fallback error=%w", backendMsgSuffix(opts.backendName), err, err2)
						}
					}
					return backend.TranslateResult{}, fmt.Errorf("yandex: AUDIO_REQUESTED fix%s failed: %w", backendMsgSuffix(opts.backendName), err)
				}
				triedAudioRequestedFix = true
				attempt--
				continue
			}
			retry = true
		case 7:
			msg = sessionRequiredMessage(opts.backendName, msg)
		default:
			msg = notReadyMessage(opts.backendName, status, msg, p.DirectURL)
		}

		isLastAttempt := !cfg.unlimitedAttempts && attempt == cfg.attempts-1
		if !retry || isLastAttempt {
			if retry {
				if remaining > 0 {
					msg = fmt.Sprintf("yandex: translation still in progress%s after %d attempts (remaining ~%d seconds)", backendMsgSuffix(opts.backendName), cfg.attempts, remaining)
				} else if msg == "" {
					msg = fmt.Sprintf("yandex: translation still in progress%s after %d attempts", backendMsgSuffix(opts.backendName), cfg.attempts)
				}
			}
			return backend.TranslateResult{}, fmt.Errorf("%s", msg)
		}

		wait := nextPollDelay(cfg.interval, remaining, cfg.fixedInterval)
		if p.Debug {
			mode := "auto"
			if cfg.fixedInterval {
				mode = "fixed"
			}
			fmt.Fprintf(os.Stderr, "[debug] [%s] next_poll_delay=%s (mode=%s base=%s remaining=%d)\n", opts.backendName, wait, mode, cfg.interval, remaining)
		}
		select {
		case <-ctx.Done():
			return backend.TranslateResult{}, ctx.Err()
		case <-time.After(wait):
		}
	}

	return backend.TranslateResult{}, fmt.Errorf("yandex: translation not ready after polling%s", backendMsgSuffix(opts.backendName))
}

func backendMsgSuffix(backendName string) string {
	if backendName == "worker" {
		return " via worker"
	}
	return ""
}

func buildAudioFileID(downloadType string, minChunkSize int, fileSize int) string {
	if minChunkSize <= 0 {
		minChunkSize = 5 * 1024 * 1024
	}
	if fileSize < 0 {
		fileSize = 0
	}
	payload := map[string]interface{}{
		"downloadType": downloadType,
		"itag":         251,
		"minChunkSize": minChunkSize,
		"fileSize":     fmt.Sprintf("%d", fileSize),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return downloadType
	}
	return string(b)
}

func selectAudioDownloadType(videoURL string) string {
	v := strings.ToLower(videoURL)
	if strings.Contains(v, "googlevideo.com") || strings.Contains(v, ".webm") {
		return "web_api_get_all_generating_urls_data_from_iframe"
	}
	return "web_api_video_src"
}

func chooseChunkSize(total int) int {
	const minChunk = 5 * 1024 * 1024
	switch {
	case total > 40*1024*1024:
		return 8 * 1024 * 1024
	case total > 20*1024*1024:
		return 6 * 1024 * 1024
	default:
		return minChunk
	}
}

func resolveDirectURLWithYtDLP(videoURL, cookiesFile, cookiesFromBrowser string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	args := []string{"-g"}
	if cookiesFile != "" {
		args = append(args, "--cookies", cookiesFile)
	}
	if cookiesFromBrowser != "" {
		args = append(args, "--cookies-from-browser", cookiesFromBrowser)
	}
	args = append(args, videoURL)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("yt-dlp failed: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	raw := strings.TrimSpace(string(output))
	if raw == "" {
		return "", fmt.Errorf("yt-dlp returned empty output")
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "://") {
			return line, nil
		}
	}
	return "", fmt.Errorf("yt-dlp did not return a direct URL")
}

func downloadAudioWithYtDLP(ctx context.Context, videoURL, cookiesFile, cookiesFromBrowser string) ([]byte, error) {
	childCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	args := []string{"-f", "bestaudio", "-o", "-", videoURL}
	if cookiesFile != "" {
		args = append([]string{"--cookies", cookiesFile}, args...)
	}
	if cookiesFromBrowser != "" {
		args = append([]string{"--cookies-from-browser", cookiesFromBrowser}, args...)
	}

	cmd := exec.CommandContext(childCtx, "yt-dlp", args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("yt-dlp audio download failed: %w (stderr: %s)", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("yt-dlp audio download returned empty payload")
	}
	return out, nil
}

func sessionRequiredMessage(backendName, msg string) string {
	if backendName == "worker" {
		if msg == "" {
			return "yandex: this video requires an authenticated Yandex session (worker, SESSION_REQUIRED). This usually means the video is restricted to logged-in Yandex Browser users; the CLI cannot bypass this."
		}
		return fmt.Sprintf("yandex: this video requires an authenticated Yandex session (worker, SESSION_REQUIRED): %s", msg)
	}
	if msg == "" {
		return "yandex: this video requires an authenticated Yandex session (SESSION_REQUIRED). This usually means the video is restricted to logged-in Yandex Browser users; the CLI cannot bypass this."
	}
	return fmt.Sprintf("yandex: this video requires an authenticated Yandex session (SESSION_REQUIRED): %s", msg)
}

func notReadyMessage(backendName string, status int32, msg string, directURL bool) string {
	var base string
	switch backendName {
	case "worker":
		if msg == "" {
			base = fmt.Sprintf("yandex: translation not ready or failed via worker (status=%d, empty url)", status)
		} else {
			base = fmt.Sprintf("yandex: translation not ready or failed via worker (status=%d): %s", status, msg)
		}
	default:
		if msg == "" {
			base = fmt.Sprintf("yandex: translation not ready or failed (status=%d, empty url)", status)
		} else {
			base = fmt.Sprintf("yandex: translation not ready or failed (status=%d): %s", status, msg)
		}
	}
	if directURL {
		if backendName == "worker" {
			base += " (unsupported direct URL: this media link may not be accepted by the worker backend; try passing the original page URL or disabling direct-url / yt-dlp direct mode)"
		} else {
			base += " (unsupported direct URL: this media link may not be accepted by Yandex; try passing the original page URL or disabling direct-url / yt-dlp direct mode)"
		}
	}
	return base
}
