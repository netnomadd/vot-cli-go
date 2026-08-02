package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

var (
	unsafeFileNameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]+`)
	multiUnderscores    = regexp.MustCompile(`_+`)
	youtubeIDRe         = regexp.MustCompile(`(?i)^[a-zA-Z0-9_-]{11}$`)
)

// downloadHTTPFile streams fileURL into destPath using a temp file + rename.
func downloadHTTPFile(ctx context.Context, fileURL, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return fmt.Errorf("build download request: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download failed with status %s", resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(destPath), ".vot-download-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return fmt.Errorf("write download: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpName, destPath); err != nil {
		// Cross-device rename may fail; fall back to copy.
		if copyErr := copyFile(tmpName, destPath); copyErr != nil {
			return fmt.Errorf("move download into place: %w (copy fallback: %v)", err, copyErr)
		}
		_ = os.Remove(tmpName)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// sanitizeFileName strips characters that are unsafe in file names.
func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	name = unsafeFileNameChars.ReplaceAllString(name, "_")
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		switch r {
		case '\n', '\r', '\t':
			return -1
		default:
			return r
		}
	}, name)
	name = strings.ReplaceAll(name, " ", "_")
	name = multiUnderscores.ReplaceAllString(name, "_")
	name = strings.Trim(name, "._ ")
	if name == "" || name == "." || name == ".." {
		return "audio"
	}
	// Keep names reasonably short for common filesystems.
	const maxLen = 120
	if len(name) > maxLen {
		name = strings.TrimRight(name[:maxLen], "._ ")
		if name == "" {
			return "audio"
		}
	}
	return name
}

// uniqueOutputPath returns destPath, or destPath with -N before the extension
// if the path already exists.
func uniqueOutputPath(destPath string) string {
	if _, err := os.Stat(destPath); err != nil {
		return destPath
	}
	ext := filepath.Ext(destPath)
	base := strings.TrimSuffix(destPath, ext)
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(candidate); err != nil {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d%s", base, time.Now().Unix(), ext)
}

// extractVideoID returns a stable short identifier for naming output files.
func extractVideoID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return shortHash(rawURL)
	}

	host := strings.ToLower(u.Hostname())
	switch {
	case host == "youtu.be":
		id := strings.Trim(strings.TrimPrefix(u.Path, "/"), "/")
		if i := strings.IndexByte(id, '/'); i >= 0 {
			id = id[:i]
		}
		if youtubeIDRe.MatchString(id) {
			return id
		}
	case strings.Contains(host, "youtube.com"):
		if id := u.Query().Get("v"); youtubeIDRe.MatchString(id) {
			return id
		}
		// /shorts/ID, /embed/ID, /live/ID
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 2 {
			switch strings.ToLower(parts[0]) {
			case "shorts", "embed", "live", "v":
				if youtubeIDRe.MatchString(parts[1]) {
					return parts[1]
				}
			}
		}
	}

	base := filepath.Base(u.Path)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = sanitizeFileName(base)
	if base != "" && base != "audio" && base != "watch" {
		return base
	}
	return shortHash(rawURL)
}

func shortHash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

// resolveAudioOutputPath builds the destination path for a downloaded audio file.
func resolveAudioOutputPath(outdir, outfile, inputURL, title string, noTitle bool) (string, error) {
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
			name += ".mp3"
		}
	} else {
		base := ""
		if !noTitle {
			base = sanitizeFileName(title)
		}
		if base == "" || base == "audio" {
			base = extractVideoID(inputURL)
		}
		name = base + ".mp3"
	}

	// Prevent path escape via outfile.
	if filepath.IsAbs(name) || strings.Contains(name, string(os.PathSeparator)) || strings.Contains(name, "/") {
		return "", fmt.Errorf("--outfile must be a bare file name, not a path")
	}

	return uniqueOutputPath(filepath.Join(dir, name)), nil
}

// getVideoTitleWithYtDLP asks yt-dlp for a human-readable title.
func getVideoTitleWithYtDLP(videoURL, cookiesFile, cookiesFromBrowser string) (string, error) {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return "", fmt.Errorf("yt-dlp not found in PATH: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	args := []string{"--print", "title", "--no-playlist"}
	if cookiesFile != "" {
		args = append(args, "--cookies", cookiesFile)
	}
	if cookiesFromBrowser != "" {
		args = append(args, "--cookies-from-browser", cookiesFromBrowser)
	}
	args = append(args, videoURL)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("yt-dlp failed to get title: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	raw := strings.TrimSpace(string(output))
	if raw == "" {
		return "", fmt.Errorf("yt-dlp returned empty title")
	}

	// Prefer the last non-empty line; warnings often precede the title.
	lines := strings.Split(raw, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		// Skip obvious warning prefixes.
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "warning:") || strings.HasPrefix(lower, "error:") {
			continue
		}
		return line, nil
	}
	return "", fmt.Errorf("yt-dlp did not return a title (output: %s)", raw)
}
