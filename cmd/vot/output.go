package main

import (
	"encoding/json"
	"fmt"
	"io"
)

// translateItemResult is one per-URL entry in --json output.
type translateItemResult struct {
	Input        string  `json:"input"`
	Status       string  `json:"status"`
	Type         string  `json:"type"`
	VideoID      *string `json:"videoId"`
	URL          *string `json:"url"`
	OutputPath   *string `json:"outputPath"`
	Error        *string `json:"error,omitempty"`
	DetectedLang string  `json:"detectedLang,omitempty"`
	Backend      string  `json:"backend,omitempty"`
	VoiceStyle   string  `json:"voiceStyle,omitempty"`
}

type translateSummary struct {
	Total   int `json:"total"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
}

type translateBatchOutput struct {
	OK      bool                  `json:"ok"`
	Summary translateSummary      `json:"summary"`
	Results []translateItemResult `json:"results"`
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func buildTranslateBatchOutput(results []translateItemResult) translateBatchOutput {
	success := 0
	failed := 0
	for _, r := range results {
		switch r.Status {
		case "success":
			success++
		case "failed":
			failed++
		}
	}
	total := len(results)
	// Explained-only batches count as ok when nothing failed.
	return translateBatchOutput{
		OK: failed == 0 && total > 0,
		Summary: translateSummary{
			Total:   total,
			Success: success,
			Failed:  failed,
		},
		Results: results,
	}
}

func writeTranslateJSON(w io.Writer, results []translateItemResult) error {
	out := buildTranslateBatchOutput(results)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}

func emitPlainTranslateLine(w io.Writer, audioURL, outputPath string, keepURL bool) {
	switch {
	case outputPath != "" && keepURL && audioURL != "":
		fmt.Fprintf(w, "%s\t%s\n", outputPath, audioURL)
	case outputPath != "":
		fmt.Fprintln(w, outputPath)
	default:
		fmt.Fprintln(w, audioURL)
	}
}
