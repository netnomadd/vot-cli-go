package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateSubsFormat(t *testing.T) {
	got, err := validateSubsFormat("SRT")
	if err != nil || got != "srt" {
		t.Fatalf("got %q err=%v", got, err)
	}
	if _, err := validateSubsFormat("ass"); err == nil {
		t.Fatal("expected error for ass")
	}
}

func TestConvertSubsFromJSON(t *testing.T) {
	data := yandexSubsJSON{
		Subtitles: []yandexSubsCue{
			{Text: "Hello", StartMs: 1000, DurationMs: 1500},
			{Text: "World", StartMs: 3000, DurationMs: 1000},
		},
	}
	srt := convertSubsFromJSON(data, "srt")
	if !strings.Contains(srt, "1\n") || !strings.Contains(srt, "00:00:01,000 --> 00:00:02,500") || !strings.Contains(srt, "Hello") {
		t.Fatalf("unexpected srt:\n%s", srt)
	}
	vtt := convertSubsFromJSON(data, "vtt")
	if !strings.HasPrefix(vtt, "WEBVTT") || !strings.Contains(vtt, "00:00:01.000 --> 00:00:02.500") {
		t.Fatalf("unexpected vtt:\n%s", vtt)
	}

	raw, _ := json.Marshal(data)
	var parsed yandexSubsJSON
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if convertSubsFromJSON(parsed, "json") == "" {
		// convertSubsFromJSON is only for srt/vtt; just ensure parse roundtrip works
	}
}

func TestPickViaResolveSubsOutputPath(t *testing.T) {
	got, err := resolveSubsOutputPath("/tmp/out", "", "https://youtu.be/X98VPQCE_WI", "", "srt", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "X98VPQCE_WI.srt") {
		t.Fatalf("got %q", got)
	}
}
