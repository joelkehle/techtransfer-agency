package patentscreen

import (
	"context"
	"fmt"
	"strings"
)

const stage2PromptContext = `Under 35 U.S.C. § 101, patentable subject matter must fall within one of four
statutory categories:

1. PROCESS: An act or series of acts or steps performed on a subject matter
   to produce a useful, concrete result. (MPEP § 2106.03(a))

2. MACHINE: A concrete thing, consisting of parts or certain devices and
   combination of devices. (MPEP § 2106.03(b))

3. MANUFACTURE: An article produced from raw or prepared materials by giving
   these materials new forms, qualities, properties, or combinations, whether
   by hand labor or by machinery. (MPEP § 2106.03(c))

4. COMPOSITION_OF_MATTER: All compositions of two or more substances and
   all composite articles, whether they are results of chemical union or of
   mechanical mixture, or whether they are gases, fluids, powders, or solids.
   (MPEP § 2106.03(d))

A claim may fall into more than one category.`

const stage3PromptContext = `Under Step 2A, Prong One (MPEP § 2106.04), determine whether the claim
recites a judicial exception.

The three categories of judicial exceptions are:

1. ABSTRACT IDEAS, which include:
   a. Mathematical concepts — mathematical relationships, formulas, equations,
      calculations. (MPEP § 2106.04(a)(2), Group I)
   b. Certain methods of organizing human activity — fundamental economic
      practices, commercial interactions, managing personal behavior or
      relationships, legal interactions. (MPEP § 2106.04(a)(2), Group II)
   c. Mental processes — concepts performed in the human mind including
      observation, evaluation, judgment, opinion. A claim recites a mental
      process when it can practically be performed in the human mind.
      (MPEP § 2106.04(a)(2), Group III)

2. LAWS OF NATURE — naturally occurring principles or relationships.
3. NATURAL PHENOMENA / PRODUCTS OF NATURE — naturally occurring things.

Important: A claim that merely involves or is based on a judicial exception
is different from a claim that recites a judicial exception.`

const stage4PromptContext = `Under Step 2A, Prong Two (MPEP § 2106.04(d)), evaluate whether the claim
as a whole integrates the judicial exception into a practical application.

A claim integrates the judicial exception into a practical application when
the additional elements (beyond the exception itself) apply, rely on, or use
the exception in a manner that imposes a meaningful limit on the exception.

Considerations indicating integration (MPEP § 2106.04(d)(1)):
- Improvement to the functioning of a computer or other technology (MPEP § 2106.05(a))
- Application of the exception with a particular machine (MPEP § 2106.05(b))
- Transformation of a particular article to a different state or thing (MPEP § 2106.05(c))
- Application of the exception in some other meaningful way beyond generally linking use
  to a particular technological environment (MPEP § 2106.05(e))

Considerations indicating NO integration:
- Adding the words "apply it" with no meaningful limit (MPEP § 2106.05(f))
- Adding insignificant extra-solution activity (MPEP § 2106.05(g))
- Generally linking use to a particular technological environment (MPEP § 2106.05(h))`

const stage5PromptContext = `Under Step 2B (MPEP § 2106.05), determine whether the additional elements,
individually and in combination, provide an inventive concept — i.e., amount
to significantly more than the judicial exception itself.

Considerations indicating significantly more:
- Adds a specific limitation beyond what is well-understood, routine, and conventional
  in the field (MPEP § 2106.05(d))
- Adds unconventional steps that confine the claim to a particular useful application
  (MPEP § 2106.05(e))

Considerations indicating NOT significantly more:
- Adding well-understood, routine, conventional activities previously known in the industry
  (MPEP § 2106.05(d), Berkheimer Memo)
- Appending well-known conventional steps at a high level of generality

Per the Berkheimer Memo (April 2018), well-understood/routine/conventional findings
must be supported by evidence or clear justification.`

const stage1SchemaPrompt = `Required JSON schema:
{
  "invention_title": "string (5-200 chars)",
  "abstract": "string (20-500 chars)",
  "problem_solved": "string (20-1000 chars)",
  "invention_description": "string (50-5000 chars)",
  "novel_elements": ["string (1-20 entries, each 10-500 chars)"],
  "technology_area": "string (5-100 chars)",
  "claims_present": "boolean",
  "claims_summary": "string or null (10-2000 chars if present)",
  "confidence_score": "float (0.0-1.0)",
  "confidence_reason": "string (min 10 chars)",
  "insufficient_information": "boolean"
}`

const stage2SchemaPrompt = `Required JSON schema:
{
  "categories": ["PROCESS | MACHINE | MANUFACTURE | COMPOSITION_OF_MATTER (0-4 entries, conditional)"],
  "explanation": "string (20-2000 chars)",
  "passes_step_1": "boolean",
  "confidence_score": "float (0.0-1.0)",
  "confidence_reason": "string (min 10 chars)",
  "insufficient_information": "boolean"
}`

const stage3SchemaPrompt = `Required JSON schema:
{
  "recites_exception": "boolean",
  "exception_type": "ABSTRACT_IDEA | LAW_OF_NATURE | NATURAL_PHENOMENON | null",
  "abstract_idea_subcategory": "MATHEMATICAL_CONCEPT | ORGANIZING_HUMAN_ACTIVITY | MENTAL_PROCESS | null",
  "reasoning": "string (50-3000 chars)",
  "mpep_reference": "string (10-200 chars)",
  "confidence_score": "float (0.0-1.0)",
  "confidence_reason": "string (min 10 chars)",
  "insufficient_information": "boolean"
}`

const stage4SchemaPrompt = `Required JSON schema:
{
  "additional_elements": ["string (1-10 entries, each 10-500 chars)"],
  "integrates_practical_application": "boolean",
  "considerations_for": ["string (0-10 entries, each 10-500 chars)"],
  "considerations_against": ["string (0-10 entries, each 10-500 chars)"],
  "reasoning": "string (50-3000 chars)",
  "mpep_reference": "string (10-200 chars)",
  "confidence_score": "float (0.0-1.0)",
  "confidence_reason": "string (min 10 chars)",
  "insufficient_information": "boolean"
}`

const stage5SchemaPrompt = `Required JSON schema:
{
  "has_inventive_concept": "boolean",
  "reasoning": "string (50-3000 chars)",
  "berkheimer_considerations": "string (20-2000 chars)",
  "mpep_reference": "string (10-200 chars)",
  "confidence_score": "float (0.0-1.0)",
  "confidence_reason": "string (min 10 chars)",
  "insufficient_information": "boolean"
}`

const stage6SchemaPrompt = `Required JSON schema:
{
  "novelty_concerns": ["string (0-10 entries, each 10-500 chars)"],
  "non_obviousness_concerns": ["string (0-10 entries, each 10-500 chars)"],
  "prior_art_search_priority": "HIGH | MEDIUM | LOW",
  "reasoning": "string (50-3000 chars)",
  "confidence_score": "float (0.0-1.0)",
  "confidence_reason": "string (min 10 chars)",
  "insufficient_information": "boolean"
}`

type StageRunner interface {
	RunStage1(ctx context.Context, req RequestEnvelope) (Stage1Output, StageAttemptMetrics, error)
	RunStage2(ctx context.Context, s1 Stage1Output) (Stage2Output, StageAttemptMetrics, error)
	RunStage3(ctx context.Context, s1 Stage1Output, s2 Stage2Output) (Stage3Output, StageAttemptMetrics, error)
	RunStage4(ctx context.Context, s1 Stage1Output, s3 Stage3Output) (Stage4Output, StageAttemptMetrics, error)
	RunStage5(ctx context.Context, s1 Stage1Output, s3 Stage3Output, s4 Stage4Output) (Stage5Output, StageAttemptMetrics, error)
	RunStage6(ctx context.Context, s1 Stage1Output) (Stage6Output, StageAttemptMetrics, error)
}

type LLMStageRunner struct {
	exec *StageExecutor
}

func NewLLMStageRunner(exec *StageExecutor) *LLMStageRunner {
	return &LLMStageRunner{exec: exec}
}

func (r *LLMStageRunner) RunStage1(ctx context.Context, req RequestEnvelope) (Stage1Output, StageAttemptMetrics, error) {
	out := Stage1Output{}
	prompt := fmt.Sprintf(
		"Stage 1: Structured Extraction.\nExtract key invention elements from the disclosure.\n\n%s\n\nInput disclosure text:\n%s",
		stage1SchemaPrompt,
		req.DisclosureText,
	)
	m, err := r.exec.Run(ctx, "stage_1", prompt, &out, func() error { return validateStage1(out) })
	return out, m, err
}

func (r *LLMStageRunner) RunStage2(ctx context.Context, s1 Stage1Output) (Stage2Output, StageAttemptMetrics, error) {
	out := Stage2Output{}
	prompt := fmt.Sprintf(
		"Stage 2: Statutory Category Classification.\n%s\n\n%s\n\nInvention description:\n%s\n\nNovel elements:\n%v\n\nClaims summary:\n%v",
		stage2PromptContext,
		stage2SchemaPrompt,
		s1.InventionDescription,
		s1.NovelElements,
		s1.ClaimsSummary,
	)
	m, err := r.exec.Run(ctx, "stage_2", prompt, &out, func() error { return validateStage2(out) })
	return out, m, err
}

func (r *LLMStageRunner) RunStage3(ctx context.Context, s1 Stage1Output, s2 Stage2Output) (Stage3Output, StageAttemptMetrics, error) {
	out := Stage3Output{}
	prompt := fmt.Sprintf(
		"Stage 3: Alice/Mayo Step 2A Prong 1 — Judicial Exception.\n%s\n\n%s\n\nInvention description:\n%s\n\nStatutory categories:\n%v",
		stage3PromptContext,
		stage3SchemaPrompt,
		s1.InventionDescription,
		s2.Categories,
	)
	m, err := r.exec.Run(ctx, "stage_3", prompt, &out, func() error { return validateStage3(out) })
	return out, m, err
}

func (r *LLMStageRunner) RunStage4(ctx context.Context, s1 Stage1Output, s3 Stage3Output) (Stage4Output, StageAttemptMetrics, error) {
	out := Stage4Output{}
	prompt := fmt.Sprintf(
		"Stage 4: Alice/Mayo Step 2A Prong 2 — Practical Application.\n%s\n\n%s\n\nInvention description:\n%s\n\nStage 3 result:\n%s",
		stage4PromptContext,
		stage4SchemaPrompt,
		s1.InventionDescription,
		s3.Reasoning,
	)
	m, err := r.exec.Run(ctx, "stage_4", prompt, &out, func() error { return validateStage4(out) })
	return out, m, err
}

func (r *LLMStageRunner) RunStage5(ctx context.Context, s1 Stage1Output, s3 Stage3Output, s4 Stage4Output) (Stage5Output, StageAttemptMetrics, error) {
	out := Stage5Output{}
	prompt := fmt.Sprintf(
		"Stage 5: Alice/Mayo Step 2B — Inventive Concept.\n%s\n\n%s\n\nInvention description:\n%s\n\nAdditional elements:\n%v\n\nStage 3 summary:\n%s",
		stage5PromptContext,
		stage5SchemaPrompt,
		s1.InventionDescription,
		s4.AdditionalElements,
		s3.Reasoning,
	)
	m, err := r.exec.Run(ctx, "stage_5", prompt, &out, func() error { return validateStage5(out) })
	return out, m, err
}

func (r *LLMStageRunner) RunStage6(ctx context.Context, s1 Stage1Output) (Stage6Output, StageAttemptMetrics, error) {
	out := Stage6Output{}
	prompt := fmt.Sprintf(
		"Stage 6: §102/§103 Preliminary Flags.\nThis stage is advisory only and does not change the legal determination.\n\n%s\n\nInvention description:\n%s\n\nNovel elements:\n%v\n\nTechnology area:\n%s",
		stage6SchemaPrompt,
		s1.InventionDescription,
		s1.NovelElements,
		s1.TechnologyArea,
	)
	m, err := r.exec.Run(ctx, "stage_6", prompt, &out, func() error { return validateStage6(out) })
	return out, m, err
}

func validateConfidence(c StageConfidence) error {
	if c.ConfidenceScore < 0 || c.ConfidenceScore > 1 {
		return fmt.Errorf("confidence_score out of range")
	}
	if len(strings.TrimSpace(c.ConfidenceReason)) < 10 {
		return fmt.Errorf("confidence_reason too short")
	}
	return nil
}

func validateStage1(s Stage1Output) error {
	if err := validateConfidence(s.StageConfidence); err != nil {
		return err
	}
	if !between(len(strings.TrimSpace(s.InventionTitle)), 5, 200) {
		return fmt.Errorf("invention_title length")
	}
	if !between(len(strings.TrimSpace(s.Abstract)), 20, 500) {
		return fmt.Errorf("abstract length")
	}
	if !between(len(strings.TrimSpace(s.ProblemSolved)), 20, 1000) {
		return fmt.Errorf("problem_solved length")
	}
	if !between(len(strings.TrimSpace(s.InventionDescription)), 50, 5000) {
		return fmt.Errorf("invention_description length")
	}
	if !between(len(strings.TrimSpace(s.TechnologyArea)), 5, 100) {
		return fmt.Errorf("technology_area length")
	}
	if len(s.NovelElements) < 1 || len(s.NovelElements) > 20 {
		return fmt.Errorf("novel_elements count")
	}
	for _, e := range s.NovelElements {
		if !between(len(strings.TrimSpace(e)), 10, 500) {
			return fmt.Errorf("novel_elements entry length")
		}
	}
	if s.ClaimsPresent {
		if s.ClaimsSummary == nil || !between(len(strings.TrimSpace(*s.ClaimsSummary)), 10, 2000) {
			return fmt.Errorf("claims_summary required when claims_present")
		}
	} else if s.ClaimsSummary != nil {
		return fmt.Errorf("claims_summary must be null when claims_present is false")
	}
	return nil
}

func validateStage2(s Stage2Output) error {
	if err := validateConfidence(s.StageConfidence); err != nil {
		return err
	}
	if !between(len(strings.TrimSpace(s.Explanation)), 20, 2000) {
		return fmt.Errorf("explanation length")
	}
	if s.PassesStep1 {
		if len(s.Categories) < 1 || len(s.Categories) > 4 {
			return fmt.Errorf("categories count invalid when passes_step_1=true")
		}
	} else if len(s.Categories) != 0 {
		return fmt.Errorf("categories must be empty when passes_step_1=false")
	}
	seen := map[Stage2Category]bool{}
	for _, c := range s.Categories {
		switch c {
		case CategoryProcess, CategoryMachine, CategoryManufacture, CategoryCompositionOfMatter:
		default:
			return fmt.Errorf("invalid category %q", c)
		}
		if seen[c] {
			return fmt.Errorf("duplicate category")
		}
		seen[c] = true
	}
	return nil
}

func validateStage3(s Stage3Output) error {
	if err := validateConfidence(s.StageConfidence); err != nil {
		return err
	}
	if !between(len(strings.TrimSpace(s.Reasoning)), 50, 3000) {
		return fmt.Errorf("reasoning length")
	}
	if !between(len(strings.TrimSpace(s.MPEPReference)), 10, 200) {
		return fmt.Errorf("mpep_reference length")
	}
	if !s.RecitesException {
		if s.ExceptionType != nil || s.AbstractIdeaSubcategory != nil {
			return fmt.Errorf("exception fields must be null")
		}
		return nil
	}
	if s.ExceptionType == nil {
		return fmt.Errorf("exception_type required")
	}
	switch *s.ExceptionType {
	case ExceptionAbstractIdea, ExceptionLawOfNature, ExceptionNaturalPhenomenon:
	default:
		return fmt.Errorf("invalid exception_type")
	}
	if *s.ExceptionType == ExceptionAbstractIdea {
		if s.AbstractIdeaSubcategory == nil {
			return fmt.Errorf("abstract_idea_subcategory required")
		}
		switch *s.AbstractIdeaSubcategory {
		case SubcategoryMathematicalConcept, SubcategoryOrganizingHumanActivity, SubcategoryMentalProcess:
		default:
			return fmt.Errorf("invalid abstract_idea_subcategory")
		}
	} else if s.AbstractIdeaSubcategory != nil {
		return fmt.Errorf("abstract_idea_subcategory must be null")
	}
	return nil
}

func validateStage4(s Stage4Output) error {
	if err := validateConfidence(s.StageConfidence); err != nil {
		return err
	}
	if len(s.AdditionalElements) < 1 || len(s.AdditionalElements) > 10 {
		return fmt.Errorf("additional_elements count")
	}
	for _, e := range s.AdditionalElements {
		if !between(len(strings.TrimSpace(e)), 10, 500) {
			return fmt.Errorf("additional_elements entry length")
		}
	}
	if len(s.ConsiderationsFor) > 10 || len(s.ConsiderationsAgainst) > 10 {
		return fmt.Errorf("considerations count")
	}
	for _, e := range append([]string{}, append(s.ConsiderationsFor, s.ConsiderationsAgainst...)...) {
		if !between(len(strings.TrimSpace(e)), 10, 500) {
			return fmt.Errorf("consideration entry length")
		}
	}
	if !between(len(strings.TrimSpace(s.Reasoning)), 50, 3000) {
		return fmt.Errorf("reasoning length")
	}
	if !between(len(strings.TrimSpace(s.MPEPReference)), 10, 200) {
		return fmt.Errorf("mpep_reference length")
	}
	return nil
}

func validateStage5(s Stage5Output) error {
	if err := validateConfidence(s.StageConfidence); err != nil {
		return err
	}
	if !between(len(strings.TrimSpace(s.Reasoning)), 50, 3000) {
		return fmt.Errorf("reasoning length")
	}
	if !between(len(strings.TrimSpace(s.BerkheimerConsiderations)), 20, 2000) {
		return fmt.Errorf("berkheimer_considerations length")
	}
	if !between(len(strings.TrimSpace(s.MPEPReference)), 10, 200) {
		return fmt.Errorf("mpep_reference length")
	}
	return nil
}

func validateStage6(s Stage6Output) error {
	if err := validateConfidence(s.StageConfidence); err != nil {
		return err
	}
	if len(s.NoveltyConcerns) > 10 || len(s.NonObviousnessConcerns) > 10 {
		return fmt.Errorf("concerns count")
	}
	for _, e := range append([]string{}, append(s.NoveltyConcerns, s.NonObviousnessConcerns...)...) {
		if !between(len(strings.TrimSpace(e)), 10, 500) {
			return fmt.Errorf("concern entry length")
		}
	}
	switch s.PriorArtSearchPriority {
	case PriorityHigh, PriorityMedium, PriorityLow:
	default:
		return fmt.Errorf("prior_art_search_priority invalid")
	}
	if !between(len(strings.TrimSpace(s.Reasoning)), 50, 3000) {
		return fmt.Errorf("reasoning length")
	}
	return nil
}

func between(v, min, max int) bool {
	return v >= min && v <= max
}
