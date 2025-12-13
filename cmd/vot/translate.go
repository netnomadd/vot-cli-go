package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/netnomadd/vot-cli-go/internal/backend"
	"github.com/netnomadd/vot-cli-go/internal/config"
	"github.com/netnomadd/vot-cli-go/internal/yandexclient"
)

// messages holds localized user-facing strings.
type messages struct {
	// Root-level help
	UsageRoot           string
	CommandsHeader      string
	CommandTranslate    string
	GlobalOptionsHeader string
	HelpHint            string

	// Translate command
	UsageTranslate       string
	RespLangRequired     string
	InvalidVoiceStyle    string
	PollIntervalTooSmall string
	PollAttemptsInvalid  string
	FailedLoadConfigFmt  string
	UnknownBackendFmt    string
	UnknownCommandFmt    string
	TimeoutErrorFmt      string
	CanceledError        string
	ErrorPrefix          string
}

// getMessages returns localized messages based on --lang flag, VOT_LANG or CLI args.
func getMessages() messages {
	lang := flagLang
	if lang == "" {
		if v := os.Getenv("VOT_LANG"); v != "" {
			lang = v
		}
	}
	// Fallback: parse --lang from os.Args (works even before flags are parsed).
	if lang == "" {
		args := os.Args[1:]
		for i := 0; i < len(args); i++ {
			arg := args[i]
			if strings.HasPrefix(arg, "--lang=") {
				lang = strings.TrimPrefix(arg, "--lang=")
				break
			}
			if arg == "--lang" && i+1 < len(args) {
				lang = args[i+1]
				break
			}
		}
	}

	switch lang {
	case "ru":
		return messages{
			UsageRoot:           "использование: vot [глобальные опции] <команда> [опции] [аргументы...]",
			CommandsHeader:      "команды:",
			CommandTranslate:    "translate   перевод видео и вывод ссылки(ок) на аудио",
			GlobalOptionsHeader: "глобальные опции:",
			HelpHint:            "запустите 'vot <команда> --help' для справки по опциям команды",

			UsageTranslate:       "использование: vot translate [опции] <url> [url2 ...]",
			RespLangRequired:     "--response-lang обязателен",
			InvalidVoiceStyle:    "--voice-style может быть только 'live' или 'tts'",
			PollIntervalTooSmall: "--poll-interval должен быть не менее 30 секунд",
			PollAttemptsInvalid:  "--poll-attempts должен быть положительным числом",
			FailedLoadConfigFmt:  "не удалось загрузить конфиг: %v",
			UnknownBackendFmt:    "неизвестный backend '%s' (ожидается 'direct' или 'worker')",
			UnknownCommandFmt:    "неизвестная команда: %s",
			TimeoutErrorFmt:      "перевод не завершился за %d секунд; попробуйте увеличить --poll-attempts или --poll-interval",
			CanceledError:        "перевод отменён",
			ErrorPrefix:          "ошибка",
		}
	default:
		return messages{
			UsageRoot:           "usage: vot [global options] <command> [options] [args...]",
			CommandsHeader:      "commands:",
			CommandTranslate:    "translate   translate video and print audio URL(s)",
			GlobalOptionsHeader: "global options:",
			HelpHint:            "run 'vot <command> --help' for command-specific options",

			UsageTranslate:       "usage: vot translate [options] <url> [url2 ...]",
			RespLangRequired:     "--response-lang is required",
			InvalidVoiceStyle:    "--voice-style must be 'live' or 'tts'",
			PollIntervalTooSmall: "--poll-interval must be at least 30 seconds",
			PollAttemptsInvalid:  "--poll-attempts must be positive",
			FailedLoadConfigFmt:  "failed to load config: %v",
			UnknownBackendFmt:    "unknown backend '%s' (expected 'direct' or 'worker')",
			UnknownCommandFmt:    "unknown command: %s",
			TimeoutErrorFmt:      "translation timed out after %d seconds; try increasing --poll-attempts or --poll-interval",
			CanceledError:        "translation canceled",
			ErrorPrefix:          "error",
		}
	}
}

// translateMain handles `vot translate` subcommand.
func translateMain(parent *flag.FlagSet, args []string) {
	msg := getMessages()
	fs := flag.NewFlagSet("translate", flag.ExitOnError)

	// Reuse global flags in this subcommand (parsed first)
	fs.AddFlagSet(parent)

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, msg.UsageTranslate)
		fmt.Fprintln(os.Stderr, "\noptions:")
		fs.PrintDefaults()
	}

	var (
		flagReqLang           string
		flagRespLang          string
		flagDirectURL         bool
		flagSubsURL           string
		flagVoiceStyle        string
		flagPollInterval      int
		flagPollAttempts      int
		flagUseYtDLP          bool
		flagYtDLPUseDirectURL bool
	)

	fs.StringVarP(&flagReqLang, "request-lang", "s", "", "source language code (empty = auto)")
	fs.StringVarP(&flagRespLang, "response-lang", "t", "", "target language code (required)")
	fs.BoolVar(&flagDirectURL, "direct-url", false, "treat input URL(s) as direct media URLs (mp4/webm)")
	fs.StringVar(&flagSubsURL, "subs-url", "", "direct subtitles URL to pass as translation help")
	fs.StringVar(&flagVoiceStyle, "voice-style", "live", "voice style: live (default) or tts")
	fs.IntVar(&flagPollInterval, "poll-interval", 30, "polling interval in seconds (min 30)")
	fs.IntVar(&flagPollAttempts, "poll-attempts", 10, "maximum number of polling attempts")
	fs.BoolVar(&flagUseYtDLP, "use-yt-dlp", false, "use yt-dlp (if available) to assist with URL handling")
	fs.BoolVar(&flagYtDLPUseDirectURL, "yt-dlp-use-direct-url", false, "when using yt-dlp, pass its direct media URL to backend instead of original URL")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	urls := fs.Args()
	if len(urls) == 0 {
		fs.Usage()
		os.Exit(1)
	}

	if flagRespLang == "" {
		fmt.Fprintln(os.Stderr, msg.RespLangRequired)
		os.Exit(1)
	}

	if flagVoiceStyle != "live" && flagVoiceStyle != "tts" {
		fmt.Fprintln(os.Stderr, msg.InvalidVoiceStyle)
		os.Exit(1)
	}

	if flagPollInterval < 30 {
		fmt.Fprintln(os.Stderr, msg.PollIntervalTooSmall)
		os.Exit(1)
	}

	if flagPollAttempts <= 0 {
		fmt.Fprintln(os.Stderr, msg.PollAttemptsInvalid)
		os.Exit(1)
	}

	// Load configuration (if any) and initialise direct backend client.
	cfg, cfgPath, err := config.Load(flagConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, msg.FailedLoadConfigFmt+"\n", err)
		os.Exit(1)
	}
	if flagDebug && !flagSilent && cfgPath != "" {
		fmt.Fprintf(os.Stderr, "[debug] using config file: %s\n", cfgPath)
	}

	// Environment variables override config file values when set.
	if ua := os.Getenv("VOT_USER_AGENT"); ua != "" {
		cfg.UserAgent = ua
	}
	if h := os.Getenv("VOT_YANDEX_HMAC_KEY"); h != "" {
		cfg.YandexHMACKey = h
	}
	if t := os.Getenv("VOT_YANDEX_TOKEN"); t != "" {
		cfg.YandexToken = t
	}
	if flagDebug && !flagSilent {
		if os.Getenv("VOT_USER_AGENT") != "" {
			fmt.Fprintln(os.Stderr, "[debug] using User-Agent from VOT_USER_AGENT")
		}
		if os.Getenv("VOT_YANDEX_HMAC_KEY") != "" {
			fmt.Fprintln(os.Stderr, "[debug] using HMAC key from VOT_YANDEX_HMAC_KEY")
		}
		if os.Getenv("VOT_YANDEX_TOKEN") != "" {
			fmt.Fprintln(os.Stderr, "[debug] using API token from VOT_YANDEX_TOKEN")
		}
	}

	// Effective yt-dlp settings: config provides defaults, CLI flags can override.
	useYtDLP := cfg != nil && cfg.UseYtDLP
	ytDLPUseDirectURL := cfg != nil && cfg.YtDLPUseDirectURL

	if f := fs.Lookup("use-yt-dlp"); f != nil && f.Changed {
		useYtDLP = flagUseYtDLP
	}
	if f := fs.Lookup("yt-dlp-use-direct-url"); f != nil && f.Changed {
		ytDLPUseDirectURL = flagYtDLPUseDirectURL
	}

	var client backend.Client
	switch flagBackend {
	case "direct", "":
		c := yandexclient.NewDirectClient()
		if cfg != nil {
			c.SetUserAgent(cfg.UserAgent)
			c.SetHMACKey(cfg.YandexHMACKey)
			c.SetAPIToken(cfg.YandexToken)
		}
		client = c
	case "worker":
		c := yandexclient.NewWorkerClient()
		if cfg != nil {
			c.SetUserAgent(cfg.UserAgent)
			c.SetHMACKey(cfg.YandexHMACKey)
			c.SetAPIToken(cfg.YandexToken)
		}
		client = c
	default:
		fmt.Fprintf(os.Stderr, "%s: "+msg.UnknownBackendFmt+"\n", msg.ErrorPrefix, flagBackend)
		os.Exit(1)
	}

	voiceStyle := backend.VoiceStyleLive
	if flagVoiceStyle == "tts" {
		voiceStyle = backend.VoiceStyleTTS
	}

	exitCode := 0
	for _, u := range urls {
		// Allow enough time for polling: interval * attempts + small overhead.
		pollTimeout := time.Duration(flagPollInterval*flagPollAttempts+30) * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), pollTimeout)

		if flagDebug && !flagSilent {
			fmt.Fprintf(os.Stderr, "[debug] url=%s req_lang=%s resp_lang=%s direct=%v voice_style=%s subs_url=%s poll_interval=%ds poll_attempts=%d use_yt_dlp=%v yt_dlp_use_direct_url=%v\n",
				u, flagReqLang, flagRespLang, flagDirectURL, flagVoiceStyle, flagSubsURL, flagPollInterval, flagPollAttempts, useYtDLP, ytDLPUseDirectURL)
		}

		effectiveURL := u
		effectiveDirect := flagDirectURL

		// Optionally use yt-dlp to resolve a direct media URL.
		if useYtDLP {
			if directURL, err := resolveDirectURLWithYtDLP(u); err != nil {
				if flagDebug && !flagSilent {
					fmt.Fprintf(os.Stderr, "[debug] yt-dlp resolution failed for %s: %v\n", u, err)
				}
			} else {
				if flagDebug && !flagSilent {
					fmt.Fprintf(os.Stderr, "[debug] yt-dlp resolved %s -> %s\n", u, directURL)
				}
				if ytDLPUseDirectURL {
					effectiveURL = directURL
					effectiveDirect = true
				}
			}
		}

		// Regular video translation.
		res, err := client.TranslateVideo(ctx, backend.TranslateParams{
			URL:             effectiveURL,
			RequestLang:     flagReqLang,
			ResponseLang:    flagRespLang,
			DirectURL:       effectiveDirect,
			SubsURL:         flagSubsURL,
			VoiceStyle:      voiceStyle,
			PollIntervalSec: flagPollInterval,
			PollAttempts:    flagPollAttempts,
			Debug:           flagDebug,
		})
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				fmt.Fprintf(os.Stderr, "%s: "+msg.TimeoutErrorFmt+"\n", msg.ErrorPrefix, int(pollTimeout.Seconds()))
			} else if errors.Is(err, context.Canceled) {
				fmt.Fprintf(os.Stderr, "%s: %s\n", msg.ErrorPrefix, msg.CanceledError)
			} else {
				fmt.Fprintf(os.Stderr, "%s: %v\n", msg.ErrorPrefix, err)
			}
			exitCode = 1
			continue
		}

		fmt.Fprintln(os.Stdout, res.AudioURL)
		if flagDebug && !flagSilent {
			detected := res.DetectedLang
			if detected == "" {
				detected = "unknown"
			}
			requested := flagReqLang
			if requested == "" {
				requested = "auto"
			}
			fmt.Fprintf(os.Stderr, "[debug] detected_lang=%s (request=%s) duration=%.1fs lively_voice=%v\n", detected, requested, res.Duration, res.IsLivelyVoice)
		}
	}

	os.Exit(exitCode)
}

// resolveDirectURLWithYtDLP tries to obtain a direct media URL for the given
// video URL using the local yt-dlp binary (if present in PATH).
func resolveDirectURLWithYtDLP(videoURL string) (string, error) {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return "", fmt.Errorf("yt-dlp not found in PATH: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "yt-dlp", "-g", videoURL)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("yt-dlp failed: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	text := strings.TrimSpace(string(output))
	if text == "" {
		return "", fmt.Errorf("yt-dlp returned empty output")
	}

	// yt-dlp -g can return multiple lines; we use the first one.
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = text[:idx]
	}

	return text, nil
}

// getVideoDurationWithYtDLP asks yt-dlp to print video duration (in seconds)
// for the given URL and parses it into float64.
func getVideoDurationWithYtDLP(videoURL string) (float64, error) {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return 0, fmt.Errorf("yt-dlp not found in PATH: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "yt-dlp", "--print", "duration", videoURL)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("yt-dlp failed to get duration: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	line := strings.TrimSpace(string(output))
	if line == "" {
		return 0, fmt.Errorf("yt-dlp returned empty duration")
	}

	// yt-dlp can theoretically print multiple lines; use the first.
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = line[:idx]
	}

	sec, err := strconv.ParseFloat(line, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse yt-dlp duration %q: %w", line, err)
	}

	return sec, nil
}
