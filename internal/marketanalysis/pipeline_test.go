package marketanalysis

import (
	"context"
	"errors"
	"testing"
)

type mockRunner struct {
	s0    Stage0Output
	s1    Stage1Output
	s2    Stage2Output
	s3    Stage3Output
	s4    Stage4Output
	s5    Stage5Output
	err   map[string]error
	calls map[string]int
}

func (m *mockRunner) RunStage0(context.Context, RequestEnvelope) (Stage0Output, StageAttemptMetrics, error) {
	m.calls["stage_0"]++
	return m.s0, StageAttemptMetrics{Attempts: 1}, m.err["stage_0"]
}
func (m *mockRunner) RunStage1(context.Context, Stage0Output) (Stage1Output, StageAttemptMetrics, error) {
	m.calls["stage_1"]++
	return m.s1, StageAttemptMetrics{Attempts: 1}, m.err["stage_1"]
}
func (m *mockRunner) RunStage2(context.Context, Stage0Output, Stage1Output) (Stage2Output, StageAttemptMetrics, error) {
	m.calls["stage_2"]++
	return m.s2, StageAttemptMetrics{Attempts: 1}, m.err["stage_2"]
}
func (m *mockRunner) RunStage3(context.Context, Stage0Output, Stage1Output, Stage2Output) (Stage3Output, StageAttemptMetrics, error) {
	m.calls["stage_3"]++
	return m.s3, StageAttemptMetrics{Attempts: 1}, m.err["stage_3"]
}
func (m *mockRunner) RunStage4(context.Context, Stage0Output, Stage1Output, Stage2Output, Stage3Output) (Stage4Output, StageAttemptMetrics, error) {
	m.calls["stage_4"]++
	return m.s4, StageAttemptMetrics{Attempts: 1}, m.err["stage_4"]
}
func (m *mockRunner) RunStage5(context.Context, Stage5Input) (Stage5Output, StageAttemptMetrics, error) {
	m.calls["stage_5"]++
	return m.s5, StageAttemptMetrics{Attempts: 1}, m.err["stage_5"]
}

func baseReq() RequestEnvelope {
	return RequestEnvelope{CaseID: "CASE-1", DisclosureText: "This disclosure describes a software platform with enough detail to satisfy minimum disclosure length and enable market analysis staging through all paths."}
}

func baseStage0() Stage0Output {
	return Stage0Output{
		InventionTitle:      NullableField{Value: "Invention", Confidence: ConfidenceHigh},
		ProblemSolved:       NullableField{Value: "Problem", Confidence: ConfidenceHigh},
		SolutionDescription: NullableField{Value: "Solution", Confidence: ConfidenceHigh},
		ClaimedAdvantages:   NullableField{Value: []string{"adv"}, Confidence: ConfidenceHigh},
		TargetUser:          NullableField{Value: "Lab", Confidence: ConfidenceHigh},
		TargetBuyer:         NullableField{Value: "Hospitals", Confidence: ConfidenceHigh},
		ApplicationDomains:  NullableField{Value: []string{"diag"}, Confidence: ConfidenceHigh},
		EvidenceLevel:       EvidencePrototype,
		CompetingApproaches: NullableField{Value: []string{"alt"}, Confidence: ConfidenceHigh},
		Dependencies:        NullableField{Value: []string{"device"}, Confidence: ConfidenceHigh},
		Sector:              "software",
	}
}

func baseStage1() Stage1Output {
	return Stage1Output{PrimaryPath: PathNonExclusive, PrimaryPathReasoning: "fit", ProductDefinition: "SDK", HasPlausibleMonetization: true}
}

func baseStage2() Stage2Output {
	s := Stage2Output{
		Scores: Stage2Scores{
			MarketPain:        ScoreReason{Score: 3, Reasoning: "x"},
			Differentiation:   ScoreReason{Score: 3, Reasoning: "x"},
			AdoptionFriction:  ScoreReason{Score: 3, Reasoning: "x"},
			DevelopmentBurden: ScoreReason{Score: 3, Reasoning: "x"},
			PartnerDensity:    ScoreReason{Score: 3, Reasoning: "x"},
			IPLeverage:        ScoreReason{Score: 4, Reasoning: "x"},
		},
		Confidence:          ConfidenceMedium,
		ConfidenceReasoning: "enough info",
		UnknownKeyFactors:   []string{},
	}
	s.CompositeScore = 3.17
	s.WeightedScore = 3.22
	return s
}

func baseStage3() Stage3Output {
	return Stage3Output{
		TAM: MarketRange{LowUSD: 10000000, HighUSD: 50000000, Unit: "annual revenue", Estimable: true, Assumptions: []Stage3Assumption{{Assumption: "a", Source: SourceEstimated}}},
		SAM: MarketRange{LowUSD: 5000000, HighUSD: 25000000, Unit: "annual revenue", Estimable: true, Assumptions: []Stage3Assumption{{Assumption: "a", Source: SourceEstimated}}},
		SOM: MarketRange{LowUSD: 500000, HighUSD: 5000000, Unit: "annual revenue", Estimable: true, Assumptions: []Stage3Assumption{{Assumption: "a", Source: SourceEstimated}}},
	}
}

func baseStage4() Stage4Output { return defaultsFromPriors(PriorForSector("software")) }

func baseStage5() Stage5Output {
	return Stage5Output{ExecutiveSummary: "Summary", KeyDrivers: []string{"a", "b", "c"}, DiligenceQuestions: []string{"a", "b", "c"}, RecommendedActions: []string{"meet inventors"}}
}

func TestPipelineFullPathComplete(t *testing.T) {
	r := &mockRunner{s0: baseStage0(), s1: baseStage1(), s2: baseStage2(), s3: baseStage3(), s4: baseStage4(), s5: baseStage5(), err: map[string]error{}, calls: map[string]int{}}
	res, err := NewPipeline(r).Run(context.Background(), baseReq())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Metadata.Mode != ReportModeComplete {
		t.Fatalf("expected COMPLETE mode, got %s", res.Metadata.Mode)
	}
	if res.Stage5 == nil {
		t.Fatal("expected stage 5")
	}
}

func TestPipelineShortCircuitStage1NoMonetization(t *testing.T) {
	s1 := baseStage1()
	s1.HasPlausibleMonetization = false
	reason := "none"
	s1.NoMonetizationReasoning = &reason
	r := &mockRunner{s0: baseStage0(), s1: s1, err: map[string]error{}, calls: map[string]int{}}
	res, err := NewPipeline(r).Run(context.Background(), baseReq())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Decision.Tier != RecommendationNoGo {
		t.Fatalf("expected NO_GO, got %s", res.Decision.Tier)
	}
	if r.calls["stage_2"] != 0 {
		t.Fatal("stage_2 should not run")
	}
}

func TestPipelineShortCircuitStage2LowScore(t *testing.T) {
	s2 := baseStage2()
	s2.WeightedScore = 1.8
	r := &mockRunner{s0: baseStage0(), s1: baseStage1(), s2: s2, err: map[string]error{}, calls: map[string]int{}}
	res, err := NewPipeline(r).Run(context.Background(), baseReq())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Decision.Tier != RecommendationNoGo {
		t.Fatalf("expected NO_GO, got %s", res.Decision.Tier)
	}
}

func TestPipelineShortCircuitStage2LowConfidenceUnknowns(t *testing.T) {
	s2 := baseStage2()
	s2.Confidence = ConfidenceLow
	s2.UnknownKeyFactors = []string{"buyer", "pricing"}
	r := &mockRunner{s0: baseStage0(), s1: baseStage1(), s2: s2, err: map[string]error{}, calls: map[string]int{}}
	res, err := NewPipeline(r).Run(context.Background(), baseReq())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Decision.Tier != RecommendationDefer {
		t.Fatalf("expected DEFER, got %s", res.Decision.Tier)
	}
}

func TestPipelineShortCircuitStage2BorderlineLowConfidence(t *testing.T) {
	s2 := baseStage2()
	s2.WeightedScore = 2.2
	s2.Confidence = ConfidenceLow
	s2.UnknownKeyFactors = []string{"pricing"}
	r := &mockRunner{s0: baseStage0(), s1: baseStage1(), s2: s2, err: map[string]error{}, calls: map[string]int{}}
	res, err := NewPipeline(r).Run(context.Background(), baseReq())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Decision.Tier != RecommendationDefer {
		t.Fatalf("expected DEFER, got %s", res.Decision.Tier)
	}
	if r.calls["stage_3"] != 0 {
		t.Fatal("stage_3 should not run")
	}
}

func TestPipelineShortCircuitStage3SOMNotEstimable(t *testing.T) {
	s3 := baseStage3()
	reason := "missing pricing"
	s3.SOM.Estimable = false
	s3.SOM.NotEstimableReason = &reason
	s3.SOM.LowUSD, s3.SOM.HighUSD = 0, 0
	r := &mockRunner{s0: baseStage0(), s1: baseStage1(), s2: baseStage2(), s3: s3, err: map[string]error{}, calls: map[string]int{}}
	res, err := NewPipeline(r).Run(context.Background(), baseReq())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Decision.Tier != RecommendationDefer {
		t.Fatalf("expected DEFER, got %s", res.Decision.Tier)
	}
}

func TestPipelineDegradedStage3Failure(t *testing.T) {
	r := &mockRunner{s0: baseStage0(), s1: baseStage1(), s2: baseStage2(), err: map[string]error{"stage_3": errors.New("boom")}, calls: map[string]int{}}
	res, err := NewPipeline(r).Run(context.Background(), baseReq())
	if err != nil {
		t.Fatalf("expected degraded result, got error: %v", err)
	}
	if res.Metadata.Mode != ReportModeDegraded {
		t.Fatalf("expected degraded mode, got %s", res.Metadata.Mode)
	}
}

func TestPipelineDegradedStage4Failure(t *testing.T) {
	r := &mockRunner{s0: baseStage0(), s1: baseStage1(), s2: baseStage2(), s3: baseStage3(), err: map[string]error{"stage_4": errors.New("boom")}, calls: map[string]int{}}
	res, err := NewPipeline(r).Run(context.Background(), baseReq())
	if err != nil {
		t.Fatalf("expected degraded result, got error: %v", err)
	}
	if res.Metadata.Mode != ReportModeDegraded {
		t.Fatalf("expected degraded mode, got %s", res.Metadata.Mode)
	}
}

func TestDetermineRecommendationOpenSourceLowIPLeverage(t *testing.T) {
	s1 := baseStage1()
	s1.PrimaryPath = PathOpenSourceServices
	s2 := baseStage2()
	s2.Scores.IPLeverage.Score = 3
	res := PipelineResult{Stage1: &s1, Stage2: &s2, Stage4Computed: &Stage4ComputedOutput{Scenarios: map[string]ScenarioOutput{"base": {ExceedsPatentCost: true}, "pessimistic": {}, "optimistic": {}}, PathModelLimitation: strPtr("open-source limitation")}}
	dec := determineRecommendation(res)
	if dec.Tier != RecommendationDefer {
		t.Fatalf("expected DEFER, got %s", dec.Tier)
	}
}

func TestDetermineRecommendationOverrideLowConfidenceUnknowns(t *testing.T) {
	s1 := baseStage1()
	s2 := baseStage2()
	s2.Confidence = ConfidenceLow
	s2.UnknownKeyFactors = []string{"buyer", "pricing"}
	res := PipelineResult{Stage1: &s1, Stage2: &s2, Stage4Computed: &Stage4ComputedOutput{Scenarios: map[string]ScenarioOutput{"base": {ExceedsPatentCost: true}, "pessimistic": {ExceedsPatentCost: true}, "optimistic": {ExceedsPatentCost: true}}}}
	dec := determineRecommendation(res)
	if dec.Tier != RecommendationDefer {
		t.Fatalf("expected DEFER due to override, got %s", dec.Tier)
	}
}

func TestDetermineRecommendationStartupForcesLowConfidence(t *testing.T) {
	s1 := baseStage1()
	s1.PrimaryPath = PathStartup
	s2 := baseStage2()
	limitation := "Royalty-based NPV model undervalues startup paths where equity dominates."
	res := PipelineResult{
		Stage1: &s1,
		Stage2: &s2,
		Stage4Computed: &Stage4ComputedOutput{
			PathModelLimitation: &limitation,
			Scenarios: map[string]ScenarioOutput{
				"base":        {ExceedsPatentCost: true},
				"pessimistic": {ExceedsPatentCost: true},
				"optimistic":  {ExceedsPatentCost: true},
			},
		},
	}
	dec := determineRecommendation(res)
	if dec.Tier != RecommendationGO {
		t.Fatalf("expected GO, got %s", dec.Tier)
	}
	if dec.Confidence != ConfidenceLow {
		t.Fatalf("expected LOW confidence for STARTUP path, got %s", dec.Confidence)
	}
}

func strPtr(s string) *string { return &s }
