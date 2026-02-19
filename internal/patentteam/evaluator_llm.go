package patentteam

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const patentEvalSystemPrompt = `You are a patent eligibility screening assistant for a university technology transfer office.
Evaluate the provided invention disclosure text and produce a structured JSON assessment.

You MUST respond with valid JSON only — no markdown, no explanation outside the JSON.

The JSON must have these exact fields:
{
  "eligibility": "<one of: likely_eligible, needs_more_info, likely_not_eligible>",
  "confidence": <float between 0.0 and 1.0>,
  "recommendation": "<brief recommendation string>",
  "summary": "<2-4 sentence summary of the assessment>",
  "eligibility_reasons": ["<reason 1>", "<reason 2>", ...],
  "questions_for_inventors": ["<question 1>", "<question 2>", ...]
}

Evaluation criteria:
1. Subject Matter Eligibility (35 USC 101): Is this a process, machine, manufacture, or composition of matter? Does it risk being classified as an abstract idea, law of nature, or natural phenomenon?
2. Novelty Indicators: Are there signals that this invention is new and not previously disclosed?
3. Non-Obviousness Signals: Would this be non-obvious to a person of ordinary skill in the art?
4. Technical Implementation: Is there sufficient technical detail describing a concrete implementation?

Generate 2-5 targeted questions for the inventors to strengthen the disclosure.
Be conservative — when in doubt, recommend "needs_more_info" rather than "likely_eligible".`

const patentEvalUserPrompt = `Please evaluate the following invention disclosure for patent eligibility.

Case ID: %s

--- BEGIN DISCLOSURE TEXT ---
%s
--- END DISCLOSURE TEXT ---

Respond with the JSON assessment only.`

// llmAssessmentResponse mirrors the JSON structure we ask Claude to return.
type llmAssessmentResponse struct {
	Eligibility       string   `json:"eligibility"`
	Confidence        float64  `json:"confidence"`
	Recommendation    string   `json:"recommendation"`
	Summary           string   `json:"summary"`
	EligibilityReason []string `json:"eligibility_reasons"`
	Questions         []string `json:"questions_for_inventors"`
}

// AnthropicClientCreator is a function type for creating the Anthropic client.
// It exists so tests can inject a mock.
type AnthropicClientCreator func(apiKey string) AnthropicMessager

// AnthropicMessager defines the subset of the Anthropic client we use.
type AnthropicMessager interface {
	New(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error)
}

// defaultAnthropicCreator creates a real Anthropic client.
func defaultAnthropicCreator(apiKey string) AnthropicMessager {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &client.Messages
}

// newAnthropicClient is the package-level creator, overridable in tests.
var newAnthropicClient AnthropicClientCreator = defaultAnthropicCreator

// EvaluatePatentEligibilityLLM calls the Claude API to evaluate patent eligibility.
func EvaluatePatentEligibilityLLM(ctx context.Context, caseID, extractedText string) (PatentAssessment, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return PatentAssessment{}, fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	messages := newAnthropicClient(apiKey)

	resp, err := messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeSonnet4_20250514,
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: patentEvalSystemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock(fmt.Sprintf(patentEvalUserPrompt, caseID, extractedText)),
			),
		},
	})
	if err != nil {
		return PatentAssessment{}, fmt.Errorf("claude API call failed: %w", err)
	}

	// Extract text from the response content blocks.
	var textParts []string
	for _, block := range resp.Content {
		if block.Type == "text" {
			textParts = append(textParts, block.Text)
		}
	}
	rawText := strings.Join(textParts, "")
	if rawText == "" {
		return PatentAssessment{}, fmt.Errorf("empty response from Claude API")
	}

	// Strip markdown code fences if Claude wraps the JSON.
	cleaned := strings.TrimSpace(rawText)
	if strings.HasPrefix(cleaned, "```") {
		if idx := strings.Index(cleaned[3:], "\n"); idx >= 0 {
			cleaned = cleaned[3+idx+1:]
		}
		if strings.HasSuffix(cleaned, "```") {
			cleaned = cleaned[:len(cleaned)-3]
		}
		cleaned = strings.TrimSpace(cleaned)
	}

	var llmResp llmAssessmentResponse
	if err := json.Unmarshal([]byte(cleaned), &llmResp); err != nil {
		return PatentAssessment{}, fmt.Errorf("failed to parse Claude response as JSON: %w\nraw response: %s", err, rawText)
	}

	eligibility := Eligibility(llmResp.Eligibility)
	switch eligibility {
	case EligibilityLikelyEligible, EligibilityNeedsMoreInfo, EligibilityLikelyNotEligible:
		// valid
	default:
		eligibility = EligibilityNeedsMoreInfo
	}

	if llmResp.Confidence < 0 {
		llmResp.Confidence = 0
	}
	if llmResp.Confidence > 1 {
		llmResp.Confidence = 1
	}

	return PatentAssessment{
		CaseID:            caseID,
		Eligibility:       eligibility,
		Confidence:        llmResp.Confidence,
		Recommendation:    llmResp.Recommendation,
		Summary:           llmResp.Summary,
		EligibilityReason: llmResp.EligibilityReason,
		Questions:         llmResp.Questions,
		Disclaimer:        "Preliminary automated screening only. Final eligibility requires qualified patent counsel.",
	}, nil
}
