package patentteam

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	maxPDFBytes = 20 * 1024 * 1024
	maxTextRun  = 24000
)

type ExtractionResult struct {
	Text      string
	Method    string
	Truncated bool
}

func ExtractPDFText(ctx context.Context, path string) (ExtractionResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ExtractionResult{}, err
	}
	if info.Size() > maxPDFBytes {
		return ExtractionResult{}, fmt.Errorf("pdf too large: %d bytes", info.Size())
	}

	if text, err := runDocCache(ctx, path); err == nil && strings.TrimSpace(text) != "" {
		return truncateExtraction(text, "doc-cache"), nil
	}

	if text, err := runPdfToText(ctx, path); err == nil && strings.TrimSpace(text) != "" {
		return truncateExtraction(text, "pdftotext"), nil
	}

	blob, err := os.ReadFile(path)
	if err != nil {
		return ExtractionResult{}, err
	}
	fallback := extractPrintableText(blob)
	if strings.TrimSpace(fallback) == "" {
		return ExtractionResult{}, errors.New("no extractable text found")
	}
	return truncateExtraction(fallback, "byte-fallback"), nil
}

func runDocCache(ctx context.Context, path string) (string, error) {
	cmdPath := os.Getenv("DOC_CACHE_PATH")
	if strings.TrimSpace(cmdPath) == "" {
		cmdPath = "doc-cache"
	}
	cmd := exec.CommandContext(ctx, cmdPath, "get", path)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func runPdfToText(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, "pdftotext", "-layout", path, "-")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func extractPrintableText(blob []byte) string {
	var runs []string
	var b strings.Builder
	flush := func() {
		s := strings.TrimSpace(b.String())
		if len(s) >= 24 {
			runs = append(runs, s)
		}
		b.Reset()
	}
	for _, c := range blob {
		r := rune(c)
		if unicode.IsPrint(r) || r == '\n' || r == '\t' || r == '\r' {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	joined := strings.Join(runs, "\n")
	joined = strings.ReplaceAll(joined, "\x00", "")
	joined = strings.TrimSpace(joined)
	return joined
}

func truncateExtraction(text, method string) ExtractionResult {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= maxTextRun {
		return ExtractionResult{Text: trimmed, Method: method}
	}
	prefix := trimmed[:maxTextRun]
	// Avoid cutting in the middle of a rune sequence.
	prefix = string(bytes.Runes([]byte(prefix)))
	return ExtractionResult{
		Text:      prefix + "\n\n[TRUNCATED]",
		Method:    method,
		Truncated: true,
	}
}

func AttachmentFilePath(att Attachment) (string, error) {
	if strings.TrimSpace(att.URL) == "" {
		return "", errors.New("attachment url is required")
	}
	if strings.HasPrefix(att.URL, "file://") {
		p := strings.TrimPrefix(att.URL, "file://")
		if p == "" {
			return "", errors.New("file attachment path is empty")
		}
		return p, nil
	}
	if filepath.IsAbs(att.URL) {
		return att.URL, nil
	}
	return "", fmt.Errorf("unsupported attachment url scheme: %s", att.URL)
}
