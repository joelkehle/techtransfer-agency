package patentscreen

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// USPTO / legal reference URLs used in the report markdown.
const (
	mpep2106URL       = "https://www.uspto.gov/web/offices/pac/mpep/s2106.html"
	mpep210603URL     = "https://www.uspto.gov/web/offices/pac/mpep/s2106.html#ch2100_d29a1b_139db_e0"
	mpep210604URL     = "https://www.uspto.gov/web/offices/pac/mpep/s2106.html#ch2100_d29a1b_13c11_1cb"
	mpep210604dURL    = "https://www.uspto.gov/web/offices/pac/mpep/s2106.html#ch2100_d29a1b_2117e_1e5"
	mpep210605URL     = "https://www.uspto.gov/web/offices/pac/mpep/s2106.html#ch2100_d29a1b_21506_344"
	mpepAliceMayoURL  = "https://www.uspto.gov/web/offices/pac/mpep/s2106.html#ch2100_d29a1b_139db_e0"
	berkheimerMemoURL = "https://www.uspto.gov/sites/default/files/documents/memo-berkheimer-20180419.pdf"
	mpep210605dURL    = "https://www.uspto.gov/web/offices/pac/mpep/s2106.html#ch2100_d29a1b_21506_344"
	usc101URL         = "https://www.law.cornell.edu/uscode/text/35/101"
	usc102URL         = "https://www.law.cornell.edu/uscode/text/35/102"
	usc103URL         = "https://www.law.cornell.edu/uscode/text/35/103"
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
	fmt.Fprintf(&b, "- Reference: %s\n", result.Request.CaseID)
	fmt.Fprintf(&b, "- Invention: %s\n", sanitizeLine(result.Stage1.InventionTitle))
	fmt.Fprintf(&b, "- Date: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Mode: %s\n\n", mode)
	fmt.Fprintf(&b, "%s\n\n", Disclaimer)
	if mode == "DEGRADED" {
		fmt.Fprintf(&b, "> DEGRADED: This report includes low-confidence or insufficient-information flags. Human review is required before acting.\n\n")
	}

	// --- Framework explainer ---
	fmt.Fprintf(&b, "## How This Report Works\n\n")
	fmt.Fprintf(&b, "This report follows the [Alice/Mayo eligibility framework](%s), "+
		"which is the test the U.S. Patent and Trademark Office (USPTO) uses to decide whether "+
		"an invention is eligible for patent protection under [35 U.S.C. § 101](%s). "+
		"The framework is documented in the USPTO's [Manual of Patent Examining Procedure (MPEP), § 2106](%s).\n\n",
		mpepAliceMayoURL, usc101URL, mpep2106URL)
	fmt.Fprintf(&b, "The test works as a series of stages. Each stage asks a yes/no question. "+
		"Depending on the answer, the analysis either continues to the next stage or exits early "+
		"because the eligibility question has already been resolved. When a stage is marked "+
		"\"Skipped,\" it means a prior stage already answered the eligibility question — the "+
		"explanation for why is provided inline.\n\n")

	fmt.Fprintf(&b, "## Executive Summary\n\n")
	fmt.Fprintf(&b, "Overall determination: **%s**.\n", result.FinalDetermination)
	fmt.Fprintf(&b, "%s\n", pathwayExplanation(string(result.Pathway)))
	if len(result.Metadata.NeedsReviewReasons) > 0 {
		fmt.Fprintf(&b, "Confidence override applied due to: %s.\n", strings.Join(result.Metadata.NeedsReviewReasons, "; "))
	}
	b.WriteString("\n")

	// --- Stage 1: Extraction ---
	fmt.Fprintf(&b, "---\n\n## Stage 1: Invention Extraction\n\n")
	fmt.Fprintf(&b, "Before evaluating eligibility, the system extracts key details from your disclosure. "+
		"**Please verify that this matches your invention** — if the extraction is wrong, "+
		"the downstream analysis will be unreliable.\n\n")
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

	// --- Stage 2: Statutory Category ---
	fmt.Fprintf(&b, "\n---\n\n## Stage 2: Statutory Category\n\n")
	fmt.Fprintf(&b, "U.S. patent law ([35 U.S.C. § 101](%s)) requires that an invention fall into "+
		"one of four categories: a **process** (method), **machine**, **manufacture** (manufactured article), "+
		"or **composition of matter** (chemical compound, mixture, etc.). If the invention does not fit "+
		"any of these, it cannot be patented regardless of how novel it is. "+
		"See [MPEP § 2106.03](%s).\n\n", usc101URL, mpep210603URL)
	fmt.Fprintf(&b, "**Question**: Does the invention fall within a statutory category?\n\n")
	if result.Stage2 == nil {
		writeSkipExplanation(&b, "stage_2", result)
	} else {
		if result.Stage2.PassesStep1 {
			fmt.Fprintf(&b, "**Conclusion**: Yes — the invention falls within a statutory category.\n\n")
		} else {
			fmt.Fprintf(&b, "**Conclusion**: No — the invention does not fall within a statutory category.\n\n")
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
		if !result.Stage2.PassesStep1 {
			fmt.Fprintf(&b, "\n**What this means**: The analysis stops here. Because the invention does not appear to fit "+
				"any of the four statutory categories, it is not eligible for patent protection under current law. "+
				"This does not mean the invention lacks value — only that patent protection may not be the right form of IP protection. "+
				"Consult patent counsel to discuss whether the claims could be reframed to fit a statutory category.\n")
		}
	}

	// --- Stage 3: Judicial Exception ---
	fmt.Fprintf(&b, "\n---\n\n## Stage 3: Judicial Exception\n\n")
	fmt.Fprintf(&b, "The Supreme Court has long held that certain categories of ideas cannot be patented by themselves. "+
		"The current framework for analyzing these exceptions was established in "+
		"[*Mayo Collaborative Services v. Prometheus*](https://supreme.justia.com/cases/federal/us/566/66/) (2012) and "+
		"[*Alice Corp. v. CLS Bank*](https://supreme.justia.com/cases/federal/us/573/208/) (2014). "+
		"These categories include **abstract ideas** "+
		"(e.g., mathematical formulas, mental processes, methods of organizing human activity), "+
		"**laws of nature**, and **natural phenomena**. These are called \"judicial exceptions.\" "+
		"This stage checks whether your invention's core concept falls into one of these categories. "+
		"See [MPEP § 2106.04](%s).\n\n", mpep210604URL)
	fmt.Fprintf(&b, "**Question**: Does the invention recite a judicial exception?\n\n")
	if result.Stage3 == nil {
		writeSkipExplanation(&b, "stage_3", result)
	} else {
		if result.Stage3.RecitesException {
			fmt.Fprintf(&b, "**Conclusion**: Yes — the invention recites a judicial exception.\n\n")
			fmt.Fprintf(&b, "- **Exception type**: %s\n", result.Stage3.ExceptionTypeString())
			fmt.Fprintf(&b, "- **Abstract idea subcategory**: %s\n\n", result.Stage3.SubcategoryString())
			fmt.Fprintf(&b, "**Important**: This does not mean your invention is unpatentable. Many patented inventions "+
				"involve abstract ideas or laws of nature. The next stage checks whether your invention "+
				"applies the abstract idea in a concrete, practical way — which is what matters for eligibility.\n\n")
		} else {
			fmt.Fprintf(&b, "**Conclusion**: No — the invention does not recite a judicial exception.\n\n")
		}
		fmt.Fprintf(&b, "**Reasoning**: %s\n\n", sanitizeLine(result.Stage3.Reasoning))
		fmt.Fprintf(&b, "**MPEP Reference**: [%s](%s)\n\n", sanitizeLine(result.Stage3.MPEPReference), mpep210604URL)
		fmt.Fprintf(&b, "> Confidence: %.2f — %s\n", result.Stage3.ConfidenceScore, sanitizeLine(result.Stage3.ConfidenceReason))
		if result.Stage3.InsufficientInformation {
			fmt.Fprintf(&b, "> [!] Insufficient information flagged.\n")
		}
		if !result.Stage3.RecitesException {
			fmt.Fprintf(&b, "\n**What this means**: The analysis stops here — and this is good news. "+
				"Because no judicial exception was identified, your invention clears the eligibility hurdle. "+
				"The remaining question is whether it is novel and non-obvious compared to prior art "+
				"(see the §102/§103 section below).\n")
		}
	}

	// --- Stage 4: Practical Application ---
	fmt.Fprintf(&b, "\n---\n\n## Stage 4: Practical Application\n\n")
	fmt.Fprintf(&b, "If the invention does involve a judicial exception (an abstract idea, law of nature, etc.), "+
		"it can still be patented if the claims integrate that exception into a **practical application** — "+
		"meaning the invention uses the abstract idea to achieve a concrete, real-world result, "+
		"not just \"apply it on a computer\" or \"use it in a general way.\" "+
		"See [MPEP § 2106.04(d)](%s).\n\n", mpep210604dURL)
	fmt.Fprintf(&b, "**Question**: Does the invention integrate the judicial exception into a practical application?\n\n")
	if result.Stage4 == nil {
		writeSkipExplanation(&b, "stage_4", result)
	} else {
		if result.Stage4.IntegratesPracticalApplication {
			fmt.Fprintf(&b, "**Conclusion**: Yes — the invention integrates the exception into a practical application.\n\n")
		} else {
			fmt.Fprintf(&b, "**Conclusion**: No — the invention does not integrate into a practical application.\n\n")
		}
		fmt.Fprintf(&b, "### Additional Elements Identified\n\n")
		fmt.Fprintf(&b, "These are the parts of your invention beyond the abstract idea itself:\n\n")
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
		fmt.Fprintf(&b, "**MPEP Reference**: [%s](%s)\n\n", sanitizeLine(result.Stage4.MPEPReference), mpep210604dURL)
		fmt.Fprintf(&b, "> Confidence: %.2f — %s\n", result.Stage4.ConfidenceScore, sanitizeLine(result.Stage4.ConfidenceReason))
		if result.Stage4.InsufficientInformation {
			fmt.Fprintf(&b, "> [!] Insufficient information flagged.\n")
		}
		if result.Stage4.IntegratesPracticalApplication {
			fmt.Fprintf(&b, "\n**What this means**: The analysis stops here — and this is good news. "+
				"Even though your invention involves an abstract idea, the way it applies that idea "+
				"is concrete and practical enough to qualify for patent eligibility under the "+
				"[Alice/Mayo framework](%s). Stage 5 (\"inventive concept\") is not needed when practical "+
				"application is established. The remaining question is novelty and non-obviousness "+
				"(see the §102/§103 section below).\n", mpepAliceMayoURL)
		}
	}

	// --- Stage 5: Inventive Concept ---
	fmt.Fprintf(&b, "\n---\n\n## Stage 5: Inventive Concept\n\n")
	fmt.Fprintf(&b, "This stage is only reached if the invention involves a judicial exception "+
		"(Stage 3) and does **not** integrate it into a practical application (Stage 4). "+
		"It asks whether the invention nonetheless has an \"inventive concept\" — something beyond "+
		"routine or conventional use of the abstract idea — that makes it patent-eligible. "+
		"See [MPEP § 2106.05](%s).\n\n", mpep210605URL)
	fmt.Fprintf(&b, "The [Berkheimer Memo](%s) (April 2018) requires that any finding of well-understood, routine, or "+
		"conventional activity must be supported by evidence — an examiner cannot simply assert it. "+
		"See [MPEP § 2106.05(d)](%s).\n\n", berkheimerMemoURL, mpep210605dURL)
	fmt.Fprintf(&b, "**Question**: Do the additional elements amount to significantly more than the judicial exception?\n\n")
	if result.Stage5 == nil {
		writeSkipExplanation(&b, "stage_5", result)
	} else {
		if result.Stage5.HasInventiveConcept {
			fmt.Fprintf(&b, "**Conclusion**: Yes — inventive concept present.\n\n")
		} else {
			fmt.Fprintf(&b, "**Conclusion**: No — no inventive concept found.\n\n")
		}
		fmt.Fprintf(&b, "**Reasoning**: %s\n\n", sanitizeLine(result.Stage5.Reasoning))
		fmt.Fprintf(&b, "**Berkheimer Considerations** ([Berkheimer Memo](%s), [MPEP § 2106.05(d)](%s)): %s\n\n",
			berkheimerMemoURL, mpep210605dURL, sanitizeLine(result.Stage5.BerkheimerConsiderations))
		fmt.Fprintf(&b, "**MPEP Reference**: [%s](%s)\n\n", sanitizeLine(result.Stage5.MPEPReference), mpep210605URL)
		fmt.Fprintf(&b, "> Confidence: %.2f — %s\n", result.Stage5.ConfidenceScore, sanitizeLine(result.Stage5.ConfidenceReason))
		if result.Stage5.InsufficientInformation {
			fmt.Fprintf(&b, "> [!] Insufficient information flagged.\n")
		}
		if !result.Stage5.HasInventiveConcept {
			fmt.Fprintf(&b, "\n**What this means**: The analysis did not find elements that go significantly "+
				"beyond the abstract idea. This suggests the invention, as currently described, may face "+
				"a [§ 101](%s) rejection during patent examination. However, this is often addressable "+
				"through better claim drafting — consult patent counsel to explore whether the claims "+
				"can be narrowed or restructured to highlight the inventive aspects.\n", usc101URL)
		}
	}

	// --- §102/§103 Advisory Flags ---
	fmt.Fprintf(&b, "\n---\n\n## Prior Art Advisory Flags\n\n")
	fmt.Fprintf(&b, "This section is separate from the eligibility analysis above. Even if an invention is "+
		"eligible for patenting, it must also be **novel** (not already known — [35 U.S.C. § 102](%s)) "+
		"and **non-obvious** (not an obvious combination of known ideas — [35 U.S.C. § 103](%s)). "+
		"The flags below are preliminary concerns based on the disclosure text alone, without "+
		"a formal prior art search.\n\n", usc102URL, usc103URL)
	fmt.Fprintf(&b, "**Prior Art Search Priority**: `%s`\n\n", result.Stage6.PriorArtSearchPriority)
	fmt.Fprintf(&b, "### Novelty Concerns (§ 102)\n\n")
	for _, c := range result.Stage6.NoveltyConcerns {
		fmt.Fprintf(&b, "- %s\n", sanitizeLine(c))
	}
	if len(result.Stage6.NoveltyConcerns) == 0 {
		fmt.Fprintf(&b, "- No explicit novelty concerns flagged from disclosure text alone.\n")
	}
	fmt.Fprintf(&b, "\n### Non-Obviousness Concerns (§ 103)\n\n")
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

	// --- Recommended Next Steps ---
	fmt.Fprintf(&b, "\n---\n\n## Recommended Next Steps\n\n")
	switch result.FinalDetermination {
	case DeterminationLikelyEligible:
		fmt.Fprintf(&b, "Your invention appears to be eligible for patent protection. The next step is a "+
			"**prior art search** to determine whether it is novel and non-obvious compared to existing "+
			"patents and publications, followed by a formal **patentability opinion** from patent counsel.\n")
	case DeterminationLikelyNotEligible:
		fmt.Fprintf(&b, "The analysis identified potential [§ 101](%s) eligibility concerns. This does **not** "+
			"mean your invention is unpatentable — these issues are often resolved through claim drafting. "+
			"We recommend reviewing the specific stages flagged above with patent counsel before "+
			"investing in a prior art search.\n", usc101URL)
	default:
		fmt.Fprintf(&b, "The automated screen could not make a confident determination. We recommend "+
			"human review of the specific stages flagged above. The low-confidence areas are noted "+
			"with [!] markers throughout the report.\n")
	}
	b.WriteString("\n")

	// --- Appendix ---
	fmt.Fprintf(&b, "---\n\n## Appendix\n\n")

	fmt.Fprintf(&b, "### Glossary\n\n")
	fmt.Fprintf(&b, "| Term | Meaning |\n|------|--------|\n")
	fmt.Fprintf(&b, "| [§ 101](%s) | The patent statute requiring inventions be useful, novel, and fall within eligible categories |\n", usc101URL)
	fmt.Fprintf(&b, "| [§ 102](%s) | The novelty requirement — the invention must not already be publicly known |\n", usc102URL)
	fmt.Fprintf(&b, "| [§ 103](%s) | The non-obviousness requirement — the invention must not be an obvious variation of known ideas |\n", usc103URL)
	fmt.Fprintf(&b, "| [Alice/Mayo](%s) | Supreme Court cases establishing the two-step test for patent eligibility |\n", mpepAliceMayoURL)
	fmt.Fprintf(&b, "| [MPEP](%s) | Manual of Patent Examining Procedure — the USPTO examiner's guidebook |\n", mpep2106URL)
	fmt.Fprintf(&b, "| Judicial exception | An abstract idea, law of nature, or natural phenomenon that cannot be patented by itself |\n")
	fmt.Fprintf(&b, "| Practical application | Using an abstract idea in a specific, concrete way that goes beyond the idea itself |\n")
	fmt.Fprintf(&b, "| Inventive concept | An element that transforms an abstract idea into something significantly more than the idea alone |\n")
	fmt.Fprintf(&b, "| [Berkheimer Memo](%s) | USPTO guidance requiring evidence for any finding that claim elements are well-understood, routine, and conventional |\n", berkheimerMemoURL)
	fmt.Fprintf(&b, "| Well-understood, routine, and conventional (WURC) | Claim elements that examiners assert are standard in the field and, under Berkheimer, must be supported by evidence |\n")
	fmt.Fprintf(&b, "| Statutory category | The four § 101 categories: process, machine, manufacture, or composition of matter |\n")
	fmt.Fprintf(&b, "| Prior art | Existing patents, publications, or public knowledge that an invention is compared against |\n\n")

	fmt.Fprintf(&b, "### Stage Outputs (JSON)\n\n```json\n%s\n```\n", prettyJSON(stageOutputs))
	fmt.Fprintf(&b, "\n### Pipeline Metadata (JSON)\n\n```json\n%s\n```\n", prettyJSON(result.Metadata))
	if len(result.Metadata.DecisionTrace) > 0 {
		fmt.Fprintf(&b, "\n### Decision Trace (JSON)\n\n```json\n%s\n```\n", prettyJSON(result.Metadata.DecisionTrace))
	}

	return b.String()
}

// writeSkipExplanation writes a human-readable explanation of why a stage was skipped.
func writeSkipExplanation(b *strings.Builder, stage string, result PipelineResult) {
	switch {
	case stage == "stage_2":
		// Stage 2 is never skipped in practice — it always runs.
		fmt.Fprintf(b, "**Conclusion**: Not evaluated.\n\n")

	case stage == "stage_3" && result.Pathway == PathwayA:
		fmt.Fprintf(b, "**Conclusion**: Not evaluated — analysis ended at Stage 2.\n\n")
		fmt.Fprintf(b, "Because Stage 2 determined that the invention does not fall within a statutory category, "+
			"the remaining eligibility stages do not apply. The [Alice/Mayo framework](%s) only requires "+
			"analysis of judicial exceptions (this stage) and beyond when the invention first qualifies "+
			"as a process, machine, manufacture, or composition of matter.\n\n", mpepAliceMayoURL)

	case stage == "stage_4" && result.Pathway == PathwayA:
		fmt.Fprintf(b, "**Conclusion**: Not evaluated — analysis ended at Stage 2.\n\n")
		fmt.Fprintf(b, "This stage was not needed because the invention did not pass the statutory category "+
			"check in Stage 2.\n\n")

	case stage == "stage_4" && result.Pathway == PathwayB1:
		fmt.Fprintf(b, "**Conclusion**: Not evaluated — analysis ended at Stage 3.\n\n")
		fmt.Fprintf(b, "Stage 3 found that your invention does **not** involve a judicial exception (abstract idea, "+
			"law of nature, or natural phenomenon). Since there is no exception to \"integrate,\" this stage "+
			"is not needed. Your invention already cleared the eligibility hurdle at Stage 3.\n\n")

	case stage == "stage_5" && result.Pathway == PathwayA:
		fmt.Fprintf(b, "**Conclusion**: Not evaluated — analysis ended at Stage 2.\n\n")
		fmt.Fprintf(b, "This stage was not needed because the invention did not pass the statutory category "+
			"check in Stage 2.\n\n")

	case stage == "stage_5" && result.Pathway == PathwayB1:
		fmt.Fprintf(b, "**Conclusion**: Not evaluated — analysis ended at Stage 3.\n\n")
		fmt.Fprintf(b, "This stage was not needed because Stage 3 found no judicial exception. "+
			"Your invention already cleared the eligibility hurdle.\n\n")

	case stage == "stage_5" && result.Pathway == PathwayB2:
		fmt.Fprintf(b, "**Conclusion**: Not evaluated — analysis ended at Stage 4.\n\n")
		fmt.Fprintf(b, "Stage 4 found that your invention integrates the judicial exception into a **practical application**. "+
			"Under the [Alice/Mayo framework](%s), once practical application is established, "+
			"the \"inventive concept\" analysis (this stage) is not required — the invention is already "+
			"eligible. See [MPEP § 2106.04(d)](%s): \"If the claim as a whole integrates the recited "+
			"judicial exception into a practical application of the exception, then the claim is not "+
			"directed to the judicial exception\" and the analysis concludes.\n\n", mpepAliceMayoURL, mpep210604dURL)

	default:
		fmt.Fprintf(b, "**Conclusion**: Not evaluated.\n\n")
		fmt.Fprintf(b, "This stage was skipped based on the results of prior stages. "+
			"See the stages above for the determination that made this stage unnecessary.\n\n")
	}
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

func pathwayExplanation(pathway string) string {
	switch pathway {
	case string(PathwayA):
		return "The invention does not appear to fall within a patentable category (process, machine, manufacture, or composition of matter)."
	case string(PathwayB1):
		return "The invention falls within a patentable category and does not involve a judicial exception (abstract idea, law of nature, or natural phenomenon)."
	case string(PathwayB2):
		return "Although the invention involves a judicial exception, it integrates that exception into a practical application with real-world utility."
	case string(PathwayC):
		return "Although the invention involves a judicial exception that is not integrated into a practical application, the specific combination of elements provides an inventive concept that goes beyond routine or conventional use."
	case string(PathwayD):
		return "The invention involves a judicial exception, does not integrate it into a practical application, and lacks an inventive concept beyond routine or conventional use of the abstract idea."
	default:
		return ""
	}
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
