package priorartsearch

import (
	"context"
	"testing"
)

func TestStage1AcceptTrim(t *testing.T) {
	s := Stage1Output{
		InventionTitle:    stringsLen(250),
		InventionSummary:  stringsLen(600),
		NovelElements:     []Stage1NovelElement{{ID: "NE1", Description: stringsLen(400)}, {ID: "NE2", Description: stringsLen(40)}, {ID: "NE3", Description: stringsLen(40)}},
		TechnologyDomains: []string{"AI"},
		QueryStrategies: []QueryStrategy{
			{
				ID:          "Q1",
				Description: stringsLen(250),
				Priority:    "primary",
				TermFamilies: []TermFamily{{
					Canonical: "federated", Synonyms: []string{"distributed"},
				}},
			},
			{
				ID:          "Q2",
				Description: stringsLen(40),
				Priority:    "secondary",
				TermFamilies: []TermFamily{{
					Canonical: "privacy", Synonyms: []string{"secure"},
				}},
			},
			{
				ID:          "Q3",
				Description: stringsLen(40),
				Priority:    "tertiary",
				TermFamilies: []TermFamily{{
					Canonical: "aggregation", Synonyms: []string{"combining"},
				}},
			},
		},
		ConfidenceScore:  0.9,
		ConfidenceReason: stringsLen(20),
	}
	if err := validateAndNormalizeStage1(&s); err != nil {
		t.Fatalf("validateAndNormalizeStage1: %v", err)
	}
	if len(s.InventionTitle) != 200 || len(s.InventionSummary) != 500 || len(s.NovelElements[0].Description) != 300 {
		t.Fatalf("expected trim behavior, got title=%d summary=%d ne=%d", len(s.InventionTitle), len(s.InventionSummary), len(s.NovelElements[0].Description))
	}
}

func TestStage4BlockingMembershipValidation(t *testing.T) {
	s2 := Stage2Output{Patents: []PatentResult{{PatentID: "1"}, {PatentID: "2"}}}
	s3 := Stage3Output{Assessments: []PatentAssessment{{PatentID: "1", Relevance: RelevanceHigh}}}
	s4 := Stage4Output{
		LandscapeDensity:          DensityDense,
		LandscapeDensityReasoning: stringsLen(60),
		BlockingRisk:              BlockingRisk{Level: BlockingRiskHigh, BlockingPatents: []string{"2", "9"}, Reasoning: stringsLen(60)},
		DesignAroundPotential:     DesignAroundPotential{Level: DesignAroundModerate, Reasoning: stringsLen(60)},
		WhiteSpace:                []string{stringsLen(30)},
		Determination:             DeterminationCrowdedField,
		DeterminationReasoning:    stringsLen(60),
		ConfidenceScore:           0.8,
		ConfidenceReason:          stringsLen(20),
	}
	if err := validateAndNormalizeStage4(&s4, s2, s3); err != nil {
		t.Fatalf("validateAndNormalizeStage4: %v", err)
	}
	if len(s4.BlockingRisk.BlockingPatents) != 0 {
		t.Fatalf("expected dropped invalid blocking patents, got %v", s4.BlockingRisk.BlockingPatents)
	}
	if s4.Determination != DeterminationCrowdedField {
		t.Fatalf("unexpected determination override: %s", s4.Determination)
	}
}

func TestStage3MissingAssessmentsFallbackOnThirdFailure(t *testing.T) {
	caller := &fakeLLMCaller{
		responses: []string{
			`{"assessments":[{"patent_id":"P1","relevance":"HIGH","overlap_description":"` + stringsLen(30) + `","novel_elements_covered":["NE1"],"confidence_score":0.9}]}`,
			`{"assessments":[{"patent_id":"P1","relevance":"HIGH","overlap_description":"` + stringsLen(30) + `","novel_elements_covered":["NE1"],"confidence_score":0.9}]}`,
			`{"assessments":[{"patent_id":"P1","relevance":"HIGH","overlap_description":"` + stringsLen(30) + `","novel_elements_covered":["NE1"],"confidence_score":0.9}]}`,
		},
	}
	r := NewLLMStageRunner(NewStageExecutor(caller), 75, 15)
	s1 := Stage1Output{
		InventionSummary: "summary",
		NovelElements:    []Stage1NovelElement{{ID: "NE1", Description: stringsLen(30)}},
	}
	s2 := Stage2Output{Patents: []PatentResult{
		{PatentID: "P1", Abstract: "a", GrantDate: "2024-01-01"},
		{PatentID: "P2", Abstract: "a", GrantDate: "2023-01-01"},
	}}
	out, _, _, err := r.RunStage3(context.Background(), s1, s2)
	if err != nil {
		t.Fatalf("RunStage3: %v", err)
	}
	if len(out.Assessments) != 1 || out.Assessments[0].PatentID != "P1" {
		t.Fatalf("expected only non-NONE assessment retained, got %+v", out.Assessments)
	}
}

func TestStage3DropsExtraAndInvalidNEIDs(t *testing.T) {
	caller := &fakeLLMCaller{
		responses: []string{
			`{"assessments":[
{"patent_id":"P1","relevance":"MEDIUM","overlap_description":"` + stringsLen(30) + `","novel_elements_covered":["NE1","NE99"],"confidence_score":0.7},
{"patent_id":"PX","relevance":"HIGH","overlap_description":"` + stringsLen(30) + `","novel_elements_covered":["NE1"],"confidence_score":0.7}
]}`,
		},
	}
	r := NewLLMStageRunner(NewStageExecutor(caller), 75, 15)
	s1 := Stage1Output{
		InventionSummary: "summary",
		NovelElements:    []Stage1NovelElement{{ID: "NE1", Description: stringsLen(30)}},
	}
	s2 := Stage2Output{Patents: []PatentResult{{PatentID: "P1", Abstract: "a", GrantDate: "2024-01-01"}}}
	out, _, _, err := r.RunStage3(context.Background(), s1, s2)
	if err != nil {
		t.Fatalf("RunStage3: %v", err)
	}
	if len(out.Assessments) != 1 {
		t.Fatalf("expected one assessment, got %d", len(out.Assessments))
	}
	if len(out.Assessments[0].NovelElementsCovered) != 1 || out.Assessments[0].NovelElementsCovered[0] != "NE1" {
		t.Fatalf("expected invalid NE dropped, got %v", out.Assessments[0].NovelElementsCovered)
	}
}

func TestStage4KeyPlayerCountAttachment(t *testing.T) {
	s2 := Stage2Output{Patents: []PatentResult{
		{PatentID: "1", Assignees: []string{"Google LLC"}},
		{PatentID: "2", Assignees: []string{"Google LLC"}},
		{PatentID: "3", Assignees: []string{"MIT"}},
	}}
	s3 := Stage3Output{Assessments: []PatentAssessment{
		{PatentID: "1", Relevance: RelevanceHigh},
		{PatentID: "2", Relevance: RelevanceMedium},
		{PatentID: "3", Relevance: RelevanceMedium},
	}}
	s4 := Stage4Output{
		LandscapeDensity:          DensityDense,
		LandscapeDensityReasoning: stringsLen(60),
		KeyPlayers:                []KeyPlayer{{Name: "Google LLC", RelevanceNote: stringsLen(30)}},
		BlockingRisk:              BlockingRisk{Level: BlockingRiskLow, Reasoning: stringsLen(60)},
		DesignAroundPotential:     DesignAroundPotential{Level: DesignAroundModerate, Reasoning: stringsLen(60)},
		WhiteSpace:                []string{stringsLen(30)},
		Determination:             DeterminationCrowdedField,
		DeterminationReasoning:    stringsLen(60),
		ConfidenceScore:           0.8,
		ConfidenceReason:          stringsLen(20),
	}
	if err := validateAndNormalizeStage4(&s4, s2, s3); err != nil {
		t.Fatalf("validateAndNormalizeStage4: %v", err)
	}
	if s4.KeyPlayers[0].PatentCount != 2 {
		t.Fatalf("expected attached patent_count=2, got %d", s4.KeyPlayers[0].PatentCount)
	}
}

func stringsLen(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}
