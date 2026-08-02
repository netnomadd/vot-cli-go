package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildTranslateBatchOutput(t *testing.T) {
	out := buildTranslateBatchOutput([]translateItemResult{
		{Input: "a", Status: "success", Type: "audio", URL: strPtr("http://x")},
		{Input: "b", Status: "failed", Type: "audio", Error: strPtr("boom")},
	})
	if out.OK {
		t.Fatal("expected ok=false when any result failed")
	}
	if out.Summary.Total != 2 || out.Summary.Success != 1 || out.Summary.Failed != 1 {
		t.Fatalf("unexpected summary: %+v", out.Summary)
	}

	explained := buildTranslateBatchOutput([]translateItemResult{
		{Input: "a", Status: "explained", Type: "audio"},
	})
	if !explained.OK {
		t.Fatal("explained-only batch should be ok")
	}
	if explained.Summary.Success != 0 || explained.Summary.Failed != 0 {
		t.Fatalf("explained should not count as success/failed: %+v", explained.Summary)
	}
}

func TestWriteTranslateJSON(t *testing.T) {
	var buf bytes.Buffer
	err := writeTranslateJSON(&buf, []translateItemResult{
		{
			Input:      "https://youtu.be/X98VPQCE_WI",
			Status:     "success",
			Type:       "audio",
			VideoID:    strPtr("X98VPQCE_WI"),
			URL:        strPtr("https://example.com/a.mp3"),
			OutputPath: strPtr("/tmp/a.mp3"),
			Backend:    "worker",
			VoiceStyle: "live",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var parsed translateBatchOutput
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, buf.String())
	}
	if !parsed.OK || parsed.Summary.Success != 1 {
		t.Fatalf("unexpected payload: %+v", parsed)
	}
	if parsed.Results[0].URL == nil || *parsed.Results[0].URL != "https://example.com/a.mp3" {
		t.Fatalf("url missing: %+v", parsed.Results[0])
	}
}

func TestEmitPlainTranslateLine(t *testing.T) {
	var buf bytes.Buffer
	emitPlainTranslateLine(&buf, "http://audio", "/tmp/a.mp3", true)
	got := buf.String()
	if got != "/tmp/a.mp3\thttp://audio\n" {
		t.Fatalf("keep-url line = %q", got)
	}

	buf.Reset()
	emitPlainTranslateLine(&buf, "http://audio", "/tmp/a.mp3", false)
	if buf.String() != "/tmp/a.mp3\n" {
		t.Fatalf("path-only line = %q", buf.String())
	}

	buf.Reset()
	emitPlainTranslateLine(&buf, "http://audio", "", false)
	if !strings.HasPrefix(buf.String(), "http://audio") {
		t.Fatalf("url-only line = %q", buf.String())
	}
}
