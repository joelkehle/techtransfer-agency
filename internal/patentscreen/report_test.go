package patentscreen

import (
	"strings"
	"testing"
)

func TestBuildResponseIncludesDisclaimer(t *testing.T) {
	summary := "claim"
	res := PipelineResult{
		FinalDetermination: DeterminationLikelyEligible,
		Pathway:            PathwayC,
		Request:            RequestEnvelope{CaseID: "2023-107"},
		Stage1: Stage1Output{
			InventionTitle:  "Title",
			ClaimsPresent:   true,
			ClaimsSummary:   &summary,
			StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "confidence high"},
		},
		Stage6:   Stage6Output{PriorArtSearchPriority: PriorityLow, Reasoning: "reasoning long enough to pass minimum requirements", StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "confidence high"}},
		Metadata: PipelineMetadata{},
	}
	env := BuildResponse(res)
	if env.Disclaimer != Disclaimer {
		t.Fatalf("expected disclaimer constant")
	}
	if env.ReportMarkdown == "" {
		t.Fatal("expected report markdown")
	}
	if !strings.Contains(env.ReportMarkdown, "## Appendix") {
		t.Fatal("expected appendix section in markdown")
	}
	if !strings.Contains(executiveSummary(env.ReportMarkdown), "Review status: **Complete**.") {
		t.Fatal("expected complete review status in executive summary")
	}
	if strings.Contains(env.ReportMarkdown, "- Mode:") {
		t.Fatal("did not expect legacy mode label in header")
	}
	if strings.Contains(env.ReportMarkdown, "- Review Status:") {
		t.Fatal("did not expect review status in header block")
	}
	if strings.Contains(env.ReportMarkdown, "DEGRADED") {
		t.Fatal("did not expect degraded label in report")
	}
	if strings.Contains(env.ReportMarkdown, "- Reference:") || strings.Contains(env.ReportMarkdown, "- Invention:") || strings.Contains(env.ReportMarkdown, "- Date:") {
		t.Fatal("did not expect reference/invention/date metadata block in markdown body")
	}
	if strings.Contains(env.ReportMarkdown, "## Determination\n") {
		t.Fatal("did not expect determination section in markdown")
	}
	if strings.Contains(executiveSummary(env.ReportMarkdown), "C — inventive concept") {
		t.Fatal("did not expect raw pathway code in executive summary")
	}
	if !strings.Contains(executiveSummary(env.ReportMarkdown), "Although the invention involves a judicial exception that is not integrated into a practical application") {
		t.Fatal("expected plain English pathway explanation in executive summary")
	}
	if strings.Contains(env.ReportMarkdown, "the Supreme Court has ruled (in cases like") {
		t.Fatal("did not expect legacy Alice/Mayo framing text")
	}
	if !strings.Contains(env.ReportMarkdown, "The Supreme Court has long held") {
		t.Fatal("expected updated Alice/Mayo framing text")
	}
	if !strings.Contains(env.ReportMarkdown, "## Decision Path (At a Glance)") {
		t.Fatal("expected decision path summary section")
	}
	if !strings.Contains(env.ReportMarkdown, "## Recommended Next Steps") {
		t.Fatal("expected recommended next steps section")
	}
	if strings.Index(env.ReportMarkdown, "## Recommended Next Steps") > strings.Index(env.ReportMarkdown, "## Stage 1: Invention Extraction") {
		t.Fatal("expected recommended next steps to appear before stage details")
	}
	if !strings.Contains(env.ReportMarkdown, "Does the invention fall within a statutory category?") {
		t.Fatal("expected stage 2 question in decision path summary")
	}
	if !strings.Contains(env.ReportMarkdown, "| Stage | Eligibility Question | Outcome |") {
		t.Fatal("expected stage column in decision path summary table")
	}
	if _, ok := env.StageOutputs["stage_4"]; ok {
		t.Fatal("did not expect skipped stage_4 in stage outputs")
	}
	if strings.Contains(env.ReportMarkdown, "### Stage Outputs (JSON)") {
		t.Fatal("did not expect stage outputs json section in user-facing report")
	}
}

func TestBuildResponseUsesHumanReviewStatusWhenFlagsPresent(t *testing.T) {
	summary := "claim"
	res := PipelineResult{
		FinalDetermination: DeterminationNeedsFurtherReview,
		Pathway:            PathwayA,
		Request:            RequestEnvelope{CaseID: "2023-107"},
		Stage1: Stage1Output{
			InventionTitle:  "Title",
			ClaimsPresent:   true,
			ClaimsSummary:   &summary,
			StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "confidence high"},
		},
		Stage6: Stage6Output{
			PriorArtSearchPriority: PriorityLow,
			Reasoning:              "reasoning long enough to pass minimum requirements",
			StageConfidence:        StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "confidence high"},
		},
		Metadata: PipelineMetadata{
			InputTruncated:     true,
			NeedsReviewReasons: []string{"stage_2: insufficient information"},
		},
	}
	env := BuildResponse(res)
	summaryText := executiveSummary(env.ReportMarkdown)
	if !strings.Contains(summaryText, "Review status: **Human review required**.") {
		t.Fatal("expected human review required status in executive summary")
	}
	if !strings.Contains(summaryText, "Human review is required before acting because this report includes low-confidence or insufficient-information flags.") {
		t.Fatal("expected human review explanation block")
	}
	if !strings.Contains(summaryText, "Input was truncated to fit processing limits.") {
		t.Fatal("expected truncated-input explanation when input was truncated")
	}
	if strings.Contains(env.ReportMarkdown, "- Review Status:") {
		t.Fatal("did not expect review status in header block")
	}
}

func TestCaseReferenceLabel(t *testing.T) {
	if got := caseReferenceLabel("2023-107"); got != "UCLA Case #2023-107" {
		t.Fatalf("expected UCLA label for UCLA-style case ID, got %q", got)
	}
	if got := caseReferenceLabel("SUB-1772327902843171040"); got != "SUB-1772327902843171040" {
		t.Fatalf("expected passthrough for non-UCLA case ID, got %q", got)
	}
	if got := caseReferenceLabel("  "); got != "-" {
		t.Fatalf("expected '-' for blank case ID, got %q", got)
	}
}

func TestPathwayExplanation(t *testing.T) {
	pathways := []Pathway{PathwayA, PathwayB1, PathwayB2, PathwayC, PathwayD}
	for _, p := range pathways {
		if got := pathwayExplanation(string(p)); got == "" {
			t.Fatalf("expected non-empty explanation for pathway %q", p)
		}
	}
	if got := pathwayExplanation("UNKNOWN"); got != "" {
		t.Fatalf("expected empty explanation for unknown pathway, got %q", got)
	}
}

func TestBuildResponseB2UsesExplanationAndNoDeterminationSection(t *testing.T) {
	summary := "claim"
	res := PipelineResult{
		FinalDetermination: DeterminationLikelyEligible,
		Pathway:            PathwayB2,
		Request:            RequestEnvelope{CaseID: "CASE-B2"},
		Stage1: Stage1Output{
			InventionTitle:  "Title",
			ClaimsPresent:   true,
			ClaimsSummary:   &summary,
			StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "confidence high"},
		},
		Stage6:   Stage6Output{PriorArtSearchPriority: PriorityLow, Reasoning: "reasoning long enough to pass minimum requirements", StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "confidence high"}},
		Metadata: PipelineMetadata{},
	}
	env := BuildResponse(res)

	if strings.Contains(env.ReportMarkdown, "Pathway: B2 — judicial exception integrated into practical application.") {
		t.Fatal("did not expect raw pathway code line in executive summary")
	}
	if strings.Contains(env.ReportMarkdown, "## Determination\n") {
		t.Fatal("did not expect determination section")
	}
	if !strings.Contains(env.ReportMarkdown, "Although the invention involves a judicial exception, it integrates that exception into a practical application with real-world utility.") {
		t.Fatal("expected B2 plain-English pathway explanation")
	}
	if !strings.Contains(env.ReportMarkdown, "| 5 | Do the additional elements amount to significantly more than the judicial exception? | Skipped — Stage 4 answered Yes |") {
		t.Fatal("expected stage 5 skipped outcome in decision path summary for B2")
	}
}

func TestBuildResponseIncludesBerkheimerLinksAndGlossaryEntries(t *testing.T) {
	summary := "claim"
	res := PipelineResult{
		FinalDetermination: DeterminationLikelyNotEligible,
		Pathway:            PathwayD,
		Request:            RequestEnvelope{CaseID: "CASE-BERK"},
		Stage1: Stage1Output{
			InventionTitle:  "Title",
			ClaimsPresent:   true,
			ClaimsSummary:   &summary,
			StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "confidence high"},
		},
		Stage5: &Stage5Output{
			HasInventiveConcept:      false,
			Reasoning:                "reasoning",
			BerkheimerConsiderations: "well-understood, routine, conventional activity",
			MPEPReference:            "MPEP § 2106.05(d)",
			StageConfidence:          StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "confidence high"},
		},
		Stage6:   Stage6Output{PriorArtSearchPriority: PriorityLow, Reasoning: "reasoning long enough to pass minimum requirements", StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "confidence high"}},
		Metadata: PipelineMetadata{},
	}
	env := BuildResponse(res)

	if !strings.Contains(env.ReportMarkdown, "**Berkheimer Considerations** ([Berkheimer Memo](") {
		t.Fatal("expected Berkheimer Considerations line to include Berkheimer Memo link")
	}
	if !strings.Contains(env.ReportMarkdown, "[MPEP § 2106.05(d)](") {
		t.Fatal("expected Berkheimer Considerations line to include MPEP 2106.05(d) link")
	}
	if !strings.Contains(env.ReportMarkdown, "| [Berkheimer Memo](") {
		t.Fatal("expected Berkheimer Memo glossary entry in appendix")
	}
	if !strings.Contains(env.ReportMarkdown, "| Well-understood, routine, and conventional (WURC) |") {
		t.Fatal("expected WURC glossary entry in appendix")
	}
	if !strings.Contains(env.ReportMarkdown, "| Statutory category |") {
		t.Fatal("expected statutory category glossary entry in appendix")
	}
}

func TestBuildResponseDoesNotExposeJSONAppendixSections(t *testing.T) {
	summary := "claim"
	disagreement := false
	res := PipelineResult{
		FinalDetermination: DeterminationNeedsFurtherReview,
		Pathway:            PathwayC,
		Request:            RequestEnvelope{CaseID: "CASE-TRACE"},
		Stage1: Stage1Output{
			InventionTitle:  "Title",
			ClaimsPresent:   true,
			ClaimsSummary:   &summary,
			StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "confidence high"},
		},
		Stage6: Stage6Output{PriorArtSearchPriority: PriorityLow, Reasoning: "reasoning long enough to pass minimum requirements", StageConfidence: StageConfidence{ConfidenceScore: 0.9, ConfidenceReason: "confidence high"}},
		Metadata: PipelineMetadata{
			Stage5BooleanAgreement: &disagreement,
			DecisionTrace: map[string]any{
				"stage_5": map[string]any{"disagreement": true},
			},
		},
	}
	env := BuildResponse(res)
	if strings.Contains(env.ReportMarkdown, "### Pipeline Metadata (JSON)") {
		t.Fatal("did not expect pipeline metadata json section in user-facing report")
	}
	if strings.Contains(env.ReportMarkdown, "### Decision Trace (JSON)") {
		t.Fatal("did not expect decision trace json section in user-facing report")
	}
}

func executiveSummary(report string) string {
	start := strings.Index(report, "## Executive Summary")
	if start == -1 {
		return ""
	}
	rest := report[start:]
	end := strings.Index(rest, "\n---\n")
	if end == -1 {
		return rest
	}
	return rest[:end]
}
