package patentteam

import (
	"context"
	"log"
	"os"
	"strings"
)

const PatentAgentSystemPrompt = `You are outside patent counsel supporting a university technology transfer office.
Your task: review the disclosure and recommend whether to file a provisional patent.
You MUST avoid inventing facts and flag missing technical depth.`

type Eligibility string

const (
	EligibilityLikelyEligible    Eligibility = "likely_eligible"
	EligibilityNeedsMoreInfo     Eligibility = "needs_more_info"
	EligibilityLikelyNotEligible Eligibility = "likely_not_eligible"
)

type PatentAssessment struct {
	CaseID            string      `json:"case_id"`
	Eligibility       Eligibility `json:"eligibility"`
	Confidence        float64     `json:"confidence"`
	Recommendation    string      `json:"recommendation"`
	Summary           string      `json:"summary"`
	EligibilityReason []string    `json:"eligibility_reasons"`
	Questions         []string    `json:"questions_for_inventors"`
	Disclaimer        string      `json:"disclaimer"`
}

func EvaluatePatentEligibility(ctx context.Context, caseID, extractedText string) PatentAssessment {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		assessment, err := EvaluatePatentEligibilityLLM(ctx, caseID, extractedText)
		if err != nil {
			log.Printf("LLM evaluator failed, falling back to keyword heuristic: %v", err)
		} else {
			return assessment
		}
	}
	return evaluatePatentEligibilityKeyword(caseID, extractedText)
}

func evaluatePatentEligibilityKeyword(caseID, extractedText string) PatentAssessment {
	text := strings.ToLower(strings.TrimSpace(extractedText))
	length := len(text)

	technicalHits := countAny(text,
		"algorithm", "model", "protocol", "architecture", "sensor", "signal", "hardware",
		"latency", "throughput", "encryption", "compiler", "dataset", "training", "inference",
	)
	abstractHits := countAny(text,
		"business method", "marketing", "pricing", "sales", "customer segment", "advertising",
	)

	assessment := PatentAssessment{
		CaseID:         caseID,
		Recommendation: "not legal advice",
		Disclaimer:     "Preliminary automated screening only. Final eligibility requires qualified patent counsel.",
	}

	switch {
	case length < 300:
		assessment.Eligibility = EligibilityNeedsMoreInfo
		assessment.Confidence = 0.30
		assessment.Summary = "Disclosure appears too short for a reliable patent-eligibility assessment."
		assessment.EligibilityReason = []string{
			"Insufficient technical detail in extracted text.",
			"Need concrete implementation details and differentiators.",
		}
		assessment.Questions = []string{
			"What specific technical problem is solved?",
			"What is the concrete implementation (algorithms, architecture, workflows)?",
			"What is different versus closest known solutions?",
		}
	case technicalHits >= 3 && abstractHits == 0:
		assessment.Eligibility = EligibilityLikelyEligible
		assessment.Confidence = 0.68
		assessment.Summary = "Disclosure contains multiple technical implementation indicators, suggesting likely patent-eligible subject matter pending legal review."
		assessment.EligibilityReason = []string{
			"Text includes concrete technical terms and implementation signals.",
			"No strong pure-business-method indicators detected.",
		}
		assessment.Questions = []string{
			"Confirm novelty against closest prior art references.",
			"Confirm any public disclosures before filing.",
		}
	default:
		assessment.Eligibility = EligibilityLikelyNotEligible
		assessment.Confidence = 0.55
		assessment.Summary = "Disclosure is more likely to be characterized as abstract/business-focused without sufficient technical implementation detail."
		assessment.EligibilityReason = []string{
			"Technical implementation evidence is weak.",
			"Potential abstract idea/business method framing risk.",
		}
		assessment.Questions = []string{
			"Can you describe concrete technical components and data flow?",
			"What measurable technical improvement is achieved?",
		}
	}

	return assessment
}

func countAny(text string, needles ...string) int {
	hits := 0
	for _, n := range needles {
		if strings.Contains(text, n) {
			hits++
		}
	}
	return hits
}
