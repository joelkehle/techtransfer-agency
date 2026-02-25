package patentscreen

import (
	"context"
	"testing"
)

type mockRunner struct {
	s1  Stage1Output
	s2  Stage2Output
	s3  Stage3Output
	s4  Stage4Output
	s5  Stage5Output
	s6  Stage6Output
	err map[string]error
}

func (m *mockRunner) RunStage1(context.Context, RequestEnvelope) (Stage1Output, StageAttemptMetrics, error) {
	return m.s1, StageAttemptMetrics{Attempts: 1}, m.err["stage_1"]
}
func (m *mockRunner) RunStage2(context.Context, Stage1Output) (Stage2Output, StageAttemptMetrics, error) {
	return m.s2, StageAttemptMetrics{Attempts: 1}, m.err["stage_2"]
}
func (m *mockRunner) RunStage3(context.Context, Stage1Output, Stage2Output) (Stage3Output, StageAttemptMetrics, error) {
	return m.s3, StageAttemptMetrics{Attempts: 1}, m.err["stage_3"]
}
func (m *mockRunner) RunStage4(context.Context, Stage1Output, Stage3Output) (Stage4Output, StageAttemptMetrics, error) {
	return m.s4, StageAttemptMetrics{Attempts: 1}, m.err["stage_4"]
}
func (m *mockRunner) RunStage5(context.Context, Stage1Output, Stage3Output, Stage4Output) (Stage5Output, StageAttemptMetrics, error) {
	return m.s5, StageAttemptMetrics{Attempts: 1}, m.err["stage_5"]
}
func (m *mockRunner) RunStage6(context.Context, Stage1Output) (Stage6Output, StageAttemptMetrics, error) {
	return m.s6, StageAttemptMetrics{Attempts: 1}, m.err["stage_6"]
}

func baseRequest() RequestEnvelope {
	return RequestEnvelope{CaseID: "CASE-1", DisclosureText: "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Vestibulum at velit vitae sem aliquam faucibus."}
}

func baseStage1() Stage1Output {
	summary := "claim one"
	return Stage1Output{
		InventionTitle:       "Invention Title",
		Abstract:             "This invention improves something in a concrete technical way.",
		ProblemSolved:        "It solves a concrete technical bottleneck in processing throughput.",
		InventionDescription: "A hardware-software pipeline with deterministic scheduling and hardware counters.",
		NovelElements:        []string{"deterministic scheduler with counter feedback"},
		TechnologyArea:       "software",
		ClaimsPresent:        true,
		ClaimsSummary:        &summary,
		StageConfidence:      StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "clear details provided", InsufficientInformation: false},
	}
}

func TestPipelinePathwayB1(t *testing.T) {
	r := &mockRunner{
		s1:  baseStage1(),
		s2:  Stage2Output{Categories: []Stage2Category{CategoryProcess}, Explanation: "fits process", PassesStep1: true, StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		s3:  Stage3Output{RecitesException: false, Reasoning: strings50(), MPEPReference: "MPEP 2106.04", StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		s6:  Stage6Output{PriorArtSearchPriority: PriorityMedium, Reasoning: strings50(), StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		err: map[string]error{},
	}
	p := NewPipeline(r)
	res, err := p.Run(context.Background(), baseRequest())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Pathway != PathwayB1 {
		t.Fatalf("expected PathwayB1, got %s", res.Pathway)
	}
	if res.FinalDetermination != DeterminationLikelyEligible {
		t.Fatalf("expected likely eligible, got %s", res.FinalDetermination)
	}
	if res.Stage6.PriorArtSearchPriority == "" {
		t.Fatal("expected stage 6 to run")
	}
	if res.Metadata.StageAttempts["stage_1"] == 0 {
		t.Fatal("expected stage attempt counters")
	}
	if _, ok := res.Metadata.StageContentRetries["stage_1"]; !ok {
		t.Fatal("expected stage content retry counters")
	}
}

func TestPipelineConfidenceOverride(t *testing.T) {
	r := &mockRunner{
		s1:  baseStage1(),
		s2:  Stage2Output{Categories: []Stage2Category{CategoryProcess}, Explanation: "fits process", PassesStep1: true, StageConfidence: StageConfidence{ConfidenceScore: 0.4, ConfidenceReason: "weak evidence", InsufficientInformation: false}},
		s3:  Stage3Output{RecitesException: false, Reasoning: strings50(), MPEPReference: "MPEP 2106.04", StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		s6:  Stage6Output{PriorArtSearchPriority: PriorityMedium, Reasoning: strings50(), StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		err: map[string]error{},
	}
	p := NewPipeline(r)
	res, err := p.Run(context.Background(), baseRequest())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.FinalDetermination != DeterminationNeedsFurtherReview {
		t.Fatalf("expected needs further review, got %s", res.FinalDetermination)
	}
	if len(res.Metadata.NeedsReviewReasons) == 0 {
		t.Fatal("expected review reasons")
	}
}

func TestPipelinePathwayA(t *testing.T) {
	r := &mockRunner{
		s1:  baseStage1(),
		s2:  Stage2Output{Categories: []Stage2Category{}, Explanation: "no statutory category", PassesStep1: false, StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		s6:  Stage6Output{PriorArtSearchPriority: PriorityLow, Reasoning: strings50(), StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		err: map[string]error{},
	}
	res, err := NewPipeline(r).Run(context.Background(), baseRequest())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Pathway != PathwayA || res.FinalDetermination != DeterminationLikelyNotEligible {
		t.Fatalf("unexpected result: pathway=%s determination=%s", res.Pathway, res.FinalDetermination)
	}
}

func TestPipelinePathwayB2(t *testing.T) {
	r := &mockRunner{
		s1:  baseStage1(),
		s2:  Stage2Output{Categories: []Stage2Category{CategoryProcess}, Explanation: "fits", PassesStep1: true, StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		s3:  Stage3Output{RecitesException: true, ExceptionType: ptrException(ExceptionAbstractIdea), AbstractIdeaSubcategory: ptrSubcategory(SubcategoryMentalProcess), Reasoning: strings50(), MPEPReference: "MPEP 2106.04", StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		s4:  Stage4Output{AdditionalElements: []string{stringsLen(30)}, IntegratesPracticalApplication: true, Reasoning: strings50(), MPEPReference: "MPEP 2106.04(d)", StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		s6:  Stage6Output{PriorArtSearchPriority: PriorityMedium, Reasoning: strings50(), StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		err: map[string]error{},
	}
	res, err := NewPipeline(r).Run(context.Background(), baseRequest())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Pathway != PathwayB2 || res.FinalDetermination != DeterminationLikelyEligible {
		t.Fatalf("unexpected result: pathway=%s determination=%s", res.Pathway, res.FinalDetermination)
	}
}

func TestPipelinePathwayD(t *testing.T) {
	r := &mockRunner{
		s1:  baseStage1(),
		s2:  Stage2Output{Categories: []Stage2Category{CategoryProcess}, Explanation: "fits", PassesStep1: true, StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		s3:  Stage3Output{RecitesException: true, ExceptionType: ptrException(ExceptionAbstractIdea), AbstractIdeaSubcategory: ptrSubcategory(SubcategoryMentalProcess), Reasoning: strings50(), MPEPReference: "MPEP 2106.04", StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		s4:  Stage4Output{AdditionalElements: []string{stringsLen(30)}, IntegratesPracticalApplication: false, Reasoning: strings50(), MPEPReference: "MPEP 2106.04(d)", StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		s5:  Stage5Output{HasInventiveConcept: false, Reasoning: strings50(), BerkheimerConsiderations: stringsLen(30), MPEPReference: "MPEP 2106.05", StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		s6:  Stage6Output{PriorArtSearchPriority: PriorityHigh, Reasoning: strings50(), StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		err: map[string]error{},
	}
	res, err := NewPipeline(r).Run(context.Background(), baseRequest())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Pathway != PathwayD || res.FinalDetermination != DeterminationLikelyNotEligible {
		t.Fatalf("unexpected result: pathway=%s determination=%s", res.Pathway, res.FinalDetermination)
	}
}

func TestPipelineInputTruncationTriggersNeedsReview(t *testing.T) {
	r := &mockRunner{
		s1:  baseStage1(),
		s2:  Stage2Output{Categories: []Stage2Category{CategoryProcess}, Explanation: "fits", PassesStep1: true, StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		s3:  Stage3Output{RecitesException: false, Reasoning: strings50(), MPEPReference: "MPEP 2106.04", StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		s6:  Stage6Output{PriorArtSearchPriority: PriorityMedium, Reasoning: strings50(), StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "good", InsufficientInformation: false}},
		err: map[string]error{},
	}
	req := baseRequest()
	req.DisclosureText = stringsLen(MaxDisclosureChars + 10)
	res, err := NewPipeline(r).Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Metadata.InputTruncated {
		t.Fatal("expected input_truncated=true")
	}
	if res.FinalDetermination != DeterminationNeedsFurtherReview {
		t.Fatalf("expected NEEDS_FURTHER_REVIEW, got %s", res.FinalDetermination)
	}
}

func strings50() string {
	return "this reasoning string is intentionally long enough to satisfy minimum length constraints"
}

func ptrException(v ExceptionType) *ExceptionType                       { return &v }
func ptrSubcategory(v AbstractIdeaSubcategory) *AbstractIdeaSubcategory { return &v }
