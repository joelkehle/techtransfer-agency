package operator

import (
	"strings"
	"testing"
)

func TestApplyPrintLayoutHooksAddsPageBreakBeforeHowThisReportWorks(t *testing.T) {
	in := "<h2>Executive Summary</h2><p>x</p><h2>How This Report Works</h2><p>y</p>"
	out := applyPrintLayoutHooks(in)
	if !strings.Contains(out, `<h2 data-page-break-before="true">How This Report Works</h2>`) {
		t.Fatalf("expected page-break class injection, got: %s", out)
	}
}

func TestApplyPrintLayoutHooksNoopWhenHeadingMissing(t *testing.T) {
	in := "<h2>Executive Summary</h2><p>x</p>"
	out := applyPrintLayoutHooks(in)
	if out != in {
		t.Fatalf("expected no change when heading absent, got: %s", out)
	}
}

func TestApplyPrintLayoutHooksMarksStageHeadings(t *testing.T) {
	in := "<h2>Stage 1: Invention Extraction</h2><p>x</p>"
	out := applyPrintLayoutHooks(in)
	if !strings.Contains(out, `<h2 data-stage-heading="true">Stage 1: Invention Extraction</h2>`) {
		t.Fatalf("expected stage heading hook injection, got: %s", out)
	}
}

func TestBuildMetaHTMLPriorArtFallbackFields(t *testing.T) {
	env := map[string]any{
		"agent":        "prior-art-search",
		"case_id":      "SUB-123",
		"ucla_case_id": "2026-124",
		"pi_name":      "Dr. Jane Smith",
		"structured_results": map[string]any{
			"search_strategy": map[string]any{
				"invention_title": "ROCIT",
			},
		},
		"metadata": map[string]any{
			"completed_at": "2026-03-04T02:48:53Z",
		},
	}
	out := buildMetaHTML(env)
	if strings.Contains(out, "SUB-123") || strings.Contains(out, "Reference:") {
		t.Fatalf("did not expect opaque submission token or generic reference label, got: %s", out)
	}
	if !strings.Contains(out, "UCLA Case ID:") || !strings.Contains(out, "2026-124") {
		t.Fatalf("expected UCLA case id in meta html, got: %s", out)
	}
	if !strings.Contains(out, "PI:") || !strings.Contains(out, "Dr. Jane Smith") {
		t.Fatalf("expected PI name in meta html, got: %s", out)
	}
	if !strings.Contains(out, "Invention:") || !strings.Contains(out, "ROCIT") {
		t.Fatalf("expected invention fallback in meta html, got: %s", out)
	}
	if !strings.Contains(out, "Date:") {
		t.Fatalf("expected date fallback in meta html, got: %s", out)
	}
}

func TestNormalizeReportMarkdownPriorArtHeader(t *testing.T) {
	env := map[string]any{"agent": "prior-art-search"}
	in := strings.Join([]string{
		"# Prior Art Search Report",
		"",
		"- Case ID: SUB-1",
		"- Invention: Test",
		"- Date: 2026-03-04T02:48:53Z",
		"",
		"DISCLAIMER: demo",
		"",
		"## Executive Summary",
	}, "\n")
	out := normalizeReportMarkdown(env, in)
	if strings.Contains(out, "- Case ID:") || strings.Contains(out, "- Invention:") || strings.Contains(out, "- Date:") {
		t.Fatalf("expected metadata bullets stripped, got: %s", out)
	}
	if !strings.Contains(out, "DISCLAIMER: demo") {
		t.Fatalf("expected disclaimer to remain, got: %s", out)
	}
	if !strings.HasPrefix(out, "# Prior Art Search Report\n\nDISCLAIMER:") {
		t.Fatalf("expected normalized header spacing, got: %q", out)
	}
}
