package yandexclient

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/netnomadd/vot-cli-go/internal/backend"
	"github.com/netnomadd/vot-cli-go/internal/yandexproto"
)

const defaultVideoDurationSec = 343.0

type pollConfig struct {
	interval          time.Duration
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
	applyAudioFix    func(ctx context.Context, sess *clientSession, videoURL, translationID string) error
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
				if p.Debug {
					fmt.Fprintf(os.Stderr, "[debug] [%s] AUDIO_REQUESTED: sending fail-audio/audio workaround for translation_id=%s video_url=%s\n", opts.backendName, translationID, audioFixURL)
				}
				if err := opts.applyAudioFix(ctx, sess, audioFixURL, translationID); err != nil {
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

		wait := nextPollDelay(cfg.interval, remaining)
		if p.Debug {
			fmt.Fprintf(os.Stderr, "[debug] [%s] next_poll_delay=%s (base=%s remaining=%d)\n", opts.backendName, wait, cfg.interval, remaining)
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
