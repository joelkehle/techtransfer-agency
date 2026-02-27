package marketanalysis

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Reference URLs used in the report markdown.
const (
	autmSurveyURL      = "https://autm.net/surveys-and-tools/surveys/licensing-activity-survey"
	rnpvMethodURL      = "https://www.investopedia.com/terms/r/rnpv.asp"
	tamSamSomURL       = "https://www.investopedia.com/terms/t/tam.asp"
	npvURL             = "https://www.investopedia.com/terms/n/npv.asp"
	discountRateURL    = "https://www.investopedia.com/terms/d/discountrate.asp"
	royaltyRateURL     = "https://www.investopedia.com/terms/r/royalty.asp"
	sensitivityURL     = "https://www.investopedia.com/terms/s/sensitivityanalysis.asp"
	somURL             = "https://www.investopedia.com/terms/s/serviceable-obtainable-market-som.asp"
	samURL             = "https://www.investopedia.com/terms/s/serviceable-available-market.asp"
)

func BuildResponse(result PipelineResult) ResponseEnvelope {
	env := ResponseEnvelope{
		CaseID:                   result.Request.CaseID,
		Recommendation:           result.Decision.Tier,
		RecommendationConfidence: result.Decision.Confidence,
		ReportMode:               result.Metadata.Mode,
		StageOutputs:             map[string]any{},
		PipelineMetadata:         result.Metadata,
		Disclaimer:               Disclaimer,
	}
	env.StageOutputs["stage_0"] = result.Stage0
	if result.Stage1 != nil {
		env.StageOutputs["stage_1"] = result.Stage1
	}
	if result.Stage2 != nil {
		env.StageOutputs["stage_2"] = result.Stage2
	}
	if result.Stage3 != nil {
		env.StageOutputs["stage_3"] = result.Stage3
	}
	if result.Stage4 != nil {
		env.StageOutputs["stage_4"] = result.Stage4
		env.StageOutputs["stage_4_computed"] = result.Stage4Computed
	}
	if result.Stage5 != nil {
		env.StageOutputs["stage_5"] = result.Stage5
	}
	env.ReportMarkdown = buildMarkdown(result, env.StageOutputs)
	return env
}

func buildMarkdown(result PipelineResult, stageOutputs map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Market Analysis Report\n\n")
	fmt.Fprintf(&b, "- Case ID: %s\n", result.Request.CaseID)
	fmt.Fprintf(&b, "- Invention: %s\n", sanitize(asString(result.Stage0.InventionTitle.Value)))
	fmt.Fprintf(&b, "- Date: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Mode: %s\n\n", result.Metadata.Mode)
	fmt.Fprintf(&b, "%s\n\n", Disclaimer)

	if result.Metadata.Mode == ReportModeDegraded {
		fmt.Fprintf(&b, "> DEGRADED: Stage `%s` failed or short-circuited. Treat this as partial analysis pending human review.\n\n", sanitize(result.Metadata.StageFailed))
	}

	// --- How This Report Works ---
	fmt.Fprintf(&b, "## How This Report Works\n\n")
	fmt.Fprintf(&b, "This report is a **speed-1 commercial triage** — a rapid, automated first look at whether "+
		"an invention has enough market potential to justify the time and cost of patent filing. "+
		"It is not a full valuation or investment recommendation.\n\n")
	fmt.Fprintf(&b, "The analysis runs through a six-stage pipeline:\n\n")
	fmt.Fprintf(&b, "1. **Extract** (Stage 0) — Pull key facts from the disclosure (title, problem, solution, target users, sector)\n")
	fmt.Fprintf(&b, "2. **Path** (Stage 1) — Identify the most viable commercialization route (exclusive license, startup, non-exclusive, etc.)\n")
	fmt.Fprintf(&b, "3. **Score** (Stage 2) — Rate the invention on six commercial dimensions using a weighted scorecard\n")
	fmt.Fprintf(&b, "4. **Size** (Stage 3) — Estimate the market opportunity ([TAM](%s) / [SAM](%s) / [SOM](%s))\n", tamSamSomURL, samURL, somURL)
	fmt.Fprintf(&b, "5. **Value** (Stage 4) — Build a risk-adjusted [net present value (rNPV)](%s) model to compare expected licensing revenue against patent costs\n", rnpvMethodURL)
	fmt.Fprintf(&b, "6. **Synthesize** (Stage 5) — Combine all evidence into a GO / DEFER / NO_GO recommendation\n\n")
	fmt.Fprintf(&b, "**Domain priors**: Each assumption starts from sector-specific defaults drawn from "+
		"[AUTM licensing survey data](%s) and industry benchmarks, then gets adjusted based on "+
		"what the AI finds in the disclosure. The source tag on each assumption tells you where it came from:\n\n", autmSurveyURL)
	fmt.Fprintf(&b, "| Tag | Meaning |\n|-----|--------|\n")
	fmt.Fprintf(&b, "| `DOMAIN_DEFAULT` | Standard value for this sector from AUTM/industry data |\n")
	fmt.Fprintf(&b, "| `ADJUSTED` | Default was modified based on disclosure-specific evidence |\n")
	fmt.Fprintf(&b, "| `DISCLOSURE_DERIVED` | Value was extracted directly from the disclosure text |\n")
	fmt.Fprintf(&b, "| `ESTIMATED` | Agent's best estimate where no default or direct evidence exists |\n")
	fmt.Fprintf(&b, "| `INFERRED` | Derived indirectly from other extracted information |\n\n")
	fmt.Fprintf(&b, "**Skipped stages**: If a stage is marked \"Skipped,\" it means a prior stage already resolved "+
		"the question — the report explains why inline. Early exits are normal and do not indicate an error.\n\n")

	// --- Executive Summary ---
	fmt.Fprintf(&b, "## Executive Summary\n\n")
	if result.Stage5 != nil {
		fmt.Fprintf(&b, "%s\n\n", sanitize(result.Stage5.ExecutiveSummary))
	} else {
		fmt.Fprintf(&b, "%s\n\n", sanitize(result.Decision.Reason))
	}

	// --- Recommendation ---
	fmt.Fprintf(&b, "## Recommendation\n\n")
	fmt.Fprintf(&b, "- Tier: `%s`\n", result.Decision.Tier)
	fmt.Fprintf(&b, "- Confidence: `%s`\n", result.Decision.Confidence)
	fmt.Fprintf(&b, "- Reason: %s\n", sanitize(result.Decision.Reason))
	for _, c := range result.Decision.Caveats {
		fmt.Fprintf(&b, "- [!] %s\n", sanitize(c))
	}
	fmt.Fprintf(&b, "\n---\n\n")

	// --- Stage 0: Extraction ---
	fmt.Fprintf(&b, "## Stage 0: Invention Extraction\n\n")
	fmt.Fprintf(&b, "Before any commercial analysis begins, the system extracts key facts from your disclosure. "+
		"**Please verify that this table matches your invention** — if the extraction is wrong, "+
		"all downstream stages will be unreliable. The Confidence column shows how certain the agent is "+
		"about each extraction, and the Notes column explains any gaps.\n\n")
	fmt.Fprintf(&b, "| Field | Value | Confidence | Notes |\n")
	fmt.Fprintf(&b, "|-------|-------|------------|-------|\n")
	writeNullableRow(&b, "Title", result.Stage0.InventionTitle)
	writeNullableRow(&b, "Problem Solved", result.Stage0.ProblemSolved)
	writeNullableRow(&b, "Solution", result.Stage0.SolutionDescription)
	writeNullableRow(&b, "Claimed Advantages", result.Stage0.ClaimedAdvantages)
	writeNullableRow(&b, "Target User", result.Stage0.TargetUser)
	writeNullableRow(&b, "Target Buyer", result.Stage0.TargetBuyer)
	writeNullableRow(&b, "Application Domains", result.Stage0.ApplicationDomains)
	fmt.Fprintf(&b, "| Evidence Level | %s | — | — |\n", sanitize(string(result.Stage0.EvidenceLevel)))
	writeNullableRow(&b, "Competing Approaches", result.Stage0.CompetingApproaches)
	writeNullableRow(&b, "Dependencies", result.Stage0.Dependencies)
	fmt.Fprintf(&b, "\n- **Sector classification**: %s\n\n", sanitize(result.Stage0.Sector))
	fmt.Fprintf(&b, "Fields marked \"—\" were not found in the disclosure. The Notes column explains why. These gaps propagate uncertainty into downstream stages.\n\n---\n\n")

	// --- Stage 1: Commercialization Path ---
	fmt.Fprintf(&b, "## Stage 1: Commercialization Path Selection\n\n")
	fmt.Fprintf(&b, "The commercialization path determines how the invention would reach the market and generate revenue. "+
		"The five options are: **exclusive license** to an established company (most common for university inventions), "+
		"**startup formation**, **non-exclusive licensing** (multiple licensees), **open source + services**, "+
		"or **research use only**. The choice here affects all downstream economic assumptions — "+
		"for example, exclusive licenses typically command higher royalty rates but depend on finding a single committed partner.\n\n")
	fmt.Fprintf(&b, "**Question**: What is the most viable path to commercialize this invention?\n\n")
	if result.Stage1 == nil {
		writeSkipExplanation(&b, "stage_1", result)
	} else {
		fmt.Fprintf(&b, "**Primary path**: `%s`\n", result.Stage1.PrimaryPath)
		fmt.Fprintf(&b, "**Reasoning**: %s\n\n", sanitize(result.Stage1.PrimaryPathReasoning))
		if result.Stage1.SecondaryPath != nil {
			fmt.Fprintf(&b, "**Secondary path**: `%s`\n", *result.Stage1.SecondaryPath)
			fmt.Fprintf(&b, "**Reasoning**: %s\n\n", sanitize(ptrStr(result.Stage1.SecondaryPathReasoning)))
		}
		fmt.Fprintf(&b, "**Product definition**: %s\n\n", sanitize(result.Stage1.ProductDefinition))
		fmt.Fprintf(&b, "**Has plausible monetization**: %s\n", yesNo(result.Stage1.HasPlausibleMonetization))
		if !result.Stage1.HasPlausibleMonetization {
			fmt.Fprintf(&b, "%s\n", sanitize(ptrStr(result.Stage1.NoMonetizationReasoning)))
			fmt.Fprintf(&b, "-> Pipeline short-circuited here. Recommendation: NO_GO.\n")
		}
		if result.Stage1.NonPatentMonetization {
			fmt.Fprintf(&b, "\n**Non-patent monetization noted**: %s\n", sanitize(ptrStr(result.Stage1.NonPatentMonetizationReasoning)))
		}
		fmt.Fprintf(&b, "\n")
	}
	fmt.Fprintf(&b, "---\n\n")

	// --- Stage 2: Triage Scorecard ---
	fmt.Fprintf(&b, "## Stage 2: Triage Scorecard\n\n")
	fmt.Fprintf(&b, "The scorecard evaluates the invention across six commercial dimensions. Each dimension "+
		"is scored 1-5 (1 = weak, 5 = strong). The scores are combined using weights that reflect "+
		"which factors matter most for technology licensing:\n\n")
	fmt.Fprintf(&b, "- **Market Pain** (weight: 2.0) — How urgently does the target market need this solution?\n")
	fmt.Fprintf(&b, "- **Differentiation** (weight: 2.0) — How clearly does this stand apart from existing approaches?\n")
	fmt.Fprintf(&b, "- **Adoption Friction** (weight: 1.5) — How easy is it for customers to switch to this? (Higher score = lower friction)\n")
	fmt.Fprintf(&b, "- **Development Burden** (weight: 1.0) — How much additional development is needed? (Higher score = less work)\n")
	fmt.Fprintf(&b, "- **Partner Density** (weight: 1.5) — Are there potential licensees/partners in this space?\n")
	fmt.Fprintf(&b, "- **IP Leverage** (weight: 1.0) — How well does patent protection create a competitive moat?\n\n")
	fmt.Fprintf(&b, "**Question**: Does this invention clear the bar for further commercial analysis?\n\n")
	if result.Stage2 != nil {
		fmt.Fprintf(&b, "| Dimension | Score (1-5) | Reasoning |\n")
		fmt.Fprintf(&b, "|-----------|-------------|-----------|\n")
		fmt.Fprintf(&b, "| Market Pain | %d | %s |\n", result.Stage2.Scores.MarketPain.Score, sanitizeCell(result.Stage2.Scores.MarketPain.Reasoning))
		fmt.Fprintf(&b, "| Differentiation | %d | %s |\n", result.Stage2.Scores.Differentiation.Score, sanitizeCell(result.Stage2.Scores.Differentiation.Reasoning))
		fmt.Fprintf(&b, "| Adoption Friction | %d | %s |\n", result.Stage2.Scores.AdoptionFriction.Score, sanitizeCell(result.Stage2.Scores.AdoptionFriction.Reasoning))
		fmt.Fprintf(&b, "| Development Burden | %d | %s |\n", result.Stage2.Scores.DevelopmentBurden.Score, sanitizeCell(result.Stage2.Scores.DevelopmentBurden.Reasoning))
		fmt.Fprintf(&b, "| Partner Density | %d | %s |\n", result.Stage2.Scores.PartnerDensity.Score, sanitizeCell(result.Stage2.Scores.PartnerDensity.Reasoning))
		fmt.Fprintf(&b, "| IP Leverage | %d | %s |\n", result.Stage2.Scores.IPLeverage.Score, sanitizeCell(result.Stage2.Scores.IPLeverage.Reasoning))
		fmt.Fprintf(&b, "\n**How to read these scores**: The composite score is the simple average of all six dimensions. "+
			"The weighted score applies the weights listed above, giving more influence to market pain and differentiation. "+
			"A weighted score below 2.0 triggers an automatic NO_GO; between 2.0 and 2.5 with low confidence triggers DEFER.\n\n")
		fmt.Fprintf(&b, "- Composite score: %.2f\n", result.Stage2.CompositeScore)
		fmt.Fprintf(&b, "- Weighted score: %.2f\n", result.Stage2.WeightedScore)
		fmt.Fprintf(&b, "- Confidence: `%s` — %s\n", result.Stage2.Confidence, sanitize(result.Stage2.ConfidenceReasoning))
		if len(result.Stage2.UnknownKeyFactors) > 0 {
			fmt.Fprintf(&b, "- Unknown key factors:\n")
			for _, k := range result.Stage2.UnknownKeyFactors {
				fmt.Fprintf(&b, "  - %s\n", sanitize(k))
			}
		}
	} else {
		writeSkipExplanation(&b, "stage_2", result)
	}
	fmt.Fprintf(&b, "\n---\n\n")

	// --- Stage 3: Market Sizing ---
	fmt.Fprintf(&b, "## Stage 3: Market Sizing\n\n")
	fmt.Fprintf(&b, "Market sizing estimates the revenue opportunity using a three-level funnel:\n\n")
	fmt.Fprintf(&b, "- [**TAM**](%s) (Total Addressable Market) — The total global revenue if every potential customer adopted the technology\n", tamSamSomURL)
	fmt.Fprintf(&b, "- [**SAM**](%s) (Serviceable Available Market) — The portion of TAM reachable through the chosen commercialization path and geography\n", samURL)
	fmt.Fprintf(&b, "- [**SOM**](%s) (Serviceable Obtainable Market) — The realistic market share achievable in the first few years, given competition and adoption rates\n\n", somURL)
	fmt.Fprintf(&b, "Each estimate includes a low-to-high range and the assumptions behind it. "+
		"The SOM figure feeds into Stage 4's revenue calculations.\n\n")
	if result.Stage3 != nil {
		writeRangeBlock(&b, "TAM", result.Stage3.TAM)
		writeRangeBlock(&b, "SAM", result.Stage3.SAM)
		writeRangeBlock(&b, "SOM", result.Stage3.SOM)
		if result.Stage3.TAMSOMRatioWarning != nil {
			fmt.Fprintf(&b, "- [!] TAM/SOM warning: %s\n", sanitize(*result.Stage3.TAMSOMRatioWarning))
		}
	} else {
		writeSkipExplanation(&b, "stage_3", result)
	}
	fmt.Fprintf(&b, "\n---\n\n")

	// --- Stage 4: Economic Viability ---
	fmt.Fprintf(&b, "## Stage 4: Economic Viability\n\n")
	fmt.Fprintf(&b, "This stage answers the core financial question: **Is the expected licensing revenue likely to exceed the cost of patenting?**\n\n")
	fmt.Fprintf(&b, "The model works as follows:\n\n")
	fmt.Fprintf(&b, "1. **Estimate royalty income**: [Royalty rate](%s) %% of licensee's annual revenue, multiplied by the license duration\n", royaltyRateURL)
	fmt.Fprintf(&b, "2. **Apply risk adjustments**: Multiply by the probability of finding a licensee (P(license)) and the probability "+
		"of commercial success (P(commercial success)) to get a risk-adjusted revenue stream\n")
	fmt.Fprintf(&b, "3. **Discount for timing**: Because money received years from now is worth less than money today, "+
		"future revenue is [discounted](%s) back to present value using a standard [discount rate](%s)\n", npvURL, discountRateURL)
	fmt.Fprintf(&b, "4. **Compare to patent cost**: If the risk-adjusted [net present value (NPV)](%s) exceeds the estimated patent cost, "+
		"the invention clears the economic viability threshold\n\n", npvURL)
	fmt.Fprintf(&b, "Three scenarios are computed — pessimistic (all low-end assumptions), base (midpoints), "+
		"and optimistic (all high-end assumptions) — to show the range of possible outcomes.\n\n")
	if result.Stage4Computed != nil {
		if result.Stage4 != nil {
			fmt.Fprintf(&b, "### Assumption Provenance\n\n")
			fmt.Fprintf(&b, "Each assumption below shows its low-to-high range, source tag (see \"How This Report Works\" above), and reasoning.\n\n")
			writeAssumptionBlock(&b, "Royalty rate (%)", result.Stage4.RoyaltyRatePct.Low, result.Stage4.RoyaltyRatePct.High, result.Stage4.RoyaltyRatePct.Source, result.Stage4.RoyaltyRatePct.Reasoning)
			writeAssumptionBlock(&b, "P(license in 3 years)", result.Stage4.PLicense3yr.Low, result.Stage4.PLicense3yr.High, result.Stage4.PLicense3yr.Source, result.Stage4.PLicense3yr.Reasoning)
			writeAssumptionBlock(&b, "P(commercial success)", result.Stage4.PCommercialSuccess.Low, result.Stage4.PCommercialSuccess.High, result.Stage4.PCommercialSuccess.Source, result.Stage4.PCommercialSuccess.Reasoning)
			writeAssumptionBlockInt(&b, "Time to license (months)", result.Stage4.TimeToLicenseMonths.Low, result.Stage4.TimeToLicenseMonths.High, result.Stage4.TimeToLicenseMonths.Source, result.Stage4.TimeToLicenseMonths.Reasoning)
			writeAssumptionBlockInt(&b, "Time from license to revenue (months)", result.Stage4.TimeFromLicenseToRevenueMonths.Low, result.Stage4.TimeFromLicenseToRevenueMonths.High, result.Stage4.TimeFromLicenseToRevenueMonths.Source, result.Stage4.TimeFromLicenseToRevenueMonths.Reasoning)
			writeAssumptionBlockInt(&b, "Annual revenue to licensee (USD)", result.Stage4.AnnualRevenueToLicenseeUSD.Low, result.Stage4.AnnualRevenueToLicenseeUSD.High, result.Stage4.AnnualRevenueToLicenseeUSD.Source, result.Stage4.AnnualRevenueToLicenseeUSD.Reasoning)
			writeAssumptionBlockInt(&b, "License duration (years)", result.Stage4.LicenseDurationYears.Low, result.Stage4.LicenseDurationYears.High, result.Stage4.LicenseDurationYears.Source, result.Stage4.LicenseDurationYears.Reasoning)
			writeAssumptionBlockInt(&b, "Patent cost (USD)", result.Stage4.PatentCostUSD.Low, result.Stage4.PatentCostUSD.High, result.Stage4.PatentCostUSD.Source, result.Stage4.PatentCostUSD.Reasoning)
		}
		fmt.Fprintf(&b, "### Scenario Results\n\n")
		for _, name := range []string{"pessimistic", "base", "optimistic"} {
			s := result.Stage4Computed.Scenarios[name]
			fmt.Fprintf(&b, "- %s: NPV $%s (exceeds patent cost: %s)\n", scenarioLabel(name), fmtUSDf(s.NPVUSD), yesNo(s.ExceedsPatentCost))
		}
		fmt.Fprintf(&b, "- Patent cost midpoint: $%s\n", fmtUSDf(result.Stage4Computed.PatentCostMidUSD))
		if len(result.Stage4Computed.SensitivityDrivers) > 0 {
			fmt.Fprintf(&b, "\n### Sensitivity Ranking\n\n")
			fmt.Fprintf(&b, "[Sensitivity analysis](%s) shows which assumptions have the biggest impact on the result. "+
				"If you change one assumption from its low value to its high value while holding everything else "+
				"at the base case, the NPV changes by the \"delta\" amount shown below. **These are the assumptions "+
				"most worth double-checking** — a small error in a high-sensitivity assumption can flip the recommendation.\n\n", sensitivityURL)
			for i, d := range result.Stage4Computed.SensitivityDrivers {
				fmt.Fprintf(&b, "%d. %s — NPV delta: $%s (%s)\n", i+1, sanitize(d.Assumption), fmtUSDf(d.NPVDeltaUSD), sanitize(d.Direction))
			}
		}
		if result.Stage4Computed.PathModelLimitation != nil {
			fmt.Fprintf(&b, "\n- Model limitation: %s\n", sanitize(*result.Stage4Computed.PathModelLimitation))
		}
	} else {
		writeSkipExplanation(&b, "stage_4", result)
	}
	fmt.Fprintf(&b, "\n---\n\n")

	// --- Stage 5: Decision Synthesis ---
	fmt.Fprintf(&b, "## Stage 5: Decision Synthesis\n\n")
	fmt.Fprintf(&b, "The recommendation tier (GO / DEFER / NO_GO) is determined by code based on the scenario results "+
		"from Stage 4 — not by the AI's subjective judgment. Stage 5 then synthesizes all prior evidence "+
		"into an executive summary, identifies the key drivers of the recommendation, and suggests next steps.\n\n")
	if result.Stage5 != nil {
		if len(result.Stage5.KeyDrivers) > 0 {
			fmt.Fprintf(&b, "### Key Drivers\n")
			for _, d := range result.Stage5.KeyDrivers {
				fmt.Fprintf(&b, "- %s\n", sanitize(d))
			}
			fmt.Fprintf(&b, "\n")
		}
		if len(result.Stage5.DiligenceQuestions) > 0 {
			fmt.Fprintf(&b, "### Diligence Questions\n")
			for i, q := range result.Stage5.DiligenceQuestions {
				fmt.Fprintf(&b, "%d. %s\n", i+1, sanitize(q))
			}
			fmt.Fprintf(&b, "\n")
		}
		if len(result.Stage5.RecommendedActions) > 0 {
			fmt.Fprintf(&b, "### Recommended Actions\n")
			for _, a := range result.Stage5.RecommendedActions {
				fmt.Fprintf(&b, "- %s\n", sanitize(a))
			}
			fmt.Fprintf(&b, "\n")
		}
		if len(result.Stage5.NonPatentActions) > 0 {
			fmt.Fprintf(&b, "### Non-Patent Actions\n")
			for _, a := range result.Stage5.NonPatentActions {
				fmt.Fprintf(&b, "- %s\n", sanitize(a))
			}
			fmt.Fprintf(&b, "\n")
		}
		if len(result.Stage5.ModelLimitations) > 0 {
			fmt.Fprintf(&b, "### Model Limitations\n")
			for _, l := range result.Stage5.ModelLimitations {
				fmt.Fprintf(&b, "- %s\n", sanitize(l))
			}
			fmt.Fprintf(&b, "\n")
		}
	} else {
		writeSkipExplanation(&b, "stage_5", result)
	}

	// --- Recommended Next Steps ---
	fmt.Fprintf(&b, "---\n\n## Recommended Next Steps\n\n")
	switch result.Decision.Tier {
	case RecommendationGO:
		fmt.Fprintf(&b, "The analysis suggests this invention has sufficient commercial potential to justify patent investment. "+
			"Recommended actions:\n\n")
		fmt.Fprintf(&b, "1. **Schedule an inventor meeting** to validate the extracted facts and fill any information gaps\n")
		fmt.Fprintf(&b, "2. **Run a patent eligibility screen** to check for Section 101 issues before investing in filing\n")
		fmt.Fprintf(&b, "3. **Identify potential licensees** based on the commercialization path and partner density analysis\n")
		fmt.Fprintf(&b, "4. **Consider a provisional patent filing** to secure priority date while conducting further due diligence\n\n")
	case RecommendationDefer:
		fmt.Fprintf(&b, "The analysis could not reach a confident GO or NO_GO — key information is missing or uncertain. "+
			"Recommended actions:\n\n")
		fmt.Fprintf(&b, "1. **Schedule an inventor meeting** to address the specific uncertainties flagged in the report "+
			"(see Diligence Questions above and Unknown Key Factors in Stage 2)\n")
		fmt.Fprintf(&b, "2. **Re-run the analysis** after the inventor meeting with updated information\n")
		fmt.Fprintf(&b, "3. **Consider a provisional filing** if the priority date window is closing — "+
			"a provisional buys 12 months and is relatively low-cost\n\n")
	case RecommendationNoGo:
		fmt.Fprintf(&b, "The analysis does not support patent investment at this time based on available information. "+
			"This does **not** mean the invention lacks value. Recommended actions:\n\n")
		fmt.Fprintf(&b, "1. **Explore non-patent commercialization** — trade secret protection, open-source licensing, "+
			"or direct partnership may be more appropriate\n")
		fmt.Fprintf(&b, "2. **Reframe the product definition** — the same technology may have a stronger commercial case "+
			"in a different application domain or market segment\n")
		fmt.Fprintf(&b, "3. **Revisit if circumstances change** — new competitive data, a potential licensee showing interest, "+
			"or additional experimental results could shift the analysis\n\n")
	}

	// --- Assumptions Audit Trail ---
	fmt.Fprintf(&b, "## Assumptions Audit Trail\n\n")
	writeAuditTrail(&b, result)

	// --- Appendix ---
	fmt.Fprintf(&b, "## Appendix\n\n")

	// Glossary
	fmt.Fprintf(&b, "### Glossary\n\n")
	fmt.Fprintf(&b, "| Term | Definition |\n|------|------------|\n")
	fmt.Fprintf(&b, "| [TAM](%s) | Total Addressable Market — the total revenue opportunity if every potential customer adopted the technology |\n", tamSamSomURL)
	fmt.Fprintf(&b, "| [SAM](%s) | Serviceable Available Market — the portion of TAM reachable through the chosen path and geography |\n", samURL)
	fmt.Fprintf(&b, "| [SOM](%s) | Serviceable Obtainable Market — the realistic share achievable given competition and adoption rates |\n", somURL)
	fmt.Fprintf(&b, "| [rNPV](%s) | Risk-adjusted Net Present Value — NPV with explicit probability adjustments for licensing and commercial risk |\n", rnpvMethodURL)
	fmt.Fprintf(&b, "| [NPV](%s) | Net Present Value — the current worth of a stream of future cash flows, discounted for time |\n", npvURL)
	fmt.Fprintf(&b, "| [Sensitivity analysis](%s) | Testing how much each assumption affects the final result, to identify which inputs matter most |\n", sensitivityURL)
	fmt.Fprintf(&b, "| Domain priors | Sector-specific default values drawn from [AUTM survey data](%s) and industry benchmarks |\n", autmSurveyURL)
	fmt.Fprintf(&b, "| [Royalty rate](%s) | The percentage of a licensee's revenue paid to the patent holder, typically 1-10%% for university licenses |\n", royaltyRateURL)
	fmt.Fprintf(&b, "| P(license) | Probability that a licensee is found within 3 years — based on sector averages and disclosure quality |\n")
	fmt.Fprintf(&b, "| P(commercial success) | Probability that a licensed product reaches the market and generates revenue |\n")
	fmt.Fprintf(&b, "| [Discount rate](%s) | The rate used to convert future dollars into today's dollars, reflecting the time value of money |\n", discountRateURL)
	fmt.Fprintf(&b, "| Composite score | Simple average of all six scorecard dimension scores (Stage 2) |\n")
	fmt.Fprintf(&b, "| Weighted score | Weighted average of scorecard scores using dimension-specific weights (Stage 2) |\n")
	fmt.Fprintf(&b, "| Market Pain | Scorecard dimension: how urgently the target market needs this solution (weight 2.0) |\n")
	fmt.Fprintf(&b, "| Differentiation | Scorecard dimension: how clearly the invention stands apart from existing approaches (weight 2.0) |\n")
	fmt.Fprintf(&b, "| Adoption Friction | Scorecard dimension: how easy it is for customers to adopt (weight 1.5; higher = less friction) |\n")
	fmt.Fprintf(&b, "| Development Burden | Scorecard dimension: how much additional R&D is needed (weight 1.0; higher = less work) |\n")
	fmt.Fprintf(&b, "| Partner Density | Scorecard dimension: availability of potential licensees/partners (weight 1.5) |\n")
	fmt.Fprintf(&b, "| IP Leverage | Scorecard dimension: how well patent protection creates a competitive moat (weight 1.0) |\n")
	fmt.Fprintf(&b, "| `DOMAIN_DEFAULT` | Source tag: standard value for this sector from AUTM/industry data |\n")
	fmt.Fprintf(&b, "| `ADJUSTED` | Source tag: default was modified based on disclosure-specific evidence |\n")
	fmt.Fprintf(&b, "| `DISCLOSURE_DERIVED` | Source tag: value was extracted directly from the disclosure text |\n")
	fmt.Fprintf(&b, "| `ESTIMATED` | Source tag: agent's best estimate where no default or direct evidence exists |\n")
	fmt.Fprintf(&b, "| `INFERRED` | Source tag: derived indirectly from other extracted information |\n\n")

	fmt.Fprintf(&b, "### Stage Outputs (JSON)\n\n```json\n%s\n```\n", prettyJSON(stageOutputs))
	fmt.Fprintf(&b, "\n### Pipeline Metadata (JSON)\n\n```json\n%s\n```\n", prettyJSON(result.Metadata))
	return b.String()
}

// writeSkipExplanation writes a human-readable explanation of why a stage was skipped,
// based on the early-exit conditions in pipeline.go.
func writeSkipExplanation(b *strings.Builder, stage string, result PipelineResult) {
	switch {
	// Degraded mode: a prior stage failed (not a normal early exit)
	case result.Metadata.Mode == ReportModeDegraded && result.Metadata.StageFailed != "" &&
		stageOrd(stage) > stageOrd(result.Metadata.StageFailed):
		fmt.Fprintf(b, "**Not evaluated** — Stage `%s` failed or could not complete, "+
			"so this stage was never reached. This is not a normal early exit; "+
			"see the DEGRADED notice at the top of the report. Human review of the "+
			"failed stage is required before this analysis can proceed.\n\n",
			result.Metadata.StageFailed)

	// Stage 1 found no plausible monetization → skips stages 2-5
	case stage != "stage_1" && result.Stage1 != nil && !result.Stage1.HasPlausibleMonetization:
		fmt.Fprintf(b, "**Skipped** — Stage 1 found no plausible commercialization path for this invention. "+
			"When there is no viable way to monetize the technology through licensing, startup, or other routes, "+
			"further scoring, sizing, and valuation would not be meaningful. This does not necessarily reflect "+
			"on the invention's scientific merit — some inventions have value that cannot be captured through "+
			"patent licensing (see Recommended Next Steps below).\n\n")

	// Stage 2 weighted score < 2.0 → skips stages 3-5
	case stage != "stage_1" && stage != "stage_2" && result.Stage2 != nil && result.Stage2.WeightedScore < 2.0:
		fmt.Fprintf(b, "**Skipped** — Stage 2's weighted triage score (%.2f) fell below the 2.0 threshold. "+
			"This means the invention scored low on the commercial dimensions most predictive of licensing success "+
			"(market pain, differentiation, partner density). Proceeding to market sizing and valuation with such "+
			"low scores would produce unreliable results. The specific dimension scores and reasoning are shown "+
			"in Stage 2 above.\n\n", result.Stage2.WeightedScore)

	// Stage 2 weighted 2.0-2.5 with low confidence → skips stages 3-5
	case stage != "stage_1" && stage != "stage_2" && result.Stage2 != nil &&
		result.Stage2.WeightedScore >= 2.0 && result.Stage2.WeightedScore < 2.5 &&
		result.Stage2.Confidence == ConfidenceLow:
		fmt.Fprintf(b, "**Skipped** — Stage 2's weighted score (%.2f) was in the borderline range (2.0-2.5) "+
			"and the agent's confidence was LOW, meaning the disclosure did not provide enough information "+
			"to score reliably. Rather than proceeding with uncertain data, the pipeline defers to allow an "+
			"inventor meeting to clarify the unknowns. This is a conservative safeguard, not a rejection — "+
			"better information may well push the score above the threshold.\n\n", result.Stage2.WeightedScore)

	// Stage 2 low confidence + ≥2 unknown factors → skips stages 3-5
	case stage != "stage_1" && stage != "stage_2" && result.Stage2 != nil &&
		result.Stage2.Confidence == ConfidenceLow && len(result.Stage2.UnknownKeyFactors) >= 2:
		fmt.Fprintf(b, "**Skipped** — Stage 2 flagged %d unknown key commercial factors with LOW confidence. "+
			"When multiple critical inputs are unknown, the downstream analysis (market sizing, valuation) "+
			"would be built on too many guesses to be useful. An inventor meeting to address these unknowns "+
			"is recommended before re-running the analysis.\n\n", len(result.Stage2.UnknownKeyFactors))

	// Stage 3 SOM not estimable → skips stages 4-5
	case (stage == "stage_4" || stage == "stage_5") && result.Stage3 != nil && !result.Stage3.SOM.Estimable:
		fmt.Fprintf(b, "**Skipped** — Stage 3 could not estimate the Serviceable Obtainable Market (SOM). "+
			"Without a credible market size estimate, the economic model in this stage would have no revenue "+
			"figure to work with. The reason SOM was not estimable is shown in Stage 3 above. "+
			"An inventor meeting to clarify target customers, pricing, and competitive positioning "+
			"would enable this analysis to proceed.\n\n")

	default:
		fmt.Fprintf(b, "**Skipped** — a prior stage resolved the analysis before reaching this point. "+
			"See the stages above for details.\n\n")
	}
}

func prettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

func sanitize(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
}

// fmtUSD formats an integer dollar amount with comma separators (e.g. 500000000 → "500,000,000").
func fmtUSD(n int64) string {
	if n < 0 {
		return "-" + fmtUSD(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	rem := len(s) % 3
	if rem > 0 {
		b.WriteString(s[:rem])
	}
	for i := rem; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// fmtUSDf formats a float dollar amount with comma separators and no decimal places.
func fmtUSDf(n float64) string {
	return fmtUSD(int64(n))
}

// sanitizeCell prepares text for use inside a markdown table cell.
// It strips newlines (like sanitize) and escapes pipe characters that would
// break the table column structure.
func sanitizeCell(s string) string {
	s = sanitize(s)
	return strings.ReplaceAll(s, "|", "\\|")
}

// stageOrd returns a numeric ordering for stage names so we can compare
// whether one stage comes after another.
func stageOrd(stage string) int {
	switch stage {
	case "stage_0":
		return 0
	case "stage_1":
		return 1
	case "stage_2":
		return 2
	case "stage_3":
		return 3
	case "stage_4":
		return 4
	case "stage_5":
		return 5
	default:
		return -1
	}
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func ptrStr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func writeNullableRow(b *strings.Builder, field string, nf NullableField) {
	val := sanitizeCell(asString(nf.Value))
	if val == "" {
		val = "—"
	}
	note := "—"
	if nf.MissingReason != nil && strings.TrimSpace(*nf.MissingReason) != "" {
		note = sanitizeCell(*nf.MissingReason)
	}
	fmt.Fprintf(b, "| %s | %s | %s | %s |\n", field, val, nf.Confidence, note)
}

func writeRangeBlock(b *strings.Builder, name string, r MarketRange) {
	fmt.Fprintf(b, "### %s\n", name)
	if !r.Estimable {
		fmt.Fprintf(b, "- Not estimable: %s\n\n", sanitize(ptrStr(r.NotEstimableReason)))
		return
	}
	fmt.Fprintf(b, "- Range: $%s to $%s (%s)\n", fmtUSD(r.LowUSD), fmtUSD(r.HighUSD), sanitize(r.Unit))
	if len(r.Assumptions) > 0 {
		fmt.Fprintf(b, "- Assumptions:\n")
		for _, a := range r.Assumptions {
			fmt.Fprintf(b, "  - [%s] %s\n", a.Source, sanitize(a.Assumption))
		}
	}
	fmt.Fprintf(b, "\n")
}

func writeAssumptionBlock(b *strings.Builder, name string, low, high float64, src SourceType, reason string) {
	fmt.Fprintf(b, "- %s: %.3f to %.3f [%s] — %s\n", name, low, high, src, sanitize(reason))
}

func writeAssumptionBlockInt(b *strings.Builder, name string, low, high int, src SourceType, reason string) {
	fmt.Fprintf(b, "- %s: %d to %d [%s] — %s\n", name, low, high, src, sanitize(reason))
}

func scenarioLabel(name string) string {
	switch name {
	case "pessimistic":
		return "Pessimistic"
	case "base":
		return "Base"
	case "optimistic":
		return "Optimistic"
	default:
		return name
	}
}

func writeAuditTrail(b *strings.Builder, result PipelineResult) {
	seen := map[string]bool{}
	add := func(key, line string) {
		if seen[key] {
			return
		}
		seen[key] = true
		fmt.Fprintf(b, "- %s\n", line)
	}

	if result.Stage3 != nil {
		for _, r := range []MarketRange{result.Stage3.TAM, result.Stage3.SAM, result.Stage3.SOM} {
			for _, a := range r.Assumptions {
				key := string(a.Source) + "|" + a.Assumption
				add(key, fmt.Sprintf("[%s] %s", a.Source, sanitize(a.Assumption)))
			}
		}
	}
	if result.Stage4 != nil {
		add("stage4_royalty", fmt.Sprintf("[%s] Royalty rate: %s", result.Stage4.RoyaltyRatePct.Source, sanitize(result.Stage4.RoyaltyRatePct.Reasoning)))
		add("stage4_plicense", fmt.Sprintf("[%s] P(license): %s", result.Stage4.PLicense3yr.Source, sanitize(result.Stage4.PLicense3yr.Reasoning)))
		add("stage4_psuccess", fmt.Sprintf("[%s] P(success): %s", result.Stage4.PCommercialSuccess.Source, sanitize(result.Stage4.PCommercialSuccess.Reasoning)))
		add("stage4_ttl", fmt.Sprintf("[%s] Time to license: %s", result.Stage4.TimeToLicenseMonths.Source, sanitize(result.Stage4.TimeToLicenseMonths.Reasoning)))
		add("stage4_ttr", fmt.Sprintf("[%s] Time from license to revenue: %s", result.Stage4.TimeFromLicenseToRevenueMonths.Source, sanitize(result.Stage4.TimeFromLicenseToRevenueMonths.Reasoning)))
		add("stage4_rev", fmt.Sprintf("[%s] Annual revenue to licensee: %s", result.Stage4.AnnualRevenueToLicenseeUSD.Source, sanitize(result.Stage4.AnnualRevenueToLicenseeUSD.Reasoning)))
		add("stage4_years", fmt.Sprintf("[%s] License duration years: %s", result.Stage4.LicenseDurationYears.Source, sanitize(result.Stage4.LicenseDurationYears.Reasoning)))
		add("stage4_cost", fmt.Sprintf("[%s] Patent cost: %s", result.Stage4.PatentCostUSD.Source, sanitize(result.Stage4.PatentCostUSD.Reasoning)))
	}
	if len(seen) == 0 {
		fmt.Fprintf(b, "- No explicit assumptions recorded.\n")
	}
	fmt.Fprintf(b, "\n")
}
