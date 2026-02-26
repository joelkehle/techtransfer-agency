package marketanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type StageRunner interface {
	RunStage0(ctx context.Context, req RequestEnvelope) (Stage0Output, StageAttemptMetrics, error)
	RunStage1(ctx context.Context, s0 Stage0Output) (Stage1Output, StageAttemptMetrics, error)
	RunStage2(ctx context.Context, s0 Stage0Output, s1 Stage1Output) (Stage2Output, StageAttemptMetrics, error)
	RunStage3(ctx context.Context, s0 Stage0Output, s1 Stage1Output, s2 Stage2Output) (Stage3Output, StageAttemptMetrics, error)
	RunStage4(ctx context.Context, s0 Stage0Output, s1 Stage1Output, s2 Stage2Output, s3 Stage3Output) (Stage4Output, StageAttemptMetrics, error)
	RunStage5(ctx context.Context, in Stage5Input) (Stage5Output, StageAttemptMetrics, error)
}

type Stage5Input struct {
	Stage0         Stage0Output
	Stage1         Stage1Output
	Stage2         Stage2Output
	Stage3         *Stage3Output
	Stage4         *Stage4Output
	Stage4Computed *Stage4ComputedOutput
	Decision       RecommendationDecision
}

type LLMStageRunner struct {
	exec *StageExecutor
}

func NewLLMStageRunner(exec *StageExecutor) *LLMStageRunner {
	return &LLMStageRunner{exec: exec}
}

const stage0SchemaPrompt = `Required JSON schema:
{
  "invention_title": {"value":"string","confidence":"LOW|MEDIUM|HIGH","missing_reason":null},
  "problem_solved": {"value":"string","confidence":"LOW|MEDIUM|HIGH","missing_reason":null},
  "solution_description": {"value":"string","confidence":"LOW|MEDIUM|HIGH","missing_reason":null},
  "claimed_advantages": {"value":["string"]|null,"confidence":"LOW|MEDIUM|HIGH","missing_reason":"string|null"},
  "target_user": {"value":"string|null","confidence":"LOW|MEDIUM|HIGH","missing_reason":"string|null"},
  "target_buyer": {"value":"string|null","confidence":"LOW|MEDIUM|HIGH","missing_reason":"string|null"},
  "application_domains": {"value":["string"]|null,"confidence":"LOW|MEDIUM|HIGH","missing_reason":"string|null"},
  "evidence_level":"CONCEPT_ONLY|IN_VITRO|ANIMAL|PROTOTYPE|PILOT|CLINICAL",
  "competing_approaches": {"value":["string"]|null,"confidence":"LOW|MEDIUM|HIGH","missing_reason":"string|null"},
  "dependencies": {"value":["string"]|null,"confidence":"LOW|MEDIUM|HIGH","missing_reason":"string|null"},
  "sector":"software|biotech_therapeutic|biotech_diagnostic|medical_device|semiconductor|materials|clean_energy|mechanical_engineering|default"
}`

const stage1PromptContext = `Classify the most likely commercialization path(s) for this invention.
Select one primary path and optionally one secondary path.

EXCLUSIVE_LICENSE_INCUMBENT
  The invention fits into an existing company's product line. The university
  licenses exclusively to one company in exchange for upfront fees, milestones,
  and royalties. Most common for medical devices, therapeutics, and specialty
  chemicals.

STARTUP_FORMATION
  The invention is the core of a new company. The university takes equity
  and/or royalties. Common for platform technologies, novel therapeutics
  with no obvious incumbent, and deep tech.

NON_EXCLUSIVE_LICENSE
  The invention is a tool, method, or enabling technology useful to many
  players. Licensed non-exclusively to multiple companies at lower royalty
  rates but broader adoption. Common for research tools, software libraries,
  and materials/coatings.

OPEN_SOURCE_PLUS_SERVICES
  The invention is best distributed as open source software or open
  standards, with revenue from services, support, or complementary products.
  Common for software infrastructure and developer tools.

RESEARCH_USE_ONLY
  The invention has academic value but no clear commercial path. Best
  distributed for research use, potentially generating sponsored research
  relationships but not license revenue.

For each selected path, explain why it fits this invention. If the invention
does not have a plausible monetization path, say so explicitly.

Note: if the invention has commercial value but NOT via patents (e.g.,
copyright, know-how licensing, data licensing, services), identify the
primary path and note "non_patent_monetization": true with an explanation.`

const stage1SchemaPrompt = `Required JSON schema:
{
  "primary_path":"EXCLUSIVE_LICENSE_INCUMBENT|STARTUP_FORMATION|NON_EXCLUSIVE_LICENSE|OPEN_SOURCE_PLUS_SERVICES|RESEARCH_USE_ONLY",
  "primary_path_reasoning":"string",
  "secondary_path":"enum|null",
  "secondary_path_reasoning":"string|null",
  "product_definition":"string",
  "has_plausible_monetization":"boolean",
  "no_monetization_reasoning":"string|null",
  "non_patent_monetization":"boolean",
  "non_patent_monetization_reasoning":"string|null"
}`

const stage2PromptContext = `Score each dimension from 1 (very weak) to 5 (very strong). Provide a
1-2 sentence justification for each score. Be conservative — default to
3 unless the disclosure provides clear evidence for a higher or lower score.

MARKET_PAIN (1-5)
  How severe is the problem this invention solves? Is there demonstrated
  willingness to pay for solutions? Score 5 if the problem causes significant
  measurable cost/harm and buyers actively seek solutions. Score 1 if the
  problem is minor, theoretical, or no one is looking for a fix.

DIFFERENTIATION (1-5)
  How clearly does this invention improve on existing approaches? Score 5
  if the advantage is large, measurable, and hard to replicate. Score 1
  if the improvement is marginal or many alternatives exist with similar
  performance.

ADOPTION_FRICTION (1-5, where 5 = LOW friction = good)
  How easy is it for the target customer to adopt this invention? Consider:
  workflow changes required, integration complexity, switching costs,
  training needed, infrastructure requirements. Score 5 if drop-in
  replacement. Score 1 if requires fundamental workflow change or new
  infrastructure.

DEVELOPMENT_BURDEN (1-5, where 5 = LOW burden = good)
  How much work remains to reach a commercially viable product? Consider:
  current TRL/evidence level, regulatory requirements, manufacturing
  complexity, timeline to market. Score 5 if near-ready. Score 1 if
  10+ years and heavy regulatory path.

PARTNER_DENSITY (1-5)
  How many plausible licensees, acquirers, or partners exist for this
  technology? Score 5 if 10+ identifiable companies with clear product-
  line fit. Score 1 if no obvious buyer or highly fragmented market with
  no natural licensee.

IP_LEVERAGE (1-5)
  Based on the disclosure alone (not a formal patent analysis), how strong
  is the likely IP position? Consider: is the invention specific and
  concrete vs. a general idea? Are design-arounds obvious? Would a
  competitor need this specific approach? Score 5 if strong, specific,
  hard to design around. Score 1 if the invention is a general concept
  with obvious alternatives.`

const stage2SchemaPrompt = `Required JSON schema:
{
  "scores":{
    "market_pain":{"score":"int 1-5","reasoning":"string"},
    "differentiation":{"score":"int 1-5","reasoning":"string"},
    "adoption_friction":{"score":"int 1-5","reasoning":"string"},
    "development_burden":{"score":"int 1-5","reasoning":"string"},
    "partner_density":{"score":"int 1-5","reasoning":"string"},
    "ip_leverage":{"score":"int 1-5","reasoning":"string"}
  },
  "confidence":"LOW|MEDIUM|HIGH",
  "confidence_reasoning":"string",
  "unknown_key_factors":["buyer|pricing|regulatory|competitive_landscape|adoption_requirements"]
}`

const stage3PromptContext = `Estimate market size ranges for the defined product. Use ranges (low/high),
not single numbers. Be explicit about what you're measuring (revenue,
units, patients, etc.).

TAM (Total Addressable Market):
  The total annual revenue opportunity if every possible customer in every
  possible segment adopted this type of solution. This is the broadest
  reasonable definition of the market.

SAM (Serviceable Addressable Market):
  The subset of TAM that this specific product could realistically serve,
  given geographic, regulatory, channel, and technical constraints.

SOM (Serviceable Obtainable Market):
  The subset of SAM that a licensee could credibly capture within 5 years,
  given competitive dynamics, adoption friction, and market penetration
  rates for comparable technologies.

For each estimate, provide:
- The range (low and high, in USD annual revenue)
- The key assumptions driving the estimate
- The source type for each assumption: DISCLOSURE_DERIVED (stated in the
  disclosure), INFERRED (reasonable inference from disclosure), or
  ESTIMATED (general knowledge, not from disclosure)

If you cannot estimate a credible range because the disclosure lacks
sufficient information, set estimable to false and explain what's missing.

IMPORTANT: SOM is required for downstream economic analysis. If you truly
cannot estimate SOM, the pipeline will defer the disclosure for further
information gathering rather than proceed with unreliable numbers.`

const stage3SchemaPrompt = `Required JSON schema:
{
  "tam":{"low_usd":"int","high_usd":"int","unit":"string","assumptions":[{"assumption":"string","source":"DISCLOSURE_DERIVED|INFERRED|ESTIMATED"}],"estimable":"boolean","not_estimable_reason":"string|null"},
  "sam":{"low_usd":"int","high_usd":"int","unit":"string","assumptions":[{"assumption":"string","source":"DISCLOSURE_DERIVED|INFERRED|ESTIMATED"}],"estimable":"boolean","not_estimable_reason":"string|null"},
  "som":{"low_usd":"int","high_usd":"int","unit":"string","assumptions":[{"assumption":"string","source":"DISCLOSURE_DERIVED|INFERRED|ESTIMATED"}],"estimable":"boolean","not_estimable_reason":"string|null"},
  "tam_som_ratio_warning":"string|null"
}`

const stage4PromptContext = `Given the invention details and the domain default assumptions below,
propose adjustments where the disclosure provides evidence for deviation
from defaults. For each adjustment, explain why.

You must output assumption ranges, NOT computed financial results. The
financial computation is done by the system, not by you.

For each assumption, provide:
- Your adjusted range (low, high)
- Whether you adjusted from the default and why
- Source: DOMAIN_DEFAULT (unchanged from the prior provided),
  ADJUSTED (changed from default with reasoning),
  or DISCLOSURE_DERIVED (directly supported by disclosure text)

IMPORTANT: Every variable has a domain default provided below. If you
have no disclosure-specific reason to adjust, keep the domain default
and mark source as DOMAIN_DEFAULT. Do not invent adjustments.`

const stage4SchemaPrompt = `Required JSON schema:
{
  "royalty_rate_pct":{"low":"float","high":"float","source":"DOMAIN_DEFAULT|ADJUSTED|DISCLOSURE_DERIVED","reasoning":"string"},
  "p_license_3yr":{"low":"float","high":"float","source":"DOMAIN_DEFAULT|ADJUSTED|DISCLOSURE_DERIVED","reasoning":"string"},
  "p_commercial_success":{"low":"float","high":"float","source":"DOMAIN_DEFAULT|ADJUSTED|DISCLOSURE_DERIVED","reasoning":"string"},
  "time_to_license_months":{"low":"int","high":"int","source":"DOMAIN_DEFAULT|ADJUSTED|DISCLOSURE_DERIVED","reasoning":"string"},
  "time_from_license_to_revenue_months":{"low":"int","high":"int","source":"DOMAIN_DEFAULT|ADJUSTED|DISCLOSURE_DERIVED","reasoning":"string"},
  "annual_revenue_to_licensee_usd":{"low":"int","high":"int","source":"DOMAIN_DEFAULT|ADJUSTED|DISCLOSURE_DERIVED","reasoning":"string"},
  "license_duration_years":{"low":"int","high":"int","source":"DOMAIN_DEFAULT|ADJUSTED|DISCLOSURE_DERIVED","reasoning":"string"},
  "patent_cost_usd":{"low":"int","high":"int","source":"DOMAIN_DEFAULT|ADJUSTED|DISCLOSURE_DERIVED","reasoning":"string"}
}`

const stage5SchemaPrompt = `Required JSON schema:
{
  "executive_summary":"string",
  "key_drivers":["string (3-5)"],
  "diligence_questions":["string (3-5)"],
  "recommended_actions":["string (>=1)"],
  "non_patent_actions":["string"],
  "model_limitations":["string"]
}`

func (r *LLMStageRunner) RunStage0(ctx context.Context, req RequestEnvelope) (Stage0Output, StageAttemptMetrics, error) {
	out := Stage0Output{}
	prompt := fmt.Sprintf(
		"Stage 0: Structured Extraction.\nExtract the key commercial elements from the disclosure text. For fields where the disclosure does not provide sufficient information, return null and provide missing_reason. Do not fabricate information.\n\n%s\n\nInput disclosure text:\n%s",
		stage0SchemaPrompt,
		req.DisclosureText,
	)
	m, err := r.exec.Run(ctx, "stage_0", prompt, &out, func() error { return validateStage0(out) })
	return out, m, err
}

func (r *LLMStageRunner) RunStage1(ctx context.Context, s0 Stage0Output) (Stage1Output, StageAttemptMetrics, error) {
	out := Stage1Output{}
	prompt := fmt.Sprintf(
		"Stage 1: Commercialization Path Selection.\n%s\n\n%s\n\nStructured extraction:\n%s",
		stage1PromptContext,
		stage1SchemaPrompt,
		mustJSON(s0),
	)
	m, err := r.exec.Run(ctx, "stage_1", prompt, &out, func() error { return validateStage1(out) })
	return out, m, err
}

func (r *LLMStageRunner) RunStage2(ctx context.Context, s0 Stage0Output, s1 Stage1Output) (Stage2Output, StageAttemptMetrics, error) {
	out := Stage2Output{}
	prompt := fmt.Sprintf(
		"Stage 2: Triage Scorecard.\n%s\n\n%s\n\nStage 0 output:\n%s\n\nStage 1 output:\n%s",
		stage2PromptContext,
		stage2SchemaPrompt,
		mustJSON(s0),
		mustJSON(s1),
	)
	m, err := r.exec.Run(ctx, "stage_2", prompt, &out, func() error { return validateStage2(out) })
	if err != nil {
		return out, m, err
	}
	out.CompositeScore = (float64(out.Scores.MarketPain.Score+out.Scores.Differentiation.Score+out.Scores.AdoptionFriction.Score+out.Scores.DevelopmentBurden.Score+out.Scores.PartnerDensity.Score+out.Scores.IPLeverage.Score) / 6.0)
	out.WeightedScore = (float64(out.Scores.MarketPain.Score)*2.0 + float64(out.Scores.Differentiation.Score)*2.0 + float64(out.Scores.AdoptionFriction.Score)*1.5 + float64(out.Scores.DevelopmentBurden.Score)*1.0 + float64(out.Scores.PartnerDensity.Score)*1.5 + float64(out.Scores.IPLeverage.Score)*1.0) / 9.0
	return out, m, nil
}

func (r *LLMStageRunner) RunStage3(ctx context.Context, s0 Stage0Output, s1 Stage1Output, s2 Stage2Output) (Stage3Output, StageAttemptMetrics, error) {
	out := Stage3Output{}
	prompt := fmt.Sprintf(
		"Stage 3: Market Sizing (TAM/SAM/SOM).\n%s\n\n%s\n\nStage 0 output:\n%s\n\nStage 1 output:\n%s\n\nStage 2 output:\n%s",
		stage3PromptContext,
		stage3SchemaPrompt,
		mustJSON(s0),
		mustJSON(s1),
		mustJSON(s2),
	)
	m, err := r.exec.Run(ctx, "stage_3", prompt, &out, func() error { return validateStage3(out) })
	return out, m, err
}

func (r *LLMStageRunner) RunStage4(ctx context.Context, s0 Stage0Output, s1 Stage1Output, s2 Stage2Output, s3 Stage3Output) (Stage4Output, StageAttemptMetrics, error) {
	out := defaultsFromPriors(PriorForSector(s0.Sector))
	prompt := fmt.Sprintf(
		"Stage 4: Quick Economic Viability Assumptions.\n%s\n\n%s\n\nDomain priors for sector %q:\n%s\n\nStage 0 output:\n%s\n\nStage 1 output:\n%s\n\nStage 2 output:\n%s\n\nStage 3 output:\n%s",
		stage4PromptContext,
		stage4SchemaPrompt,
		s0.Sector,
		mustJSON(PriorForSector(s0.Sector)),
		mustJSON(s0),
		mustJSON(s1),
		mustJSON(s2),
		mustJSON(s3),
	)
	m, err := r.exec.Run(ctx, "stage_4", prompt, &out, func() error { return validateStage4(out) })
	return out, m, err
}

func (r *LLMStageRunner) RunStage5(ctx context.Context, in Stage5Input) (Stage5Output, StageAttemptMetrics, error) {
	out := Stage5Output{}
	prompt := fmt.Sprintf(
		"Stage 5: Recommendation & Diligence Questions.\nWrite a narrative recommendation anchored to the computed outputs. Do not contradict the computed recommendation tier.\n\n%s\n\nComputed decision:\n%s\n\nStage 0 output:\n%s\n\nStage 1 output:\n%s\n\nStage 2 output:\n%s\n\nStage 3 output:\n%s\n\nStage 4 assumptions:\n%s\n\nStage 4 computed outputs:\n%s",
		stage5SchemaPrompt,
		mustJSON(in.Decision),
		mustJSON(in.Stage0),
		mustJSON(in.Stage1),
		mustJSON(in.Stage2),
		mustJSON(in.Stage3),
		mustJSON(in.Stage4),
		mustJSON(in.Stage4Computed),
	)
	m, err := r.exec.Run(ctx, "stage_5", prompt, &out, func() error { return validateStage5(out) })
	return out, m, err
}

func validateStage0(s Stage0Output) error {
	if err := validateRequiredField(s.InventionTitle); err != nil {
		return fmt.Errorf("invention_title: %w", err)
	}
	if err := validateRequiredField(s.ProblemSolved); err != nil {
		return fmt.Errorf("problem_solved: %w", err)
	}
	if err := validateRequiredField(s.SolutionDescription); err != nil {
		return fmt.Errorf("solution_description: %w", err)
	}
	optional := []NullableField{s.ClaimedAdvantages, s.TargetUser, s.TargetBuyer, s.ApplicationDomains, s.CompetingApproaches, s.Dependencies}
	for i := range optional {
		if err := validateOptionalField(optional[i]); err != nil {
			return err
		}
	}
	switch s.EvidenceLevel {
	case EvidenceConceptOnly, EvidenceInVitro, EvidenceAnimal, EvidencePrototype, EvidencePilot, EvidenceClinical:
	default:
		return fmt.Errorf("invalid evidence_level")
	}
	if _, ok := DefaultPriors[s.Sector]; !ok {
		return fmt.Errorf("sector must map to priors")
	}
	return nil
}

func validateRequiredField(f NullableField) error {
	if !validConfidence(f.Confidence) {
		return fmt.Errorf("invalid confidence")
	}
	if strings.TrimSpace(asString(f.Value)) == "" {
		return fmt.Errorf("missing value")
	}
	if f.MissingReason != nil {
		return fmt.Errorf("missing_reason must be null for required fields")
	}
	return nil
}

func validateOptionalField(f NullableField) error {
	if !validConfidence(f.Confidence) {
		return fmt.Errorf("invalid confidence")
	}
	if isNilLike(f.Value) {
		if f.MissingReason == nil || strings.TrimSpace(*f.MissingReason) == "" {
			return fmt.Errorf("missing_reason required when value is null")
		}
		return nil
	}
	if f.MissingReason != nil && strings.TrimSpace(*f.MissingReason) != "" {
		return fmt.Errorf("missing_reason must be empty/null when value provided")
	}
	return nil
}

func validateStage1(s Stage1Output) error {
	if !validPath(s.PrimaryPath) {
		return fmt.Errorf("invalid primary_path")
	}
	if strings.TrimSpace(s.PrimaryPathReasoning) == "" {
		return fmt.Errorf("primary_path_reasoning required")
	}
	if strings.TrimSpace(s.ProductDefinition) == "" {
		return fmt.Errorf("product_definition required")
	}
	if s.SecondaryPath != nil {
		if !validPath(*s.SecondaryPath) {
			return fmt.Errorf("invalid secondary_path")
		}
		if *s.SecondaryPath == s.PrimaryPath {
			return fmt.Errorf("secondary_path must differ from primary_path")
		}
		if s.SecondaryPathReasoning == nil || strings.TrimSpace(*s.SecondaryPathReasoning) == "" {
			return fmt.Errorf("secondary_path_reasoning required")
		}
	}
	if !s.HasPlausibleMonetization && (s.NoMonetizationReasoning == nil || strings.TrimSpace(*s.NoMonetizationReasoning) == "") {
		return fmt.Errorf("no_monetization_reasoning required")
	}
	if s.NonPatentMonetization && (s.NonPatentMonetizationReasoning == nil || strings.TrimSpace(*s.NonPatentMonetizationReasoning) == "") {
		return fmt.Errorf("non_patent_monetization_reasoning required")
	}
	return nil
}

func validateStage2(s Stage2Output) error {
	ss := []ScoreReason{s.Scores.MarketPain, s.Scores.Differentiation, s.Scores.AdoptionFriction, s.Scores.DevelopmentBurden, s.Scores.PartnerDensity, s.Scores.IPLeverage}
	for _, sc := range ss {
		if sc.Score < 1 || sc.Score > 5 {
			return fmt.Errorf("score out of range")
		}
		if strings.TrimSpace(sc.Reasoning) == "" {
			return fmt.Errorf("score reasoning required")
		}
	}
	if !validConfidence(s.Confidence) {
		return fmt.Errorf("invalid confidence")
	}
	return nil
}

func validateStage3(s Stage3Output) error {
	if err := validateMarketRange(s.TAM); err != nil {
		return fmt.Errorf("tam: %w", err)
	}
	if err := validateMarketRange(s.SAM); err != nil {
		return fmt.Errorf("sam: %w", err)
	}
	if err := validateMarketRange(s.SOM); err != nil {
		return fmt.Errorf("som: %w", err)
	}
	if s.SAM.LowUSD > s.TAM.LowUSD || s.SAM.HighUSD > s.TAM.HighUSD {
		return fmt.Errorf("sam must be <= tam")
	}
	if s.SOM.LowUSD > s.SAM.LowUSD || s.SOM.HighUSD > s.SAM.HighUSD {
		return fmt.Errorf("som must be <= sam")
	}
	return nil
}

func validateMarketRange(m MarketRange) error {
	if strings.TrimSpace(m.Unit) == "" {
		return fmt.Errorf("unit required")
	}
	if len(m.Assumptions) < 1 {
		return fmt.Errorf("assumptions required")
	}
	for _, a := range m.Assumptions {
		if strings.TrimSpace(a.Assumption) == "" {
			return fmt.Errorf("assumption text required")
		}
		switch a.Source {
		case SourceDisclosure, SourceInferred, SourceEstimated:
		default:
			return fmt.Errorf("invalid assumption source")
		}
	}
	if m.Estimable {
		if m.LowUSD <= 0 || m.HighUSD <= 0 || m.LowUSD > m.HighUSD {
			return fmt.Errorf("invalid positive low/high")
		}
		if m.NotEstimableReason != nil {
			return fmt.Errorf("not_estimable_reason must be null")
		}
	} else {
		if m.NotEstimableReason == nil || strings.TrimSpace(*m.NotEstimableReason) == "" {
			return fmt.Errorf("not_estimable_reason required")
		}
		if m.LowUSD != 0 || m.HighUSD != 0 {
			return fmt.Errorf("non-estimable range must be zero")
		}
	}
	return nil
}

func validateStage4(s Stage4Output) error {
	if err := validateFloatRange(s.RoyaltyRatePct, 0, 25); err != nil {
		return fmt.Errorf("royalty_rate_pct: %w", err)
	}
	if err := validateFloatRange(s.PLicense3yr, 0, 1); err != nil {
		return fmt.Errorf("p_license_3yr: %w", err)
	}
	if err := validateFloatRange(s.PCommercialSuccess, 0, 1); err != nil {
		return fmt.Errorf("p_commercial_success: %w", err)
	}
	if err := validateIntRange(s.TimeToLicenseMonths, 1); err != nil {
		return fmt.Errorf("time_to_license_months: %w", err)
	}
	if err := validateIntRange(s.TimeFromLicenseToRevenueMonths, 1); err != nil {
		return fmt.Errorf("time_from_license_to_revenue_months: %w", err)
	}
	if err := validateIntRange(s.AnnualRevenueToLicenseeUSD, 0); err != nil {
		return fmt.Errorf("annual_revenue_to_licensee_usd: %w", err)
	}
	if err := validateIntRange(s.LicenseDurationYears, 1); err != nil {
		return fmt.Errorf("license_duration_years: %w", err)
	}
	if err := validateIntRange(s.PatentCostUSD, 0); err != nil {
		return fmt.Errorf("patent_cost_usd: %w", err)
	}
	if midpointInt(s.TimeToLicenseMonths.Low, s.TimeToLicenseMonths.High)+midpointInt(s.TimeFromLicenseToRevenueMonths.Low, s.TimeFromLicenseToRevenueMonths.High) <= 0 {
		return fmt.Errorf("combined time to revenue must be > 0")
	}
	return nil
}

func validateFloatRange(v AssumptionRangeFloat, lo, hi float64) error {
	if v.Low > v.High {
		return fmt.Errorf("low must be <= high")
	}
	if v.Low < lo || v.High > hi {
		return fmt.Errorf("range out of bounds")
	}
	if !validStage4Source(v.Source) {
		return fmt.Errorf("invalid source")
	}
	if strings.TrimSpace(v.Reasoning) == "" {
		return fmt.Errorf("reasoning required")
	}
	return nil
}

func validateIntRange(v AssumptionRangeInt, min int) error {
	if v.Low > v.High {
		return fmt.Errorf("low must be <= high")
	}
	if v.Low < min || v.High < min {
		return fmt.Errorf("out of bounds")
	}
	if !validStage4Source(v.Source) {
		return fmt.Errorf("invalid source")
	}
	if strings.TrimSpace(v.Reasoning) == "" {
		return fmt.Errorf("reasoning required")
	}
	return nil
}

func validateStage5(s Stage5Output) error {
	if strings.TrimSpace(s.ExecutiveSummary) == "" {
		return fmt.Errorf("executive_summary required")
	}
	if len(s.KeyDrivers) < 3 || len(s.KeyDrivers) > 5 {
		return fmt.Errorf("key_drivers must have 3-5 entries")
	}
	if len(s.DiligenceQuestions) < 3 || len(s.DiligenceQuestions) > 5 {
		return fmt.Errorf("diligence_questions must have 3-5 entries")
	}
	if len(s.RecommendedActions) < 1 {
		return fmt.Errorf("recommended_actions must be non-empty")
	}
	return nil
}

func defaultsFromPriors(p DomainPriors) Stage4Output {
	return Stage4Output{
		RoyaltyRatePct:                 AssumptionRangeFloat{Low: p.TypicalRoyaltyRangePct[0], High: p.TypicalRoyaltyRangePct[1], Source: SourceDomainDefault, Reasoning: "domain default"},
		PLicense3yr:                    AssumptionRangeFloat{Low: p.PLicense3yr[0], High: p.PLicense3yr[1], Source: SourceDomainDefault, Reasoning: "domain default"},
		PCommercialSuccess:             AssumptionRangeFloat{Low: p.PCommercialSuccess[0], High: p.PCommercialSuccess[1], Source: SourceDomainDefault, Reasoning: "domain default"},
		TimeToLicenseMonths:            AssumptionRangeInt{Low: p.TimeToLicenseMonths[0], High: p.TimeToLicenseMonths[1], Source: SourceDomainDefault, Reasoning: "domain default"},
		TimeFromLicenseToRevenueMonths: AssumptionRangeInt{Low: p.TimeFromLicenseToRevMonths[0], High: p.TimeFromLicenseToRevMonths[1], Source: SourceDomainDefault, Reasoning: "domain default"},
		AnnualRevenueToLicenseeUSD:     AssumptionRangeInt{Low: p.AnnualRevToLicenseeUSD[0], High: p.AnnualRevToLicenseeUSD[1], Source: SourceDomainDefault, Reasoning: "domain default"},
		LicenseDurationYears:           AssumptionRangeInt{Low: p.LicenseDurationYears[0], High: p.LicenseDurationYears[1], Source: SourceDomainDefault, Reasoning: "domain default"},
		PatentCostUSD:                  AssumptionRangeInt{Low: p.PatentCostRangeUSD[0], High: p.PatentCostRangeUSD[1], Source: SourceDomainDefault, Reasoning: "domain default"},
	}
}

func validPath(v CommercializationPath) bool {
	switch v {
	case PathExclusiveLicense, PathStartup, PathNonExclusive, PathOpenSourceServices, PathResearchUseOnly:
		return true
	default:
		return false
	}
}

func validConfidence(v ConfidenceLevel) bool {
	return v == ConfidenceLow || v == ConfidenceMedium || v == ConfidenceHigh
}

func validStage4Source(v SourceType) bool {
	return v == SourceDomainDefault || v == SourceAdjusted || v == SourceDisclosure
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func isNilLike(v any) bool {
	if v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case []string:
		return len(t) == 0
	case []any:
		return len(t) == 0
	default:
		return false
	}
}

func mustJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}
