package patentteam

import (
	"context"
	"fmt"
	"os"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// mockMessager implements AnthropicMessager for testing.
type mockMessager struct {
	response *anthropic.Message
	err      error
}

func (m *mockMessager) New(_ context.Context, _ anthropic.MessageNewParams, _ ...option.RequestOption) (*anthropic.Message, error) {
	return m.response, m.err
}

func newMockMessage(text string) *anthropic.Message {
	return &anthropic.Message{
		Content: []anthropic.ContentBlockUnion{
			{Type: "text", Text: text},
		},
	}
}

func withMockClient(mock *mockMessager) func() {
	old := newAnthropicClient
	newAnthropicClient = func(_ string) AnthropicMessager { return mock }
	return func() { newAnthropicClient = old }
}

func TestLLMEvaluatorLikelyEligible(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	cleanup := withMockClient(&mockMessager{
		response: newMockMessage(`{
			"eligibility": "likely_eligible",
			"confidence": 0.85,
			"recommendation": "Proceed with provisional filing",
			"summary": "The disclosure describes a novel sensor fusion algorithm with concrete technical implementation.",
			"eligibility_reasons": ["Concrete technical implementation described", "Novel algorithm for signal processing"],
			"questions_for_inventors": ["What prior art have you reviewed?"]
		}`),
	})
	defer cleanup()

	got, err := EvaluatePatentEligibilityLLM(context.Background(), "CASE-LLM-1", "test disclosure text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.CaseID != "CASE-LLM-1" {
		t.Errorf("case_id=%s want=CASE-LLM-1", got.CaseID)
	}
	if got.Eligibility != EligibilityLikelyEligible {
		t.Errorf("eligibility=%s want=%s", got.Eligibility, EligibilityLikelyEligible)
	}
	if got.Confidence != 0.85 {
		t.Errorf("confidence=%f want=0.85", got.Confidence)
	}
	if got.Disclaimer == "" {
		t.Error("expected disclaimer to be set")
	}
}

func TestLLMEvaluatorNeedsMoreInfo(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	cleanup := withMockClient(&mockMessager{
		response: newMockMessage(`{
			"eligibility": "needs_more_info",
			"confidence": 0.40,
			"recommendation": "Request additional details from inventors",
			"summary": "Disclosure lacks sufficient technical detail.",
			"eligibility_reasons": ["Insufficient implementation detail"],
			"questions_for_inventors": ["What specific algorithm is used?", "What is the data flow?"]
		}`),
	})
	defer cleanup()

	got, err := EvaluatePatentEligibilityLLM(context.Background(), "CASE-LLM-2", "short text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Eligibility != EligibilityNeedsMoreInfo {
		t.Errorf("eligibility=%s want=%s", got.Eligibility, EligibilityNeedsMoreInfo)
	}
	if len(got.Questions) != 2 {
		t.Errorf("questions count=%d want=2", len(got.Questions))
	}
}

func TestLLMEvaluatorHandlesCodeFences(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	cleanup := withMockClient(&mockMessager{
		response: newMockMessage("```json\n" + `{
			"eligibility": "likely_not_eligible",
			"confidence": 0.70,
			"recommendation": "Do not file",
			"summary": "Business method only.",
			"eligibility_reasons": ["Abstract business method"],
			"questions_for_inventors": ["Can you describe technical components?"]
		}` + "\n```"),
	})
	defer cleanup()

	got, err := EvaluatePatentEligibilityLLM(context.Background(), "CASE-LLM-3", "marketing plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Eligibility != EligibilityLikelyNotEligible {
		t.Errorf("eligibility=%s want=%s", got.Eligibility, EligibilityLikelyNotEligible)
	}
}

func TestLLMEvaluatorInvalidEligibilityDefaultsToNeedsMoreInfo(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	cleanup := withMockClient(&mockMessager{
		response: newMockMessage(`{
			"eligibility": "unknown_value",
			"confidence": 0.50,
			"recommendation": "Review needed",
			"summary": "Unclear status.",
			"eligibility_reasons": [],
			"questions_for_inventors": []
		}`),
	})
	defer cleanup()

	got, err := EvaluatePatentEligibilityLLM(context.Background(), "CASE-LLM-4", "some text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Eligibility != EligibilityNeedsMoreInfo {
		t.Errorf("eligibility=%s want=%s", got.Eligibility, EligibilityNeedsMoreInfo)
	}
}

func TestLLMEvaluatorAPIError(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	cleanup := withMockClient(&mockMessager{
		err: fmt.Errorf("API rate limit exceeded"),
	})
	defer cleanup()

	_, err := EvaluatePatentEligibilityLLM(context.Background(), "CASE-ERR", "text")
	if err == nil {
		t.Fatal("expected error from API failure")
	}
}

func TestLLMEvaluatorEmptyResponse(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	cleanup := withMockClient(&mockMessager{
		response: &anthropic.Message{Content: []anthropic.ContentBlockUnion{}},
	})
	defer cleanup()

	_, err := EvaluatePatentEligibilityLLM(context.Background(), "CASE-EMPTY", "text")
	if err == nil {
		t.Fatal("expected error for empty response")
	}
}

func TestLLMEvaluatorNoAPIKey(t *testing.T) {
	os.Unsetenv("ANTHROPIC_API_KEY")

	_, err := EvaluatePatentEligibilityLLM(context.Background(), "CASE-NOKEY", "text")
	if err == nil {
		t.Fatal("expected error when ANTHROPIC_API_KEY is not set")
	}
}

func TestEvaluatePatentEligibilityFallsBackOnLLMFailure(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	cleanup := withMockClient(&mockMessager{
		err: fmt.Errorf("connection refused"),
	})
	defer cleanup()

	// Long technical text that the keyword heuristic would classify as likely_eligible.
	text := `This disclosure describes a protocol and algorithm for sensor signal fusion.
The architecture includes hardware acceleration for model inference and reduced latency.
It details throughput improvements and encryption of data paths.
The method defines concrete processing stages, memory layouts, and compute scheduling.
Implementation details cover inference batching, signal pre-processing, and error correction.`

	got := EvaluatePatentEligibility(context.Background(), "CASE-FALLBACK", text)
	if got.Eligibility != EligibilityLikelyEligible {
		t.Errorf("eligibility=%s want=%s (keyword fallback)", got.Eligibility, EligibilityLikelyEligible)
	}
}

func TestLLMEvaluatorConfidenceClamping(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	cleanup := withMockClient(&mockMessager{
		response: newMockMessage(`{
			"eligibility": "likely_eligible",
			"confidence": 1.5,
			"recommendation": "File immediately",
			"summary": "Extremely novel.",
			"eligibility_reasons": ["Novel"],
			"questions_for_inventors": []
		}`),
	})
	defer cleanup()

	got, err := EvaluatePatentEligibilityLLM(context.Background(), "CASE-CLAMP", "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Confidence != 1.0 {
		t.Errorf("confidence=%f want=1.0 (clamped)", got.Confidence)
	}
}
