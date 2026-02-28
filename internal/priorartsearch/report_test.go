package priorartsearch

import (
	"strings"
	"testing"
)

func TestReportNormalIncludesLandscape(t *testing.T) {
	res := basePipelineResult()
	res.Stage3 = &Stage3Output{Assessments: []PatentAssessment{{PatentID: "123", Relevance: RelevanceHigh, OverlapDescription: "overlap overlap overlap", ConfidenceScore: 0.8}}}
	res.Stage4 = &Stage4Output{DeterminationReasoning: "reason", Determination: DeterminationBlockingArt, BlockingRisk: BlockingRisk{Level: BlockingRiskHigh}, DesignAroundPotential: DesignAroundPotential{Level: DesignAroundDifficult}, LandscapeDensity: DensityDense, LandscapeDensityReasoning: "dense", WhiteSpace: []string{"foo"}}
	res.Determination = DeterminationBlockingArt
	res.NovelCoverage = []NovelElementCoverage{{ID: "NE1", Description: "desc", TotalCount: 1}}

	r := BuildReportMarkdown(res)
	if !strings.Contains(r, "Landscape Analysis") {
		t.Fatalf("expected landscape section")
	}
}

func TestReportStage3Degraded(t *testing.T) {
	res := basePipelineResult()
	res.Stage3 = nil
	reason := "stage3 failed"
	res.Metadata.DegradedReason = &reason
	r := BuildReportMarkdown(res)
	if !strings.Contains(r, "Raw Results") {
		t.Fatalf("expected raw results section")
	}
}

func TestReportStage4DegradedLayout(t *testing.T) {
	res := basePipelineResult()
	res.Stage3 = &Stage3Output{Assessments: []PatentAssessment{{PatentID: "123", Relevance: RelevanceMedium, OverlapDescription: "overlap overlap overlap"}}}
	res.Stage4 = nil
	reason := "stage4 failed"
	res.Metadata.DegradedReason = &reason
	res.AssigneeFrequency = []AssigneeCount{{Name: "Org", Count: 2}}
	res.CPCHistogram = []CPCCount{{Subclass: "G06N", Count: 3}}
	res.NovelCoverage = []NovelElementCoverage{{ID: "NE1", Description: "desc", TotalCount: 0}}
	r := BuildReportMarkdown(res)
	if !strings.Contains(r, "Code-Generated Landscape Statistics") {
		t.Fatalf("expected stage4 degraded landscape section")
	}
}

func basePipelineResult() PipelineResult {
	return PipelineResult{
		Request: RequestEnvelope{CaseID: "case-1"},
		Stage1: Stage1Output{
			InventionTitle: "Test Invention",
			QueryStrategies: []QueryStrategy{{
				ID:          "Q1",
				Priority:    PriorityPrimary,
				Description: "strategy description",
				TermFamilies: []TermFamily{{
					Canonical: "federated",
				}},
			}},
		},
		Stage2:   Stage2Output{Patents: []PatentResult{{PatentID: "123", Title: "Patent", GrantDate: "2024-01-01", Assignees: []string{"Org"}, Abstract: "abc"}}},
		Metadata: PipelineMetadata{StagesExecuted: []string{"stage_1"}, Model: "m"},
	}
}
