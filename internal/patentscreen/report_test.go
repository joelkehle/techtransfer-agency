package patentscreen

import (
	"strings"
	"testing"
)

func TestBuildResponseIncludesDisclaimer(t *testing.T) {
	summary := "claim"
	res := PipelineResult{
		FinalDetermination: DeterminationLikelyEligible,
		Pathway:            PathwayB1,
		Request:            RequestEnvelope{CaseID: "CASE-1"},
		Stage1: Stage1Output{
			InventionTitle:  "Title",
			ClaimsPresent:   true,
			ClaimsSummary:   &summary,
			StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "confidence high"},
		},
		Stage6:   Stage6Output{PriorArtSearchPriority: PriorityLow, Reasoning: "reasoning long enough to pass minimum requirements", StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "confidence high"}},
		Metadata: PipelineMetadata{},
	}
	env := BuildResponse(res)
	if env.Disclaimer != Disclaimer {
		t.Fatalf("expected disclaimer constant")
	}
	if env.ReportMarkdown == "" {
		t.Fatal("expected report markdown")
	}
	if !strings.Contains(env.ReportMarkdown, "## Appendix") {
		t.Fatal("expected appendix section in markdown")
	}
	if _, ok := env.StageOutputs["stage_4"]; ok {
		t.Fatal("did not expect skipped stage_4 in stage outputs")
	}
}
