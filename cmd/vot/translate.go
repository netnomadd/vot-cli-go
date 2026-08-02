package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/netnomadd/vot-cli-go/internal/backend"
	"github.com/netnomadd/vot-cli-go/internal/config"
	"github.com/netnomadd/vot-cli-go/internal/proxyutil"
	"github.com/netnomadd/vot-cli-go/internal/yandexclient"
)

// messages holds localized user-facing strings.
type messages struct {
	// Root-level help
	UsageRoot            string
	CommandsHeader       string
	CommandTranslate     string
	CommandDoctor        string
	GlobalOptionsHeader  string
	GlobalOptionsConfig  string
	GlobalOptionsSilent  string
	GlobalOptionsDebug   string
	GlobalOptionsLang    string
	GlobalOptionsBackend string
	GlobalOptionsJSON    string
	GlobalOptionsProxy   string
	HelpHint             string

	// Translate command
	UsageTranslate                   string
	TranslateOptionsHeader           string
	FlagReqLangHelp                  string
	FlagRespLangHelp                 string
	FlagDirectURLHelp                string
	FlagSubsURLHelp                  string
	FlagVoiceStyleHelp               string
	FlagPollIntervalHelp             string
	FlagPollAttemptsHelp             string
	FlagMaxWaitHelp                  string
	FlagUseYtDLPHelp                 string
	FlagYtDLPUseDirectURLHelp        string
	FlagYtDLPCookiesHelp             string
	FlagYtDLPCookiesFromBrowserHelp  string
	FlagExplainHelp                  string
	FlagAllowLongVideoHelp           string
	FlagAudioRequestedRealUploadHelp string
	FlagOutdirHelp                   string
	FlagOutfileHelp                  string
	FlagNoTitleHelp                  string
	FlagKeepURLHelp                  string
	FlagProxyYtDLPHelp               string
	RespLangRequired                 string
	InvalidVoiceStyle                string
	PollIntervalTooSmall             string
	PollAttemptsInvalid              string
	OutfileNeedsSingleURL            string
	OutfileMustBeBareName            string
	InvalidProxyFmt                  string
	FailedLoadConfigFmt              string
	InvalidWorkerURLFmt              string
	UnknownBackendFmt                string
	UnknownCommandFmt                string
	TimeoutErrorFmt                  string
	CanceledError                    string
	ErrorPrefix                      string
	DownloadFailedFmt                string
	YtDLPNotFoundFmt                 string
	YtDLPAuthHint                    string
	YtDLPFailureFmt                  string
	SessionRequiredHelp              string
	UnsupportedDirectURLHelp         string
	VideoTooLongFmt                  string

	// Doctor command
	UsageDoctor               string
	DoctorChecksHeader        string
	DoctorConfigPathLabel     string
	DoctorConfigStatusOK      string
	DoctorConfigStatusMissing string
	DoctorNoConfigPath        string
	DoctorEnvHeader           string
	DoctorFieldSet            string
	DoctorFieldMissing        string
	DoctorWorkerHeader        string
	DoctorWorkerBuiltIn       string
	DoctorWorkerDisabled      string
	DoctorWorkerURLLabel      string
	DoctorWorkerURLInvalidFmt string
	DoctorYtDLPHeader         string
	DoctorYtDLPFound          string
	DoctorYtDLPNotFoundFmt    string
	DoctorSummaryOK           string
}

const maxVideoDurationSec = 4 * 60 * 60

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
			UsageRoot:            "использование: vot [глобальные опции] <команда> [опции] [аргументы...]",
			CommandsHeader:       "команды:",
			CommandTranslate:     "translate   перевод видео: URL аудио и/или скачивание файла",
			CommandDoctor:        "doctor      проверка конфигурации и окружения",
			GlobalOptionsHeader:  "глобальные опции:",
			GlobalOptionsConfig:  "путь к файлу конфигурации",
			GlobalOptionsSilent:  "тихий режим: только результат в stdout (URL или путь), ошибки в stderr",
			GlobalOptionsDebug:   "включить отладочный вывод",
			GlobalOptionsLang:    "язык сообщений CLI (например, ru, en)",
			GlobalOptionsBackend: "backend для перевода: direct или worker",
			GlobalOptionsJSON:    "вывести итог в stdout как JSON (без прогресса; перекрывает --silent)",
			GlobalOptionsProxy:   "HTTP(S) прокси: [http://][user:pass@]host:port (Yandex/worker, скачивание; yt-dlp — см. --proxy-yt-dlp)",
			HelpHint:             "запустите 'vot <команда> --help' для справки по опциям команды",

			UsageTranslate:                   "использование: vot translate [опции] <url> [url2 ...]",
			TranslateOptionsHeader:           "опции:",
			FlagReqLangHelp:                  "код языка оригинала (пусто = автоопределение)",
			FlagRespLangHelp:                 "код языка перевода (по умолчанию: ru)",
			FlagDirectURLHelp:                "считать входные URL прямыми ссылками на медиа (mp4/webm и т.п.)",
			FlagSubsURLHelp:                  "прямая ссылка на субтитры, которые помогут при переводе",
			FlagVoiceStyleHelp:               "стиль голоса: live (по умолчанию) или tts",
			FlagPollIntervalHelp:             "интервал опроса в секундах: 0 = auto-режим по remainingTime, >=30 = фиксированный интервал",
			FlagPollAttemptsHelp:             "максимальное число попыток опроса (0 = без лимита, до --max-wait; по умолчанию 0)",
			FlagMaxWaitHelp:                  "максимальное время ожидания (duration, например 30m, 1h; по умолчанию 30m)",
			FlagUseYtDLPHelp:                 "использовать yt-dlp (если установлен) для работы с URL",
			FlagYtDLPUseDirectURLHelp:        "при использовании yt-dlp передавать на backend прямой медиа‑URL вместо исходной страницы",
			FlagYtDLPCookiesHelp:             "путь к файлу cookies, передаваемому в yt-dlp (--cookies)",
			FlagYtDLPCookiesFromBrowserHelp:  "браузер для передачи cookies в yt-dlp (--cookies-from-browser)",
			FlagExplainHelp:                  "показать, как будет обработан каждый URL, и выйти, не обращаясь к backend'ам",
			FlagAllowLongVideoHelp:           "разрешить видео длиннее 4 часов (отключить локальную pre-check проверку лимита длительности)",
			FlagAudioRequestedRealUploadHelp: "при AUDIO_REQUESTED пытаться загрузить реальное аудио (иначе поведение как в vot.js: только fake/empty audio)",
			FlagOutdirHelp:                   "каталог для сохранения переведённого аудио (без флага — только URL)",
			FlagOutfileHelp:                  "имя выходного файла (только для одного URL; иначе videoId/title.mp3)",
			FlagNoTitleHelp:                  "в имени файла использовать videoId/hash, не title из yt-dlp",
			FlagKeepURLHelp:                  "при скачивании дополнительно печатать URL (plain: path<TAB>url; в --json оба поля всегда)",
			FlagProxyYtDLPHelp:               "передавать --proxy в yt-dlp (по умолчанию включено, если задан прокси)",
			RespLangRequired:                 "--response-lang обязателен",
			InvalidVoiceStyle:                "--voice-style может быть только 'live' или 'tts'",
			PollIntervalTooSmall:             "--poll-interval должен быть 0 (auto) или не менее 30 секунд",
			PollAttemptsInvalid:              "--poll-attempts должен быть неотрицательным числом",
			OutfileNeedsSingleURL:            "--outfile можно использовать только с одним URL",
			OutfileMustBeBareName:            "--outfile должен быть именем файла без пути",
			InvalidProxyFmt:                  "некорректный --proxy %q: %v",
			FailedLoadConfigFmt:              "не удалось загрузить конфиг: %v",
			InvalidWorkerURLFmt:              "некорректный worker URL %q: %v",
			UnknownBackendFmt:                "неизвестный backend '%s' (ожидается 'direct' или 'worker')",
			UnknownCommandFmt:                "неизвестная команда: %s",
			TimeoutErrorFmt:                  "перевод не завершился за %d секунд; попробуйте увеличить --max-wait (и при необходимости --poll-attempts/--poll-interval)",
			CanceledError:                    "перевод отменён",
			ErrorPrefix:                      "ошибка",
			DownloadFailedFmt:                "не удалось скачать аудио для %s: %v",
			YtDLPNotFoundFmt:                 "yt-dlp включён (флаг/конфиг), но бинарник не найден в PATH: %v; продолжаю без yt-dlp",
			YtDLPAuthHint:                    "подсказка: ошибки yt-dlp для YouTube часто означают, что нужны cookies авторизации; см. https://github.com/yt-dlp/yt-dlp/wiki/FAQ#how-do-i-pass-cookies-to-yt-dlp и раздел \"Проблемы с yt-dlp\" в README vot.",
			YtDLPFailureFmt:                  "yt-dlp не смог обработать этот URL %s; продолжаю без yt-dlp.",
			SessionRequiredHelp:              "Подсказка: SESSION_REQUIRED обычно означает, что видео доступно только авторизованным пользователям Яндекс-Браузера; CLI не может обойти это ограничение.",
			UnsupportedDirectURLHelp:         "Подсказка: backend не принимает этот прямой URL; попробуйте передать исходный URL страницы или отключить режим прямых ссылок / yt-dlp direct.",
			VideoTooLongFmt:                  "видео слишком длинное: %.0f секунд (лимит %d секунд / 4 часа)",

			UsageDoctor:               "использование: vot doctor [глобальные опции]",
			DoctorChecksHeader:        "проверки:",
			DoctorConfigPathLabel:     "конфиг:",
			DoctorConfigStatusOK:      "  статус: найден и успешно разобран",
			DoctorConfigStatusMissing: "  статус: файл не найден (используются встроенные значения по умолчанию)",
			DoctorNoConfigPath:        "конфиг: для этой ОС нет стандартного пути; используйте --config",
			DoctorEnvHeader:           "переменные окружения:",
			DoctorFieldSet:            "задано",
			DoctorFieldMissing:        "не задано",
			DoctorWorkerHeader:        "worker backend:",
			DoctorWorkerBuiltIn:       "  поддержка worker: включена в эту сборку",
			DoctorWorkerDisabled:      "  поддержка worker: отключена в этой сборке",
			DoctorWorkerURLLabel:      "  endpoint:",
			DoctorWorkerURLInvalidFmt: "  endpoint: некорректный URL %q: %v",
			DoctorYtDLPHeader:         "yt-dlp:",
			DoctorYtDLPFound:          "  бинарник: найден в PATH",
			DoctorYtDLPNotFoundFmt:    "  бинарник: не найден в PATH: %v",
			DoctorSummaryOK:           "проверки завершены; если проблемы сохраняются, запустите 'vot translate --debug ...' и изучите вывод.",
		}
	default:
		return messages{
			UsageRoot:            "usage: vot [global options] <command> [options] [args...]",
			CommandsHeader:       "commands:",
			CommandTranslate:     "translate   translate video: print audio URL(s) and/or download file(s)",
			CommandDoctor:        "doctor      run configuration and environment diagnostics",
			GlobalOptionsHeader:  "global options:",
			GlobalOptionsConfig:  "path to config file",
			GlobalOptionsSilent:  "silent mode: only result (URL or path) to stdout, errors to stderr",
			GlobalOptionsDebug:   "enable debug logging",
			GlobalOptionsLang:    "UI language (e.g. ru, en)",
			GlobalOptionsBackend: "backend to use: direct or worker",
			GlobalOptionsJSON:    "write final result to stdout as JSON (no progress; overrides --silent)",
			GlobalOptionsProxy:   "HTTP(S) proxy: [http://][user:pass@]host:port (Yandex/worker, download; yt-dlp via --proxy-yt-dlp)",
			HelpHint:             "run 'vot <command> --help' for command-specific options",

			UsageTranslate:                   "usage: vot translate [options] <url> [url2 ...]",
			TranslateOptionsHeader:           "options:",
			FlagReqLangHelp:                  "source language code (empty = auto)",
			FlagRespLangHelp:                 "target language code (default: ru)",
			FlagDirectURLHelp:                "treat input URL(s) as direct media URLs (mp4/webm)",
			FlagSubsURLHelp:                  "direct subtitles URL to pass as translation help",
			FlagVoiceStyleHelp:               "voice style: live (default) or tts",
			FlagPollIntervalHelp:             "poll interval in seconds: 0 = auto mode via remainingTime, >=30 = fixed interval",
			FlagPollAttemptsHelp:             "maximum polling attempts (0 = unlimited, bounded by --max-wait; default 0)",
			FlagMaxWaitHelp:                  "maximum overall wait time (duration, e.g. 30m, 1h; default 30m)",
			FlagUseYtDLPHelp:                 "use yt-dlp (if available) to assist with URL handling",
			FlagYtDLPUseDirectURLHelp:        "when using yt-dlp, pass its direct media URL to backend instead of original URL",
			FlagYtDLPCookiesHelp:             "path to cookies file to pass to yt-dlp (--cookies)",
			FlagYtDLPCookiesFromBrowserHelp:  "browser spec to pass to yt-dlp (--cookies-from-browser)",
			FlagExplainHelp:                  "print how each URL will be handled and exit without contacting backends",
			FlagAllowLongVideoHelp:           "allow videos longer than 4 hours (disable local pre-check duration limit)",
			FlagAudioRequestedRealUploadHelp: "for AUDIO_REQUESTED, try real-audio upload (otherwise vot.js-like behaviour: fake/empty audio only)",
			FlagOutdirHelp:                   "directory to save translated audio (omit to print URL only)",
			FlagOutfileHelp:                  "output file name (single URL only; otherwise videoId/title.mp3)",
			FlagNoTitleHelp:                  "use videoId/hash in file name instead of yt-dlp title",
			FlagKeepURLHelp:                  "when downloading, also print the audio URL (plain: path<TAB>url; always both fields with --json)",
			FlagProxyYtDLPHelp:               "pass --proxy to yt-dlp (enabled by default when a proxy is set)",
			RespLangRequired:                 "--response-lang is required",
			InvalidVoiceStyle:                "--voice-style must be 'live' or 'tts'",
			PollIntervalTooSmall:             "--poll-interval must be 0 (auto) or at least 30 seconds",
			PollAttemptsInvalid:              "--poll-attempts must be non-negative",
			OutfileNeedsSingleURL:            "--outfile can only be used with a single URL",
			OutfileMustBeBareName:            "--outfile must be a bare file name, not a path",
			InvalidProxyFmt:                  "invalid --proxy %q: %v",
			FailedLoadConfigFmt:              "failed to load config: %v",
			InvalidWorkerURLFmt:              "invalid worker URL %q: %v",
			UnknownBackendFmt:                "unknown backend '%s' (expected 'direct' or 'worker')",
			UnknownCommandFmt:                "unknown command: %s",
			TimeoutErrorFmt:                  "translation timed out after %d seconds; try increasing --max-wait (and optionally --poll-attempts/--poll-interval)",
			CanceledError:                    "translation canceled",
			ErrorPrefix:                      "error",
			DownloadFailedFmt:                "failed to download audio for %s: %v",
			YtDLPNotFoundFmt:                 "yt-dlp is enabled (flag/config) but not found in PATH: %v; continuing without yt-dlp",
			YtDLPAuthHint:                    "hint: yt-dlp errors for YouTube often mean that authentication cookies are required; see https://github.com/yt-dlp/yt-dlp/wiki/FAQ#how-do-i-pass-cookies-to-yt-dlp and the \"Issues with yt-dlp\" section in the vot README.",
			YtDLPFailureFmt:                  "yt-dlp failed for this URL %s; continuing without yt-dlp.",
			SessionRequiredHelp:              "hint: SESSION_REQUIRED usually means the video is limited to logged-in Yandex Browser users; the CLI cannot bypass this restriction.",
			UnsupportedDirectURLHelp:         "hint: the backend does not accept this direct URL; try passing the original page URL or disabling direct-url / yt-dlp direct mode.",
			VideoTooLongFmt:                  "video is too long: %.0f seconds (limit %d seconds / 4 hours)",

			UsageDoctor:               "usage: vot doctor [global options]",
			DoctorChecksHeader:        "checks:",
			DoctorConfigPathLabel:     "config:",
			DoctorConfigStatusOK:      "  status: found and parsed OK",
			DoctorConfigStatusMissing: "  status: file not found (using built-in defaults)",
			DoctorNoConfigPath:        "config: no default path for this OS; use --config",
			DoctorEnvHeader:           "environment overrides:",
			DoctorFieldSet:            "set",
			DoctorFieldMissing:        "not set",
			DoctorWorkerHeader:        "worker backend:",
			DoctorWorkerBuiltIn:       "  worker support: compiled into this build",
			DoctorWorkerDisabled:      "  worker support: disabled in this build",
			DoctorWorkerURLLabel:      "  endpoint:",
			DoctorWorkerURLInvalidFmt: "  endpoint: invalid URL %q: %v",
			DoctorYtDLPHeader:         "yt-dlp:",
			DoctorYtDLPFound:          "  binary: found in PATH",
			DoctorYtDLPNotFoundFmt:    "  binary: not found in PATH: %v",
			DoctorSummaryOK:           "checks completed; if issues persist, run 'vot translate --debug ...' and review the output.",
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
		if msg.TranslateOptionsHeader != "" {
			fmt.Fprintln(os.Stderr, "\n"+msg.TranslateOptionsHeader)
		} else {
			fmt.Fprintln(os.Stderr, "\noptions:")
		}
		fs.PrintDefaults()
	}

	var (
		flagReqLang                  string
		flagRespLang                 string
		flagDirectURL                bool
		flagSubsURL                  string
		flagVoiceStyle               string
		flagPollInterval             int
		flagPollAttempts             int
		flagMaxWait                  time.Duration
		flagUseYtDLP                 bool
		flagYtDLPUseDirectURL        bool
		flagYtDLPCookies             string
		flagYtDLPCookiesFromBrowser  string
		flagExplain                  bool
		flagAllowLongVideo           bool
		flagAudioRequestedRealUpload bool
		flagOutdir                   string
		flagOutfile                  string
		flagNoTitle                  bool
		flagKeepURL                  bool
		flagProxyYtDLP               bool
	)
	pollIntervalFlagChanged := false

	fs.StringVarP(&flagReqLang, "request-lang", "s", "", msg.FlagReqLangHelp)
	fs.StringVarP(&flagRespLang, "response-lang", "t", "ru", msg.FlagRespLangHelp)
	fs.BoolVar(&flagDirectURL, "direct-url", false, msg.FlagDirectURLHelp)
	fs.StringVar(&flagSubsURL, "subs-url", "", msg.FlagSubsURLHelp)
	fs.StringVar(&flagVoiceStyle, "voice-style", "live", msg.FlagVoiceStyleHelp)
	fs.IntVar(&flagPollInterval, "poll-interval", 0, msg.FlagPollIntervalHelp)
	fs.IntVar(&flagPollAttempts, "poll-attempts", 0, msg.FlagPollAttemptsHelp)
	fs.DurationVar(&flagMaxWait, "max-wait", 30*time.Minute, msg.FlagMaxWaitHelp)
	fs.BoolVar(&flagUseYtDLP, "use-yt-dlp", false, msg.FlagUseYtDLPHelp)
	fs.BoolVar(&flagYtDLPUseDirectURL, "yt-dlp-use-direct-url", false, msg.FlagYtDLPUseDirectURLHelp)
	fs.StringVar(&flagYtDLPCookies, "yt-dlp-cookies", "", msg.FlagYtDLPCookiesHelp)
	fs.StringVar(&flagYtDLPCookiesFromBrowser, "yt-dlp-cookies-from-browser", "", msg.FlagYtDLPCookiesFromBrowserHelp)
	fs.BoolVar(&flagExplain, "explain", false, msg.FlagExplainHelp)
	fs.BoolVar(&flagAllowLongVideo, "allow-long-video", false, msg.FlagAllowLongVideoHelp)
	fs.BoolVar(&flagAudioRequestedRealUpload, "audio-requested-real-upload", false, msg.FlagAudioRequestedRealUploadHelp)
	fs.StringVarP(&flagOutdir, "outdir", "o", "", msg.FlagOutdirHelp)
	fs.StringVar(&flagOutfile, "outfile", "", msg.FlagOutfileHelp)
	fs.BoolVar(&flagNoTitle, "no-title", false, msg.FlagNoTitleHelp)
	fs.BoolVar(&flagKeepURL, "keep-url", false, msg.FlagKeepURLHelp)
	fs.BoolVar(&flagProxyYtDLP, "proxy-yt-dlp", true, msg.FlagProxyYtDLPHelp)

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

	if flagPollInterval != 0 && flagPollInterval < 30 {
		fmt.Fprintln(os.Stderr, msg.PollIntervalTooSmall)
		os.Exit(1)
	}

	if flagPollAttempts < 0 {
		fmt.Fprintln(os.Stderr, msg.PollAttemptsInvalid)
		os.Exit(1)
	}
	if flagMaxWait <= 0 {
		fmt.Fprintf(os.Stderr, "%s: --max-wait must be positive\n", msg.ErrorPrefix)
		os.Exit(1)
	}
	if flagOutfile != "" && len(urls) != 1 {
		fmt.Fprintln(os.Stderr, msg.OutfileNeedsSingleURL)
		os.Exit(1)
	}
	wantDownload := flagOutdir != "" || flagOutfile != ""
	// --json wins over --silent for stdout formatting.
	useJSON := flagJSON
	if f := fs.Lookup("poll-interval"); f != nil && f.Changed {
		pollIntervalFlagChanged = true
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
	applyConfigEnvOverrides(cfg)
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
		if os.Getenv("VOT_WORKER_URL") != "" {
			fmt.Fprintln(os.Stderr, "[debug] using worker URL from VOT_WORKER_URL")
		}
		if os.Getenv("VOT_PROXY") != "" {
			fmt.Fprintln(os.Stderr, "[debug] using proxy from VOT_PROXY")
		}
	}

	// Effective proxy: explicit --proxy wins over config/env.
	effectiveProxy := ""
	if cfg != nil {
		effectiveProxy = cfg.Proxy
	}
	if f := fs.Lookup("proxy"); f != nil && f.Changed {
		effectiveProxy = flagProxy
	}
	if effectiveProxy != "" {
		normalized, err := proxyutil.Normalize(effectiveProxy)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: "+msg.InvalidProxyFmt+"\n", msg.ErrorPrefix, effectiveProxy, err)
			os.Exit(1)
		}
		effectiveProxy = normalized
	}
	ytDLPProxy := ""
	if flagProxyYtDLP {
		ytDLPProxy = effectiveProxy
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
		if err := workerClient.SetWorkerURL(cfg.WorkerURL); err != nil {
			fmt.Fprintf(os.Stderr, "%s: "+msg.InvalidWorkerURLFmt+"\n", msg.ErrorPrefix, cfg.WorkerURL, err)
			os.Exit(1)
		}
	}
	if err := directClient.SetHTTPProxy(effectiveProxy); err != nil {
		fmt.Fprintf(os.Stderr, "%s: "+msg.InvalidProxyFmt+"\n", msg.ErrorPrefix, effectiveProxy, err)
		os.Exit(1)
	}
	if err := workerClient.SetHTTPProxy(effectiveProxy); err != nil {
		fmt.Fprintf(os.Stderr, "%s: "+msg.InvalidProxyFmt+"\n", msg.ErrorPrefix, effectiveProxy, err)
		os.Exit(1)
	}
	if flagDebug && !flagSilent && effectiveProxy != "" {
		fmt.Fprintf(os.Stderr, "[debug] using HTTP proxy %s (yt-dlp proxy=%v)\n", effectiveProxy, flagProxyYtDLP)
	}

	quietStderr := flagSilent || useJSON
	exitCode := 0
	var jsonResults []translateItemResult

	for _, u := range urls {
		pollTimeout := flagMaxWait
		ctx, cancel := context.WithTimeout(context.Background(), pollTimeout)

		effectiveURL := u
		effectiveDirect := flagDirectURL
		effectiveReqLang := flagReqLang
		effectiveBackend := flagBackend
		effectiveVoiceStyle := flagVoiceStyle
		var durationSec float64
		videoID := extractVideoID(u)

		failItem := func(errText string) {
			exitCode = 1
			if useJSON {
				jsonResults = append(jsonResults, translateItemResult{
					Input:      u,
					Status:     "failed",
					Type:       "audio",
					VideoID:    strPtr(videoID),
					URL:        nil,
					OutputPath: nil,
					Error:      strPtr(errText),
					Backend:    effectiveBackend,
					VoiceStyle: effectiveVoiceStyle,
				})
			}
		}

		// Adjust yt-dlp behaviour for this URL based on source-specific rules,
		// unless the user explicitly overrode the corresponding flags.
		useYtDLPForURL, ytDLPUseDirectURLForURL := applySourceRules(u, sourceRules, useYtDLP && ytDLPAvailable, ytDLPUseDirectURL, useYtDLPFlagChanged, ytDLPDirectFlagChanged)
		effectiveReqLang, effectiveBackend = applySourceLangAndBackend(u, sourceRules, effectiveReqLang, effectiveBackend, reqLangFlagChanged, backendFlagChanged)
		effectiveVoiceStyle = applySourceVoiceStyle(u, sourceRules, effectiveVoiceStyle, voiceStyleFlagChanged)
		cookies, cookiesFromBrowser := applySourceYtDLPCookies(u, sourceRules, ytDLPCookies, ytDLPCookiesFromBrowser, ytDLPCookiesFlagChanged, ytDLPCookiesFromBrowserFlagChanged)
		effectiveURL = applySourceURLRewrites(effectiveURL, sourceRules)
		if normalized := normalizeYtDLPCookiesPath(cookies); normalized == "" && cookies != "" {
			if !quietStderr {
				fmt.Fprintf(os.Stderr, "[warn] yt-dlp cookies path %q is unusable (parent dir missing); ignoring --cookies for this URL\n", cookies)
			}
			cookies = ""
		}

		// Optionally use yt-dlp to resolve a direct media URL and obtain duration.
		if useYtDLPForURL {
			if d, err := getVideoDurationWithYtDLP(effectiveURL, cookies, cookiesFromBrowser, ytDLPProxy); err != nil {
				if !quietStderr {
					if flagDebug {
						fmt.Fprintf(os.Stderr, "[debug] yt-dlp failed to get duration for %s: %v\n", effectiveURL, err)
					}
					fmt.Fprintf(os.Stderr, "%s: "+msg.YtDLPFailureFmt+"\n", msg.ErrorPrefix, u)
					fmt.Fprintln(os.Stderr, msg.YtDLPAuthHint)
				}
			} else {
				durationSec = d
				if flagDebug && !quietStderr {
					fmt.Fprintf(os.Stderr, "[debug] yt-dlp duration for %s: %.1fs\n", effectiveURL, d)
				}
			}

			if directURL, err := resolveDirectURLWithYtDLP(effectiveURL, cookies, cookiesFromBrowser, ytDLPProxy); err != nil {
				if !quietStderr {
					if flagDebug {
						fmt.Fprintf(os.Stderr, "[debug] yt-dlp resolution failed for %s: %v\n", effectiveURL, err)
					}
					fmt.Fprintf(os.Stderr, "%s: "+msg.YtDLPFailureFmt+"\n", msg.ErrorPrefix, u)
					fmt.Fprintln(os.Stderr, msg.YtDLPAuthHint)
				}
			} else {
				if flagDebug && !quietStderr {
					fmt.Fprintf(os.Stderr, "[debug] yt-dlp resolved %s -> %s\n", effectiveURL, directURL)
				}
				if ytDLPUseDirectURLForURL {
					effectiveURL = directURL
					effectiveDirect = true
				}
			}
		}

		sourceKind := classifySource(effectiveURL)

		if durationSec > maxVideoDurationSec && !flagAllowLongVideo {
			errText := fmt.Sprintf(msg.VideoTooLongFmt, durationSec, maxVideoDurationSec)
			if !useJSON {
				fmt.Fprintf(os.Stderr, "%s: %s: %s\n", msg.ErrorPrefix, u, errText)
			} else if !flagSilent {
				fmt.Fprintf(os.Stderr, "%s: %s: %s\n", msg.ErrorPrefix, u, errText)
			}
			cancel()
			failItem(errText)
			continue
		}

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
			if effectiveBackend == "" {
				effectiveBackend = "direct"
			}
		case "worker":
			client = workerClient
		default:
			errText := fmt.Sprintf(msg.UnknownBackendFmt, effectiveBackend)
			if !useJSON || !flagSilent {
				fmt.Fprintf(os.Stderr, "%s: %s\n", msg.ErrorPrefix, errText)
			}
			cancel()
			failItem(errText)
			continue
		}

		matchedRules := matchedSourceRuleSummaries(u, sourceRules)
		matchedSummary := "-"
		if len(matchedRules) > 0 {
			matchedSummary = strings.Join(matchedRules, ";")
		}

		fixedPollInterval := pollIntervalFlagChanged && flagPollInterval > 0
		pollMode := "auto"
		if fixedPollInterval {
			pollMode = "fixed"
		}

		if flagDebug && !quietStderr {
			fmt.Fprintf(os.Stderr, "[debug] url=%s effective_url=%s source=%s backend=%s req_lang=%s resp_lang=%s direct=%v voice_style=%s subs_url=%s poll_mode=%s poll_interval=%ds poll_attempts=%d use_yt_dlp=%v yt_dlp_use_direct_url=%v yt_dlp_cookies=%s yt_dlp_cookies_from_browser=%s source_rules=%s\n",
				u, effectiveURL, sourceKind, effectiveBackend, effectiveReqLang, flagRespLang, directForBackend, effectiveVoiceStyle, flagSubsURL, pollMode, flagPollInterval, flagPollAttempts, useYtDLPForURL, ytDLPUseDirectURLForURL, cookies, cookiesFromBrowser, matchedSummary)
		}

		// In explain mode we only show how the URL would be processed and skip
		// any calls to Yandex backends.
		if flagExplain {
			if !quietStderr {
				fmt.Fprintf(os.Stderr, "[explain] url=%s effective_url=%s source=%s backend=%s req_lang=%s resp_lang=%s direct=%v voice_style=%s subs_url=%s poll_mode=%s poll_interval=%ds poll_attempts=%d use_yt_dlp=%v yt_dlp_use_direct_url=%v yt_dlp_cookies=%s yt_dlp_cookies_from_browser=%s source_rules=%s\n",
					u, effectiveURL, sourceKind, effectiveBackend, effectiveReqLang, flagRespLang, directForBackend, effectiveVoiceStyle, flagSubsURL, pollMode, flagPollInterval, flagPollAttempts, useYtDLPForURL, ytDLPUseDirectURLForURL, cookies, cookiesFromBrowser, matchedSummary)
			}
			if useJSON {
				jsonResults = append(jsonResults, translateItemResult{
					Input:      u,
					Status:     "explained",
					Type:       "audio",
					VideoID:    strPtr(videoID),
					Backend:    effectiveBackend,
					VoiceStyle: effectiveVoiceStyle,
				})
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
			effectiveVoiceStyle = "live"
		case "tts":
			voiceStyle = backend.VoiceStyleTTS
		default:
			if flagDebug && !quietStderr {
				fmt.Fprintf(os.Stderr, "[debug] ignoring invalid voice_style %q from source rules for %s; falling back to live\n", effectiveVoiceStyle, u)
			}
			voiceStyle = backend.VoiceStyleLive
			effectiveVoiceStyle = "live"
		}

		// Regular video translation.
		res, err := client.TranslateVideo(ctx, backend.TranslateParams{
			URL:                      effectiveURL,
			OriginalURL:              u,
			RequestLang:              effectiveReqLang,
			ResponseLang:             flagRespLang,
			DirectURL:                directForBackend,
			SubsURL:                  flagSubsURL,
			VoiceStyle:               voiceStyle,
			DurationSec:              durationSec,
			UseYtDLP:                 useYtDLPForURL,
			YtDLPCookies:             cookies,
			YtDLPCookiesFromBrowser:  cookiesFromBrowser,
			HTTPProxy:                ytDLPProxy,
			AudioRequestedRealUpload: flagAudioRequestedRealUpload,
			PollIntervalSec:          flagPollInterval,
			FixedPollInterval:        fixedPollInterval,
			PollAttempts:             flagPollAttempts,
			Debug:                    flagDebug,
		})
		cancel()
		if err != nil {
			var errText string
			switch {
			case errors.Is(err, context.DeadlineExceeded):
				errText = fmt.Sprintf(msg.TimeoutErrorFmt, int(pollTimeout.Seconds()))
			case errors.Is(err, context.Canceled):
				errText = msg.CanceledError
			default:
				errText = err.Error()
				if strings.Contains(errText, "SESSION_REQUIRED") && msg.SessionRequiredHelp != "" {
					errText = errText + " " + msg.SessionRequiredHelp
				}
				if strings.Contains(errText, "unsupported direct URL") && msg.UnsupportedDirectURLHelp != "" {
					errText = errText + " " + msg.UnsupportedDirectURLHelp
				}
			}
			if !useJSON || !flagSilent {
				fmt.Fprintf(os.Stderr, "%s: %s: %s\n", msg.ErrorPrefix, u, errText)
			}
			failItem(errText)
			continue
		}

		outputPath := ""
		if wantDownload {
			title := ""
			if !flagNoTitle {
				// Use the original page URL so yt-dlp can resolve a title even when
				// the backend request was rewritten to a direct media URL.
				if t, titleErr := getVideoTitleWithYtDLP(u, cookies, cookiesFromBrowser, ytDLPProxy); titleErr != nil {
					if flagDebug && !quietStderr {
						fmt.Fprintf(os.Stderr, "[debug] yt-dlp title lookup failed for %s: %v\n", u, titleErr)
					}
				} else {
					title = t
				}
			}

			dest, destErr := resolveAudioOutputPath(flagOutdir, flagOutfile, u, title, flagNoTitle)
			if destErr != nil {
				errText := destErr.Error()
				if strings.Contains(errText, "bare file name") {
					errText = msg.OutfileMustBeBareName
				}
				if !useJSON || !flagSilent {
					fmt.Fprintf(os.Stderr, "%s: %s\n", msg.ErrorPrefix, errText)
				}
				failItem(errText)
				continue
			}

			dlCtx, dlCancel := context.WithTimeout(context.Background(), 15*time.Minute)
			dlErr := downloadHTTPFile(dlCtx, res.AudioURL, dest, effectiveProxy)
			dlCancel()
			if dlErr != nil {
				errText := fmt.Sprintf(msg.DownloadFailedFmt, u, dlErr)
				if !useJSON || !flagSilent {
					fmt.Fprintf(os.Stderr, "%s: %s\n", msg.ErrorPrefix, errText)
				}
				failItem(errText)
				continue
			}
			outputPath = dest
			if flagDebug && !quietStderr {
				fmt.Fprintf(os.Stderr, "[debug] saved audio to %s\n", outputPath)
			}
		}

		if useJSON {
			item := translateItemResult{
				Input:      u,
				Status:     "success",
				Type:       "audio",
				VideoID:    strPtr(videoID),
				URL:        strPtr(res.AudioURL),
				OutputPath: strPtr(outputPath),
				Backend:    effectiveBackend,
				VoiceStyle: effectiveVoiceStyle,
			}
			if res.DetectedLang != "" {
				item.DetectedLang = res.DetectedLang
			}
			jsonResults = append(jsonResults, item)
		} else {
			emitPlainTranslateLine(os.Stdout, res.AudioURL, outputPath, flagKeepURL)
		}

		if flagDebug && !quietStderr {
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

	if useJSON {
		if err := writeTranslateJSON(os.Stdout, jsonResults); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", msg.ErrorPrefix, err)
			os.Exit(1)
		}
	}

	os.Exit(exitCode)
}

// resolveDirectURLWithYtDLP tries to obtain a direct media URL for the given
// video URL using the local yt-dlp binary (if present in PATH).
func resolveDirectURLWithYtDLP(videoURL, cookiesFile, cookiesFromBrowser, proxyURL string) (string, error) {
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
func getVideoDurationWithYtDLP(videoURL, cookiesFile, cookiesFromBrowser, proxyURL string) (float64, error) {
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
	args = proxyutil.AppendYtDLPArgs(args, proxyURL)
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

type sourceRewriteRule struct {
	re      *regexp.Regexp
	replace string
}

// sourceRule is an internal compiled representation of a per-source rule
// from config. All fields are optional except at least one regexp pattern.
type sourceRule struct {
	patterns                []*regexp.Regexp
	patternTexts            []string
	useYtDLP                *bool
	ytDLPUseDirectURL       *bool
	ytDLPCookies            *string
	ytDLPCookiesFromBrowser *string
	requestLang             *string
	backend                 *string
	voiceStyle              *string
	rewrite                 []sourceRewriteRule
}

func (r sourceRule) matches(rawURL string) bool {
	for _, re := range r.patterns {
		if re.MatchString(rawURL) {
			return true
		}
	}
	return false
}

func (r sourceRule) summary() string {
	if len(r.patternTexts) == 0 {
		return "<unknown>"
	}
	if len(r.patternTexts) == 1 {
		return r.patternTexts[0]
	}
	return strings.Join(r.patternTexts, ",")
}

func boolPtr(v bool) *bool {
	return &v
}

func stringPtr(v string) *string {
	return &v
}

func builtInSourceRuleConfigs() []config.SourceRuleConfig {
	var workerBackend *string
	if yandexclient.WorkerBackendAvailable() {
		workerBackend = stringPtr("worker")
	}

	return []config.SourceRuleConfig{
		{
			Patterns: []string{
				`(?i)^https?://(www\.)?youtube\.com/watch`,
				`(?i)^https?://(www\.)?youtu\.be/`,
			},
			UseYtDLP:          boolPtr(true),
			YtDLPUseDirectURL: boolPtr(false),
			Backend:           workerBackend,
			VoiceStyle:        stringPtr("live"),
		},
		{
			Pattern:           `(?i)^https?://[^/]*(invidio\.us|invidious|piped)[^/]*/`,
			UseYtDLP:          boolPtr(true),
			YtDLPUseDirectURL: boolPtr(false),
			Backend:           workerBackend,
		},
		{
			Pattern:           `(?i)^https?://www\.zdf\.de/play/`,
			UseYtDLP:          boolPtr(true),
			YtDLPUseDirectURL: boolPtr(true),
			RequestLang:       stringPtr("de"),
			Backend:           workerBackend,
			VoiceStyle:        stringPtr("tts"),
		},
	}
}

func compileSourceRules(cfgRules []config.SourceRuleConfig) []sourceRule {
	var rules []sourceRule

	for _, cfgRule := range cfgRules {
		var rawPatterns []string
		if cfgRule.Pattern != "" {
			rawPatterns = append(rawPatterns, cfgRule.Pattern)
		}
		rawPatterns = append(rawPatterns, cfgRule.Patterns...)

		var compiledPatterns []*regexp.Regexp
		var patternTexts []string
		for _, rawPattern := range rawPatterns {
			if rawPattern == "" {
				continue
			}
			re, err := regexp.Compile(rawPattern)
			if err != nil {
				continue
			}
			compiledPatterns = append(compiledPatterns, re)
			patternTexts = append(patternTexts, rawPattern)
		}
		if len(compiledPatterns) == 0 {
			continue
		}

		var rewrites []sourceRewriteRule
		for _, rewrite := range cfgRule.Rewrite {
			if rewrite.Pattern == "" {
				continue
			}
			re, err := regexp.Compile(rewrite.Pattern)
			if err != nil {
				continue
			}
			rewrites = append(rewrites, sourceRewriteRule{
				re:      re,
				replace: rewrite.Replace,
			})
		}

		rules = append(rules, sourceRule{
			patterns:                compiledPatterns,
			patternTexts:            patternTexts,
			useYtDLP:                cfgRule.UseYtDLP,
			ytDLPUseDirectURL:       cfgRule.YtDLPUseDirectURL,
			ytDLPCookies:            cfgRule.YtDLPCookies,
			ytDLPCookiesFromBrowser: cfgRule.YtDLPCookiesFromBrowser,
			requestLang:             cfgRule.RequestLang,
			backend:                 cfgRule.Backend,
			voiceStyle:              cfgRule.VoiceStyle,
			rewrite:                 rewrites,
		})
	}

	return rules
}

// buildSourceRulesFromConfig compiles source rules from built-ins and config.
// Invalid regexp entries are ignored so a broken rule does not break the CLI.
func buildSourceRulesFromConfig(cfg *config.Config) []sourceRule {
	rules := compileSourceRules(builtInSourceRuleConfigs())
	if cfg == nil {
		return rules
	}
	return append(rules, compileSourceRules(cfg.SourceRules)...)
}

// applySourceRules adjusts yt-dlp usage flags for a specific URL based on
// matching source rules. CLI flags take precedence over rules when changed.
func applySourceRules(rawURL string, rules []sourceRule, baseUseYtDLP, baseYtDLPUseDirectURL bool, useFlagChanged, directFlagChanged bool) (bool, bool) {
	use := baseUseYtDLP
	direct := baseYtDLPUseDirectURL

	for _, r := range rules {
		if !r.matches(rawURL) {
			continue
		}
		if !useFlagChanged && r.useYtDLP != nil {
			use = *r.useYtDLP
		}
		if !directFlagChanged && r.ytDLPUseDirectURL != nil {
			direct = *r.ytDLPUseDirectURL
		}
	}

	return use, direct
}

// applySourceLangAndBackend adjusts request language and backend based on
// matching source rules. CLI flags again take precedence when explicitly set.
func applySourceLangAndBackend(rawURL string, rules []sourceRule, baseReqLang, baseBackend string, reqLangFlagChanged, backendFlagChanged bool) (string, string) {
	reqLang := baseReqLang
	backend := baseBackend

	for _, r := range rules {
		if !r.matches(rawURL) {
			continue
		}
		if !reqLangFlagChanged && r.requestLang != nil {
			reqLang = *r.requestLang
		}
		if !backendFlagChanged && r.backend != nil {
			backend = *r.backend
		}
	}

	return reqLang, backend
}

// applySourceVoiceStyle adjusts voice style based on matching rules unless
// the user explicitly set --voice-style.
func applySourceVoiceStyle(rawURL string, rules []sourceRule, baseVoiceStyle string, voiceStyleFlagChanged bool) string {
	voiceStyle := baseVoiceStyle

	for _, r := range rules {
		if !r.matches(rawURL) {
			continue
		}
		if !voiceStyleFlagChanged && r.voiceStyle != nil {
			voiceStyle = *r.voiceStyle
		}
	}

	return voiceStyle
}

// applySourceYtDLPCookies adjusts yt-dlp cookies settings from matching rules
// unless the corresponding CLI flags were explicitly set.
func applySourceYtDLPCookies(rawURL string, rules []sourceRule, baseCookies, baseCookiesFromBrowser string, cookiesFlagChanged, cookiesFromBrowserFlagChanged bool) (string, string) {
	cookies := baseCookies
	cookiesFromBrowser := baseCookiesFromBrowser

	for _, r := range rules {
		if !r.matches(rawURL) {
			continue
		}
		if !cookiesFlagChanged && r.ytDLPCookies != nil {
			cookies = *r.ytDLPCookies
		}
		if !cookiesFromBrowserFlagChanged && r.ytDLPCookiesFromBrowser != nil {
			cookiesFromBrowser = *r.ytDLPCookiesFromBrowser
		}
	}

	return cookies, cookiesFromBrowser
}

// applySourceURLRewrites applies rewrite rules from matching source rules in
// order. Each matched rule can rewrite the URL before it is sent to yt-dlp or
// directly to the backend.
func applySourceURLRewrites(rawURL string, rules []sourceRule) string {
	rewritten := rawURL

	for _, r := range rules {
		if !r.matches(rewritten) {
			continue
		}
		for _, rewrite := range r.rewrite {
			rewritten = rewrite.re.ReplaceAllString(rewritten, rewrite.replace)
		}
	}

	return rewritten
}

// matchedSourceRuleSummaries returns a stable debug/explain summary of rules
// that matched the original URL.
func matchedSourceRuleSummaries(rawURL string, rules []sourceRule) []string {
	var summaries []string
	for _, r := range rules {
		if r.matches(rawURL) {
			summaries = append(summaries, r.summary())
		}
	}
	return summaries
}

func normalizeYtDLPCookiesPath(path string) string {
	if path == "" {
		return ""
	}
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ""
	}
	return path
}
