package proxyutil

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Parse normalizes and validates an HTTP(S) proxy URL.
// Bare "host:port" is treated as "http://host:port".
func Parse(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty proxy URL")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("proxy URL missing host")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q (expected http or https)", u.Scheme)
	}
	return u, nil
}

// Normalize returns a canonical proxy URL string suitable for http.Client and yt-dlp.
func Normalize(raw string) (string, error) {
	u, err := Parse(raw)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// ApplyHTTPClient configures client.Transport to use the given proxy.
// An empty proxy clears a previously applied ProxyURL by installing a fresh
// transport that still respects environment proxies via ProxyFromEnvironment.
func ApplyHTTPClient(client *http.Client, proxyRaw string) error {
	if client == nil {
		return fmt.Errorf("nil http.Client")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyRaw == "" {
		client.Transport = transport
		return nil
	}
	u, err := Parse(proxyRaw)
	if err != nil {
		return err
	}
	transport.Proxy = http.ProxyURL(u)
	client.Transport = transport
	return nil
}

// AppendYtDLPArgs appends yt-dlp --proxy when proxyRaw is non-empty.
func AppendYtDLPArgs(args []string, proxyRaw string) []string {
	if proxyRaw == "" {
		return args
	}
	return append(args, "--proxy", proxyRaw)
}
