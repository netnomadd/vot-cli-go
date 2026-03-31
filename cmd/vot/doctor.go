package main

import (
	"fmt"
	"os"
	"os/exec"

	flag "github.com/spf13/pflag"

	"github.com/netnomadd/vot-cli-go/internal/config"
	"github.com/netnomadd/vot-cli-go/internal/yandexclient"
)

// doctorMain handles `vot doctor` / `vot check` subcommand.
func doctorMain(parent *flag.FlagSet, args []string) {
	msg := getMessages()
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)

	// Reuse global flags (config, debug, lang, etc.).
	fs.AddFlagSet(parent)

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, msg.UsageDoctor)
	}

	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Load configuration.
	cfg, cfgPath, err := config.Load(flagConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: "+msg.FailedLoadConfigFmt+"\n", msg.ErrorPrefix, err)
		os.Exit(1)
	}
	applyConfigEnvOverrides(cfg)

	fmt.Fprintln(os.Stderr, msg.DoctorChecksHeader)

	// Config status.
	if cfgPath == "" {
		fmt.Fprintln(os.Stderr, msg.DoctorNoConfigPath)
	} else {
		fmt.Fprintf(os.Stderr, "%s %s\n", msg.DoctorConfigPathLabel, cfgPath)
		if _, err := os.Stat(cfgPath); err == nil {
			fmt.Fprintln(os.Stderr, msg.DoctorConfigStatusOK)
		} else if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, msg.DoctorConfigStatusMissing)
		} else {
			fmt.Fprintf(os.Stderr, "%s: "+msg.FailedLoadConfigFmt+"\n", msg.ErrorPrefix, err)
		}
	}

	// Environment overrides.
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, msg.DoctorEnvHeader)
	printEnvStatus := func(name string) {
		_, ok := os.LookupEnv(name)
		status := msg.DoctorFieldMissing
		if ok {
			status = msg.DoctorFieldSet
		}
		fmt.Fprintf(os.Stderr, "  %s=%s\n", name, status)
	}
	printEnvStatus("VOT_USER_AGENT")
	printEnvStatus("VOT_YANDEX_HMAC_KEY")
	printEnvStatus("VOT_YANDEX_TOKEN")
	printEnvStatus("VOT_WORKER_URL")
	printEnvStatus("VOT_LANG")

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, msg.DoctorWorkerHeader)
	if yandexclient.WorkerBackendAvailable() {
		fmt.Fprintln(os.Stderr, msg.DoctorWorkerBuiltIn)
	} else {
		fmt.Fprintln(os.Stderr, msg.DoctorWorkerDisabled)
	}
	workerURL := configuredWorkerURL(cfg)
	if _, _, err := yandexclient.NormalizeWorkerURL(workerURL); err != nil {
		fmt.Fprintf(os.Stderr, msg.DoctorWorkerURLInvalidFmt+"\n", workerURL, err)
	} else {
		fmt.Fprintf(os.Stderr, "%s %s\n", msg.DoctorWorkerURLLabel, workerURL)
	}

	// yt-dlp availability.
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, msg.DoctorYtDLPHeader)
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		fmt.Fprintf(os.Stderr, msg.DoctorYtDLPNotFoundFmt+"\n", err)
	} else {
		fmt.Fprintln(os.Stderr, msg.DoctorYtDLPFound)
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, msg.DoctorSummaryOK)
}
