package yandexclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/netnomadd/vot-cli-go/internal/backend"
	"github.com/netnomadd/vot-cli-go/internal/proxyutil"
	"github.com/netnomadd/vot-cli-go/internal/yandexproto"
)

const defaultVideoDurationSec = 343.0

// yandexMinChunkSize is config.minChunkSize from vot.js. Partial audio chunks
// must be at least this large, except the final chunk.
const yandexMinChunkSize = 5295308

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
	// Yandex keys the job by the exact URL string. vot.js sends youtu.be for
	// both the translate request and the later audio upload.
	requestURL := p.URL
	if normalized, ok := youtubeAudioWorkaroundURL(p.URL); ok {
		requestURL = normalized
	}
	if p.Debug && requestURL != p.URL {
		fmt.Fprintf(os.Stderr, "[debug] [%s] normalized video url to %s\n", opts.backendName, requestURL)
	}
	gotTranslateResponse := false

	for attempt := 0; cfg.unlimitedAttempts || attempt < cfg.attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return backend.TranslateResult{}, err
		}

		// vot.js defaults this to true on every translate call. Sending false
		// (the proto3 zero value, so the field is omitted) makes Yandex answer
		// HTTP 400 on the poll after an audio upload.
		const firstRequest = true
		body, err := opts.encodeRequest(requestURL, dur, p.RequestLang, p.ResponseLang, p, firstRequest)
		if err != nil {
			return backend.TranslateResult{}, err
		}

		vtransHeaders := opts.secHeaders("Vtrans", sess, body, path)
		if opts.beforeTranslate != nil {
			opts.beforeTranslate(vtransHeaders, p)
		}

		resBytes, err := opts.sendTranslate(ctx, path, body, vtransHeaders)
		if err != nil {
			// Only the opening request is retried with the fallback duration.
			// A 400 after Yandex already accepted the video is a different failure,
			// and replacing a known duration would hide it.
			if strings.Contains(err.Error(), "400 Bad Request") && p.DurationSec > 0 && !retriedWithDefaultDuration && !gotTranslateResponse {
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
		gotTranslateResponse = true

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

		// PART_CONTENT (5) is only the first ~10 minutes. The URL is real, but
		// remainingTime says when the full audio replaces it with FINISHED (1).
		if audioURL := resp.GetUrl(); audioURL != "" && status != 5 {
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
		case 5: // PART_CONTENT
			if p.Debug {
				fmt.Fprintf(os.Stderr, "[debug] [%s] partial audio only (%.0fs); waiting for the full translation\n", opts.backendName, resp.GetDuration())
			}
			retry = true
		case 0, 2, 3, 6: // FAILED / WAITING / LONG_WAITING / AUDIO_REQUESTED
			audioFixURL, canApplyAudioFix := youtubeAudioWorkaroundURL(p.URL)
			// shouldRetry=1 means the previous audio upload failed. A later run
			// with real audio must still upload into that translation id; status
			// stays 0 and never returns to AUDIO_REQUESTED.
			recoverFailedAudio := status == 0 && shouldRetry == 1 && p.AudioRequestedRealUpload
			if status == 2 || status == 3 || ((status == 6 || recoverFailedAudio) && !triedAudioRequestedFix && canApplyAudioFix) {
				if status != 6 && !recoverFailedAudio {
					retry = true
					break
				}
				if recoverFailedAudio && p.Debug {
					fmt.Fprintf(os.Stderr, "[debug] [%s] status=0 should_retry=1; uploading real audio into translation_id=%s\n", opts.backendName, translationID)
				}
				if translationID == "" {
					msg = fmt.Sprintf("yandex: AUDIO_REQUESTED returned empty translationId%s", backendMsgSuffix(opts.backendName))
					break
				}

				payload := &audioUploadPayload{
					videoURL: audioFixURL,
					fileID:   emptyAudioFileID(audioFixURL),
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

				if p.AudioRequestedRealUpload && len(payload.audio) == 0 {
					return backend.TranslateResult{}, fmt.Errorf("yandex: real audio was not downloaded%s", backendMsgSuffix(opts.backendName))
				}
				if p.Debug {
					mode := "full"
					if payload.chunked {
						mode = "chunked"
					}
					action := "audio"
					if len(payload.audio) == 0 {
						action = "fail-audio/audio"
					}
					fmt.Fprintf(os.Stderr, "[debug] [%s] AUDIO_REQUESTED: sending %s for translation_id=%s video_url=%s mode=%s file_id=%s audio_bytes=%d\n",
						opts.backendName, action, translationID, payload.videoURL, mode, payload.fileID, len(payload.audio))
				}
				if err := opts.applyAudioFix(ctx, sess, payload.videoURL, translationID, payload.fileID, payload.audio, payload.chunked, payload.chunkLen); err != nil {
					if payload.chunked || len(payload.audio) > 0 {
						// Some servers reject partial/chunked upload transport-wise.
						// Retry once with the same real audio as a full payload.
						if payload.chunked && len(payload.audio) > 0 {
							if p.Debug {
								fmt.Fprintf(os.Stderr, "[debug] [%s] AUDIO_REQUESTED: chunked upload failed (%v), retrying full real-audio upload\n", opts.backendName, err)
							}
							if errFull := opts.applyAudioFix(ctx, sess, payload.videoURL, translationID, payload.fileID, payload.audio, false, 0); errFull == nil {
								if p.Debug {
									fmt.Fprintf(os.Stderr, "[debug] [%s] AUDIO_REQUESTED: real-audio upload success (mode=full)\n", opts.backendName)
								}
								triedAudioRequestedFix = true
								attempt--
								continue
							} else if p.Debug {
								fmt.Fprintf(os.Stderr, "[debug] [%s] AUDIO_REQUESTED: full real-audio retry failed (%v), falling back to fake audio payload\n", opts.backendName, errFull)
							}
						}
						if p.AudioRequestedRealUpload {
							return backend.TranslateResult{}, fmt.Errorf("yandex: AUDIO_REQUESTED fix%s failed: %w", backendMsgSuffix(opts.backendName), err)
						}
						if p.Debug {
							fmt.Fprintf(os.Stderr, "[debug] [%s] AUDIO_REQUESTED: real audio upload failed (%v), falling back to fake audio payload\n", opts.backendName, err)
						}
						fallback := &audioUploadPayload{
							videoURL: audioFixURL,
							fileID:   emptyAudioFileID(audioFixURL),
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
				if p.Debug && len(payload.audio) > 0 {
					mode := "full"
					if payload.chunked {
						mode = "chunked"
					}
					fmt.Fprintf(os.Stderr, "[debug] [%s] AUDIO_REQUESTED: real-audio upload success (mode=%s)\n", opts.backendName, mode)
				}
				triedAudioRequestedFix = true
				attempt--
				continue
			}
			if status == 0 {
				msg = notReadyMessage(opts.backendName, status, msg, p.DirectURL)
				break
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

func emptyAudioFileID(videoURL string) string {
	id := strings.TrimPrefix(videoURL, "https://youtu.be/")
	if i := strings.IndexAny(id, "?/"); i >= 0 {
		id = id[:i]
	}
	id = strings.Trim(id, "/")
	if id == "" {
		id = "unknown"
	}
	return "fallback-empty-audio:video-translation:" + id
}

func buildAudioFileID(downloadType string, minChunkSize int, fileSize int) string {
	if minChunkSize <= 0 {
		minChunkSize = yandexMinChunkSize
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
	switch {
	case total > 40*1024*1024:
		return 8 * 1024 * 1024
	case total > 20*1024*1024:
		return 6 * 1024 * 1024
	default:
		return yandexMinChunkSize
	}
}

// validateAudioUploadResponse checks the protobuf status Yandex returns after
// a real audio PUT. 1 means more chunks are expected, 2 means the file is complete.
func validateAudioUploadResponse(body []byte, last bool) error {
	var resp yandexproto.VideoTranslationAudioResponse
	if err := proto.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("decode audio upload response: %w", err)
	}
	status := resp.GetStatus()
	if last {
		if status != 2 {
			return fmt.Errorf("audio upload not complete (status=%d, want 2, remaining=%v)", status, resp.GetRemainingChunks())
		}
		return nil
	}
	if status != 1 && status != 2 {
		return fmt.Errorf("unexpected audio upload status %d (remaining=%v)", status, resp.GetRemainingChunks())
	}
	return nil
}

func isTransientAudioUploadError(err error) bool {
	if err == nil {
		return false
	}
	if ne, ok := err.(net.Error); ok {
		return ne.Timeout() || ne.Temporary()
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "internal_error") ||
		strings.Contains(msg, "http/1.x transport connection broken") ||
		strings.Contains(msg, "malformed http response")
}

func resolveDirectURLWithYtDLP(videoURL, cookiesFile, cookiesFromBrowser, proxyURL string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	args := []string{"-g"}
	if cookiesFile != "" {
		args = append(args, "--cookies", cookiesFile)
	}
	if cookiesFromBrowser != "" {
		args = append(args, "--cookies-from-browser", cookiesFromBrowser)
	}
	args = proxyutil.AppendYtDLPArgs(args, proxyURL)
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

func downloadAudioWithYtDLP(ctx context.Context, videoURL, cookiesFile, cookiesFromBrowser, proxyURL string) ([]byte, error) {
	// Give yt-dlp enough time on long videos; a hard 2-minute limit often
	// kills download right before completion and prevents chunked upload.
	timeout := 20 * time.Minute
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline) - time.Minute
		if remaining <= 0 {
			return nil, context.DeadlineExceeded
		}
		if remaining < timeout {
			timeout = remaining
		}
	}
	childCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"-f", "bestaudio", "-o", "-", videoURL}
	if cookiesFile != "" {
		args = append([]string{"--cookies", cookiesFile}, args...)
	}
	if cookiesFromBrowser != "" {
		args = append([]string{"--cookies-from-browser", cookiesFromBrowser}, args...)
	}
	args = proxyutil.AppendYtDLPArgs(args, proxyURL)

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
