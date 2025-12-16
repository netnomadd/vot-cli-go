package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
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
	YtDLPNotFoundFmt     string
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
			YtDLPNotFoundFmt:     "yt-dlp включён (флаг/конфиг), но бинарник не найден в PATH: %v; продолжаю без yt-dlp",
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
			YtDLPNotFoundFmt:     "yt-dlp is enabled (flag/config) but not found in PATH: %v; continuing without yt-dlp",
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
		flagReqLang                 string
		flagRespLang                string
		flagDirectURL               bool
		flagSubsURL                 string
		flagVoiceStyle              string
		flagPollInterval            int
		flagPollAttempts            int
		flagUseYtDLP                bool
		flagYtDLPUseDirectURL       bool
		flagYtDLPCookies            string
		flagYtDLPCookiesFromBrowser string
		flagExplain                 bool
	)

	fs.StringVarP(&flagReqLang, "request-lang", "s", "", "source language code (empty = auto)")
	fs.StringVarP(&flagRespLang, "response-lang", "t", "ru", "target language code (default: ru)")
	fs.BoolVar(&flagDirectURL, "direct-url", false, "treat input URL(s) as direct media URLs (mp4/webm)")
	fs.StringVar(&flagSubsURL, "subs-url", "", "direct subtitles URL to pass as translation help")
	fs.StringVar(&flagVoiceStyle, "voice-style", "live", "voice style: live (default) or tts")
	fs.IntVar(&flagPollInterval, "poll-interval", 30, "polling interval in seconds (min 30)")
	fs.IntVar(&flagPollAttempts, "poll-attempts", 10, "maximum number of polling attempts")
	fs.BoolVar(&flagUseYtDLP, "use-yt-dlp", false, "use yt-dlp (if available) to assist with URL handling")
	fs.BoolVar(&flagYtDLPUseDirectURL, "yt-dlp-use-direct-url", false, "when using yt-dlp, pass its direct media URL to backend instead of original URL")
	fs.StringVar(&flagYtDLPCookies, "yt-dlp-cookies", "", "path to cookies file to pass to yt-dlp (--cookies)")
	fs.StringVar(&flagYtDLPCookiesFromBrowser, "yt-dlp-cookies-from-browser", "", "browser spec to pass to yt-dlp (--cookies-from-browser)")
	fs.BoolVar(&flagExplain, "explain", false, "print how each URL will be handled and exit without contacting backends")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	urls := fs.Args()
	if len(urls) == 0 {
		fs.Usage()
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

	// If config defines a default target language and the user did not
	// explicitly pass --response-lang, prefer the config value over the
	// built-in default "ru".
	if cfg != nil && cfg.DefaultResponseLang != "" {
		if f := fs.Lookup("response-lang"); f != nil && !f.Changed {
			flagRespLang = cfg.DefaultResponseLang
		}
	}

	// Effective yt-dlp settings: config provides defaults, CLI flags can override.
	useYtDLP := cfg != nil && cfg.UseYtDLP
	ytDLPUseDirectURL := cfg != nil && cfg.YtDLPUseDirectURL
	var ytDLPCookies, ytDLPCookiesFromBrowser string
	if cfg != nil {
		ytDLPCookies = cfg.YtDLPCookies
		ytDLPCookiesFromBrowser = cfg.YtDLPCookiesFromBrowser
	}

	useYtDLPFlagChanged := false
	ytDLPDirectFlagChanged := false
	ytDLPCookiesFlagChanged := false
	ytDLPCookiesFromBrowserFlagChanged := false
	reqLangFlagChanged := false
	backendFlagChanged := false
	voiceStyleFlagChanged := false

	if f := fs.Lookup("use-yt-dlp"); f != nil && f.Changed {
		useYtDLP = flagUseYtDLP
		useYtDLPFlagChanged = true
	}
	if f := fs.Lookup("yt-dlp-use-direct-url"); f != nil && f.Changed {
		ytDLPUseDirectURL = flagYtDLPUseDirectURL
		ytDLPDirectFlagChanged = true
	}
	if f := fs.Lookup("yt-dlp-cookies"); f != nil && f.Changed {
		ytDLPCookies = flagYtDLPCookies
		ytDLPCookiesFlagChanged = true
	}
	if f := fs.Lookup("yt-dlp-cookies-from-browser"); f != nil && f.Changed {
		ytDLPCookiesFromBrowser = flagYtDLPCookiesFromBrowser
		ytDLPCookiesFromBrowserFlagChanged = true
	}

	// Check availability of yt-dlp once, so that explicit --use-yt-dlp or
	// config settings do not silently do nothing when the binary is missing.
	ytDLPAvailable := true
	if useYtDLP {
		if _, err := exec.LookPath("yt-dlp"); err != nil {
			ytDLPAvailable = false
			if !flagSilent {
				fmt.Fprintf(os.Stderr, msg.YtDLPNotFoundFmt+"\n", err)
			}
		}
	}
	if f := fs.Lookup("request-lang"); f != nil && f.Changed {
		reqLangFlagChanged = true
	}
	if f := fs.Lookup("backend"); f != nil && f.Changed {
		backendFlagChanged = true
	}
	if f := fs.Lookup("voice-style"); f != nil && f.Changed {
		voiceStyleFlagChanged = true
	}

	// Build effective source rules: start with built-ins and then apply optional
	// overrides/extra rules from config (if any).
	sourceRules := buildSourceRulesFromConfig(cfg)

	// Validate base backend value early. Source rules may override it per-URL
	// when the user did not explicitly set --backend.
	switch flagBackend {
	case "direct", "", "worker":
	default:
		fmt.Fprintf(os.Stderr, "%s: "+msg.UnknownBackendFmt+"\n", msg.ErrorPrefix, flagBackend)
		os.Exit(1)
	}

	// Prepare backend clients; per-URL rules may choose between them.
	directClient := yandexclient.NewDirectClient()
	workerClient := yandexclient.NewWorkerClient()
	if cfg != nil {
		directClient.SetUserAgent(cfg.UserAgent)
		directClient.SetHMACKey(cfg.YandexHMACKey)
		directClient.SetAPIToken(cfg.YandexToken)

		workerClient.SetUserAgent(cfg.UserAgent)
		workerClient.SetHMACKey(cfg.YandexHMACKey)
		workerClient.SetAPIToken(cfg.YandexToken)
	}

	exitCode := 0
	for _, u := range urls {
		// Allow enough time for polling: interval * attempts + small overhead.
		pollTimeout := time.Duration(flagPollInterval*flagPollAttempts+30) * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), pollTimeout)

		effectiveURL := u
		effectiveDirect := flagDirectURL
		effectiveReqLang := flagReqLang
		effectiveBackend := flagBackend
		effectiveVoiceStyle := flagVoiceStyle
		var durationSec float64

		// Adjust yt-dlp behaviour for this URL based on source-specific rules,
		// unless the user explicitly overrode the corresponding flags.
		useYtDLPForURL, ytDLPUseDirectURLForURL := applySourceRules(u, sourceRules, useYtDLP && ytDLPAvailable, ytDLPUseDirectURL, useYtDLPFlagChanged, ytDLPDirectFlagChanged)
		effectiveReqLang, effectiveBackend = applySourceLangAndBackend(u, sourceRules, effectiveReqLang, effectiveBackend, reqLangFlagChanged, backendFlagChanged)
		effectiveVoiceStyle = applySourceVoiceStyle(u, sourceRules, effectiveVoiceStyle, voiceStyleFlagChanged)
		cookies, cookiesFromBrowser := applySourceYtDLPCookies(u, sourceRules, ytDLPCookies, ytDLPCookiesFromBrowser, ytDLPCookiesFlagChanged, ytDLPCookiesFromBrowserFlagChanged)

		// Optionally use yt-dlp to resolve a direct media URL and obtain duration.
		if useYtDLPForURL {
			if d, err := getVideoDurationWithYtDLP(u, cookies, cookiesFromBrowser); err != nil {
				if flagDebug && !flagSilent {
					fmt.Fprintf(os.Stderr, "[debug] yt-dlp failed to get duration for %s: %v\n", u, err)
					fmt.Fprintln(os.Stderr, "[debug] hint: yt-dlp errors for YouTube often mean that authentication cookies are required; see https://github.com/yt-dlp/yt-dlp/wiki/FAQ#how-do-i-pass-cookies-to-yt-dlp and the \"Проблемы с yt-dlp\" section in the vot README.")
				}
			} else {
				durationSec = d
				if flagDebug && !flagSilent {
					fmt.Fprintf(os.Stderr, "[debug] yt-dlp duration for %s: %.1fs\n", u, d)
				}
			}

			if directURL, err := resolveDirectURLWithYtDLP(u, cookies, cookiesFromBrowser); err != nil {
				if flagDebug && !flagSilent {
					fmt.Fprintf(os.Stderr, "[debug] yt-dlp resolution failed for %s: %v\n", u, err)
					fmt.Fprintln(os.Stderr, "[debug] hint: yt-dlp errors for YouTube often mean that authentication cookies are required; see https://github.com/yt-dlp/yt-dlp/wiki/FAQ#how-do-i-pass-cookies-to-yt-dlp and the \"Проблемы с yt-dlp\" section in the vot README.")
				}
			} else {
				if flagDebug && !flagSilent {
					fmt.Fprintf(os.Stderr, "[debug] yt-dlp resolved %s -> %s\n", u, directURL)
				}
				if ytDLPUseDirectURLForURL {
					effectiveURL = directURL
					effectiveDirect = true
				}
			}
		}

		sourceKind := classifySource(effectiveURL)

		directForBackend := effectiveDirect
		if sourceKind == "direct_media" {
			directForBackend = true
		}

		// Choose backend client for this URL, allowing per-source rules to
		// redirect between direct and worker when the user did not override it.
		var client backend.Client
		switch effectiveBackend {
		case "", "direct":
			client = directClient
		case "worker":
			client = workerClient
		default:
			fmt.Fprintf(os.Stderr, "%s: "+msg.UnknownBackendFmt+"\n", msg.ErrorPrefix, effectiveBackend)
			cancel()
			exitCode = 1
			continue
		}

		if flagDebug && !flagSilent {
			fmt.Fprintf(os.Stderr, "[debug] url=%s effective_url=%s source=%s backend=%s req_lang=%s resp_lang=%s direct=%v voice_style=%s subs_url=%s poll_interval=%ds poll_attempts=%d use_yt_dlp=%v yt_dlp_use_direct_url=%v yt_dlp_cookies=%s yt_dlp_cookies_from_browser=%s\n",
				u, effectiveURL, sourceKind, effectiveBackend, effectiveReqLang, flagRespLang, directForBackend, effectiveVoiceStyle, flagSubsURL, flagPollInterval, flagPollAttempts, useYtDLPForURL, ytDLPUseDirectURLForURL, cookies, cookiesFromBrowser)
		}

		// In explain mode we only show how the URL would be processed and skip
		// any calls to Yandex backends.
		if flagExplain {
			if !flagSilent {
				fmt.Fprintf(os.Stderr, "[explain] url=%s effective_url=%s source=%s backend=%s req_lang=%s resp_lang=%s direct=%v voice_style=%s subs_url=%s poll_interval=%ds poll_attempts=%d use_yt_dlp=%v yt_dlp_use_direct_url=%v yt_dlp_cookies=%s yt_dlp_cookies_from_browser=%s\n",
					u, effectiveURL, sourceKind, effectiveBackend, effectiveReqLang, flagRespLang, directForBackend, effectiveVoiceStyle, flagSubsURL, flagPollInterval, flagPollAttempts, useYtDLPForURL, ytDLPUseDirectURLForURL, cookies, cookiesFromBrowser)
			}
			cancel()
			continue
		}

		// Map effective voice style string to backend enum; on invalid value from
		// rules, fall back to live and optionally log in debug mode.
		voiceStyle := backend.VoiceStyleLive
		switch effectiveVoiceStyle {
		case "live", "":
			voiceStyle = backend.VoiceStyleLive
		case "tts":
			voiceStyle = backend.VoiceStyleTTS
		default:
			if flagDebug && !flagSilent {
				fmt.Fprintf(os.Stderr, "[debug] ignoring invalid voice_style %q from source rules for %s; falling back to live\n", effectiveVoiceStyle, u)
			}
			voiceStyle = backend.VoiceStyleLive
		}

		// Regular video translation.
		res, err := client.TranslateVideo(ctx, backend.TranslateParams{
			URL:             effectiveURL,
			RequestLang:     effectiveReqLang,
			ResponseLang:    flagRespLang,
			DirectURL:       directForBackend,
			SubsURL:         flagSubsURL,
			VoiceStyle:      voiceStyle,
			DurationSec:     durationSec,
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
				fmt.Fprintf(os.Stderr, "%s: %s: %v\n", msg.ErrorPrefix, u, err)
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
			requested := effectiveReqLang
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
func resolveDirectURLWithYtDLP(videoURL, cookiesFile, cookiesFromBrowser string) (string, error) {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return "", fmt.Errorf("yt-dlp not found in PATH: %w", err)
	}

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

	// yt-dlp -g can return multiple lines and warnings; pick the first
	// line that looks like a URL (contains "://").
	lines := strings.Split(raw, "\n")
	var url string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "://") {
			url = line
			break
		}
	}

	if url == "" {
		return "", fmt.Errorf("yt-dlp did not return a direct URL (output: %s)", raw)
	}

	return url, nil
}

// getVideoDurationWithYtDLP asks yt-dlp to print video duration (in seconds)
// for the given URL and parses it into float64.
func getVideoDurationWithYtDLP(videoURL, cookiesFile, cookiesFromBrowser string) (float64, error) {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return 0, fmt.Errorf("yt-dlp not found in PATH: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	args := []string{"--print", "duration"}
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
		return 0, fmt.Errorf("yt-dlp failed to get duration: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	raw := strings.TrimSpace(string(output))
	if raw == "" {
		return 0, fmt.Errorf("yt-dlp returned empty duration")
	}

	// yt-dlp can emit warnings plus the duration; search from the end
	// for the first line that parses as a float.
	lines := strings.Split(raw, "\n")
	var value string
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if _, err := strconv.ParseFloat(line, 64); err == nil {
			value = line
			break
		}
	}

	if value == "" {
		return 0, fmt.Errorf("yt-dlp did not return a numeric duration (output: %s)", raw)
	}

	sec, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse yt-dlp duration %q: %w", value, err)
	}

	return sec, nil
}

// classifySource returns a coarse-grained classification of the given URL
// (YouTube/Invidious page, direct media, other page, unknown).
func classifySource(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return "unknown"
	}

	host := strings.ToLower(u.Host)
	path := strings.ToLower(u.Path)

	if isDirectMediaPath(path) {
		return "direct_media"
	}

	if strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be") {
		return "youtube_page"
	}
	if strings.Contains(host, "invidious") || strings.Contains(host, "piped") {
		return "invidious_page"
	}

	if host != "" {
		return "page"
	}
	return "unknown"
}

// isDirectMediaPath reports whether the path looks like a direct media resource
// (mp4/webm/audio/m3u8/etc.), as opposed to an HTML watch page.
func isDirectMediaPath(p string) bool {
	p = strings.ToLower(p)
	switch {
	case strings.HasSuffix(p, ".mp4"),
		strings.HasSuffix(p, ".webm"),
		strings.HasSuffix(p, ".m4a"),
		strings.HasSuffix(p, ".m4v"),
		strings.HasSuffix(p, ".mp3"),
		strings.HasSuffix(p, ".aac"),
		strings.HasSuffix(p, ".ogg"),
		strings.HasSuffix(p, ".opus"),
		strings.HasSuffix(p, ".ts"),
		strings.HasSuffix(p, ".m3u8"):
		return true
	default:
		return false
	}
}

