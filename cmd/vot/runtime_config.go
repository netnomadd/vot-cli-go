package main

import (
	"os"

	"github.com/netnomadd/vot-cli-go/internal/config"
	"github.com/netnomadd/vot-cli-go/internal/yandexclient"
)

func applyConfigEnvOverrides(cfg *config.Config) {
	if cfg == nil {
		return
	}

	if ua := os.Getenv("VOT_USER_AGENT"); ua != "" {
		cfg.UserAgent = ua
	}
	if h := os.Getenv("VOT_YANDEX_HMAC_KEY"); h != "" {
		cfg.YandexHMACKey = h
	}
	if t := os.Getenv("VOT_YANDEX_TOKEN"); t != "" {
		cfg.YandexToken = t
	}
	if workerURL := os.Getenv("VOT_WORKER_URL"); workerURL != "" {
		cfg.WorkerURL = workerURL
	}
	if proxy := os.Getenv("VOT_PROXY"); proxy != "" {
		cfg.Proxy = proxy
	}
	if votHost := os.Getenv("VOT_HOST"); votHost != "" {
		cfg.VotHost = votHost
	}
}

func configuredVOTHost(cfg *config.Config) string {
	if cfg != nil && cfg.VotHost != "" {
		return cfg.VotHost
	}
	return yandexclient.DefaultVOTHostURL()
}

func configuredWorkerURL(cfg *config.Config) string {
	if cfg != nil && cfg.WorkerURL != "" {
		return cfg.WorkerURL
	}
	return yandexclient.DefaultWorkerURL()
}
