package patentscreen

import (
	"context"
	"errors"
	"testing"
)

type fakeCaller struct {
	responses []string
	errs      []error
	i         int
}

func (f *fakeCaller) GenerateJSON(context.Context, string) (string, error) {
	idx := f.i
	f.i++
	if idx < len(f.errs) && f.errs[idx] != nil {
		return "", f.errs[idx]
	}
	if idx < len(f.responses) {
		return f.responses[idx], nil
	}
	return "", nil
}

func TestValidateStage1ClaimsDependency(t *testing.T) {
	s := Stage1Output{
		InventionTitle:       "Title here",
		Abstract:             stringsLen(30),
		ProblemSolved:        stringsLen(30),
		InventionDescription: stringsLen(60),
		NovelElements:        []string{stringsLen(20)},
		TechnologyArea:       "software",
		ClaimsPresent:        true,
		ClaimsSummary:        nil,
		StageConfidence:      StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: stringsLen(20)},
	}
	if err := validateStage1(s); err == nil {
		t.Fatal("expected validation error for missing claims_summary")
	}
}

func TestValidateStage2EnumCaseAndConditional(t *testing.T) {
	s := Stage2Output{Categories: []Stage2Category{"process"}, Explanation: stringsLen(40), PassesStep1: true, StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: stringsLen(20)}}
	if err := validateStage2(s); err == nil {
		t.Fatal("expected invalid enum casing failure")
	}
	s = Stage2Output{Categories: []Stage2Category{CategoryProcess}, Explanation: stringsLen(40), PassesStep1: false, StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: stringsLen(20)}}
	if err := validateStage2(s); err == nil {
		t.Fatal("expected categories must be empty failure")
	}
}

func TestValidateStage3NullabilityRules(t *testing.T) {
	s := Stage3Output{RecitesException: false, ExceptionType: ptrException(ExceptionAbstractIdea), Reasoning: stringsLen(60), MPEPReference: stringsLen(20), StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: stringsLen(20)}}
	if err := validateStage3(s); err == nil {
		t.Fatal("expected nullability failure")
	}
}

func TestValidateStage4LengthConstraints(t *testing.T) {
	s := Stage4Output{AdditionalElements: []string{stringsLen(5)}, IntegratesPracticalApplication: true, Reasoning: stringsLen(60), MPEPReference: stringsLen(20), StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: stringsLen(20)}}
	if err := validateStage4(s); err == nil {
		t.Fatal("expected additional element length failure")
	}
}

func TestValidateStage5LengthConstraints(t *testing.T) {
	s := Stage5Output{HasInventiveConcept: true, Reasoning: stringsLen(60), BerkheimerConsiderations: stringsLen(10), MPEPReference: stringsLen(20), StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: stringsLen(20)}}
	if err := validateStage5(s); err == nil {
		t.Fatal("expected berkheimer length failure")
	}
}

func TestValidateStage6PriorityEnum(t *testing.T) {
	s := Stage6Output{PriorArtSearchPriority: "mid", Reasoning: stringsLen(60), StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: stringsLen(20)}}
	if err := validateStage6(s); err == nil {
		t.Fatal("expected priority enum failure")
	}
}

func TestStageExecutorParseRetryThenSuccess(t *testing.T) {
	caller := &fakeCaller{responses: []string{"not-json", `{"invention_title":"Title Here","abstract":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","problem_solved":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","invention_description":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccc","novel_elements":["dddddddddddddddddddd"],"technology_area":"software","claims_present":false,"claims_summary":null,"confidence_score":0.9,"confidence_reason":"sufficient confidence","insufficient_information":false}`}}
	exec := NewStageExecutor(caller)
	out := Stage1Output{}
	metrics, err := exec.Run(context.Background(), "stage_1", "prompt", &out, func() error { return validateStage1(out) })
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if metrics.Attempts != 2 || metrics.ContentRetries != 1 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
}

func TestStageExecutorSchemaRetryThenSuccess(t *testing.T) {
	caller := &fakeCaller{responses: []string{
		`{"categories":["process"],"explanation":"` + stringsLen(30) + `","passes_step_1":true,"confidence_score":0.9,"confidence_reason":"` + stringsLen(20) + `","insufficient_information":false}`,
		`{"categories":["PROCESS"],"explanation":"` + stringsLen(30) + `","passes_step_1":true,"confidence_score":0.9,"confidence_reason":"` + stringsLen(20) + `","insufficient_information":false}`,
	}}
	exec := NewStageExecutor(caller)
	out := Stage2Output{}
	metrics, err := exec.Run(context.Background(), "stage_2", "prompt", &out, func() error { return validateStage2(out) })
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if metrics.Attempts != 2 || metrics.ContentRetries != 1 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
}

func TestStageExecutorTransportRetriesThenFail(t *testing.T) {
	caller := &fakeCaller{errs: []error{errors.New("status code: 500"), errors.New("status code: 500"), errors.New("status code: 500")}}
	exec := NewStageExecutor(caller)
	out := Stage1Output{}
	_, err := exec.Run(context.Background(), "stage_1", "prompt", &out, func() error { return nil })
	if err == nil {
		t.Fatal("expected transport error")
	}
}

func stringsLen(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}
