package priorartsearch

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeStageRunner struct {
	s1      Stage1Output
	s3      Stage3Output
	s4      Stage4Output
	errS1   error
	errS3   error
	errS4   error
	seenReq RequestEnvelope
}

func (f *fakeStageRunner) RunStage1(ctx context.Context, req RequestEnvelope) (Stage1Output, StageAttemptMetrics, error) {
	f.seenReq = req
	if f.errS1 != nil {
		return Stage1Output{}, StageAttemptMetrics{Attempts: 1}, f.errS1
	}
	if len(f.s1.QueryStrategies) == 0 {
		f.s1 = Stage1Output{InventionTitle: "inv", QueryStrategies: []QueryStrategy{{ID: "Q1", Priority: PriorityPrimary, Description: "desc desc desc desc desc", TermFamilies: []TermFamily{{Canonical: "federated"}}, Phrases: []string{"federated learning"}, CPCSubclasses: []string{"G06N"}}}, NovelElements: []Stage1NovelElement{{ID: "NE1", Description: strings.Repeat("x", 30)}}, InventionSummary: strings.Repeat("x", 60), TechnologyDomains: []string{"AI"}, ConfidenceScore: 0.8, ConfidenceReason: strings.Repeat("x", 20)}
	}
	return f.s1, StageAttemptMetrics{Attempts: 1}, nil
}

func (f *fakeStageRunner) RunStage3(ctx context.Context, s1 Stage1Output, s2 Stage2Output) (Stage3Output, Stage3RunMetadata, StageAttemptMetrics, error) {
	if f.errS3 != nil {
		return Stage3Output{}, Stage3RunMetadata{}, StageAttemptMetrics{Attempts: 3}, f.errS3
	}
	return f.s3, Stage3RunMetadata{}, StageAttemptMetrics{Attempts: 1}, nil
}

func (f *fakeStageRunner) RunStage4(ctx context.Context, s1 Stage1Output, s2 Stage2Output, s3 Stage3Output) (Stage4Output, StageAttemptMetrics, error) {
	if f.errS4 != nil {
		return Stage4Output{}, StageAttemptMetrics{Attempts: 3}, f.errS4
	}
	if f.s4.Determination == "" {
		f.s4 = Stage4Output{Determination: DeterminationClearField, BlockingRisk: BlockingRisk{Level: BlockingRiskNone}}
	}
	return f.s4, StageAttemptMetrics{Attempts: 1}, nil
}

type fakeSearchRunner struct {
	out Stage2Output
	err error
}

func (f *fakeSearchRunner) Run(ctx context.Context, s1 Stage1Output) (Stage2Output, error) {
	if f.err != nil {
		return Stage2Output{}, f.err
	}
	return f.out, nil
}

func TestPipelineFullNormal(t *testing.T) {
	r := &fakeStageRunner{
		s3: Stage3Output{Assessments: []PatentAssessment{{PatentID: "P1", Relevance: RelevanceMedium}}},
		s4: Stage4Output{Determination: DeterminationCrowdedField, BlockingRisk: BlockingRisk{Level: BlockingRiskLow}},
	}
	s := &fakeSearchRunner{out: Stage2Output{Patents: []PatentResult{{PatentID: "P1", Abstract: "a"}}, TotalAPICalls: 1}}
	p := NewPipeline(r, s)
	res, err := p.Run(context.Background(), RequestEnvelope{CaseID: "c1", DisclosureText: strings.Repeat("x", 120)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Metadata.Degraded {
		t.Fatalf("expected non-degraded")
	}
	if res.Determination != DeterminationCrowdedField {
		t.Fatalf("unexpected determination %s", res.Determination)
	}
}

func TestPipelineWithPriorContextPassesThrough(t *testing.T) {
	r := &fakeStageRunner{s3: Stage3Output{}, s4: Stage4Output{Determination: DeterminationClearField, BlockingRisk: BlockingRisk{Level: BlockingRiskNone}}}
	s := &fakeSearchRunner{out: Stage2Output{}}
	p := NewPipeline(r, s)
	req := RequestEnvelope{CaseID: "c1", DisclosureText: strings.Repeat("x", 120), PriorContext: &PriorContext{Stage1Output: &Stage1PriorOutput{InventionTitle: "T"}}}
	_, err := p.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if r.seenReq.PriorContext == nil || r.seenReq.PriorContext.Stage1Output == nil {
		t.Fatalf("expected prior context passed to stage1")
	}
}

func TestPipelineZeroResultsFlow(t *testing.T) {
	r := &fakeStageRunner{s3: Stage3Output{Assessments: nil}, s4: Stage4Output{Determination: DeterminationClearField, BlockingRisk: BlockingRisk{Level: BlockingRiskNone}}}
	s := &fakeSearchRunner{out: Stage2Output{Patents: nil}}
	p := NewPipeline(r, s)
	res, err := p.Run(context.Background(), RequestEnvelope{CaseID: "c1", DisclosureText: strings.Repeat("x", 120)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Metadata.TotalPatentsRetrieved != 0 {
		t.Fatalf("expected 0 patents")
	}
}

func TestPipelineStage3Degraded(t *testing.T) {
	r := &fakeStageRunner{errS3: errors.New("boom")}
	s := &fakeSearchRunner{out: Stage2Output{Patents: []PatentResult{{PatentID: "P1", Abstract: "a"}}}}
	p := NewPipeline(r, s)
	res, err := p.Run(context.Background(), RequestEnvelope{CaseID: "c1", DisclosureText: strings.Repeat("x", 120)})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Metadata.Degraded || res.Stage3 != nil {
		t.Fatalf("expected stage3 degraded")
	}
	if !strings.Contains(BuildReportMarkdown(res), "Raw Results") {
		t.Fatalf("expected stage3 degraded report layout")
	}
}

func TestPipelineStage4Degraded(t *testing.T) {
	r := &fakeStageRunner{s3: Stage3Output{Assessments: []PatentAssessment{{PatentID: "P1", Relevance: RelevanceMedium}}}, errS4: errors.New("boom")}
	s := &fakeSearchRunner{out: Stage2Output{Patents: []PatentResult{{PatentID: "P1", Abstract: "a", Assignees: []string{"Org"}}}}}
	p := NewPipeline(r, s)
	res, err := p.Run(context.Background(), RequestEnvelope{CaseID: "c1", DisclosureText: strings.Repeat("x", 120)})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Metadata.Degraded || res.Stage4 != nil {
		t.Fatalf("expected stage4 degraded")
	}
	if !strings.Contains(BuildReportMarkdown(res), "Code-Generated Landscape Statistics") {
		t.Fatalf("expected stage4 degraded report layout")
	}
}

func TestPipelineConsistencyOverrideBlockingHigh(t *testing.T) {
	r := &fakeStageRunner{
		s3: Stage3Output{Assessments: []PatentAssessment{{PatentID: "P1", Relevance: RelevanceHigh}}},
		s4: Stage4Output{Determination: DeterminationCrowdedField, BlockingRisk: BlockingRisk{Level: BlockingRiskHigh}},
	}
	s := &fakeSearchRunner{out: Stage2Output{Patents: []PatentResult{{PatentID: "P1", Abstract: "a"}}}}
	p := NewPipeline(r, s)
	res, err := p.Run(context.Background(), RequestEnvelope{CaseID: "c1", DisclosureText: strings.Repeat("x", 120)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Determination != DeterminationBlockingArt {
		t.Fatalf("expected blocking override, got %s", res.Determination)
	}
}
