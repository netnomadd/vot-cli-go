package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/netnomadd/vot-cli-go/internal/proxyutil"
)

// yandexSubsJSON is the JSON payload served at Yandex subtitle URLs.
type yandexSubsJSON struct {
	ContainsTokens bool            `json:"containsTokens"`
	Subtitles      []yandexSubsCue `json:"subtitles"`
}

type yandexSubsCue struct {
	Text       string `json:"text"`
	StartMs    int64  `json:"startMs"`
	DurationMs int64  `json:"durationMs"`
	SpeakerID  string `json:"speakerId"`
}

func validateSubsFormat(format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "srt":
		return "srt", nil
	case "vtt":
		return "vtt", nil
	case "json":
		return "json", nil
	default:
		return "", fmt.Errorf("unsupported --subs-format %q (expected srt, vtt, or json)", format)
	}
}

func convertToStrTime(ms int64, delimiter string) string {
	if ms < 0 {
		ms = 0
	}
	hours := ms / 3_600_000
	minutes := (ms % 3_600_000) / 60_000
	seconds := (ms % 60_000) / 1000
	millis := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d%s%03d", hours, minutes, seconds, delimiter, millis)
}

func convertSubsFromJSON(data yandexSubsJSON, format string) string {
	isVTT := format == "vtt"
	delimiter := ","
	if isVTT {
		delimiter = "."
	}
	var b strings.Builder
	if isVTT {
		b.WriteString("WEBVTT\n\n")
	}
	for i, cue := range data.Subtitles {
		if !isVTT {
			fmt.Fprintf(&b, "%d\n", i+1)
		}
		start := convertToStrTime(cue.StartMs, delimiter)
		end := convertToStrTime(cue.StartMs+cue.DurationMs, delimiter)
		fmt.Fprintf(&b, "%s --> %s\n%s\n\n", start, end, cue.Text)
	}
	return strings.TrimSpace(b.String())
}

// downloadSubtitleFile downloads a Yandex subtitle URL and writes it in the
// requested format (json/srt/vtt).
func downloadSubtitleFile(ctx context.Context, fileURL, destPath, format, proxyURL string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return fmt.Errorf("build subtitle request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	if err := proxyutil.ApplyHTTPClient(client, proxyURL); err != nil {
		return fmt.Errorf("configure subtitle proxy: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("subtitle download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("subtitle download failed with status %s", resp.Status)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read subtitle body: %w", err)
	}

	var content string
	switch format {
	case "json":
		content = string(raw)
	default:
		var parsed yandexSubsJSON
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return fmt.Errorf("parse subtitle JSON: %w", err)
		}
		content = convertSubsFromJSON(parsed, format)
	}

	tmp, err := os.CreateTemp(filepath.Dir(destPath), ".vot-subs-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.WriteString(content); err != nil {
		return fmt.Errorf("write subtitle: %w", err)
	}
	if !strings.HasSuffix(content, "\n") {
		if _, err := tmp.WriteString("\n"); err != nil {
			return fmt.Errorf("write subtitle newline: %w", err)
		}
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, destPath); err != nil {
		if copyErr := copyFile(tmpName, destPath); copyErr != nil {
			return fmt.Errorf("move subtitle into place: %w (copy fallback: %v)", err, copyErr)
		}
		_ = os.Remove(tmpName)
	}
	return nil
}

// resolveSubsOutputPath builds the destination path for downloaded subtitles.
func resolveSubsOutputPath(outdir, outfile, inputURL, title, format string, noTitle bool) (string, error) {
	if outdir == "" && outfile == "" {
		return "", fmt.Errorf("no output path requested")
	}
	dir := outdir
	if dir == "" {
		dir = "."
	}

	var name string
	if outfile != "" {
		name = outfile
		if filepath.Ext(name) == "" {
			name += "." + format
		}
	} else {
		base := ""
		if !noTitle {
			base = sanitizeFileName(title)
		}
		if base == "" || base == "audio" {
			base = extractVideoID(inputURL)
		}
		name = base + "." + format
	}

	if filepath.IsAbs(name) || strings.Contains(name, string(os.PathSeparator)) || strings.Contains(name, "/") {
		return "", fmt.Errorf("--outfile must be a bare file name, not a path")
	}
	return uniqueOutputPath(filepath.Join(dir, name)), nil
}
