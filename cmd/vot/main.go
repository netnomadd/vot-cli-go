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

func printRootUsage(root *flag.FlagSet) {
	msg := getMessages()
	fmt.Fprintln(os.Stderr, msg.UsageRoot)
	fmt.Fprintln(os.Stderr, "\n"+msg.CommandsHeader)
	fmt.Fprintln(os.Stderr, "  "+msg.CommandTranslate)
	fmt.Fprintln(os.Stderr, "\n"+msg.GlobalOptionsHeader)
	root.PrintDefaults()
	fmt.Fprintln(os.Stderr, "\n"+msg.HelpHint)
}

func main() {
	root := flag.NewFlagSet("vot", flag.ExitOnError)
	root.StringVarP(&flagConfig, "config", "c", "", "path to config file")
	root.BoolVarP(&flagSilent, "silent", "q", false, "silent mode: only result URLs to stdout, errors to stderr")
	root.BoolVarP(&flagDebug, "debug", "d", false, "enable debug logging")
	root.StringVar(&flagLang, "lang", "", "UI language (e.g. ru, en)")
	root.StringVar(&flagBackend, "backend", "direct", "backend to use: direct or worker")

	root.Usage = func() { printRootUsage(root) }

	if len(os.Args) < 2 {
		// No subcommand, show help
		root.Usage()
		os.Exit(1)
	}

	subcmd := os.Args[1]
	switch subcmd {
	case "translate":
		translateMain(root, os.Args[2:])
	case "help", "-h", "--help":
		root.Usage()
		os.Exit(0)
	default:
		msg := getMessages()
		fmt.Fprintf(os.Stderr, "%s: "+msg.UnknownCommandFmt+"\n\n", msg.ErrorPrefix, subcmd)
		root.Usage()
		os.Exit(1)
	}
}
