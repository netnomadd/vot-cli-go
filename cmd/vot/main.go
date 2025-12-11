package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"
)

// Global flags
var (
	flagConfig  string
	flagSilent  bool
	flagDebug   bool
	flagLang    string
	flagBackend string
)

func main() {
	root := flag.NewFlagSet("vot", flag.ExitOnError)
	root.StringVarP(&flagConfig, "config", "c", "", "path to config file")
	root.BoolVarP(&flagSilent, "silent", "q", false, "silent mode: only result URLs to stdout, errors to stderr")
	root.BoolVarP(&flagDebug, "debug", "d", false, "enable debug logging")
	root.StringVar(&flagLang, "lang", "", "UI language (e.g. ru, en)")
	root.StringVar(&flagBackend, "backend", "direct", "backend to use: direct or worker")

	if len(os.Args) < 2 {
		// No subcommand, show help
		fmt.Fprintln(os.Stderr, "usage: vot <command> [options]")
		os.Exit(1)
	}

	subcmd := os.Args[1]
	switch subcmd {
	case "translate":
		translateMain(root, os.Args[2:])
	case "help", "-h", "--help":
		fmt.Fprintln(os.Stderr, "usage: vot <command> [options]\n\ncommands:\n  translate  translate video and print audio URL(s)")
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", subcmd)
		os.Exit(1)
	}
}
