//go:build no_worker

package yandexclient

import (
	"context"
	"errors"

	"github.com/netnomadd/vot-cli-go/internal/backend"
)

var errWorkerBackendDisabled = errors.New("worker backend is disabled in this build; rebuild without -tags no_worker or use --backend=direct")

type WorkerClient struct{}

func NewWorkerClient() *WorkerClient {
	return &WorkerClient{}
}

func WorkerBackendAvailable() bool {
	return false
}

func (c *WorkerClient) SetUserAgent(string) {}

func (c *WorkerClient) SetHMACKey(string) {}

func (c *WorkerClient) SetAPIToken(string) {}

func (c *WorkerClient) SetWorkerURL(string) error {
	return nil
}

func (c *WorkerClient) TranslateVideo(context.Context, backend.TranslateParams) (backend.TranslateResult, error) {
	return backend.TranslateResult{}, errWorkerBackendDisabled
}

func (c *WorkerClient) TranslateStream(context.Context, backend.StreamParams) (backend.StreamResult, error) {
	return backend.StreamResult{}, errWorkerBackendDisabled
}
