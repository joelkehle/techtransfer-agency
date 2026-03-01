package patentscreen

import (
	"strings"
	"testing"
	"time"
)

func TestPipelineResultFromResponseEnvelope(t *testing.T) {
	summary := "claim summary"
	env := ResponseEnvelope{
		CaseID:        "2023-107",
		Determination: DeterminationLikelyEligible,
		Pathway:       string(PathwayB2),
		PipelineMetadata: PipelineMetadata{
			CompletedAt: time.Date(2026, 3, 1, 3, 52, 4, 0, time.UTC),
		},
		StageOutputs: map[string]any{
			"stage_1": Stage1Output{
				InventionTitle:  "Title",
				ClaimsPresent:   true,
				ClaimsSummary:   &summary,
				StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "ok"},
			},
			"stage_2": Stage2Output{
				PassesStep1: true,
			},
			"stage_3": Stage3Output{
				RecitesException: true,
				Reasoning:        "reasoning",
			},
			"stage_4": Stage4Output{
				IntegratesPracticalApplication: true,
				Reasoning:                      "reasoning",
			},
			"stage_6": Stage6Output{
				PriorArtSearchPriority: PriorityHigh,
				Reasoning:              "reasoning",
			},
		},
	}

	res, err := PipelineResultFromResponseEnvelope(env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Request.CaseID != "2023-107" {
		t.Fatalf("unexpected case id: %s", res.Request.CaseID)
	}
	if res.Pathway != PathwayB2 {
		t.Fatalf("unexpected pathway: %s", res.Pathway)
	}
	if res.Stage4 == nil || !res.Stage4.IntegratesPracticalApplication {
		t.Fatal("expected stage 4 to decode")
	}
	if res.Stage5 != nil {
		t.Fatal("expected stage 5 to remain nil")
	}
}

func TestPipelineResultFromResponseEnvelopeMissingRequiredStage(t *testing.T) {
	_, err := PipelineResultFromResponseEnvelope(ResponseEnvelope{
		CaseID:           "2023-107",
		Determination:    DeterminationLikelyEligible,
		Pathway:          string(PathwayB2),
		StageOutputs:     map[string]any{},
		PipelineMetadata: PipelineMetadata{},
	})
	if err == nil {
		t.Fatal("expected error for missing required stage output")
	}
	if !strings.Contains(err.Error(), "stage_1") {
		t.Fatalf("expected stage_1 error, got: %v", err)
	}
}

func TestRebuildResponseFromEnvelope(t *testing.T) {
	summary := "claim summary"
	env := ResponseEnvelope{
		CaseID:        "2023-107",
		Determination: DeterminationLikelyEligible,
		Pathway:       string(PathwayB2),
		PipelineMetadata: PipelineMetadata{
			CompletedAt: time.Date(2026, 3, 1, 3, 52, 4, 0, time.UTC),
		},
		StageOutputs: map[string]any{
			"stage_1": Stage1Output{
				InventionTitle:  "Title",
				ClaimsPresent:   true,
				ClaimsSummary:   &summary,
				StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "ok"},
			},
			"stage_2": Stage2Output{PassesStep1: true},
			"stage_3": Stage3Output{RecitesException: true, Reasoning: "reasoning"},
			"stage_4": Stage4Output{IntegratesPracticalApplication: true, Reasoning: "reasoning"},
			"stage_6": Stage6Output{
				PriorArtSearchPriority: PriorityHigh,
				Reasoning:              "reasoning",
				StageConfidence:        StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "ok"},
			},
		},
	}

	rebuilt, err := RebuildResponseFromEnvelope(env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rebuilt.ReportMarkdown == "" {
		t.Fatal("expected rebuilt markdown")
	}
	if strings.Contains(rebuilt.ReportMarkdown, "## Determination\n") {
		t.Fatal("did not expect determination section in rebuilt markdown")
	}
	if !strings.Contains(rebuilt.ReportMarkdown, "Although the invention involves a judicial exception, it integrates that exception into a practical application with real-world utility.") {
		t.Fatal("expected pathway explanation in rebuilt markdown")
	}
}
