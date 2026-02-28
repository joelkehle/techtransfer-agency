package priorartsearch

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func BuildReportMarkdown(result PipelineResult) string {
	if result.Stage3 == nil {
		return buildStage3DegradedReport(result)
	}
	if result.Stage4 == nil {
		return buildStage4DegradedReport(result)
	}
	return buildNormalReport(result)
}

func buildHeader(b *strings.Builder, result PipelineResult) {
	fmt.Fprintf(b, "# Prior Art Search Report\n\n")
	fmt.Fprintf(b, "- Case ID: %s\n", result.Request.CaseID)
	fmt.Fprintf(b, "- Invention: %s\n", safe(result.Stage1.InventionTitle))
	fmt.Fprintf(b, "- Date: %s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(b, "%s\n\n", Disclaimer)
}

func buildNormalReport(result PipelineResult) string {
	var b strings.Builder
	buildHeader(&b, result)
	fmt.Fprintf(&b, "## Executive Summary\n\n")
	fmt.Fprintf(&b, "**Determination:** `%s`\n\n", result.Determination)
	fmt.Fprintf(&b, "%s\n\n", safe(result.Stage4.DeterminationReasoning))

	buildSearchCoverage(&b, result)
	buildTopResults(&b, result, 15)
	buildLandscapeSection(&b, result)
	buildNovelCoverageMatrix(&b, result)
	buildFullResultsTable(&b, result)
	buildMetadata(&b, result)
	return b.String()
}

func buildStage3DegradedReport(result PipelineResult) string {
	var b strings.Builder
	buildHeader(&b, result)
	fmt.Fprintf(&b, "## Executive Summary\n\n")
	fmt.Fprintf(&b, "**INCONCLUSIVE — relevance assessment failed**\n\n")
	if result.Metadata.DegradedReason != nil {
		fmt.Fprintf(&b, "%s\n\n", *result.Metadata.DegradedReason)
	}
	buildSearchCoverage(&b, result)
	fmt.Fprintf(&b, "## Raw Results\n\n")
	for _, p := range result.Stage2.Patents {
		fmt.Fprintf(&b, "- [%s](%s) — %s (%s)\n", p.PatentID, patentURL(p.PatentID), safe(p.Title), safe(p.GrantDate))
		fmt.Fprintf(&b, "  - Assignee: %s\n", safe(firstOrUnknown(p.Assignees)))
		fmt.Fprintf(&b, "  - Abstract: %s\n", safe(clampString(p.Abstract, 200)))
	}
	b.WriteString("\n")
	buildMetadata(&b, result)
	return b.String()
}

func buildStage4DegradedReport(result PipelineResult) string {
	var b strings.Builder
	buildHeader(&b, result)
	fmt.Fprintf(&b, "## Executive Summary\n\n")
	fmt.Fprintf(&b, "**Determination:** `%s` (code-only fallback)\n\n", result.Determination)
	if result.Metadata.DegradedReason != nil {
		fmt.Fprintf(&b, "%s\n\n", *result.Metadata.DegradedReason)
	}
	buildSearchCoverage(&b, result)
	buildTopResults(&b, result, 15)
	fmt.Fprintf(&b, "## Code-Generated Landscape Statistics\n\n")
	fmt.Fprintf(&b, "### Top Assignees\n\n")
	for _, a := range result.AssigneeFrequency {
		fmt.Fprintf(&b, "- %s: %d\n", safe(a.Name), a.Count)
	}
	fmt.Fprintf(&b, "\n### CPC Histogram\n\n")
	for _, c := range result.CPCHistogram {
		fmt.Fprintf(&b, "- %s: %d\n", safe(c.Subclass), c.Count)
	}
	b.WriteString("\n")
	buildNovelCoverageMatrix(&b, result)
	buildFullResultsTable(&b, result)
	buildMetadata(&b, result)
	return b.String()
}

func buildSearchCoverage(b *strings.Builder, result PipelineResult) {
	fmt.Fprintf(b, "## Search Coverage\n\n")
	fmt.Fprintf(b, "- Strategies: %d\n", len(result.Stage1.QueryStrategies))
	fmt.Fprintf(b, "- Queries executed: %d\n", result.Stage2.QueriesExecuted)
	fmt.Fprintf(b, "- Queries failed: %d\n", result.Stage2.QueriesFailed)
	fmt.Fprintf(b, "- Queries skipped: %d\n", result.Stage2.QueriesSkipped)
	fmt.Fprintf(b, "- Total patents retrieved: %d\n", len(result.Stage2.Patents))
	fmt.Fprintf(b, "- Total assessed: %d\n\n", result.Metadata.TotalPatentsAssessed)
	fmt.Fprintf(b, "### Strategies\n\n")
	for _, s := range result.Stage1.QueryStrategies {
		fmt.Fprintf(b, "- `%s` (%s): %s\n", s.ID, s.Priority, safe(s.Description))
		for _, tf := range s.TermFamilies {
			parts := []string{fmt.Sprintf("canonical=%s", safe(tf.Canonical))}
			if len(tf.Synonyms) > 0 {
				parts = append(parts, "synonyms="+strings.Join(tf.Synonyms, ", "))
			}
			if len(tf.Acronyms) > 0 {
				parts = append(parts, "acronyms="+strings.Join(tf.Acronyms, ", "))
			}
			if len(tf.PatentVariants) > 0 {
				parts = append(parts, "patent_variants="+strings.Join(tf.PatentVariants, ", "))
			}
			fmt.Fprintf(b, "  - term_family: %s\n", strings.Join(parts, " | "))
		}
	}
	b.WriteString("\n")
	fmt.Fprintf(b, "### Total Hits By Query\n\n")
	keys := make([]string, 0, len(result.Stage2.TotalHitsByQuery))
	for k := range result.Stage2.TotalHitsByQuery {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(b, "| Query ID | Total Hits |\n|---|---:|\n")
	for _, k := range keys {
		fmt.Fprintf(b, "| %s | %d |\n", k, result.Stage2.TotalHitsByQuery[k])
	}
	b.WriteString("\n")
	if result.Metadata.AbstractsMissing > 0 || result.Metadata.AssessmentTruncated {
		fmt.Fprintf(b, "Warnings: missing_abstracts=%d, assessment_truncated=%t\n\n", result.Metadata.AbstractsMissing, result.Metadata.AssessmentTruncated)
	}
}

func buildTopResults(b *strings.Builder, result PipelineResult, max int) {
	fmt.Fprintf(b, "## Top Results\n\n")
	if result.Stage3 == nil || len(result.Stage3.Assessments) == 0 {
		fmt.Fprintf(b, "No scored results available.\n\n")
		return
	}
	byID := map[string]PatentResult{}
	for _, p := range result.Stage2.Patents {
		byID[p.PatentID] = p
	}
	count := 0
	for _, a := range result.Stage3.Assessments {
		if a.Relevance != RelevanceHigh && a.Relevance != RelevanceMedium {
			continue
		}
		p, ok := byID[a.PatentID]
		if !ok {
			continue
		}
		fmt.Fprintf(b, "### [%s](%s) — %s\n\n", p.PatentID, patentURL(p.PatentID), safe(p.Title))
		fmt.Fprintf(b, "- Grant date: %s\n", safe(p.GrantDate))
		if p.FilingDate != nil {
			fmt.Fprintf(b, "- Filing date: %s\n", safe(*p.FilingDate))
		}
		fmt.Fprintf(b, "- Assignee: %s\n", safe(firstOrUnknown(p.Assignees)))
		fmt.Fprintf(b, "- Relevance: `%s`\n", a.Relevance)
		fmt.Fprintf(b, "- Overlap: %s\n", safe(a.OverlapDescription))
		if len(a.NovelElementsCovered) > 0 {
			fmt.Fprintf(b, "- Novel elements covered: %s\n", strings.Join(a.NovelElementsCovered, ", "))
		}
		b.WriteString("\n")
		count++
		if count >= max {
			break
		}
	}
}

func buildLandscapeSection(b *strings.Builder, result PipelineResult) {
	fmt.Fprintf(b, "## Landscape Analysis\n\n")
	s := result.Stage4
	fmt.Fprintf(b, "- Density: `%s`\n", s.LandscapeDensity)
	fmt.Fprintf(b, "- Density rationale: %s\n", safe(s.LandscapeDensityReasoning))
	fmt.Fprintf(b, "- Blocking risk: `%s`\n", s.BlockingRisk.Level)
	if len(s.BlockingRisk.BlockingPatents) > 0 {
		fmt.Fprintf(b, "- Blocking patents: %s\n", strings.Join(s.BlockingRisk.BlockingPatents, ", "))
	}
	fmt.Fprintf(b, "- Design-around: `%s`\n", s.DesignAroundPotential.Level)
	fmt.Fprintf(b, "- White space: %s\n\n", strings.Join(s.WhiteSpace, "; "))
	if len(s.KeyPlayers) > 0 {
		fmt.Fprintf(b, "### Key Players\n\n")
		for _, kp := range s.KeyPlayers {
			fmt.Fprintf(b, "- %s (%d): %s\n", safe(kp.Name), kp.PatentCount, safe(kp.RelevanceNote))
		}
		b.WriteString("\n")
	}
}

func buildNovelCoverageMatrix(b *strings.Builder, result PipelineResult) {
	fmt.Fprintf(b, "## Novel Element Coverage Matrix\n\n")
	fmt.Fprintf(b, "| NE ID | Description | HIGH | MEDIUM | Total |\n|---|---|---:|---:|---:|\n")
	for _, row := range result.NovelCoverage {
		desc := safe(row.Description)
		if row.TotalCount == 0 {
			desc = "**" + desc + "**"
		}
		fmt.Fprintf(b, "| %s | %s | %d | %d | %d |\n", row.ID, desc, row.HighCount, row.MediumCount, row.TotalCount)
	}
	b.WriteString("\n")
}

func buildFullResultsTable(b *strings.Builder, result PipelineResult) {
	fmt.Fprintf(b, "## Full Results Table\n\n")
	if result.Stage3 == nil || len(result.Stage3.Assessments) == 0 {
		fmt.Fprintf(b, "No assessed patents.\n\n")
		return
	}
	byID := map[string]PatentResult{}
	for _, p := range result.Stage2.Patents {
		byID[p.PatentID] = p
	}
	fmt.Fprintf(b, "| Patent | Title | Relevance | Matched Strategies | Grant Date |\n|---|---|---|---|---|\n")
	for _, a := range result.Stage3.Assessments {
		p, ok := byID[a.PatentID]
		if !ok {
			continue
		}
		fmt.Fprintf(b, "| [%s](%s) | %s | %s | %s | %s |\n", p.PatentID, patentURL(p.PatentID), safe(p.Title), a.Relevance, strings.Join(p.MatchedQueries, ", "), safe(p.GrantDate))
	}
	b.WriteString("\n")
}

func buildMetadata(b *strings.Builder, result PipelineResult) {
	fmt.Fprintf(b, "## Metadata\n\n")
	fmt.Fprintf(b, "- Runtime (ms): %d\n", result.Metadata.DurationMS)
	fmt.Fprintf(b, "- API calls: %d\n", result.Metadata.APICallsMade)
	fmt.Fprintf(b, "- Model: %s\n", result.Metadata.Model)
	fmt.Fprintf(b, "- Stages executed: %s\n", strings.Join(result.Metadata.StagesExecuted, ", "))
	if len(result.Metadata.StagesFailed) > 0 {
		fmt.Fprintf(b, "- Stages failed: %s\n", strings.Join(result.Metadata.StagesFailed, ", "))
	}
	b.WriteString("\n")
}

func patentURL(id string) string {
	return "https://patents.google.com/patent/US" + strings.TrimSpace(id)
}

func safe(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "(none)"
	}
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
