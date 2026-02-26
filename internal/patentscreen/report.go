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
	mode := "COMPLETE"
	if result.Metadata.InputTruncated || len(result.Metadata.NeedsReviewReasons) > 0 {
		mode = "DEGRADED"
	}

	fmt.Fprintf(&b, "# Patent Eligibility Screen Report\n\n")
	fmt.Fprintf(&b, "- Case ID: %s\n", result.Request.CaseID)
	fmt.Fprintf(&b, "- Invention: %s\n", sanitizeLine(result.Stage1.InventionTitle))
	fmt.Fprintf(&b, "- Date: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Mode: %s\n\n", mode)
	fmt.Fprintf(&b, "%s\n\n", Disclaimer)
	if mode == "DEGRADED" {
		fmt.Fprintf(&b, "> DEGRADED: This report includes low-confidence or insufficient-information flags. Human review is required before acting.\n\n")
	}

	fmt.Fprintf(&b, "## Executive Summary\n\n")
	fmt.Fprintf(&b, "Overall determination: **%s**.\n", result.FinalDetermination)
	fmt.Fprintf(&b, "Pathway: **%s**.\n", result.Pathway)
	if len(result.Metadata.NeedsReviewReasons) > 0 {
		fmt.Fprintf(&b, "Confidence override applied due to: %s.\n", strings.Join(result.Metadata.NeedsReviewReasons, "; "))
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## Determination\n\n")
	fmt.Fprintf(&b, "- Result: `%s`\n", result.FinalDetermination)
	fmt.Fprintf(&b, "- Pathway: `%s`\n\n", result.Pathway)

	fmt.Fprintf(&b, "---\n\n## Stage 1: Invention Extraction\n\n")
	fmt.Fprintf(&b, "This is what the agent understood from the disclosure. Verify this matches your reading.\n\n")
	fmt.Fprintf(&b, "- **Title**: %s\n", sanitizeLine(result.Stage1.InventionTitle))
	fmt.Fprintf(&b, "- **Abstract**: %s\n", sanitizeLine(result.Stage1.Abstract))
	fmt.Fprintf(&b, "- **Problem Solved**: %s\n", sanitizeLine(result.Stage1.ProblemSolved))
	fmt.Fprintf(&b, "- **Technology Area**: %s\n", sanitizeLine(result.Stage1.TechnologyArea))
	fmt.Fprintf(&b, "- **Claims Present**: %s\n", yesNo(result.Stage1.ClaimsPresent))
	if result.Stage1.ClaimsPresent && result.Stage1.ClaimsSummary != nil && strings.TrimSpace(*result.Stage1.ClaimsSummary) != "" {
		fmt.Fprintf(&b, "- **Claims Summary**: %s\n\n", sanitizeLine(*result.Stage1.ClaimsSummary))
	} else {
		fmt.Fprintf(&b, "- **Claims Summary**: No claims found in disclosure\n\n")
	}
	fmt.Fprintf(&b, "### Invention Description\n\n%s\n\n", sanitizeLine(result.Stage1.InventionDescription))
	fmt.Fprintf(&b, "### Novel Elements\n\n")
	if len(result.Stage1.NovelElements) == 0 {
		fmt.Fprintf(&b, "1. (none provided)\n")
	} else {
		for i, v := range result.Stage1.NovelElements {
			fmt.Fprintf(&b, "%d. %s\n", i+1, sanitizeLine(v))
		}
	}
	fmt.Fprintf(&b, "\n> Extraction confidence: %.2f — %s\n", result.Stage1.ConfidenceScore, sanitizeLine(result.Stage1.ConfidenceReason))
	if result.Stage1.InsufficientInformation {
		fmt.Fprintf(&b, "> [!] The agent flagged insufficient information at this stage.\n")
	}

	fmt.Fprintf(&b, "\n---\n\n## Stage 2: Statutory Category (MPEP § 2106.03)\n\n")
	fmt.Fprintf(&b, "**Question**: Does the invention fall within a statutory category (process, machine, manufacture, or composition of matter)?\n\n")
	if result.Stage2 == nil {
		fmt.Fprintf(&b, "**Conclusion**: Skipped.\n\n")
		fmt.Fprintf(&b, "> Flow decision: %s\n", flowDecision("stage_2", result))
	} else {
		if result.Stage2.PassesStep1 {
			fmt.Fprintf(&b, "**Conclusion**: Yes — passes Step 1.\n\n")
		} else {
			fmt.Fprintf(&b, "**Conclusion**: No — does not fall within a statutory category.\n\n")
		}
		if len(result.Stage2.Categories) > 0 {
			fmt.Fprintf(&b, "**Categories identified**: %s\n\n", categoriesList(result.Stage2.Categories))
		} else {
			fmt.Fprintf(&b, "**Categories identified**: None\n\n")
		}
		fmt.Fprintf(&b, "**Reasoning**: %s\n\n", sanitizeLine(result.Stage2.Explanation))
		fmt.Fprintf(&b, "> Confidence: %.2f — %s\n", result.Stage2.ConfidenceScore, sanitizeLine(result.Stage2.ConfidenceReason))
		if result.Stage2.InsufficientInformation {
			fmt.Fprintf(&b, "> [!] Insufficient information flagged.\n")
		}
		fmt.Fprintf(&b, "> Flow decision: %s\n", flowDecision("stage_2", result))
		if !result.Stage2.PassesStep1 {
			fmt.Fprintf(&b, "\nAnalysis exits the eligibility track here. The invention does not appear to fall within a statutory category.\n")
		}
	}

	fmt.Fprintf(&b, "\n---\n\n## Stage 3: Judicial Exception — Step 2A, Prong 1 (MPEP § 2106.04)\n\n")
	fmt.Fprintf(&b, "**Question**: Does the claim recite a judicial exception (abstract idea, law of nature, or natural phenomenon)?\n\n")
	if result.Stage3 == nil {
		fmt.Fprintf(&b, "**Conclusion**: Skipped.\n\n> Flow decision: %s\n", flowDecision("stage_3", result))
	} else {
		if result.Stage3.RecitesException {
			fmt.Fprintf(&b, "**Conclusion**: Yes — recites a judicial exception.\n\n")
			fmt.Fprintf(&b, "- **Exception type**: %s\n", result.Stage3.ExceptionTypeString())
			fmt.Fprintf(&b, "- **Abstract idea subcategory**: %s\n\n", result.Stage3.SubcategoryString())
		} else {
			fmt.Fprintf(&b, "**Conclusion**: No — does not recite a judicial exception.\n\n")
		}
		fmt.Fprintf(&b, "**Reasoning**: %s\n\n", sanitizeLine(result.Stage3.Reasoning))
		fmt.Fprintf(&b, "**MPEP Reference**: %s\n\n", sanitizeLine(result.Stage3.MPEPReference))
		fmt.Fprintf(&b, "> Confidence: %.2f — %s\n", result.Stage3.ConfidenceScore, sanitizeLine(result.Stage3.ConfidenceReason))
		if result.Stage3.InsufficientInformation {
			fmt.Fprintf(&b, "> [!] Insufficient information flagged.\n")
		}
		fmt.Fprintf(&b, "> Flow decision: %s\n", flowDecision("stage_3", result))
		if !result.Stage3.RecitesException {
			fmt.Fprintf(&b, "\nAnalysis exits the eligibility track here. No judicial exception was identified, so no further eligibility analysis is needed.\n")
		}
	}

	fmt.Fprintf(&b, "\n---\n\n## Stage 4: Practical Application — Step 2A, Prong 2 (MPEP § 2106.04(d))\n\n")
	fmt.Fprintf(&b, "**Question**: Does the claim as a whole integrate the judicial exception into a practical application?\n\n")
	if result.Stage4 == nil {
		fmt.Fprintf(&b, "**Conclusion**: Skipped.\n\n> Flow decision: %s\n", flowDecision("stage_4", result))
	} else {
		if result.Stage4.IntegratesPracticalApplication {
			fmt.Fprintf(&b, "**Conclusion**: Yes — integrates into practical application.\n\n")
		} else {
			fmt.Fprintf(&b, "**Conclusion**: No — does not integrate into practical application.\n\n")
		}
		fmt.Fprintf(&b, "### Additional Elements Identified\n\n")
		if len(result.Stage4.AdditionalElements) == 0 {
			fmt.Fprintf(&b, "1. (none identified)\n")
		} else {
			for i, v := range result.Stage4.AdditionalElements {
				fmt.Fprintf(&b, "%d. %s\n", i+1, sanitizeLine(v))
			}
		}
		fmt.Fprintf(&b, "\n### Considerations Supporting Integration\n\n")
		if len(result.Stage4.ConsiderationsFor) == 0 {
			fmt.Fprintf(&b, "- (none listed)\n")
		} else {
			for _, v := range result.Stage4.ConsiderationsFor {
				fmt.Fprintf(&b, "- %s\n", sanitizeLine(v))
			}
		}
		fmt.Fprintf(&b, "\n### Considerations Against Integration\n\n")
		if len(result.Stage4.ConsiderationsAgainst) == 0 {
			fmt.Fprintf(&b, "- (none listed)\n")
		} else {
			for _, v := range result.Stage4.ConsiderationsAgainst {
				fmt.Fprintf(&b, "- %s\n", sanitizeLine(v))
			}
		}
		fmt.Fprintf(&b, "\n**Reasoning**: %s\n\n", sanitizeLine(result.Stage4.Reasoning))
		fmt.Fprintf(&b, "**MPEP Reference**: %s\n\n", sanitizeLine(result.Stage4.MPEPReference))
		fmt.Fprintf(&b, "> Confidence: %.2f — %s\n", result.Stage4.ConfidenceScore, sanitizeLine(result.Stage4.ConfidenceReason))
		if result.Stage4.InsufficientInformation {
			fmt.Fprintf(&b, "> [!] Insufficient information flagged.\n")
		}
		fmt.Fprintf(&b, "> Flow decision: %s\n", flowDecision("stage_4", result))
		if result.Stage4.IntegratesPracticalApplication {
			fmt.Fprintf(&b, "\nAnalysis exits the eligibility track here. The judicial exception is integrated into a practical application.\n")
		}
	}

	fmt.Fprintf(&b, "\n---\n\n## Stage 5: Inventive Concept — Step 2B (MPEP § 2106.05)\n\n")
	fmt.Fprintf(&b, "**Question**: Do the additional elements, individually or in combination, amount to significantly more than the judicial exception?\n\n")
	if result.Stage5 == nil {
		fmt.Fprintf(&b, "**Conclusion**: Skipped.\n")
	} else {
		if result.Stage5.HasInventiveConcept {
			fmt.Fprintf(&b, "**Conclusion**: Yes — inventive concept present.\n\n")
		} else {
			fmt.Fprintf(&b, "**Conclusion**: No — no inventive concept found.\n\n")
		}
		fmt.Fprintf(&b, "**Reasoning**: %s\n\n", sanitizeLine(result.Stage5.Reasoning))
		fmt.Fprintf(&b, "**Berkheimer Considerations**: %s\n\n", sanitizeLine(result.Stage5.BerkheimerConsiderations))
		fmt.Fprintf(&b, "**MPEP Reference**: %s\n\n", sanitizeLine(result.Stage5.MPEPReference))
		fmt.Fprintf(&b, "> Confidence: %.2f — %s\n", result.Stage5.ConfidenceScore, sanitizeLine(result.Stage5.ConfidenceReason))
		if result.Stage5.InsufficientInformation {
			fmt.Fprintf(&b, "> [!] Insufficient information flagged.\n")
		}
	}

	fmt.Fprintf(&b, "\n---\n\n## §102/§103 Advisory Flags\n\n")
	fmt.Fprintf(&b, "This section is advisory only and does not change the eligibility determination above.\n\n")
	fmt.Fprintf(&b, "**Prior Art Search Priority**: `%s`\n\n", result.Stage6.PriorArtSearchPriority)
	fmt.Fprintf(&b, "### Novelty Concerns (§102)\n\n")
	if len(result.Stage6.NoveltyConcerns) == 0 && len(result.Stage6.NonObviousnessConcerns) == 0 {
		fmt.Fprintf(&b, "- No explicit novelty/non-obviousness concerns flagged from disclosure text alone.\n")
	}
	for _, c := range result.Stage6.NoveltyConcerns {
		fmt.Fprintf(&b, "- %s\n", sanitizeLine(c))
	}
	if len(result.Stage6.NoveltyConcerns) == 0 {
		fmt.Fprintf(&b, "- No explicit novelty concerns flagged from disclosure text alone.\n")
	}
	fmt.Fprintf(&b, "\n### Non-Obviousness Concerns (§103)\n\n")
	for _, c := range result.Stage6.NonObviousnessConcerns {
		fmt.Fprintf(&b, "- %s\n", sanitizeLine(c))
	}
	if len(result.Stage6.NonObviousnessConcerns) == 0 {
		fmt.Fprintf(&b, "- No explicit non-obviousness concerns flagged from disclosure text alone.\n")
	}
	fmt.Fprintf(&b, "\n**Reasoning**: %s\n\n", sanitizeLine(result.Stage6.Reasoning))
	fmt.Fprintf(&b, "> Confidence: %.2f — %s\n", result.Stage6.ConfidenceScore, sanitizeLine(result.Stage6.ConfidenceReason))
	if result.Stage6.InsufficientInformation {
		fmt.Fprintf(&b, "> [!] Insufficient information flagged.\n")
	}

	fmt.Fprintf(&b, "\n---\n\n## Recommended Next Steps\n\n")
	switch result.FinalDetermination {
	case DeterminationLikelyEligible:
		fmt.Fprintf(&b, "Proceed to prior art search and patentability opinion.\n")
	case DeterminationLikelyNotEligible:
		fmt.Fprintf(&b, "Review the § 101 concerns with patent counsel before investing in prior art search. The eligibility issues identified may be addressable through claim drafting.\n")
	default:
		fmt.Fprintf(&b, "The automated screen could not make a confident determination. Recommend human review of the specific stages flagged.\n")
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "---\n\n## Appendix\n\n")
	fmt.Fprintf(&b, "### Stage Outputs (JSON)\n\n```json\n%s\n```\n", prettyJSON(stageOutputs))
	fmt.Fprintf(&b, "\n### Pipeline Metadata (JSON)\n\n```json\n%s\n```\n", prettyJSON(result.Metadata))

	return b.String()
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

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func categoriesList(v []Stage2Category) string {
	if len(v) == 0 {
		return "None"
	}
	var out []string
	for _, c := range v {
		out = append(out, string(c))
	}
	return strings.Join(out, ", ")
}

func (s *Stage3Output) ExceptionTypeString() string {
	if s.ExceptionType == nil {
		return "N/A"
	}
	return string(*s.ExceptionType)
}

func (s *Stage3Output) SubcategoryString() string {
	if s.AbstractIdeaSubcategory == nil {
		return "N/A"
	}
	return string(*s.AbstractIdeaSubcategory)
}
