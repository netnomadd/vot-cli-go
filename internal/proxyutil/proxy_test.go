package proxyutil

import (
	"net/http"
	"testing"
)

func TestParse(t *testing.T) {
	u, err := Parse("127.0.0.1:7890")
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "http" || u.Host != "127.0.0.1:7890" {
		t.Fatalf("got %v", u)
	}

	u, err = Parse("http://user:pass@proxy.example:8080")
	if err != nil {
		t.Fatal(err)
	}
	if u.User.Username() != "user" {
		t.Fatalf("user = %q", u.User.Username())
	}
	if pass, ok := u.User.Password(); !ok || pass != "pass" {
		t.Fatalf("password = %q ok=%v", pass, ok)
	}

	if _, err := Parse("socks5://127.0.0.1:1080"); err == nil {
		t.Fatal("expected socks5 to be rejected")
	}
	if _, err := Parse(""); err == nil {
		t.Fatal("expected empty to fail")
	}
}

func TestApplyHTTPClient(t *testing.T) {
	client := &http.Client{}
	if err := ApplyHTTPClient(client, "http://127.0.0.1:7890"); err != nil {
		t.Fatal(err)
	}
	tr, ok := client.Transport.(*http.Transport)
	if !ok || tr.Proxy == nil {
		t.Fatalf("expected transport with Proxy, got %#v", client.Transport)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	proxyURL, err := tr.Proxy(req)
	if err != nil || proxyURL == nil || proxyURL.Host != "127.0.0.1:7890" {
		t.Fatalf("proxy URL = %v err=%v", proxyURL, err)
	}
}

func TestAppendYtDLPArgs(t *testing.T) {
	got := AppendYtDLPArgs([]string{"-g"}, "http://127.0.0.1:7890")
	if len(got) != 3 || got[1] != "--proxy" || got[2] != "http://127.0.0.1:7890" {
		t.Fatalf("got %#v", got)
	}
	if kept := AppendYtDLPArgs([]string{"-g"}, ""); len(kept) != 1 {
		t.Fatalf("empty proxy should not change args: %#v", kept)
	}
}
