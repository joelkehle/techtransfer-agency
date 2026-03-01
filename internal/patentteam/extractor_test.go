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

func TestExtractCaseNumber(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "labeled uc case number",
			text: "Invention Disclosure\nUC Case Number: 2023-107\nTitle: Sensor fusion platform",
			want: "2023-107",
		},
		{
			name: "labeled case no alphanumeric",
			text: "Case No. ABC-7782\nDisclosed by inventors...",
			want: "ABC-7782",
		},
		{
			name: "fallback year pattern",
			text: "Reference in header 2024-998 and technical details follow",
			want: "2024-998",
		},
		{
			name: "not found",
			text: "This text has no obvious case number",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractCaseNumber(tt.text)
			if got != tt.want {
				t.Fatalf("ExtractCaseNumber()=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestResolveCaseID(t *testing.T) {
	text := "UC Case Number: 2023-107"

	if got := ResolveCaseID("SUB-12345", text); got != "2023-107" {
		t.Fatalf("expected SUB fallback to extracted case number, got %q", got)
	}
	if got := ResolveCaseID("UCLA-999", text); got != "UCLA-999" {
		t.Fatalf("expected explicit non-SUB case id preserved, got %q", got)
	}
	if got := ResolveCaseID("", text); got != "2023-107" {
		t.Fatalf("expected empty case id to use extracted case number, got %q", got)
	}
	if got := ResolveCaseID("SUB-12345", "no case number here"); got != "SUB-12345" {
		t.Fatalf("expected original case id when no extracted number, got %q", got)
	}
}
