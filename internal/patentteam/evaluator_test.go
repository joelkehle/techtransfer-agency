package patentteam

import (
	"context"
	"os"
	"testing"
)

func TestEvaluatePatentEligibilityKeywordLikelyEligible(t *testing.T) {
	// Ensure LLM path is not triggered in keyword tests.
	os.Unsetenv("ANTHROPIC_API_KEY")

	text := `This disclosure describes a protocol and algorithm for sensor signal fusion.
The architecture includes hardware acceleration for model inference and reduced latency.
It details throughput improvements and encryption of data paths.
The method defines concrete processing stages, memory layouts, and compute scheduling.
Implementation details cover inference batching, signal pre-processing, and error correction.
Benchmarks show lower latency and better throughput on representative hardware.
The specification includes protocol flow, edge-device integration, and deployment details.`

	got := EvaluatePatentEligibility(context.Background(), "CASE-ELIG", text)
	if got.Eligibility != EligibilityLikelyEligible {
		t.Fatalf("eligibility=%s want=%s", got.Eligibility, EligibilityLikelyEligible)
	}
}

func TestEvaluatePatentEligibilityKeywordNeedsInfo(t *testing.T) {
	os.Unsetenv("ANTHROPIC_API_KEY")

	got := EvaluatePatentEligibility(context.Background(), "CASE-SHORT", "short summary")
	if got.Eligibility != EligibilityNeedsMoreInfo {
		t.Fatalf("eligibility=%s want=%s", got.Eligibility, EligibilityNeedsMoreInfo)
	}
}
