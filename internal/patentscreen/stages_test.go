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
