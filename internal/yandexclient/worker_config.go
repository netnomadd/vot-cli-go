package yandexclient

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	defaultWorkerSchema = "https"
	defaultWorkerHost   = "vot-worker.toil.cc"
)

func DefaultWorkerURL() string {
	return defaultWorkerSchema + "://" + defaultWorkerHost
}

func NormalizeWorkerURL(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultWorkerSchema, defaultWorkerHost, nil
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("parse worker URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", "", fmt.Errorf("worker URL must include scheme and host")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", "", fmt.Errorf("worker URL must not include a path")
	}
	if parsed.RawQuery != "" {
		return "", "", fmt.Errorf("worker URL must not include a query string")
	}
	if parsed.Fragment != "" {
		return "", "", fmt.Errorf("worker URL must not include a fragment")
	}

	return strings.ToLower(parsed.Scheme), strings.ToLower(parsed.Host), nil
}
