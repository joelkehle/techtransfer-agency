package priorartsearch

import (
	"context"
	"errors"
	"testing"
)

type fakeLLMCaller struct {
	responses []string
	errs      []error
	idx       int
}

func (f *fakeLLMCaller) GenerateJSON(context.Context, string) (string, error) {
	i := f.idx
	f.idx++
	if i < len(f.errs) && f.errs[i] != nil {
		return "", f.errs[i]
	}
	if i < len(f.responses) {
		return f.responses[i], nil
	}
	return "", nil
}

func (f *fakeLLMCaller) ModelName() string { return "test-model" }

func TestStageExecutorAcceptsMarkdownFences(t *testing.T) {
	exec := NewStageExecutor(&fakeLLMCaller{responses: []string{"```json\n{\"ok\":true}\n```"}})
	var out struct {
		OK bool `json:"ok"`
	}
	m, err := exec.Run(context.Background(), "stage", "prompt", &out, func() error { return nil })
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !out.OK || m.Attempts != 1 {
		t.Fatalf("unexpected output=%+v metrics=%+v", out, m)
	}
}

func TestStageExecutorRetriesValidationThenSuccess(t *testing.T) {
	exec := NewStageExecutor(&fakeLLMCaller{responses: []string{"{\"score\":2}", "{\"score\":1}"}})
	var out struct {
		Score int `json:"score"`
	}
	m, err := exec.Run(context.Background(), "stage", "prompt", &out, func() error {
		if out.Score != 1 {
			return errors.New("score must be 1")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if m.Attempts != 2 || m.ContentRetries != 1 {
		t.Fatalf("unexpected metrics: %+v", m)
	}
}

func TestStageExecutorFailsAfterThreeAttempts(t *testing.T) {
	exec := NewStageExecutor(&fakeLLMCaller{responses: []string{"not-json", "not-json", "not-json"}})
	var out struct{}
	_, err := exec.Run(context.Background(), "stage", "prompt", &out, func() error { return nil })
	if err == nil {
		t.Fatal("expected failure")
	}
}
