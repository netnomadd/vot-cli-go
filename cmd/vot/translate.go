package main

import (
	"context"
	"fmt"
	"os"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/netnomadd/vot-cli-go/internal/backend"
	"github.com/netnomadd/vot-cli-go/internal/config"
	"github.com/netnomadd/vot-cli-go/internal/yandexclient"
)

// messages holds localized user-facing strings for the translate command.
type messages struct {
	UsageTranslate    string
	RespLangRequired  string
	InvalidVoiceStyle string
	ErrorPrefix       string
}

// getMessages returns localized messages based on --lang flag or VOT_LANG.
func getMessages() messages {
	lang := flagLang
	if lang == "" {
		if v := os.Getenv("VOT_LANG"); v != "" {
			lang = v
		}
	}

	switch lang {
	case "ru":
		return messages{
			UsageTranslate:    "использование: vot translate [опции] <url> [url2 ...]",
			RespLangRequired:  "--response-lang обязателен",
			InvalidVoiceStyle: "--voice-style может быть только 'live' или 'tts'",
			ErrorPrefix:       "ошибка",
		}
	default:
		return messages{
			UsageTranslate:    "usage: vot translate [options] <url> [url2 ...]",
			RespLangRequired:  "--response-lang is required",
			InvalidVoiceStyle: "--voice-style must be 'live' or 'tts'",
			ErrorPrefix:       "error",
		}
	}
}

// translateMain handles `vot translate` subcommand.
func translateMain(parent *flag.FlagSet, args []string) {
	msg := getMessages()
	fs := flag.NewFlagSet("translate", flag.ExitOnError)

	// Reuse global flags in this subcommand (parsed first)
	fs.AddFlagSet(parent)

	var (
		flagReqLang      string
		flagRespLang     string
		flagDirectURL    bool
		flagSubsURL      string
		flagVoiceStyle   string
		flagPollInterval int
		flagPollAttempts int
	)

	fs.StringVarP(&flagReqLang, "request-lang", "s", "", "source language code (empty = auto)")
	fs.StringVarP(&flagRespLang, "response-lang", "t", "", "target language code (required)")
	fs.BoolVar(&flagDirectURL, "direct-url", false, "treat input URL(s) as direct media URLs (mp4/webm)")
	fs.StringVar(&flagSubsURL, "subs-url", "", "direct subtitles URL to pass as translation help")
	fs.StringVar(&flagVoiceStyle, "voice-style", "live", "voice style: live (default) or tts")
	fs.IntVar(&flagPollInterval, "poll-interval", 30, "polling interval in seconds (min 30)")
	fs.IntVar(&flagPollAttempts, "poll-attempts", 10, "maximum number of polling attempts")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	urls := fs.Args()
	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, msg.UsageTranslate)
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
		fmt.Fprintln(os.Stderr, "--poll-interval must be at least 30 seconds")
		os.Exit(1)
	}

	if flagPollAttempts <= 0 {
		fmt.Fprintln(os.Stderr, "--poll-attempts must be positive")
		os.Exit(1)
	}

	// Load configuration (if any) and initialise direct backend client.
	cfg, cfgPath, err := config.Load(flagConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}
	if flagDebug && !flagSilent && cfgPath != "" {
		fmt.Fprintf(os.Stderr, "[debug] using config file: %s\n", cfgPath)
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
		fmt.Fprintf(os.Stderr, "%s: unknown backend '%s' (expected 'direct' or 'worker')\n", msg.ErrorPrefix, flagBackend)
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
			fmt.Fprintf(os.Stderr, "[debug] url=%s req_lang=%s resp_lang=%s direct=%v voice_style=%s subs_url=%s poll_interval=%ds poll_attempts=%d\n",
				u, flagReqLang, flagRespLang, flagDirectURL, flagVoiceStyle, flagSubsURL, flagPollInterval, flagPollAttempts)
		}

		// Regular video translation.
		res, err := client.TranslateVideo(ctx, backend.TranslateParams{
			URL:             u,
			RequestLang:     flagReqLang,
			ResponseLang:    flagRespLang,
			DirectURL:       flagDirectURL,
			SubsURL:         flagSubsURL,
			VoiceStyle:      voiceStyle,
			PollIntervalSec: flagPollInterval,
			PollAttempts:    flagPollAttempts,
			Debug:           flagDebug,
		})
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", msg.ErrorPrefix, err)
			exitCode = 1
			continue
		}

		fmt.Fprintln(os.Stdout, res.AudioURL)
	}

	os.Exit(exitCode)
}
