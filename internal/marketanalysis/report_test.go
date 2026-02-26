package marketanalysis

import (
	"strings"
	"testing"
)

func TestBuildResponseIncludesModeAndRecommendation(t *testing.T) {
	res := PipelineResult{
		Request:  RequestEnvelope{CaseID: "CASE-1"},
		Stage0:   Stage0Output{InventionTitle: NullableField{Value: "Test invention", Confidence: ConfidenceHigh}},
		Decision: RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceLow, Reason: "Insufficient data"},
		Metadata: PipelineMetadata{Mode: ReportModeDegraded},
	}
	env := BuildResponse(res)
	if env.Recommendation != RecommendationDefer {
		t.Fatalf("unexpected recommendation: %s", env.Recommendation)
	}
	if env.ReportMode != ReportModeDegraded {
		t.Fatalf("unexpected report mode: %s", env.ReportMode)
	}
	if !strings.Contains(env.ReportMarkdown, "DEGRADED") {
		t.Fatal("expected degraded warning in markdown")
	}
}
