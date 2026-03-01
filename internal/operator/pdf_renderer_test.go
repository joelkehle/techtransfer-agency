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
