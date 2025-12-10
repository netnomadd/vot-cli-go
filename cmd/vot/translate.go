package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"
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
	fs.BoolVar(&flagM3U8, "m3u8", false, "request streaming/m3u8 output instead of mp3 (if supported)")
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

	// TODO: wire up config loading, i18n, backend selection and actual translation.
	// For now, just echo parsed parameters for debugging.
	for _, u := range urls {
		if !flagSilent {
			fmt.Fprintf(os.Stderr, "[debug] url=%s req_lang=%s resp_lang=%s direct=%v m3u8=%v voice_style=%s subs_url=%s\n",
				u, flagReqLang, flagRespLang, flagDirectURL, flagM3U8, flagVoiceStyle, flagSubsURL)
		}
		// Placeholder: in real implementation here we would call domain.TranslateVideo(...)
		// and print resulting audio URL to stdout (or nothing on error).
		fmt.Fprintf(os.Stdout, "%s\n", u)
	}
}
