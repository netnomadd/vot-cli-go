package yandexclient

import "testing"

func TestIsCustomLink(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://cdn.example.com/a.m3u8", true},
		{"https://cdn.example.com/a.m3u8?token=1", true},
		{"https://cdn.example.com/a.mpd", true},
		{"https://cdn.example.com/a.m4a", true},
		{"https://cdn.qstv.on.epicgames.com/x", true},
		{"https://www.youtube.com/watch?v=abc", false},
		{"https://cdn.example.com/a.mp4", false},
	}
	for _, tc := range cases {
		if got := IsCustomLink(tc.url); got != tc.want {
			t.Fatalf("IsCustomLink(%q)=%v want %v", tc.url, got, tc.want)
		}
	}
}

func TestNormalizeVOTHost(t *testing.T) {
	schema, host, err := NormalizeVOTHost("https://vot.example.com/v1/")
	if err != nil {
		t.Fatal(err)
	}
	if schema != "https" || host != "vot.example.com/v1" {
		t.Fatalf("got %s %s", schema, host)
	}
	if _, _, err := NormalizeVOTHost("https://vot.example.com/v1?x=1"); err == nil {
		t.Fatal("expected query rejection")
	}
	schema, host, err = NormalizeVOTHost("")
	if err != nil || schema != "https" || host != defaultVOTHost {
		t.Fatalf("defaults: %s %s err=%v", schema, host, err)
	}
}
