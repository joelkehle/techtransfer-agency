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
		Request:            RequestEnvelope{CaseID: "CASE-1"},
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
	if _, ok := env.StageOutputs["stage_4"]; ok {
		t.Fatal("did not expect skipped stage_4 in stage outputs")
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

func TestBuildResponseIncludesDecisionTraceAppendixWhenPresent(t *testing.T) {
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
	if !strings.Contains(env.ReportMarkdown, "### Decision Trace (JSON)") {
		t.Fatal("expected decision trace appendix section")
	}
	if !strings.Contains(env.ReportMarkdown, "\"disagreement\": true") {
		t.Fatal("expected disagreement flag in decision trace json")
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
