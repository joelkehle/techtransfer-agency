package patentscreen

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func BuildResponse(result PipelineResult) ResponseEnvelope {
	env := ResponseEnvelope{
		CaseID:           result.Request.CaseID,
		Determination:    result.FinalDetermination,
		Pathway:          string(result.Pathway),
		StageOutputs:     map[string]any{},
		PipelineMetadata: result.Metadata,
		Disclaimer:       Disclaimer,
	}
	env.StageOutputs["stage_1"] = result.Stage1
	env.StageOutputs["stage_6"] = result.Stage6
	if result.Stage2 != nil {
		env.StageOutputs["stage_2"] = result.Stage2
	}
	if result.Stage3 != nil {
		env.StageOutputs["stage_3"] = result.Stage3
	}
	if result.Stage4 != nil {
		env.StageOutputs["stage_4"] = result.Stage4
	}
	if result.Stage5 != nil {
		env.StageOutputs["stage_5"] = result.Stage5
	}
	env.ReportMarkdown = buildMarkdown(result, env.StageOutputs)
	return env
}

func buildMarkdown(result PipelineResult, stageOutputs map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Patent Eligibility Screen Report\n\n")
	fmt.Fprintf(&b, "- Case ID: %s\n", result.Request.CaseID)
	fmt.Fprintf(&b, "- Invention: %s\n", result.Stage1.InventionTitle)
	fmt.Fprintf(&b, "- Date: %s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "%s\n\n", Disclaimer)

	fmt.Fprintf(&b, "## Executive Summary\n\n")
	fmt.Fprintf(&b, "Overall determination: **%s**.\n", result.FinalDetermination)
	fmt.Fprintf(&b, "Base pathway: **%s**.\n", result.Pathway)
	if len(result.Metadata.NeedsReviewReasons) > 0 {
		fmt.Fprintf(&b, "Confidence override applied due to: %s.\n", strings.Join(result.Metadata.NeedsReviewReasons, "; "))
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## Determination\n\n")
	fmt.Fprintf(&b, "- Value: `%s`\n", result.FinalDetermination)
	fmt.Fprintf(&b, "- Pathway: `%s`\n\n", result.Pathway)

	fmt.Fprintf(&b, "## Eligibility Analysis\n\n")
	appendStageAnalysis(&b, "Stage 2: Statutory Category (MPEP § 2106.03)", result.Stage2 != nil, stage2Determination(result), stage2Reasoning(result), stage2Confidence(result), flowDecision("stage_2", result))
	appendStageAnalysis(&b, "Stage 3: Step 2A Prong 1 Judicial Exception (MPEP § 2106.04)", result.Stage3 != nil, stage3Determination(result), stage3Reasoning(result), stage3Confidence(result), flowDecision("stage_3", result))
	appendStageAnalysis(&b, "Stage 4: Step 2A Prong 2 Practical Application (MPEP § 2106.04(d))", result.Stage4 != nil, stage4Determination(result), stage4Reasoning(result), stage4Confidence(result), flowDecision("stage_4", result))
	appendStageAnalysis(&b, "Stage 5: Step 2B Inventive Concept (MPEP § 2106.05)", result.Stage5 != nil, stage5Determination(result), stage5Reasoning(result), stage5Confidence(result), flowDecision("stage_5", result))

	fmt.Fprintf(&b, "## §102/§103 Flags\n\n")
	if len(result.Stage6.NoveltyConcerns) == 0 && len(result.Stage6.NonObviousnessConcerns) == 0 {
		fmt.Fprintf(&b, "- No explicit novelty/non-obviousness concerns flagged from disclosure text alone.\n")
	}
	for _, c := range result.Stage6.NoveltyConcerns {
		fmt.Fprintf(&b, "- Novelty concern: %s\n", c)
	}
	for _, c := range result.Stage6.NonObviousnessConcerns {
		fmt.Fprintf(&b, "- Non-obviousness concern: %s\n", c)
	}
	fmt.Fprintf(&b, "- Prior art search priority: `%s`\n\n", result.Stage6.PriorArtSearchPriority)

	fmt.Fprintf(&b, "## Recommended Next Steps\n\n")
	switch result.FinalDetermination {
	case DeterminationLikelyEligible:
		fmt.Fprintf(&b, "Proceed to prior art search and patentability opinion.\n")
	case DeterminationLikelyNotEligible:
		fmt.Fprintf(&b, "Review the § 101 concerns with patent counsel before investing in prior art search. The eligibility issues identified may be addressable through claim drafting.\n")
	default:
		fmt.Fprintf(&b, "The automated screen could not make a confident determination. Recommend human review of the specific stages flagged.\n")
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## Appendix\n\n")
	fmt.Fprintf(&b, "### Stage Outputs (JSON)\n\n```json\n%s\n```\n", prettyJSON(stageOutputs))
	fmt.Fprintf(&b, "\n### Pipeline Metadata (JSON)\n\n```json\n%s\n```\n", prettyJSON(result.Metadata))

	return b.String()
}

func appendStageAnalysis(b *strings.Builder, title string, executed bool, determination, reasoning, confidence, flow string) {
	fmt.Fprintf(b, "### %s\n\n", title)
	if !executed {
		fmt.Fprintf(b, "- Status: skipped\n")
		fmt.Fprintf(b, "- Flow decision: %s\n\n", flow)
		return
	}
	fmt.Fprintf(b, "- Determination: %s\n", determination)
	fmt.Fprintf(b, "- Reasoning: %s\n", sanitizeLine(reasoning))
	fmt.Fprintf(b, "- Confidence: %s\n", sanitizeLine(confidence))
	fmt.Fprintf(b, "- Flow decision: %s\n\n", flow)
}

func stage2Determination(r PipelineResult) string {
	if r.Stage2 == nil {
		return "skipped"
	}
	if r.Stage2.PassesStep1 {
		return "passes step 1 statutory category"
	}
	return "fails step 1 statutory category"
}

func stage2Reasoning(r PipelineResult) string {
	if r.Stage2 == nil {
		return ""
	}
	return r.Stage2.Explanation
}

func stage2Confidence(r PipelineResult) string {
	if r.Stage2 == nil {
		return ""
	}
	return fmt.Sprintf("%.2f — %s", r.Stage2.ConfidenceScore, r.Stage2.ConfidenceReason)
}

func stage3Determination(r PipelineResult) string {
	if r.Stage3 == nil {
		return "skipped"
	}
	if r.Stage3.RecitesException {
		return "recites judicial exception"
	}
	return "does not recite judicial exception"
}

func stage3Reasoning(r PipelineResult) string {
	if r.Stage3 == nil {
		return ""
	}
	return fmt.Sprintf("%s (ref: %s)", r.Stage3.Reasoning, r.Stage3.MPEPReference)
}

func stage3Confidence(r PipelineResult) string {
	if r.Stage3 == nil {
		return ""
	}
	return fmt.Sprintf("%.2f — %s", r.Stage3.ConfidenceScore, r.Stage3.ConfidenceReason)
}

func stage4Determination(r PipelineResult) string {
	if r.Stage4 == nil {
		return "skipped"
	}
	if r.Stage4.IntegratesPracticalApplication {
		return "integrates exception into practical application"
	}
	return "does not integrate exception into practical application"
}

func stage4Reasoning(r PipelineResult) string {
	if r.Stage4 == nil {
		return ""
	}
	return fmt.Sprintf("%s (ref: %s)", r.Stage4.Reasoning, r.Stage4.MPEPReference)
}

func stage4Confidence(r PipelineResult) string {
	if r.Stage4 == nil {
		return ""
	}
	return fmt.Sprintf("%.2f — %s", r.Stage4.ConfidenceScore, r.Stage4.ConfidenceReason)
}

func stage5Determination(r PipelineResult) string {
	if r.Stage5 == nil {
		return "skipped"
	}
	if r.Stage5.HasInventiveConcept {
		return "inventive concept present"
	}
	return "no inventive concept found"
}

func stage5Reasoning(r PipelineResult) string {
	if r.Stage5 == nil {
		return ""
	}
	return fmt.Sprintf("%s (Berkheimer: %s; ref: %s)", r.Stage5.Reasoning, r.Stage5.BerkheimerConsiderations, r.Stage5.MPEPReference)
}

func stage5Confidence(r PipelineResult) string {
	if r.Stage5 == nil {
		return ""
	}
	return fmt.Sprintf("%.2f — %s", r.Stage5.ConfidenceScore, r.Stage5.ConfidenceReason)
}

func flowDecision(stage string, r PipelineResult) string {
	for _, s := range r.Metadata.StagesSkipped {
		if s == stage {
			return "skipped via early exit"
		}
	}
	switch {
	case stage == "stage_2" && r.Pathway == PathwayA:
		return "exited eligibility track"
	case stage == "stage_3" && r.Pathway == PathwayB1:
		return "exited eligibility track"
	case stage == "stage_4" && r.Pathway == PathwayB2:
		return "exited eligibility track"
	}
	return "continued / executed"
}

func prettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

func sanitizeLine(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if s == "" {
		return "-"
	}
	return s
}
