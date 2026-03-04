package operator

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type priorArtNovelElement struct {
	ID          string
	Description string
}

type priorArtPatent struct {
	PatentID  string
	Title     string
	GrantDate string
	USPTOURL  string
	GoogleURL string
}

type priorArtAssessment struct {
	PatentID             string
	Relevance            string
	OverlapDescription   string
	NovelElementsCovered []string
}

type priorArtCoverage struct {
	High     int
	Medium   int
	Examples []string
}

// buildPriorArtReportMarkdown creates a PI-friendly report from structured prior-art outputs.
// Returns (markdown, true) when a structured rebuild is possible.
func buildPriorArtReportMarkdown(env map[string]any) (string, bool) {
	if !strings.EqualFold(stringValue(env["agent"]), "prior-art-search") {
		return "", false
	}
	structured, ok := env["structured_results"].(map[string]any)
	if !ok || structured == nil {
		return "", false
	}
	searchStrategy, ok := structured["search_strategy"].(map[string]any)
	if !ok || searchStrategy == nil {
		return "", false
	}

	title := stringValue(searchStrategy["invention_title"])
	summary := normalizeWhitespace(stringValue(searchStrategy["invention_summary"]))
	novelElements := parsePriorArtNovelElements(searchStrategy["novel_elements"])
	uclaCaseID := priorArtUCLACaseID(env)
	piName := priorArtPIName(env)
	patentsFound, _ := structured["patents_found"].(map[string]any)
	patents := parsePriorArtPatents(patentsFound["patents"])
	assessments := parsePriorArtAssessments(structured["assessments"])

	if title == "" && len(novelElements) == 0 && len(assessments) == 0 {
		return "", false
	}

	metadata, _ := env["metadata"].(map[string]any)
	determination := strings.ToUpper(stringValue(env["determination"]))
	queriesExecuted := intValue(patentsFound["queries_executed"])
	totalHits := sumIntValuesMap(patentsFound["total_hits_by_query"])
	totalRetrieved := intValue(metadata["total_patents_retrieved"])
	if totalRetrieved == 0 {
		totalRetrieved = len(patents)
	}
	totalAssessed := intValue(metadata["total_patents_assessed"])
	if totalAssessed == 0 {
		totalAssessed = len(assessments)
	}
	processingNote := priorArtProcessingNote(metadata)

	coverage := buildPriorArtCoverage(novelElements, assessments)
	closest := rankPriorArtAssessments(assessments)
	if len(closest) > 8 {
		closest = closest[:8]
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf(
		"# Prior Art Search Report — %s — %s\n\n",
		priorArtHeaderValue(uclaCaseID),
		priorArtHeaderValue(piName),
	))
	b.WriteString("**Disclaimer (read first):** This is a preliminary automated prior-art screen, not a legal opinion. It reviews granted U.S. patents available via PatentsView and can miss relevant references (including foreign patents, applications, and non-patent literature). Use this report to focus attorney-led follow-up, not to make filing decisions alone.\n\n")

	b.WriteString("## Executive Summary\n\n")
	b.WriteString(fmt.Sprintf("**Bottom line:** %s\n\n", priorArtBottomLine(determination, len(closest))))
	if processingNote != "" {
		b.WriteString(fmt.Sprintf("**Important processing note:** %s\n\n", processingNote))
	}

	b.WriteString("## Invention Snapshot\n\n")
	b.WriteString(fmt.Sprintf("- **UCLA Case ID:** %s\n", sanitizeMarkdownCell(uclaCaseID)))
	b.WriteString(fmt.Sprintf("- **PI:** %s\n", sanitizeMarkdownCell(piName)))
	if title != "" {
		b.WriteString(fmt.Sprintf("- **Title:** %s\n", sanitizeMarkdownInline(title)))
	}
	if summary != "" {
		b.WriteString(fmt.Sprintf("- **Summary:** %s\n", sanitizeMarkdownInline(summary)))
	}
	if len(novelElements) > 0 {
		b.WriteString("- **Core invention elements used for comparison:**\n")
		for _, ne := range novelElements {
			b.WriteString(fmt.Sprintf("  - `%s`: %s\n", sanitizeMarkdownInline(ne.ID), sanitizeMarkdownInline(ne.Description)))
		}
	}
	b.WriteString("\n")

	b.WriteString("## How Broad The Search Was\n\n")
	b.WriteString("- **Source searched:** USPTO PatentsView (granted U.S. patents).\n")
	b.WriteString("- **Queries executed:** " + strconv.Itoa(queriesExecuted) + "\n")
	b.WriteString("- **Total hits:** " + strconv.Itoa(totalHits) + "\n")
	b.WriteString("- **Patents pulled for screening:** " + strconv.Itoa(totalRetrieved) + "\n")
	b.WriteString("- **Patents assessed for relevance:** " + strconv.Itoa(totalAssessed) + "\n")
	b.WriteString("\n`Hits` means raw keyword matches before detailed reading. Large hit counts are normal and do not imply high relevance.\n\n")

	if len(closest) > 0 {
		b.WriteString("## Closest Patents to Review\n\n")
		b.WriteString("| Patent | Relevance | Why It Matters | Elements Covered |\n")
		b.WriteString("|---|---|---|---|\n")
		for _, a := range closest {
			p := patents[a.PatentID]
			patentLabel := "US" + sanitizeMarkdownInline(a.PatentID)
			if p.PatentID != "" {
				patentLabel = "US" + sanitizeMarkdownInline(p.PatentID)
			}
			links := []string{}
			if p.USPTOURL != "" {
				links = append(links, fmt.Sprintf("[USPTO](%s)", p.USPTOURL))
			}
			if p.GoogleURL != "" {
				links = append(links, fmt.Sprintf("[Google](%s)", p.GoogleURL))
			}
			patentCell := patentLabel
			if len(links) > 0 {
				patentCell = fmt.Sprintf("%s (%s)", patentLabel, strings.Join(links, " · "))
			}
			if p.Title != "" {
				patentCell = patentCell + " — " + sanitizeMarkdownInline(p.Title)
			}
			if p.GrantDate != "" {
				patentCell = patentCell + " (" + sanitizeMarkdownInline(p.GrantDate) + ")"
			}
			b.WriteString(fmt.Sprintf(
				"| %s | %s | %s | %s |\n",
				patentCell,
				sanitizeMarkdownCell(strings.ToUpper(strings.TrimSpace(a.Relevance))),
				sanitizeMarkdownCell(truncateText(normalizeWhitespace(a.OverlapDescription), 220)),
				sanitizeMarkdownCell(strings.Join(a.NovelElementsCovered, ", ")),
			))
		}
		b.WriteString("\n")
	}

	if len(novelElements) > 0 {
		b.WriteString("## Novel Element Coverage (Claim-Chart Style)\n\n")
		b.WriteString("This is a practical proxy claim chart based on disclosed invention elements (not formal legal claims).\n\n")
		b.WriteString("| Element | High Overlap | Medium Overlap | Example Patents | Practical Takeaway |\n")
		b.WriteString("|---|---:|---:|---|---|\n")
		for _, ne := range novelElements {
			c := coverage[ne.ID]
			high := 0
			medium := 0
			examples := "—"
			if c != nil {
				high = c.High
				medium = c.Medium
				examples = formatCoverageExamplePatents(c.Examples, patents)
			}
			b.WriteString(fmt.Sprintf(
				"| `%s` %s | %d | %d | %s | %s |\n",
				sanitizeMarkdownCell(ne.ID),
				sanitizeMarkdownCell(truncateText(ne.Description, 90)),
				high,
				medium,
				examples,
				sanitizeMarkdownCell(priorArtTakeaway(high, medium)),
			))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Recommended Next Steps\n\n")
	for _, step := range priorArtNextSteps(determination, novelElements, coverage) {
		b.WriteString("- " + sanitizeMarkdownInline(step) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(buildPriorArtTraceabilitySection(
		searchStrategy,
		patentsFound,
		structured["landscape"],
		assessments,
		novelElements,
		coverage,
		metadata,
		determination,
	))

	return b.String(), true
}

func priorArtBottomLine(determination string, closestCount int) string {
	switch strings.ToUpper(strings.TrimSpace(determination)) {
	case "CLEAR_FIELD":
		if closestCount > 0 {
			return "No clearly blocking patent was found in this screened set; review the listed closest patents with counsel before filing."
		}
		return "No clearly blocking patent was found in this screened set, but counsel review is still required before filing."
	case "POTENTIAL_CONFLICT":
		return "At least one assessed patent appears to overlap key invention elements and should be reviewed with counsel before filing."
	case "CROWDED_FIELD":
		return "The field appears crowded; expect narrower claim scope and more iteration with counsel."
	default:
		return "This is an early screening result and should be used to focus counsel-led filing strategy."
	}
}

func priorArtNextSteps(determination string, novelElements []priorArtNovelElement, coverage map[string]*priorArtCoverage) []string {
	var uncovered []string
	for _, ne := range novelElements {
		c := coverage[ne.ID]
		if c == nil || (c.High == 0 && c.Medium == 0) {
			uncovered = append(uncovered, ne.ID)
		}
	}

	steps := []string{
		"Review the top patents with patent counsel and validate whether the overlap is technical, legal, or both.",
		"Refine your invention narrative around elements that are differentiated and experimentally supported.",
	}
	if len(uncovered) > 0 {
		steps = append(steps, "Prioritize claim drafting around currently uncovered elements: "+strings.Join(uncovered, ", ")+".")
	}
	switch strings.ToUpper(strings.TrimSpace(determination)) {
	case "POTENTIAL_CONFLICT", "CROWDED_FIELD":
		steps = append(steps, "Run a deeper counsel-led search (US applications, foreign patents, and key non-patent literature) before filing.")
	default:
		steps = append(steps, "Proceed to provisional claim drafting, then validate with a deeper counsel-led prior-art search before non-provisional filing.")
	}
	return steps
}

func priorArtTakeaway(high, medium int) string {
	switch {
	case high > 0:
		return "High collision risk. Treat as likely examination pressure point."
	case medium >= 2:
		return "Moderate risk. Differentiate with technical detail and narrower claim language."
	case medium == 1:
		return "Some overlap. Keep but support with concrete implementation details."
	default:
		return "No direct overlap in assessed set. Candidate differentiator."
	}
}

func buildPriorArtTraceabilitySection(
	searchStrategy map[string]any,
	patentsFound map[string]any,
	landscapeRaw any,
	assessments []priorArtAssessment,
	novelElements []priorArtNovelElement,
	coverage map[string]*priorArtCoverage,
	metadata map[string]any,
	determination string,
) string {
	stage1Status, stage2Status, stage3Status, stage4Status := priorArtStageStatuses(metadata)
	queryStrategies := anySliceLen(searchStrategy["query_strategies"])
	queriesExecuted := intValue(patentsFound["queries_executed"])
	queriesFailed := intValue(patentsFound["queries_failed"])
	queriesSkipped := intValue(patentsFound["queries_skipped"])
	totalRetrieved := intValue(metadata["total_patents_retrieved"])
	if totalRetrieved == 0 {
		totalRetrieved = anySliceLen(patentsFound["patents"])
	}
	totalAssessed := intValue(metadata["total_patents_assessed"])
	if totalAssessed == 0 {
		totalAssessed = len(assessments)
	}
	relevance := priorArtRelevanceCounts(assessments)
	coveredElements := priorArtCoveredElementsCount(novelElements, coverage)
	assessmentTruncated := boolValue(metadata["assessment_truncated"])
	degraded := boolValue(metadata["degraded"])
	queryHitsSummary := priorArtQueryHitsSummary(patentsFound["total_hits_by_query"], 6)

	landscape, _ := landscapeRaw.(map[string]any)
	landscapeDensity := strings.ToUpper(stringValue(landscape["landscape_density"]))
	blockingRisk, _ := landscape["blocking_risk"].(map[string]any)
	blockingLevel := strings.ToUpper(stringValue(blockingRisk["level"]))
	blockingPatents := stringSliceFromAny(blockingRisk["blocking_patents"])

	stage4Output := "Not completed in this run; fallback determination used: " + sanitizeMarkdownInline(strings.ToUpper(strings.TrimSpace(determination))) + "."
	if strings.EqualFold(stage4Status, "Completed") {
		var details []string
		if strings.TrimSpace(landscapeDensity) != "" {
			details = append(details, "landscape density "+strings.ToLower(landscapeDensity))
		}
		if strings.TrimSpace(blockingLevel) != "" {
			details = append(details, "blocking risk "+strings.ToLower(blockingLevel))
		}
		if len(blockingPatents) > 0 {
			details = append(details, fmt.Sprintf("%d cited blocking patent(s)", len(blockingPatents)))
		}
		details = append(details, "determination "+strings.ToUpper(strings.TrimSpace(determination)))
		stage4Output = strings.Join(details, "; ") + "."
	}

	var b strings.Builder
	b.WriteString("## Traceability Trail (Stage Inputs and Outputs)\n\n")
	b.WriteString("Large hit counts are shown in compact form (for example, `47.3k`) to keep this section readable.\n\n")
	b.WriteString("| Stage | Inputs Used | Outputs Produced |\n")
	b.WriteString("|---|---|---|\n")
	b.WriteString(fmt.Sprintf(
		"| Stage 1 — Search Strategy (%s) | Disclosure text for this case (plus prior context when available). | Invention summary and `%d` novel element(s); `%d` query strateg(ies). |\n",
		sanitizeMarkdownCell(stage1Status),
		len(novelElements),
		queryStrategies,
	))
	b.WriteString(fmt.Sprintf(
		"| Stage 2 — Patent Retrieval (%s) | Stage 1 term families, phrases, and CPC subclasses. | Queries run: %d (failed %d, skipped %d); patents retrieved: %s; per-query hits: %s |\n",
		sanitizeMarkdownCell(stage2Status),
		queriesExecuted,
		queriesFailed,
		queriesSkipped,
		formatCompactCount(totalRetrieved),
		sanitizeMarkdownCell(queryHitsSummary),
	))
	stage3Output := fmt.Sprintf(
		"Assessed patents returned: %s (HIGH %d, MEDIUM %d, LOW %d); elements with overlap: %d/%d.",
		formatCompactCount(totalAssessed),
		relevance.High,
		relevance.Medium,
		relevance.Low,
		coveredElements,
		len(novelElements),
	)
	if assessmentTruncated {
		stage3Output += " Assessment scope was capped in this run."
	}
	b.WriteString(fmt.Sprintf(
		"| Stage 3 — Relevance Assessment (%s) | Retrieved patents with abstracts, compared against disclosed invention elements. | %s |\n",
		sanitizeMarkdownCell(stage3Status),
		sanitizeMarkdownCell(stage3Output),
	))
	b.WriteString(fmt.Sprintf(
		"| Stage 4 — Landscape Synthesis (%s) | Stage 3 assessments plus computed assignee/CPC statistics. | %s |\n",
		sanitizeMarkdownCell(stage4Status),
		sanitizeMarkdownCell(stage4Output),
	))
	b.WriteString(fmt.Sprintf(
		"| Report Builder (Completed) | Structured stage outputs and run metadata. | Human-readable summary, closest patents, claim-chart coverage, recommendations, and this traceability trail. |\n",
	))
	b.WriteString("\n")
	b.WriteString("- **Run flags:** fallback mode = " + yesNo(degraded) + "; assessment subset capped = " + yesNo(assessmentTruncated) + ".\n")
	stagesExecuted := stringSliceFromAny(metadata["stages_executed"])
	stagesFailed := stringSliceFromAny(metadata["stages_failed"])
	if len(stagesExecuted) > 0 {
		b.WriteString("- **Stages executed:** `" + sanitizeMarkdownInline(strings.Join(stagesExecuted, ", ")) + "`.\n")
	}
	if len(stagesFailed) > 0 {
		b.WriteString("- **Stages failed:** `" + sanitizeMarkdownInline(strings.Join(stagesFailed, ", ")) + "`.\n")
	}
	b.WriteString("\n")
	return b.String()
}

type priorArtRelevanceCount struct {
	High int
	Low  int
	// Medium is tracked separately because it is often the main triage signal.
	Medium int
}

func priorArtRelevanceCounts(assessments []priorArtAssessment) priorArtRelevanceCount {
	var out priorArtRelevanceCount
	for _, a := range assessments {
		switch strings.ToUpper(strings.TrimSpace(a.Relevance)) {
		case "HIGH":
			out.High++
		case "MEDIUM":
			out.Medium++
		case "LOW":
			out.Low++
		}
	}
	return out
}

func priorArtCoveredElementsCount(novelElements []priorArtNovelElement, coverage map[string]*priorArtCoverage) int {
	if len(novelElements) == 0 {
		return 0
	}
	covered := 0
	for _, ne := range novelElements {
		c := coverage[ne.ID]
		if c == nil {
			continue
		}
		if c.High > 0 || c.Medium > 0 {
			covered++
		}
	}
	return covered
}

func priorArtQueryHitsSummary(v any, maxItems int) string {
	m, _ := v.(map[string]any)
	if len(m) == 0 {
		return "not reported."
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if maxItems <= 0 {
		maxItems = len(keys)
	}
	shown := keys
	if len(shown) > maxItems {
		shown = shown[:maxItems]
	}
	parts := make([]string, 0, len(shown))
	for _, k := range shown {
		parts = append(parts, fmt.Sprintf("%s %s", sanitizeMarkdownInline(k), formatCompactCount(intValue(m[k]))))
	}
	summary := strings.Join(parts, "; ")
	if len(keys) > len(shown) {
		summary += fmt.Sprintf("; +%d more query bucket(s)", len(keys)-len(shown))
	}
	if strings.TrimSpace(summary) == "" {
		return "not reported."
	}
	return summary + "."
}

func priorArtStageStatuses(metadata map[string]any) (stage1, stage2, stage3, stage4 string) {
	executed := stringSliceFromAny(metadata["stages_executed"])
	failed := stringSliceFromAny(metadata["stages_failed"])
	return priorArtStageStatus("stage_1", executed, failed),
		priorArtStageStatus("stage_2", executed, failed),
		priorArtStageStatus("stage_3", executed, failed),
		priorArtStageStatus("stage_4", executed, failed)
}

func priorArtStageStatus(stage string, executed, failed []string) string {
	stage = strings.ToLower(strings.TrimSpace(stage))
	for _, s := range failed {
		if strings.ToLower(strings.TrimSpace(s)) == stage {
			return "Fallback Used"
		}
	}
	for _, s := range executed {
		if strings.ToLower(strings.TrimSpace(s)) == stage {
			return "Completed"
		}
	}
	if len(executed) == 0 && len(failed) == 0 {
		return "Not Reported"
	}
	return "Not Run"
}

func stringSliceFromAny(v any) []string {
	switch t := v.(type) {
	case []string:
		out := make([]string, 0, len(t))
		for _, item := range t {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			out = append(out, item)
		}
		return out
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s := strings.TrimSpace(fmt.Sprintf("%v", item))
			if s == "" {
				continue
			}
			out = append(out, s)
		}
		return out
	default:
		return nil
	}
}

func anySliceLen(v any) int {
	switch t := v.(type) {
	case []any:
		return len(t)
	case []string:
		return len(t)
	default:
		return 0
	}
}

func formatCompactCount(n int) string {
	if n < 0 {
		return "-" + formatCompactCount(-n)
	}
	if n < 1000 {
		return strconv.Itoa(n)
	}
	scaled := float64(n)
	suffixes := []string{"k", "M", "B", "T"}
	suffixIndex := -1
	for scaled >= 1000 && suffixIndex < len(suffixes)-1 {
		scaled /= 1000
		suffixIndex++
	}
	if suffixIndex < 0 {
		return strconv.Itoa(n)
	}
	out := fmt.Sprintf("%.1f%s", scaled, suffixes[suffixIndex])
	out = strings.Replace(out, ".0"+suffixes[suffixIndex], suffixes[suffixIndex], 1)
	return out
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func parsePriorArtNovelElements(v any) []priorArtNovelElement {
	rows, _ := v.([]any)
	var out []priorArtNovelElement
	for _, row := range rows {
		m, _ := row.(map[string]any)
		id := stringValue(m["id"])
		desc := normalizeWhitespace(stringValue(m["description"]))
		if id == "" && desc == "" {
			continue
		}
		out = append(out, priorArtNovelElement{ID: id, Description: desc})
	}
	return out
}

func parsePriorArtPatents(v any) map[string]priorArtPatent {
	rows, _ := v.([]any)
	out := map[string]priorArtPatent{}
	for _, row := range rows {
		m, _ := row.(map[string]any)
		id := stringValue(m["patent_id"])
		if id == "" {
			continue
		}
		out[id] = priorArtPatent{
			PatentID:  id,
			Title:     normalizeWhitespace(stringValue(m["patent_title"])),
			GrantDate: normalizeWhitespace(stringValue(m["patent_date"])),
			USPTOURL:  patentUSPTOURL(id),
			GoogleURL: patentGoogleLink(id),
		}
	}
	return out
}

func parsePriorArtAssessments(v any) []priorArtAssessment {
	rows, _ := v.([]any)
	var out []priorArtAssessment
	for _, row := range rows {
		m, _ := row.(map[string]any)
		id := stringValue(m["patent_id"])
		if id == "" {
			continue
		}
		out = append(out, priorArtAssessment{
			PatentID:             id,
			Relevance:            stringValue(m["relevance"]),
			OverlapDescription:   stringValue(m["overlap_description"]),
			NovelElementsCovered: stringSliceValue(m["novel_elements_covered"]),
		})
	}
	return out
}

func formatCoverageExamplePatents(examplePatentIDs []string, patents map[string]priorArtPatent) string {
	if len(examplePatentIDs) == 0 {
		return "—"
	}
	seen := map[string]bool{}
	var formatted []string
	for _, rawID := range examplePatentIDs {
		id := strings.TrimSpace(rawID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		p := patents[id]
		if p.PatentID == "" {
			p = priorArtPatent{
				PatentID:  id,
				USPTOURL:  patentUSPTOURL(id),
				GoogleURL: patentGoogleLink(id),
			}
		}
		patentLabel := "US" + sanitizeMarkdownInline(p.PatentID)
		links := []string{}
		if p.USPTOURL != "" {
			links = append(links, fmt.Sprintf("[USPTO](%s)", p.USPTOURL))
		}
		if p.GoogleURL != "" {
			links = append(links, fmt.Sprintf("[Google](%s)", p.GoogleURL))
		}
		entry := patentLabel
		if len(links) > 0 {
			entry = fmt.Sprintf("%s (%s)", patentLabel, strings.Join(links, " · "))
		}
		formatted = append(formatted, entry)
	}
	if len(formatted) == 0 {
		return "—"
	}
	return strings.Join(formatted, "; ")
}

func buildPriorArtCoverage(novelElements []priorArtNovelElement, assessments []priorArtAssessment) map[string]*priorArtCoverage {
	out := map[string]*priorArtCoverage{}
	for _, ne := range novelElements {
		out[ne.ID] = &priorArtCoverage{}
	}
	for _, a := range assessments {
		rank := priorArtRelevanceRank(a.Relevance)
		for _, id := range a.NovelElementsCovered {
			c := out[id]
			if c == nil {
				continue
			}
			switch rank {
			case 0:
				c.High++
			case 1:
				c.Medium++
			}
			if len(c.Examples) < 3 {
				c.Examples = append(c.Examples, a.PatentID)
			}
		}
	}
	return out
}

func rankPriorArtAssessments(in []priorArtAssessment) []priorArtAssessment {
	out := append([]priorArtAssessment(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		ri := priorArtRelevanceRank(out[i].Relevance)
		rj := priorArtRelevanceRank(out[j].Relevance)
		if ri != rj {
			return ri < rj
		}
		return out[i].PatentID < out[j].PatentID
	})
	return out
}

func priorArtRelevanceRank(v string) int {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "HIGH":
		return 0
	case "MEDIUM":
		return 1
	case "LOW":
		return 2
	default:
		return 3
	}
}

func priorArtUCLACaseID(env map[string]any) string {
	candidates := []string{
		stringValue(env["ucla_case_id"]),
		stringValue(env["technology_id"]),
		stringValue(env["tech_id"]),
		lookupString(env, "metadata", "ucla_case_id"),
		lookupString(env, "metadata", "technology_id"),
		lookupString(env, "metadata", "tech_id"),
	}
	for _, c := range candidates {
		if looksLikeUCLACaseID(c) {
			return c
		}
	}
	caseID := stringValue(env["case_id"])
	if looksLikeUCLACaseID(caseID) {
		return caseID
	}
	return ""
}

func priorArtPIName(env map[string]any) string {
	candidates := []string{
		stringValue(env["pi_name"]),
		stringValue(env["principal_investigator"]),
		stringValue(env["pi"]),
		lookupString(env, "metadata", "pi_name"),
		lookupString(env, "metadata", "principal_investigator"),
		lookupString(env, "metadata", "pi"),
		lookupString(env, "structured_results", "search_strategy", "pi_name"),
		lookupString(env, "structured_results", "search_strategy", "principal_investigator"),
	}
	for _, c := range candidates {
		if strings.TrimSpace(c) != "" {
			return strings.TrimSpace(c)
		}
	}
	return ""
}

func priorArtHeaderValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "Not provided"
	}
	return sanitizeMarkdownInline(v)
}

func priorArtProcessingNote(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	var notes []string
	if boolValue(metadata["degraded"]) {
		notes = append(notes, "The automated deep-comparison step did not complete for all candidate patents, so overlap ratings should be treated as directional.")
	}
	if boolValue(metadata["assessment_truncated"]) {
		notes = append(notes, "Only a subset of retrieved patents received full relevance assessment in this run.")
	}
	reason := strings.ToLower(strings.TrimSpace(stringValue(metadata["degraded_reason"])))
	if strings.Contains(reason, "stage 4") || strings.Contains(reason, "degraded") {
		// Replace pipeline jargon with an explicit user-facing explanation.
		if len(notes) == 0 {
			notes = append(notes, "The automated deep-comparison step did not complete for all candidate patents, so overlap ratings should be treated as directional.")
		}
	} else if strings.TrimSpace(reason) != "" {
		notes = append(notes, normalizeWhitespace(stringValue(metadata["degraded_reason"])))
	}
	return strings.Join(dedupeStrings(notes), " ")
}

func looksLikeUCLACaseID(v string) bool {
	id := strings.TrimSpace(v)
	if len(id) != 8 || id[4] != '-' {
		return false
	}
	for i := 0; i < len(id); i++ {
		if i == 4 {
			continue
		}
		if id[i] < '0' || id[i] > '9' {
			return false
		}
	}
	return true
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func sumIntValuesMap(v any) int {
	m, _ := v.(map[string]any)
	total := 0
	for _, raw := range m {
		total += intValue(raw)
	}
	return total
}

func intValue(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int8:
		return int(t)
	case int16:
		return int(t)
	case int32:
		return int(t)
	case int64:
		return int(t)
	case uint:
		return int(t)
	case uint8:
		return int(t)
	case uint16:
		return int(t)
	case uint32:
		return int(t)
	case uint64:
		return int(t)
	case float32:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

func boolValue(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		b, _ := strconv.ParseBool(strings.TrimSpace(t))
		return b
	default:
		return false
	}
}

func stringSliceValue(v any) []string {
	rows, _ := v.([]any)
	var out []string
	for _, row := range rows {
		s := strings.TrimSpace(fmt.Sprintf("%v", row))
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func patentGoogleLink(patentID string) string {
	id := strings.TrimSpace(patentID)
	if id == "" {
		return ""
	}
	return "https://patents.google.com/patent/US" + id
}

func patentUSPTOURL(patentID string) string {
	id := strings.TrimSpace(patentID)
	if id == "" {
		return ""
	}
	// Direct USPTO PDF link for granted patents.
	return "https://pdfpiw.uspto.gov/.piw?Docid=" + id
}

func normalizeWhitespace(v string) string {
	parts := strings.Fields(strings.TrimSpace(v))
	return strings.Join(parts, " ")
}

func truncateText(v string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(v))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max-1]) + "…"
}

func sanitizeMarkdownInline(v string) string {
	s := strings.TrimSpace(v)
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "|", "\\|")
}

func sanitizeMarkdownCell(v string) string {
	s := sanitizeMarkdownInline(v)
	if s == "" {
		return "—"
	}
	return s
}
