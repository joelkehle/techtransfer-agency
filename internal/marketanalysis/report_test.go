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

func TestDegradedSkipExplanationShowsFailureLanguage(t *testing.T) {
	// Simulate stage_2 failure: Stage1 ran successfully, Stage2+ are nil,
	// metadata indicates degraded mode with StageFailed = "stage_2".
	res := PipelineResult{
		Request: RequestEnvelope{CaseID: "CASE-DEG"},
		Stage0:  Stage0Output{InventionTitle: NullableField{Value: "Degraded test", Confidence: ConfidenceHigh}},
		Stage1: &Stage1Output{
			PrimaryPath:              PathExclusiveLicense,
			PrimaryPathReasoning:     "test",
			ProductDefinition:        "test product",
			HasPlausibleMonetization: true,
		},
		// Stage2 is nil (it failed)
		Decision: RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceLow, Reason: "Stage failure"},
		Metadata: PipelineMetadata{
			Mode:        ReportModeDegraded,
			StageFailed: "stage_2",
		},
	}
	env := BuildResponse(res)
	md := env.ReportMarkdown

	// Stages 3, 4, 5 should show degraded failure language, not normal skip explanations
	for _, stage := range []string{"Stage 3", "Stage 4", "Stage 5"} {
		if !strings.Contains(md, "**Not evaluated**") {
			t.Fatalf("expected degraded failure language for %s", stage)
		}
	}
	if !strings.Contains(md, "stage_2") {
		t.Fatal("expected failed stage name in skip explanation")
	}
	// Should NOT contain normal skip language like "Stage 1 found no plausible"
	if strings.Contains(md, "no plausible commercialization path") {
		t.Fatal("degraded mode should not show normal early-exit explanation")
	}
}

func TestDegradedStage2SkipShowsFailureNotNormalSkip(t *testing.T) {
	// Stage 1 failure: Stage0 ran, Stage1+ are nil.
	res := PipelineResult{
		Request:  RequestEnvelope{CaseID: "CASE-DEG2"},
		Stage0:   Stage0Output{InventionTitle: NullableField{Value: "S1 fail test", Confidence: ConfidenceHigh}},
		Decision: RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceLow, Reason: "Stage failure"},
		Metadata: PipelineMetadata{
			Mode:        ReportModeDegraded,
			StageFailed: "stage_1",
		},
	}
	env := BuildResponse(res)
	md := env.ReportMarkdown

	// Stage 2 should show degraded language referencing stage_1
	if !strings.Contains(md, "**Not evaluated**") {
		t.Fatal("expected degraded failure language for stage_2")
	}
	if !strings.Contains(md, "stage_1") {
		t.Fatal("expected failed stage name (stage_1) in skip explanation")
	}
}

func TestTableCellPipeEscaping(t *testing.T) {
	// Field value containing pipe characters should be escaped in the table
	reason := "foo | bar"
	res := PipelineResult{
		Request: RequestEnvelope{CaseID: "CASE-PIPE"},
		Stage0: Stage0Output{
			InventionTitle: NullableField{Value: "Pipe | Test", Confidence: ConfidenceHigh},
			ProblemSolved:  NullableField{Value: "solves x | y", Confidence: ConfidenceMedium},
		},
		Stage1: &Stage1Output{
			PrimaryPath:              PathExclusiveLicense,
			PrimaryPathReasoning:     "test",
			ProductDefinition:        "test",
			HasPlausibleMonetization: true,
		},
		Stage2: &Stage2Output{
			Scores: Stage2Scores{
				MarketPain:        ScoreReason{Score: 3, Reasoning: reason},
				Differentiation:   ScoreReason{Score: 3, Reasoning: "ok"},
				AdoptionFriction:  ScoreReason{Score: 3, Reasoning: "ok"},
				DevelopmentBurden: ScoreReason{Score: 3, Reasoning: "ok"},
				PartnerDensity:    ScoreReason{Score: 3, Reasoning: "ok"},
				IPLeverage:        ScoreReason{Score: 3, Reasoning: "ok"},
			},
			CompositeScore:      3.0,
			WeightedScore:       3.0,
			Confidence:          ConfidenceHigh,
			ConfidenceReasoning: "solid",
		},
		Decision: RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceMedium, Reason: "test"},
		Metadata: PipelineMetadata{Mode: ReportModeComplete},
	}
	env := BuildResponse(res)
	md := env.ReportMarkdown

	// The raw pipe in "Pipe | Test" should be escaped to "Pipe \| Test" inside table rows
	if strings.Contains(md, "| Pipe | Test |") {
		t.Fatal("pipe in field value was not escaped — would corrupt table structure")
	}
	if !strings.Contains(md, `Pipe \| Test`) {
		t.Fatal("expected escaped pipe in title cell")
	}

	// Stage 2 reasoning "foo | bar" should also be escaped
	if !strings.Contains(md, `foo \| bar`) {
		t.Fatal("expected escaped pipe in scorecard reasoning cell")
	}
}

func TestArrayFieldsRenderInTable(t *testing.T) {
	// NullableField values that are arrays ([]any from JSON) should render
	// as semicolon-separated text, not as "—".
	res := PipelineResult{
		Request: RequestEnvelope{CaseID: "CASE-ARR"},
		Stage0: Stage0Output{
			InventionTitle:    NullableField{Value: "Array Test", Confidence: ConfidenceHigh},
			ClaimedAdvantages: NullableField{Value: []any{"Fast", "Cheap", "Good"}, Confidence: ConfidenceHigh},
			ApplicationDomains: NullableField{
				Value:      []any{"Healthcare", "Finance"},
				Confidence: ConfidenceMedium,
			},
			CompetingApproaches: NullableField{
				Value:      []any{"Manual process"},
				Confidence: ConfidenceLow,
			},
		},
		Decision: RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceLow, Reason: "test"},
		Metadata: PipelineMetadata{Mode: ReportModeComplete},
	}
	env := BuildResponse(res)
	md := env.ReportMarkdown

	if !strings.Contains(md, "Fast; Cheap; Good") {
		t.Fatal("expected array values joined with semicolons for Claimed Advantages")
	}
	if !strings.Contains(md, "Healthcare; Finance") {
		t.Fatal("expected array values joined with semicolons for Application Domains")
	}
	if !strings.Contains(md, "Manual process") {
		t.Fatal("expected single-element array to render for Competing Approaches")
	}
}

func TestDollarFormatting(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{500000000, "500,000,000"},
		{2000000000, "2,000,000,000"},
		{32500, "32,500"},
		{1073698, "1,073,698"},
	}
	for _, tt := range tests {
		got := fmtUSD(tt.input)
		if got != tt.want {
			t.Errorf("fmtUSD(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
