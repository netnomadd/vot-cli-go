package yandexclient

import (
	"net/http"

	"github.com/netnomadd/vot-cli-go/internal/proxyutil"
)

func applyHTTPProxy(client *http.Client, proxyURL string) error {
	return proxyutil.ApplyHTTPClient(client, proxyURL)
}
