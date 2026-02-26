package marketanalysis

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
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

	fmt.Fprintf(&b, "## Executive Summary\n\n")
	if result.Stage5 != nil {
		fmt.Fprintf(&b, "%s\n\n", sanitize(result.Stage5.ExecutiveSummary))
	} else {
		fmt.Fprintf(&b, "%s\n\n", sanitize(result.Decision.Reason))
	}

	fmt.Fprintf(&b, "## Recommendation\n\n")
	fmt.Fprintf(&b, "- Tier: `%s`\n", result.Decision.Tier)
	fmt.Fprintf(&b, "- Confidence: `%s`\n", result.Decision.Confidence)
	fmt.Fprintf(&b, "- Reason: %s\n", sanitize(result.Decision.Reason))
	for _, c := range result.Decision.Caveats {
		fmt.Fprintf(&b, "- [!] %s\n", sanitize(c))
	}
	fmt.Fprintf(&b, "\n---\n\n")

	fmt.Fprintf(&b, "## Stage 0: Invention Extraction\n\n")
	fmt.Fprintf(&b, "This is what the agent understood from the disclosure. Verify this matches your reading.\n\n")
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

	fmt.Fprintf(&b, "## Stage 1: Commercialization Path Selection\n\n")
	fmt.Fprintf(&b, "**Question**: What is the most viable path to commercialize this invention?\n\n")
	if result.Stage1 == nil {
		fmt.Fprintf(&b, "Stage skipped.\n\n")
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

	fmt.Fprintf(&b, "## Stage 2: Triage Scorecard\n\n")
	fmt.Fprintf(&b, "**Question**: Does this invention clear the bar for further commercial analysis?\n\n")
	if result.Stage2 != nil {
		fmt.Fprintf(&b, "| Dimension | Score (1-5) | Reasoning |\n")
		fmt.Fprintf(&b, "|-----------|-------------|-----------|\n")
		fmt.Fprintf(&b, "| Market Pain | %d | %s |\n", result.Stage2.Scores.MarketPain.Score, sanitize(result.Stage2.Scores.MarketPain.Reasoning))
		fmt.Fprintf(&b, "| Differentiation | %d | %s |\n", result.Stage2.Scores.Differentiation.Score, sanitize(result.Stage2.Scores.Differentiation.Reasoning))
		fmt.Fprintf(&b, "| Adoption Friction | %d | %s |\n", result.Stage2.Scores.AdoptionFriction.Score, sanitize(result.Stage2.Scores.AdoptionFriction.Reasoning))
		fmt.Fprintf(&b, "| Development Burden | %d | %s |\n", result.Stage2.Scores.DevelopmentBurden.Score, sanitize(result.Stage2.Scores.DevelopmentBurden.Reasoning))
		fmt.Fprintf(&b, "| Partner Density | %d | %s |\n", result.Stage2.Scores.PartnerDensity.Score, sanitize(result.Stage2.Scores.PartnerDensity.Reasoning))
		fmt.Fprintf(&b, "| IP Leverage | %d | %s |\n", result.Stage2.Scores.IPLeverage.Score, sanitize(result.Stage2.Scores.IPLeverage.Reasoning))
		fmt.Fprintf(&b, "\n- Composite score: %.2f\n", result.Stage2.CompositeScore)
		fmt.Fprintf(&b, "- Weighted score: %.2f\n", result.Stage2.WeightedScore)
		fmt.Fprintf(&b, "- Confidence: `%s` — %s\n", result.Stage2.Confidence, sanitize(result.Stage2.ConfidenceReasoning))
		if len(result.Stage2.UnknownKeyFactors) > 0 {
			fmt.Fprintf(&b, "- Unknown key factors:\n")
			for _, k := range result.Stage2.UnknownKeyFactors {
				fmt.Fprintf(&b, "  - %s\n", sanitize(k))
			}
		}
	} else {
		fmt.Fprintf(&b, "Stage skipped.\n")
	}
	fmt.Fprintf(&b, "\n---\n\n")

	fmt.Fprintf(&b, "## Stage 3: Market Sizing\n\n")
	if result.Stage3 != nil {
		writeRangeBlock(&b, "TAM", result.Stage3.TAM)
		writeRangeBlock(&b, "SAM", result.Stage3.SAM)
		writeRangeBlock(&b, "SOM", result.Stage3.SOM)
		if result.Stage3.TAMSOMRatioWarning != nil {
			fmt.Fprintf(&b, "- [!] TAM/SOM warning: %s\n", sanitize(*result.Stage3.TAMSOMRatioWarning))
		}
	} else {
		fmt.Fprintf(&b, "Stage skipped.\n")
	}
	fmt.Fprintf(&b, "\n---\n\n")

	fmt.Fprintf(&b, "## Stage 4: Economic Viability\n\n")
	if result.Stage4Computed != nil {
		if result.Stage4 != nil {
			fmt.Fprintf(&b, "### Assumption Provenance\n\n")
			writeAssumptionBlock(&b, "Royalty rate (%)", result.Stage4.RoyaltyRatePct.Low, result.Stage4.RoyaltyRatePct.High, result.Stage4.RoyaltyRatePct.Source, result.Stage4.RoyaltyRatePct.Reasoning)
			writeAssumptionBlock(&b, "P(license in 3 years)", result.Stage4.PLicense3yr.Low, result.Stage4.PLicense3yr.High, result.Stage4.PLicense3yr.Source, result.Stage4.PLicense3yr.Reasoning)
			writeAssumptionBlock(&b, "P(commercial success)", result.Stage4.PCommercialSuccess.Low, result.Stage4.PCommercialSuccess.High, result.Stage4.PCommercialSuccess.Source, result.Stage4.PCommercialSuccess.Reasoning)
		}
		fmt.Fprintf(&b, "### Scenario Results\n\n")
		for _, name := range []string{"pessimistic", "base", "optimistic"} {
			s := result.Stage4Computed.Scenarios[name]
			fmt.Fprintf(&b, "- %s: NPV $%.0f (exceeds patent cost: %s)\n", strings.Title(name), s.NPVUSD, yesNo(s.ExceedsPatentCost))
		}
		fmt.Fprintf(&b, "- Patent cost midpoint: $%.0f\n", result.Stage4Computed.PatentCostMidUSD)
		if len(result.Stage4Computed.SensitivityDrivers) > 0 {
			fmt.Fprintf(&b, "\n### Sensitivity Ranking\n\n")
			for i, d := range result.Stage4Computed.SensitivityDrivers {
				fmt.Fprintf(&b, "%d. %s — NPV delta: $%.0f (%s)\n", i+1, sanitize(d.Assumption), d.NPVDeltaUSD, sanitize(d.Direction))
			}
		}
		if result.Stage4Computed.PathModelLimitation != nil {
			fmt.Fprintf(&b, "\n- Model limitation: %s\n", sanitize(*result.Stage4Computed.PathModelLimitation))
		}
	} else {
		fmt.Fprintf(&b, "Stage skipped.\n")
	}
	fmt.Fprintf(&b, "\n---\n\n")

	fmt.Fprintf(&b, "## Stage 5: Decision Synthesis\n\n")
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
	}

	fmt.Fprintf(&b, "## Assumptions Audit Trail\n\n")
	writeAuditTrail(&b, result)

	fmt.Fprintf(&b, "## Appendix\n\n")
	fmt.Fprintf(&b, "### Stage Outputs (JSON)\n\n```json\n%s\n```\n", prettyJSON(stageOutputs))
	fmt.Fprintf(&b, "\n### Pipeline Metadata (JSON)\n\n```json\n%s\n```\n", prettyJSON(result.Metadata))
	return b.String()
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
	val := sanitize(asString(nf.Value))
	if val == "" {
		val = "—"
	}
	note := "—"
	if nf.MissingReason != nil && strings.TrimSpace(*nf.MissingReason) != "" {
		note = sanitize(*nf.MissingReason)
	}
	fmt.Fprintf(b, "| %s | %s | %s | %s |\n", field, val, nf.Confidence, note)
}

func writeRangeBlock(b *strings.Builder, name string, r MarketRange) {
	fmt.Fprintf(b, "### %s\n", name)
	if !r.Estimable {
		fmt.Fprintf(b, "- Not estimable: %s\n\n", sanitize(ptrStr(r.NotEstimableReason)))
		return
	}
	fmt.Fprintf(b, "- Range: $%d to $%d (%s)\n", r.LowUSD, r.HighUSD, sanitize(r.Unit))
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
	}
	if len(seen) == 0 {
		fmt.Fprintf(b, "- No explicit assumptions recorded.\n")
	}
	fmt.Fprintf(b, "\n")
}
