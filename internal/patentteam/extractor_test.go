package patentteam

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttachmentFilePath(t *testing.T) {
	p, err := AttachmentFilePath(Attachment{URL: "file:///tmp/example.pdf"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != "/tmp/example.pdf" {
		t.Fatalf("path=%q", p)
	}
}

func TestExtractPDFTextFallbackFromBytes(t *testing.T) {
	t.Setenv("DOC_CACHE_PATH", filepath.Join(t.TempDir(), "missing-doc-cache"))

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.pdf")
	body := []byte("%PDF-1.4\nThis invention includes an algorithm and protocol for sensor fusion with low latency.\n%%EOF")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	out, err := ExtractPDFText(t.Context(), path)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !strings.Contains(strings.ToLower(out.Text), "algorithm") {
		t.Fatalf("unexpected extraction output: %q", out.Text)
	}
}
