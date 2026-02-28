package priorartsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
)

type StageRunner interface {
	RunStage1(ctx context.Context, req RequestEnvelope) (Stage1Output, StageAttemptMetrics, error)
	RunStage3(ctx context.Context, s1 Stage1Output, s2 Stage2Output) (Stage3Output, Stage3RunMetadata, StageAttemptMetrics, error)
	RunStage4(ctx context.Context, s1 Stage1Output, s2 Stage2Output, s3 Stage3Output) (Stage4Output, StageAttemptMetrics, error)
}

type Stage3RunMetadata struct {
	AbstractsMissing    int
	AssessmentTruncated bool
	AssessedNone        int
}

type LLMStageRunner struct {
	exec      *StageExecutor
	maxAssess int
	batchSize int
}

func NewLLMStageRunner(exec *StageExecutor, maxAssess, batchSize int) *LLMStageRunner {
	if maxAssess <= 0 {
		maxAssess = DefaultMaxAssess
	}
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	return &LLMStageRunner{exec: exec, maxAssess: maxAssess, batchSize: batchSize}
}

func (r *LLMStageRunner) RunStage1(ctx context.Context, req RequestEnvelope) (Stage1Output, StageAttemptMetrics, error) {
	out := Stage1Output{}
	prompt := buildStage1Prompt(req)
	m, err := r.exec.Run(ctx, "stage_1", prompt, &out, func() error {
		return validateAndNormalizeStage1(&out)
	})
	return out, m, err
}

func buildStage1Prompt(req RequestEnvelope) string {
	var b strings.Builder
	b.WriteString("Return valid JSON only. No markdown fences, no commentary.\n\n")
	b.WriteString(`You are a patent search strategist. Given an invention disclosure, produce
a search plan for the USPTO PatentsView API and extract key invention
metadata.

IMPORTANT: Return valid JSON only. No markdown fences, no commentary, no
preamble. Your entire response must be a single JSON object matching the
schema below.

PART 1 — INVENTION EXTRACTION

Extract from the disclosure:
- A concise title for the invention (10-200 chars)
- A one-paragraph summary (50-500 chars)
- The 3-10 novel elements that distinguish this invention from prior work.
  Assign each a stable ID (NE1, NE2, ... NE10).
- The broad technology domains (1-5)

PART 2 — SEARCH PLAN

Cover the invention from multiple angles:
1. DIRECT MATCH: Terms describing the core mechanism or method.
2. COMPONENT MATCH: Terms for individual components/subsystems.
3. PROBLEM-SOLUTION MATCH: Terms for the problem and general solution
   category.

For each query strategy, provide TERM FAMILIES. Patent literature uses
different vocabulary than disclosures. For each core concept, provide:
- The canonical term (as used in the disclosure)
- Synonyms and near-synonyms (how patents describe the same concept)
- Abbreviations and acronyms
- Common patent-ese variants

IMPORTANT RULES FOR TERM FAMILIES:
- Multi-word terms (2+ words) should be listed as PHRASES, not as
  entries in the synonyms list. The search system tokenizes on whitespace,
  so "secure aggregation" in the synonyms list becomes two separate
  words "secure" and "aggregation." Put multi-word terms in the phrases
  list instead.
- Single-word synonyms go in the synonyms list.
- Keep synonym lists to genuinely relevant technical terms. Don't pad.

For CPC codes: provide SUBCLASS-level codes only (4 characters, e.g.,
"G06N", "A61K", "H04L"). Do NOT provide group-level codes (which
contain slashes like "G06N3/08").

ORDERING: List items most-important-first within all arrays:
- term_families: most central concept first
- phrases: most specific/discriminating phrase first
- cpc_subclasses: most relevant subclass first

Generate between 3 and 5 query strategies.`)

	if req.PriorContext != nil && req.PriorContext.Stage6Output != nil {
		s6 := req.PriorContext.Stage6Output
		b.WriteString("\n\nThe Patent Eligibility Screen agent identified these concerns:\n\n")
		b.WriteString("Novelty concerns: [" + strings.Join(s6.NoveltyConcerns, "; ") + "]\n")
		b.WriteString("Non-obviousness concerns: [" + strings.Join(s6.NonObviousnessConcerns, "; ") + "]\n")
		b.WriteString("Search priority: [" + s6.PriorArtSearchPriority + "]\n\n")
		b.WriteString("Use these to sharpen your search. Novelty concerns indicate where prior\n")
		b.WriteString("art is most likely.\n")
	}
	if req.PriorContext != nil && req.PriorContext.Stage1Output != nil {
		s1 := req.PriorContext.Stage1Output
		b.WriteString("\nThe Patent Eligibility Screen agent extracted:\n\n")
		b.WriteString("Title: [" + s1.InventionTitle + "]\n")
		b.WriteString("Technology area: [" + s1.TechnologyArea + "]\n")
		b.WriteString("Novel elements: [" + strings.Join(s1.NovelElements, "; ") + "]\n")
		b.WriteString("Description: [" + s1.InventionDescription + "]\n\n")
		b.WriteString("Use this to inform your term families and CPC classification. You may\n")
		b.WriteString("adopt the title and novel elements directly or refine them.\n")
	}
	b.WriteString("\nRequired output schema:\n")
	b.WriteString(`{
  "invention_title": "string (10-200 chars)",
  "invention_summary": "string (50-500 chars)",
  "novel_elements": [
    {
      "id": "string (NE1, NE2, ... NE10)",
      "description": "string (20-300 chars)"
    }
  ],
  "technology_domains": ["string (1-5 entries)"],
  "query_strategies": [
    {
      "id": "string (Q1, Q2, ...)",
      "description": "string (20-200 chars)",
      "term_families": [
        {
          "canonical": "string (single word or short compound noun)",
          "synonyms": ["string (single-word synonyms only, 0-8 entries)"],
          "acronyms": ["string (0-4 entries)"],
          "patent_variants": ["string (0-4 entries)"]
        }
      ],
      "phrases": ["string (multi-word exact phrases, 0-5 entries)"],
      "cpc_subclasses": ["string (0-5 entries, e.g. 'G06N')"],
      "priority": "PRIMARY | SECONDARY | TERTIARY"
    }
  ],
  "confidence_score": "float (0.0-1.0)",
  "confidence_reason": "string (min 10 chars)"
}`)
	b.WriteString("\n\nDISCLOSURE:\n" + req.DisclosureText)
	return b.String()
}

func validateAndNormalizeStage1(s *Stage1Output) error {
	s.InventionTitle = clampString(s.InventionTitle, 200)
	s.InventionSummary = clampString(s.InventionSummary, 500)
	s.ConfidenceReason = strings.TrimSpace(s.ConfidenceReason)
	if len(strings.TrimSpace(s.InventionTitle)) < 10 {
		return fmt.Errorf("invention_title too short")
	}
	if len(strings.TrimSpace(s.InventionSummary)) < 50 {
		return fmt.Errorf("invention_summary too short")
	}
	if s.ConfidenceScore < 0 || s.ConfidenceScore > 1 {
		return fmt.Errorf("confidence_score out of range")
	}
	if len(s.ConfidenceReason) < 10 {
		return fmt.Errorf("confidence_reason too short")
	}
	if len(s.TechnologyDomains) < 1 || len(s.TechnologyDomains) > 5 {
		return fmt.Errorf("technology_domains count")
	}
	for i := range s.TechnologyDomains {
		s.TechnologyDomains[i] = strings.TrimSpace(s.TechnologyDomains[i])
		if s.TechnologyDomains[i] == "" {
			return fmt.Errorf("technology_domains entry empty")
		}
	}

	if len(s.NovelElements) < 3 || len(s.NovelElements) > 10 {
		return fmt.Errorf("novel_elements count")
	}
	seenNE := map[string]struct{}{}
	for i := range s.NovelElements {
		ne := &s.NovelElements[i]
		ne.ID = strings.ToUpper(strings.TrimSpace(ne.ID))
		ne.Description = clampString(ne.Description, 300)
		if !isValidNEID(ne.ID) {
			return fmt.Errorf("invalid novel element id")
		}
		if _, ok := seenNE[ne.ID]; ok {
			return fmt.Errorf("duplicate novel element id")
		}
		seenNE[ne.ID] = struct{}{}
		if len(strings.TrimSpace(ne.Description)) < 20 {
			return fmt.Errorf("novel element description too short")
		}
		expectedID := fmt.Sprintf("NE%d", i+1)
		if ne.ID != expectedID {
			return fmt.Errorf("novel element IDs must be sequential")
		}
	}

	if len(s.QueryStrategies) < 3 || len(s.QueryStrategies) > 5 {
		return fmt.Errorf("query_strategies count")
	}
	hasPrimary := false
	for i := range s.QueryStrategies {
		st := &s.QueryStrategies[i]
		st.ID = strings.TrimSpace(st.ID)
		st.Description = clampString(st.Description, 200)
		st.Priority = normalizePriority(st.Priority)
		if st.ID == "" {
			return fmt.Errorf("strategy id required")
		}
		if len(strings.TrimSpace(st.Description)) < 20 {
			return fmt.Errorf("strategy description too short")
		}
		if len(st.TermFamilies) < 1 || len(st.TermFamilies) > 8 {
			return fmt.Errorf("term_families count")
		}
		if len(st.Phrases) > 5 || len(st.CPCSubclasses) > 5 {
			return fmt.Errorf("phrases/cpc count")
		}
		if st.Priority == PriorityPrimary {
			hasPrimary = true
		}
		for j := range st.TermFamilies {
			tf := &st.TermFamilies[j]
			tf.Canonical = strings.TrimSpace(tf.Canonical)
			if tf.Canonical == "" {
				return fmt.Errorf("canonical term required")
			}
			if len(tf.Synonyms) > 8 || len(tf.Acronyms) > 4 || len(tf.PatentVariants) > 4 {
				return fmt.Errorf("term family array limits")
			}
		}
		for j := range st.CPCSubclasses {
			c := strings.ToUpper(strings.TrimSpace(st.CPCSubclasses[j]))
			if cpcSubclassRe.MatchString(c) {
				st.CPCSubclasses[j] = c
			} else {
				st.CPCSubclasses[j] = ""
				log.Printf("prior-art-search dropping invalid cpc_subclass=%q", c)
			}
		}
		st.CPCSubclasses = compactStrings(st.CPCSubclasses)
	}
	if !hasPrimary {
		return fmt.Errorf("at least one PRIMARY strategy required")
	}
	return nil
}

func (r *LLMStageRunner) RunStage3(ctx context.Context, s1 Stage1Output, s2 Stage2Output) (Stage3Output, Stage3RunMetadata, StageAttemptMetrics, error) {
	meta := Stage3RunMetadata{}
	if len(s2.Patents) == 0 {
		return Stage3Output{Assessments: []PatentAssessment{}}, meta, StageAttemptMetrics{}, nil
	}

	patents := make([]PatentResult, 0, len(s2.Patents))
	for _, p := range s2.Patents {
		if strings.TrimSpace(p.Abstract) == "" {
			meta.AbstractsMissing++
			continue
		}
		patents = append(patents, p)
	}
	sort.SliceStable(patents, func(i, j int) bool {
		if patents[i].StrategyCount != patents[j].StrategyCount {
			return patents[i].StrategyCount > patents[j].StrategyCount
		}
		if patents[i].GrantDate != patents[j].GrantDate {
			return patents[i].GrantDate > patents[j].GrantDate
		}
		return patents[i].PatentID < patents[j].PatentID
	})
	if len(patents) > r.maxAssess {
		patents = patents[:r.maxAssess]
		meta.AssessmentTruncated = true
	}

	all := make([]PatentAssessment, 0, len(patents))
	metrics := StageAttemptMetrics{}
	for i := 0; i < len(patents); i += r.batchSize {
		end := i + r.batchSize
		if end > len(patents) {
			end = len(patents)
		}
		batch := patents[i:end]
		batchAssess, m, err := r.runStage3Batch(ctx, s1, batch)
		metrics.Attempts += m.Attempts
		metrics.ContentRetries += m.ContentRetries
		if err != nil {
			return Stage3Output{}, meta, metrics, err
		}
		all = append(all, batchAssess...)
	}

	noneCount := 0
	filtered := make([]PatentAssessment, 0, len(all))
	for _, a := range all {
		if a.Relevance == RelevanceNone {
			noneCount++
			continue
		}
		filtered = append(filtered, a)
	}
	meta.AssessedNone = noneCount
	grantDateByID := map[string]string{}
	for _, p := range patents {
		grantDateByID[p.PatentID] = p.GrantDate
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		ri, rj := relevanceRank(filtered[i].Relevance), relevanceRank(filtered[j].Relevance)
		if ri != rj {
			return ri < rj
		}
		if len(filtered[i].NovelElementsCovered) != len(filtered[j].NovelElementsCovered) {
			return len(filtered[i].NovelElementsCovered) > len(filtered[j].NovelElementsCovered)
		}
		if grantDateByID[filtered[i].PatentID] != grantDateByID[filtered[j].PatentID] {
			return grantDateByID[filtered[i].PatentID] > grantDateByID[filtered[j].PatentID]
		}
		return filtered[i].PatentID < filtered[j].PatentID
	})
	return Stage3Output{Assessments: filtered}, meta, metrics, nil
}

func (r *LLMStageRunner) runStage3Batch(ctx context.Context, s1 Stage1Output, batch []PatentResult) ([]PatentAssessment, StageAttemptMetrics, error) {
	metrics := StageAttemptMetrics{}
	validNE := map[string]struct{}{}
	for _, ne := range s1.NovelElements {
		validNE[ne.ID] = struct{}{}
	}
	batchByID := map[string]PatentResult{}
	for _, p := range batch {
		batchByID[p.PatentID] = p
	}

	prompt := buildStage3Prompt(s1, batch)
	var parsed struct {
		Assessments []PatentAssessment `json:"assessments"`
	}
	for attempt := 1; attempt <= 3; attempt++ {
		metrics.Attempts = attempt
		raw, err := r.exec.caller.GenerateJSON(ctx, prompt)
		if err != nil {
			if attempt < 3 {
				metrics.ContentRetries++
				continue
			}
			return nil, metrics, fmt.Errorf("stage_3 transport failure: %w", err)
		}
		clean := stripCodeFences(strings.TrimSpace(raw))
		if err := json.Unmarshal([]byte(clean), &parsed); err != nil {
			if attempt < 3 {
				metrics.ContentRetries++
				continue
			}
			return nil, metrics, fmt.Errorf("stage_3 failed json parse: %w", err)
		}

		filtered := map[string]PatentAssessment{}
		for _, a := range parsed.Assessments {
			id := strings.TrimSpace(a.PatentID)
			if _, ok := batchByID[id]; !ok {
				log.Printf("prior-art-search stage3 dropped unknown patent_id=%s", id)
				continue
			}
			filtered[id] = normalizeAssessment(a, validNE)
		}
		missing := missingPatentIDs(batch, filtered)
		if len(missing) > 0 && attempt < 3 {
			metrics.ContentRetries++
			continue
		}

		out := make([]PatentAssessment, 0, len(batch))
		for _, p := range batch {
			a, ok := filtered[p.PatentID]
			if !ok {
				log.Printf("prior-art-search stage3 missing assessment fallback patent_id=%s", p.PatentID)
				a = PatentAssessment{PatentID: p.PatentID, Relevance: RelevanceNone, OverlapDescription: "Assessment not returned by model", NovelElementsCovered: []string{}, ConfidenceScore: 0}
			}
			out = append(out, a)
		}
		return out, metrics, nil
	}
	return nil, metrics, fmt.Errorf("stage_3 batch failed")
}

func buildStage3Prompt(s1 Stage1Output, batch []PatentResult) string {
	var b strings.Builder
	b.WriteString("Return valid JSON only. No markdown fences, no commentary.\n\n")
	b.WriteString(`You are a patent analyst assessing prior art relevance.

IMPORTANT: Return valid JSON only. No markdown fences, no commentary, no
preamble. Your entire response must be a single JSON object.

INVENTION SUMMARY:
` + s1.InventionSummary + `

NOVEL ELEMENTS:
`)
	for _, ne := range s1.NovelElements {
		b.WriteString(ne.ID + ": " + ne.Description + "\n")
	}
	b.WriteString("\nPATENTS TO ASSESS (assess each one):\n")
	for i, p := range batch {
		assignee := "Unknown"
		if len(p.Assignees) > 0 {
			assignee = p.Assignees[0]
		}
		abs := p.Abstract
		if len(abs) > 600 {
			abs = abs[:600]
		}
		b.WriteString("INDEX: " + strconv.Itoa(i) + "\n")
		b.WriteString("PATENT_ID: " + p.PatentID + "\n")
		b.WriteString("TITLE: " + p.Title + "\n")
		b.WriteString("ABSTRACT: " + abs + "\n")
		b.WriteString("GRANT_DATE: " + p.GrantDate + "\n")
		b.WriteString("ASSIGNEE: " + assignee + "\n\n")
	}
	b.WriteString(`For each patent, provide:
1. RELEVANCE:
   - HIGH: Same problem, same/very similar approach. Potential §102/§103.
   - MEDIUM: Related problem or overlapping techniques. Possible §103
     when combined. Worth reviewing.
   - LOW: Same domain, different problem/approach. Unlikely cited.
   - NONE: Not relevant.
   Default MEDIUM if unsure HIGH/MEDIUM. Default LOW if unsure MEDIUM/LOW.

2. OVERLAP DESCRIPTION: 1-3 sentences on what overlaps (or doesn't).

3. NOVEL ELEMENTS COVERED: Which elements does this patent address?
   Use IDs (NE1, NE2, ...). Empty array if none.

Return one assessment per patent, in the same order as input.`)
	return b.String()
}

func normalizeAssessment(a PatentAssessment, validNE map[string]struct{}) PatentAssessment {
	a.PatentID = strings.TrimSpace(a.PatentID)
	a.Relevance = normalizeRelevance(a.Relevance)
	a.OverlapDescription = clampString(a.OverlapDescription, 500)
	if len(strings.TrimSpace(a.OverlapDescription)) < 20 {
		log.Printf("prior-art-search stage3 short overlap_description patent_id=%s", a.PatentID)
	}
	if a.ConfidenceScore < 0 {
		a.ConfidenceScore = 0
	}
	if a.ConfidenceScore > 1 {
		a.ConfidenceScore = 1
	}
	filtered := make([]string, 0, len(a.NovelElementsCovered))
	seen := map[string]struct{}{}
	for _, id := range a.NovelElementsCovered {
		id = strings.ToUpper(strings.TrimSpace(id))
		if _, ok := validNE[id]; !ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		filtered = append(filtered, id)
	}
	a.NovelElementsCovered = filtered
	return a
}

func missingPatentIDs(batch []PatentResult, assessed map[string]PatentAssessment) []string {
	out := []string{}
	for _, p := range batch {
		if _, ok := assessed[p.PatentID]; !ok {
			out = append(out, p.PatentID)
		}
	}
	return out
}

func relevanceRank(r Relevance) int {
	switch r {
	case RelevanceHigh:
		return 0
	case RelevanceMedium:
		return 1
	case RelevanceLow:
		return 2
	default:
		return 3
	}
}

func (r *LLMStageRunner) RunStage4(ctx context.Context, s1 Stage1Output, s2 Stage2Output, s3 Stage3Output) (Stage4Output, StageAttemptMetrics, error) {
	out := Stage4Output{}
	prompt := buildStage4Prompt(s1, s2, s3)
	m, err := r.exec.Run(ctx, "stage_4", prompt, &out, func() error {
		return validateAndNormalizeStage4(&out, s2, s3)
	})
	return out, m, err
}

func buildStage4Prompt(s1 Stage1Output, s2 Stage2Output, s3 Stage3Output) string {
	stats := computeLandscapeStats(s1, s2, s3)
	var b strings.Builder
	b.WriteString("Return valid JSON only. No markdown fences, no commentary.\n\n")
	b.WriteString(`You are a patent landscape analyst preparing a summary for a tech transfer
officer.

IMPORTANT: Return valid JSON only. No markdown fences, no commentary, no
preamble.

INVENTION SUMMARY:
` + s1.InventionSummary + "\n\nNOVEL ELEMENTS:\n")
	for _, ne := range s1.NovelElements {
		b.WriteString(ne.ID + ": " + ne.Description + "\n")
	}
	b.WriteString("\nASSIGNEE FREQUENCY (computed, accurate):\n")
	for _, a := range stats.AssigneeFrequency {
		b.WriteString(fmt.Sprintf("%s: %d patents\n", a.Name, a.Count))
	}
	b.WriteString("\nCPC DISTRIBUTION (computed, accurate):\n")
	for _, c := range stats.CPCHistogram {
		b.WriteString(fmt.Sprintf("%s: %d patents\n", c.Subclass, c.Count))
	}
	b.WriteString("\nNOVEL ELEMENT COVERAGE (computed, accurate):\n")
	for _, ne := range stats.NovelCoverage {
		b.WriteString(fmt.Sprintf("%s: %d patents\n", ne.ID, ne.TotalCount))
	}
	b.WriteString(fmt.Sprintf("\nSEARCH STATISTICS:\nTotal patents retrieved: %d\nTotal assessed: %d\nHIGH relevance: %d\nMEDIUM relevance: %d\n\n", len(s2.Patents), len(s3.Assessments), stats.HighCount, stats.MediumCount))
	b.WriteString("TOP PRIOR ART (up to 20, HIGH and MEDIUM only):\n")
	for _, p := range stats.TopPriorArt {
		b.WriteString(fmt.Sprintf("PATENT_ID: %s\nTITLE: %s\nABSTRACT: %s\nGRANT_DATE: %s\nASSIGNEE: %s\nRELEVANCE: %s\nOVERLAP: %s\nELEMENTS COVERED: %s\n\n",
			p.PatentID, p.Title, clampString(p.Abstract, 200), p.GrantDate, p.PrimaryAssignee, p.Relevance, p.OverlapDescription, strings.Join(p.NovelElementsCovered, ", ")))
	}
	b.WriteString(`Analyze:

1. LANDSCAPE DENSITY: Crowded, moderate, or sparse?

2. KEY PLAYERS: Using the assignee frequency above, identify top 3-5
   and explain why they matter. Do NOT invent patent counts — use the
   numbers provided above. When you output key_players[].name, copy the
   assignee name EXACTLY as it appears in ASSIGNEE FREQUENCY (e.g.,
   "Google LLC" not "Google").

3. BLOCKING RISK: Any patents that directly anticipate (§102) or render
   obvious (§103) the core invention? Cite specific patent IDs from the
   TOP PRIOR ART list above. Only cite patents that appear in that list.

4. DESIGN-AROUND POTENTIAL: Are existing patents narrow or broad?

5. WHITE SPACE: Using novel element coverage, which aspects have least
   coverage? These are the strongest claim candidates.

6. DETERMINATION: CLEAR_FIELD, CROWDED_FIELD, BLOCKING_ART_FOUND, or
   INCONCLUSIVE.`)
	return b.String()
}

func validateAndNormalizeStage4(s *Stage4Output, stage2 Stage2Output, stage3 Stage3Output) error {
	s.LandscapeDensity = normalizeDensity(s.LandscapeDensity)
	s.BlockingRisk.Level = normalizeBlockingRisk(s.BlockingRisk.Level)
	s.DesignAroundPotential.Level = normalizeDesignAround(s.DesignAroundPotential.Level)
	s.Determination = normalizeDetermination(s.Determination)
	s.ConfidenceReason = strings.TrimSpace(s.ConfidenceReason)

	s.LandscapeDensityReasoning = clampString(s.LandscapeDensityReasoning, 500)
	s.BlockingRisk.Reasoning = clampString(s.BlockingRisk.Reasoning, 1000)
	s.DesignAroundPotential.Reasoning = clampString(s.DesignAroundPotential.Reasoning, 500)
	s.DeterminationReasoning = clampString(s.DeterminationReasoning, 1000)
	for i := range s.WhiteSpace {
		s.WhiteSpace[i] = clampString(s.WhiteSpace[i], 200)
	}

	if len(s.WhiteSpace) < 1 || len(s.WhiteSpace) > 5 {
		return fmt.Errorf("white_space count")
	}
	if len(s.KeyPlayers) > 5 {
		return fmt.Errorf("key_players count")
	}
	if s.ConfidenceScore < 0 || s.ConfidenceScore > 1 {
		return fmt.Errorf("confidence_score out of range")
	}
	if len(s.ConfidenceReason) < 10 {
		return fmt.Errorf("confidence_reason too short")
	}

	assigneeLookup := map[string]int{}
	for _, a := range computeLandscapeStats(Stage1Output{}, stage2, stage3).AssigneeFrequency {
		assigneeLookup[strings.ToLower(strings.TrimSpace(a.Name))] = a.Count
	}
	for i := range s.KeyPlayers {
		kp := &s.KeyPlayers[i]
		kp.Name = strings.TrimSpace(kp.Name)
		kp.RelevanceNote = clampString(kp.RelevanceNote, 200)
		if kp.Name == "" {
			continue
		}
		kp.PatentCount = assigneeLookup[strings.ToLower(kp.Name)]
	}

	validIDs := map[string]struct{}{}
	for _, a := range stage3.Assessments {
		if a.Relevance == RelevanceHigh || a.Relevance == RelevanceMedium {
			validIDs[a.PatentID] = struct{}{}
		}
	}
	filtered := make([]string, 0, len(s.BlockingRisk.BlockingPatents))
	for _, id := range s.BlockingRisk.BlockingPatents {
		id = strings.TrimSpace(id)
		if _, ok := validIDs[id]; ok {
			filtered = append(filtered, id)
		}
	}
	s.BlockingRisk.BlockingPatents = filtered
	if s.BlockingRisk.Level == BlockingRiskHigh && len(s.BlockingRisk.BlockingPatents) == 0 {
		s.BlockingRisk.Level = BlockingRiskMedium
	}
	if s.BlockingRisk.Level == BlockingRiskNone {
		s.BlockingRisk.BlockingPatents = nil
	}
	if s.BlockingRisk.Level == BlockingRiskHigh && s.Determination != DeterminationBlockingArt {
		s.Determination = DeterminationBlockingArt
	}
	if s.BlockingRisk.Level == BlockingRiskNone && s.LandscapeDensity == DensitySparse && s.Determination != DeterminationClearField {
		log.Printf("prior-art-search warning inconsistent stage4 output: sparse + no blocking risk but determination=%s", s.Determination)
	}
	return nil
}

type assessedPatent struct {
	PatentID             string
	Title                string
	Abstract             string
	GrantDate            string
	PrimaryAssignee      string
	Relevance            Relevance
	OverlapDescription   string
	NovelElementsCovered []string
}

type landscapeStats struct {
	AssigneeFrequency []AssigneeCount
	CPCHistogram      []CPCCount
	NovelCoverage     []NovelElementCoverage
	HighCount         int
	MediumCount       int
	TopPriorArt       []assessedPatent
}

func computeLandscapeStats(s1 Stage1Output, s2 Stage2Output, s3 Stage3Output) landscapeStats {
	stats := landscapeStats{}
	byID := map[string]PatentResult{}
	for _, p := range s2.Patents {
		byID[p.PatentID] = p
	}
	assigneeFreq := map[string]int{}
	cpcFreq := map[string]int{}
	neFreqHigh := map[string]int{}
	neFreqMed := map[string]int{}

	for _, p := range s2.Patents {
		for _, c := range p.CPCSubclasses {
			cpcFreq[c]++
		}
	}
	for _, a := range s3.Assessments {
		if a.Relevance != RelevanceHigh && a.Relevance != RelevanceMedium {
			continue
		}
		p, ok := byID[a.PatentID]
		if !ok {
			continue
		}
		if len(p.Assignees) > 0 {
			assigneeFreq[normalizeAssignee(p.Assignees[0])]++
		}
		for _, id := range a.NovelElementsCovered {
			if a.Relevance == RelevanceHigh {
				neFreqHigh[id]++
			} else {
				neFreqMed[id]++
			}
		}
		if a.Relevance == RelevanceHigh {
			stats.HighCount++
		} else {
			stats.MediumCount++
		}
		stats.TopPriorArt = append(stats.TopPriorArt, assessedPatent{
			PatentID: a.PatentID, Title: p.Title, Abstract: p.Abstract, GrantDate: p.GrantDate,
			PrimaryAssignee: firstOrUnknown(p.Assignees), Relevance: a.Relevance,
			OverlapDescription: a.OverlapDescription, NovelElementsCovered: a.NovelElementsCovered,
		})
	}
	sort.SliceStable(stats.TopPriorArt, func(i, j int) bool {
		if relevanceRank(stats.TopPriorArt[i].Relevance) != relevanceRank(stats.TopPriorArt[j].Relevance) {
			return relevanceRank(stats.TopPriorArt[i].Relevance) < relevanceRank(stats.TopPriorArt[j].Relevance)
		}
		return stats.TopPriorArt[i].GrantDate > stats.TopPriorArt[j].GrantDate
	})
	if len(stats.TopPriorArt) > 20 {
		stats.TopPriorArt = stats.TopPriorArt[:20]
	}

	for name, count := range assigneeFreq {
		stats.AssigneeFrequency = append(stats.AssigneeFrequency, AssigneeCount{Name: name, Count: count})
	}
	sort.Slice(stats.AssigneeFrequency, func(i, j int) bool {
		if stats.AssigneeFrequency[i].Count != stats.AssigneeFrequency[j].Count {
			return stats.AssigneeFrequency[i].Count > stats.AssigneeFrequency[j].Count
		}
		return stats.AssigneeFrequency[i].Name < stats.AssigneeFrequency[j].Name
	})
	if len(stats.AssigneeFrequency) > 10 {
		stats.AssigneeFrequency = stats.AssigneeFrequency[:10]
	}

	for c, count := range cpcFreq {
		stats.CPCHistogram = append(stats.CPCHistogram, CPCCount{Subclass: c, Count: count})
	}
	sort.Slice(stats.CPCHistogram, func(i, j int) bool {
		if stats.CPCHistogram[i].Count != stats.CPCHistogram[j].Count {
			return stats.CPCHistogram[i].Count > stats.CPCHistogram[j].Count
		}
		return stats.CPCHistogram[i].Subclass < stats.CPCHistogram[j].Subclass
	})
	if len(stats.CPCHistogram) > 10 {
		stats.CPCHistogram = stats.CPCHistogram[:10]
	}

	for _, ne := range s1.NovelElements {
		stats.NovelCoverage = append(stats.NovelCoverage, NovelElementCoverage{
			ID: ne.ID, Description: ne.Description,
			HighCount: neFreqHigh[ne.ID], MediumCount: neFreqMed[ne.ID],
			TotalCount: neFreqHigh[ne.ID] + neFreqMed[ne.ID],
		})
	}
	return stats
}

func normalizePriority(p StrategyPriority) StrategyPriority {
	switch strings.ToUpper(strings.TrimSpace(string(p))) {
	case string(PriorityPrimary):
		return PriorityPrimary
	case string(PrioritySecondary):
		return PrioritySecondary
	default:
		return PriorityTertiary
	}
}

func normalizeRelevance(r Relevance) Relevance {
	switch strings.ToUpper(strings.TrimSpace(string(r))) {
	case string(RelevanceHigh):
		return RelevanceHigh
	case string(RelevanceMedium):
		return RelevanceMedium
	case string(RelevanceLow):
		return RelevanceLow
	default:
		return RelevanceNone
	}
}

func normalizeDensity(d LandscapeDensity) LandscapeDensity {
	switch strings.ToUpper(strings.TrimSpace(string(d))) {
	case string(DensitySparse):
		return DensitySparse
	case string(DensityModerate):
		return DensityModerate
	default:
		return DensityDense
	}
}

func normalizeBlockingRisk(b BlockingRiskLevel) BlockingRiskLevel {
	switch strings.ToUpper(strings.TrimSpace(string(b))) {
	case string(BlockingRiskHigh):
		return BlockingRiskHigh
	case string(BlockingRiskMedium):
		return BlockingRiskMedium
	case string(BlockingRiskLow):
		return BlockingRiskLow
	default:
		return BlockingRiskNone
	}
}

func normalizeDesignAround(d DesignAroundLevel) DesignAroundLevel {
	switch strings.ToUpper(strings.TrimSpace(string(d))) {
	case string(DesignAroundEasy):
		return DesignAroundEasy
	case string(DesignAroundModerate):
		return DesignAroundModerate
	default:
		return DesignAroundDifficult
	}
}

func normalizeDetermination(d Determination) Determination {
	switch strings.ToUpper(strings.TrimSpace(string(d))) {
	case string(DeterminationClearField):
		return DeterminationClearField
	case string(DeterminationCrowdedField):
		return DeterminationCrowdedField
	case string(DeterminationBlockingArt):
		return DeterminationBlockingArt
	default:
		return DeterminationInconclusive
	}
}

func isValidNEID(id string) bool {
	if !strings.HasPrefix(id, "NE") {
		return false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, "NE"))
	if err != nil {
		return false
	}
	return n >= 1 && n <= 10
}

func compactStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func clampString(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func firstOrUnknown(items []string) string {
	if len(items) == 0 {
		return "Unknown"
	}
	if strings.TrimSpace(items[0]) == "" {
		return "Unknown"
	}
	return items[0]
}
