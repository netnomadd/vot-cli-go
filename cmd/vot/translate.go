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

// translateMain handles `vot translate` subcommand.
func translateMain(parent *flag.FlagSet, args []string) {
	fs := flag.NewFlagSet("translate", flag.ExitOnError)

	// Reuse global flags in this subcommand (parsed first)
	fs.AddFlagSet(parent)

	var (
		flagReqLang    string
		flagRespLang   string
		flagDirectURL  bool
		flagSubsURL    string
		flagM3U8       bool
		flagVoiceStyle string
	)

	fs.StringVarP(&flagReqLang, "request-lang", "s", "", "source language code (empty = auto)")
	fs.StringVarP(&flagRespLang, "response-lang", "t", "", "target language code (required)")
	fs.BoolVar(&flagDirectURL, "direct-url", false, "treat input URL(s) as direct media URLs (mp4/webm/m3u8)")
	fs.StringVar(&flagSubsURL, "subs-url", "", "direct subtitles URL to pass as translation help")
	fs.BoolVar(&flagM3U8, "m3u8", false, "treat input URL(s) as stream (m3u8) and request stream translation")
	fs.StringVar(&flagVoiceStyle, "voice-style", "live", "voice style: live (default) or tts")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	urls := fs.Args()
	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "usage: vot translate [options] <url> [url2 ...]")
		os.Exit(1)
	}

	if flagRespLang == "" {
		fmt.Fprintln(os.Stderr, "--response-lang is required")
		os.Exit(1)
	}

	if flagVoiceStyle != "live" && flagVoiceStyle != "tts" {
		fmt.Fprintln(os.Stderr, "--voice-style must be 'live' or 'tts'")
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

	client := yandexclient.NewDirectClient()
	if cfg != nil {
		client.SetUserAgent(cfg.UserAgent)
		client.SetHMACKey(cfg.YandexHMACKey)
		client.SetAPIToken(cfg.YandexToken)
	}

	voiceStyle := backend.VoiceStyleLive
	if flagVoiceStyle == "tts" {
		voiceStyle = backend.VoiceStyleTTS
	}

	exitCode := 0
	for _, u := range urls {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)

		if flagDebug && !flagSilent {
			fmt.Fprintf(os.Stderr, "[debug] url=%s req_lang=%s resp_lang=%s direct=%v m3u8=%v voice_style=%s subs_url=%s\n",
				u, flagReqLang, flagRespLang, flagDirectURL, flagM3U8, flagVoiceStyle, flagSubsURL)
		}

		if flagM3U8 {
			// Stream translation: input URL is treated as stream source (e.g. m3u8).
			res, err := client.TranslateStream(ctx, backend.StreamParams{
				URL:          u,
				RequestLang:  flagReqLang,
				ResponseLang: flagRespLang,
			})
			cancel()
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				exitCode = 1
				continue
			}
			fmt.Fprintln(os.Stdout, res.StreamURL)
			continue
		}

		// Regular video translation.
		res, err := client.TranslateVideo(ctx, backend.TranslateParams{
			URL:          u,
			RequestLang:  flagReqLang,
			ResponseLang: flagRespLang,
			DirectURL:    flagDirectURL,
			SubsURL:      flagSubsURL,
			VoiceStyle:   voiceStyle,
		})
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			exitCode = 1
			continue
		}

		fmt.Fprintln(os.Stdout, res.AudioURL)
	}

	os.Exit(exitCode)
}
