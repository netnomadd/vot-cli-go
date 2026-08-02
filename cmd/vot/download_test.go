package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeFileName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Hello World", "Hello_World"},
		{"a/b\\c:d*e?", "a_b_c_d_e"},
		{"  ..  ", "audio"},
		{"", "audio"},
		{strings.Repeat("x", 200), strings.Repeat("x", 120)},
	}
	for _, tc := range cases {
		got := sanitizeFileName(tc.in)
		if got != tc.want {
			t.Fatalf("sanitizeFileName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestExtractVideoID(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://www.youtube.com/watch?v=X98VPQCE_WI", "X98VPQCE_WI"},
		{"https://youtu.be/X98VPQCE_WI", "X98VPQCE_WI"},
		{"https://www.youtube.com/shorts/X98VPQCE_WI", "X98VPQCE_WI"},
		{"https://www.youtube.com/embed/X98VPQCE_WI", "X98VPQCE_WI"},
	}
	for _, tc := range cases {
		got := extractVideoID(tc.in)
		if got != tc.want {
			t.Fatalf("extractVideoID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	if got := extractVideoID("https://example.com/watch/abc"); got != "abc" {
		t.Fatalf("extractVideoID(generic path) = %q, want abc", got)
	}
	hashed := extractVideoID("https://example.com/")
	if len(hashed) != 12 {
		t.Fatalf("expected hashed fallback for bare host URL, got %q", hashed)
	}
}

func TestUniqueOutputPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "track.mp3")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := uniqueOutputPath(path)
	want := filepath.Join(dir, "track-1.mp3")
	if got != want {
		t.Fatalf("uniqueOutputPath() = %q, want %q", got, want)
	}
}

func TestResolveAudioOutputPath(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveAudioOutputPath(dir, "", "https://youtu.be/X98VPQCE_WI", "Cool Video!", false)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "Cool_Video!.mp3" {
		t.Fatalf("unexpected path %q", got)
	}

	got, err = resolveAudioOutputPath(dir, "custom", "https://youtu.be/X98VPQCE_WI", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "custom.mp3" {
		t.Fatalf("outfile without ext: got %q", got)
	}

	_, err = resolveAudioOutputPath(dir, "../evil.mp3", "https://youtu.be/X98VPQCE_WI", "", true)
	if err == nil {
		t.Fatal("expected path escape in outfile to fail")
	}

	got, err = resolveAudioOutputPath("", "only-name.mp3", "https://youtu.be/X98VPQCE_WI", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "only-name.mp3" && got != filepath.Join(".", "only-name.mp3") {
		// filepath.Join(".", "only-name.mp3") == "only-name.mp3" on Unix
		if filepath.Base(got) != "only-name.mp3" {
			t.Fatalf("outdir empty with outfile: got %q", got)
		}
	}
}
