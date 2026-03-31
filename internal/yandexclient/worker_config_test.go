package yandexclient

import "testing"

func TestNormalizeWorkerURL(t *testing.T) {
	scheme, host, err := NormalizeWorkerURL("https://worker.example.com/")
	if err != nil {
		t.Fatalf("NormalizeWorkerURL returned error: %v", err)
	}
	if scheme != "https" || host != "worker.example.com" {
		t.Fatalf("NormalizeWorkerURL = (%q, %q), want (%q, %q)", scheme, host, "https", "worker.example.com")
	}
}

func TestNormalizeWorkerURLRejectsPath(t *testing.T) {
	if _, _, err := NormalizeWorkerURL("https://worker.example.com/api"); err == nil {
		t.Fatalf("expected path validation error, got nil")
	}
}
