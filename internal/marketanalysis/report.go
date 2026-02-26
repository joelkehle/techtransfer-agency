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
	fmt.Fprintf(&b, "- Invention: %s\n", asString(result.Stage0.InventionTitle.Value))
	fmt.Fprintf(&b, "- Date: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Mode: %s\n\n", result.Metadata.Mode)
	fmt.Fprintf(&b, "%s\n\n", Disclaimer)

	if result.Metadata.Mode == ReportModeDegraded {
		fmt.Fprintf(&b, "> INCOMPLETE ANALYSIS: %s could not be completed. This report contains partial results only. Do not treat this as a complete assessment.\n\n", result.Metadata.StageFailed)
	}

	fmt.Fprintf(&b, "## Executive Summary\n\n")
	if result.Stage5 != nil {
		fmt.Fprintf(&b, "%s\n\n", result.Stage5.ExecutiveSummary)
	} else {
		fmt.Fprintf(&b, "%s\n\n", result.Decision.Reason)
	}

	fmt.Fprintf(&b, "## Recommendation\n\n")
	fmt.Fprintf(&b, "- Tier: `%s`\n", result.Decision.Tier)
	fmt.Fprintf(&b, "- Confidence: `%s`\n", result.Decision.Confidence)
	fmt.Fprintf(&b, "- Reason: %s\n\n", result.Decision.Reason)

	if result.Stage1 != nil {
		fmt.Fprintf(&b, "## Commercialization Path\n\n")
		fmt.Fprintf(&b, "- Primary path: `%s`\n", result.Stage1.PrimaryPath)
		fmt.Fprintf(&b, "- Product definition: %s\n\n", result.Stage1.ProductDefinition)
	}

	if result.Stage2 != nil {
		fmt.Fprintf(&b, "## Triage Scorecard\n\n")
		fmt.Fprintf(&b, "- Composite score: %.2f\n", result.Stage2.CompositeScore)
		fmt.Fprintf(&b, "- Weighted score: %.2f\n", result.Stage2.WeightedScore)
		fmt.Fprintf(&b, "- Confidence: %s\n\n", result.Stage2.Confidence)
	}

	if result.Stage3 != nil {
		fmt.Fprintf(&b, "## Market Sizing\n\n")
		fmt.Fprintf(&b, "- TAM: $%d to $%d\n", result.Stage3.TAM.LowUSD, result.Stage3.TAM.HighUSD)
		fmt.Fprintf(&b, "- SAM: $%d to $%d\n", result.Stage3.SAM.LowUSD, result.Stage3.SAM.HighUSD)
		fmt.Fprintf(&b, "- SOM: $%d to $%d\n\n", result.Stage3.SOM.LowUSD, result.Stage3.SOM.HighUSD)
	}

	if result.Stage4Computed != nil {
		fmt.Fprintf(&b, "## Economic Viability\n\n")
		for _, name := range []string{"pessimistic", "base", "optimistic"} {
			s := result.Stage4Computed.Scenarios[name]
			fmt.Fprintf(&b, "- %s: NPV $%.0f (exceeds patent cost: %t)\n", strings.Title(name), s.NPVUSD, s.ExceedsPatentCost)
		}
		fmt.Fprintf(&b, "- Patent cost midpoint: $%.0f\n\n", result.Stage4Computed.PatentCostMidUSD)
	}

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
