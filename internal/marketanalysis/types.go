package marketanalysis

import "time"

const Disclaimer = "This is a preliminary automated market assessment, not a valuation or investment recommendation. " +
	"Estimates are based on limited disclosure information and domain default assumptions."

const (
	CapabilityMarketAnalysis = "market-analysis-pipeline"
	MaxDisclosureChars       = 100000
	MinDisclosureChars       = 100
)

type RecommendationTier string

const (
	RecommendationGO    RecommendationTier = "GO"
	RecommendationDefer RecommendationTier = "DEFER"
	RecommendationNoGo  RecommendationTier = "NO_GO"
)

type ConfidenceLevel string

const (
	ConfidenceLow    ConfidenceLevel = "LOW"
	ConfidenceMedium ConfidenceLevel = "MEDIUM"
	ConfidenceHigh   ConfidenceLevel = "HIGH"
)

type ReportMode string

const (
	ReportModeComplete ReportMode = "COMPLETE"
	ReportModeDegraded ReportMode = "DEGRADED"
)

type SourceType string

const (
	SourceDomainDefault SourceType = "DOMAIN_DEFAULT"
	SourceAdjusted      SourceType = "ADJUSTED"
	SourceDisclosure    SourceType = "DISCLOSURE_DERIVED"
	SourceInferred      SourceType = "INFERRED"
	SourceEstimated     SourceType = "ESTIMATED"
)

type CommercializationPath string

const (
	PathExclusiveLicense   CommercializationPath = "EXCLUSIVE_LICENSE_INCUMBENT"
	PathStartup            CommercializationPath = "STARTUP_FORMATION"
	PathNonExclusive       CommercializationPath = "NON_EXCLUSIVE_LICENSE"
	PathOpenSourceServices CommercializationPath = "OPEN_SOURCE_PLUS_SERVICES"
	PathResearchUseOnly    CommercializationPath = "RESEARCH_USE_ONLY"
)

type EvidenceLevel string

const (
	EvidenceConceptOnly EvidenceLevel = "CONCEPT_ONLY"
	EvidenceInVitro     EvidenceLevel = "IN_VITRO"
	EvidenceAnimal      EvidenceLevel = "ANIMAL"
	EvidencePrototype   EvidenceLevel = "PROTOTYPE"
	EvidencePilot       EvidenceLevel = "PILOT"
	EvidenceClinical    EvidenceLevel = "CLINICAL"
)

type RequestMetadata struct {
	SourceFilename   string `json:"source_filename,omitempty"`
	ExtractionMethod string `json:"extraction_method,omitempty"`
	Truncated        bool   `json:"truncated,omitempty"`
}

type RequestEnvelope struct {
	CaseID         string          `json:"case_id"`
	DisclosureText string          `json:"disclosure_text"`
	Metadata       RequestMetadata `json:"metadata,omitempty"`
}

type PipelineMetadata struct {
	StagesExecuted      []string       `json:"stages_executed"`
	StagesSkipped       []string       `json:"stages_skipped"`
	StageFailed         string         `json:"stage_failed,omitempty"`
	StartedAt           time.Time      `json:"started_at"`
	CompletedAt         time.Time      `json:"completed_at"`
	InputTruncated      bool           `json:"input_truncated"`
	Mode                ReportMode     `json:"mode"`
	EarlyExitReason     string         `json:"early_exit_reason,omitempty"`
	TotalLLMCalls       int            `json:"total_llm_calls"`
	TotalRetries        int            `json:"total_retries"`
	StageAttempts       map[string]int `json:"stage_attempts,omitempty"`
	StageContentRetries map[string]int `json:"stage_content_retries,omitempty"`
}

type ResponseEnvelope struct {
	CaseID                   string             `json:"case_id"`
	Recommendation           RecommendationTier `json:"recommendation"`
	RecommendationConfidence ConfidenceLevel    `json:"recommendation_confidence"`
	ReportMode               ReportMode         `json:"report_mode"`
	ReportMarkdown           string             `json:"report_markdown"`
	StageOutputs             map[string]any     `json:"stage_outputs"`
	PipelineMetadata         PipelineMetadata   `json:"pipeline_metadata"`
	Disclaimer               string             `json:"disclaimer"`
}

type StageAttemptMetrics struct {
	Attempts       int
	ContentRetries int
}

type NullableField struct {
	Value         any             `json:"value"`
	Confidence    ConfidenceLevel `json:"confidence"`
	MissingReason *string         `json:"missing_reason"`
}

type Stage0Output struct {
	InventionTitle      NullableField `json:"invention_title"`
	ProblemSolved       NullableField `json:"problem_solved"`
	SolutionDescription NullableField `json:"solution_description"`
	ClaimedAdvantages   NullableField `json:"claimed_advantages"`
	TargetUser          NullableField `json:"target_user"`
	TargetBuyer         NullableField `json:"target_buyer"`
	ApplicationDomains  NullableField `json:"application_domains"`
	EvidenceLevel       EvidenceLevel `json:"evidence_level"`
	CompetingApproaches NullableField `json:"competing_approaches"`
	Dependencies        NullableField `json:"dependencies"`
	Sector              string        `json:"sector"`
}

type Stage1Output struct {
	PrimaryPath                    CommercializationPath  `json:"primary_path"`
	PrimaryPathReasoning           string                 `json:"primary_path_reasoning"`
	SecondaryPath                  *CommercializationPath `json:"secondary_path"`
	SecondaryPathReasoning         *string                `json:"secondary_path_reasoning"`
	ProductDefinition              string                 `json:"product_definition"`
	HasPlausibleMonetization       bool                   `json:"has_plausible_monetization"`
	NoMonetizationReasoning        *string                `json:"no_monetization_reasoning"`
	NonPatentMonetization          bool                   `json:"non_patent_monetization"`
	NonPatentMonetizationReasoning *string                `json:"non_patent_monetization_reasoning"`
}

type ScoreReason struct {
	Score     int    `json:"score"`
	Reasoning string `json:"reasoning"`
}

type Stage2Scores struct {
	MarketPain        ScoreReason `json:"market_pain"`
	Differentiation   ScoreReason `json:"differentiation"`
	AdoptionFriction  ScoreReason `json:"adoption_friction"`
	DevelopmentBurden ScoreReason `json:"development_burden"`
	PartnerDensity    ScoreReason `json:"partner_density"`
	IPLeverage        ScoreReason `json:"ip_leverage"`
}

type Stage2Output struct {
	Scores              Stage2Scores    `json:"scores"`
	Confidence          ConfidenceLevel `json:"confidence"`
	ConfidenceReasoning string          `json:"confidence_reasoning"`
	UnknownKeyFactors   []string        `json:"unknown_key_factors"`
	CompositeScore      float64         `json:"composite_score"`
	WeightedScore       float64         `json:"weighted_score"`
}

type Stage3Assumption struct {
	Assumption string     `json:"assumption"`
	Source     SourceType `json:"source"`
}

type MarketRange struct {
	LowUSD             int64              `json:"low_usd"`
	HighUSD            int64              `json:"high_usd"`
	Unit               string             `json:"unit"`
	Assumptions        []Stage3Assumption `json:"assumptions"`
	Estimable          bool               `json:"estimable"`
	NotEstimableReason *string            `json:"not_estimable_reason"`
}

type Stage3Output struct {
	TAM                MarketRange `json:"tam"`
	SAM                MarketRange `json:"sam"`
	SOM                MarketRange `json:"som"`
	TAMSOMRatioWarning *string     `json:"tam_som_ratio_warning"`
}

type AssumptionRangeFloat struct {
	Low       float64    `json:"low"`
	High      float64    `json:"high"`
	Source    SourceType `json:"source"`
	Reasoning string     `json:"reasoning"`
}

type AssumptionRangeInt struct {
	Low       int        `json:"low"`
	High      int        `json:"high"`
	Source    SourceType `json:"source"`
	Reasoning string     `json:"reasoning"`
}

type Stage4Output struct {
	RoyaltyRatePct                 AssumptionRangeFloat `json:"royalty_rate_pct"`
	PLicense3yr                    AssumptionRangeFloat `json:"p_license_3yr"`
	PCommercialSuccess             AssumptionRangeFloat `json:"p_commercial_success"`
	TimeToLicenseMonths            AssumptionRangeInt   `json:"time_to_license_months"`
	TimeFromLicenseToRevenueMonths AssumptionRangeInt   `json:"time_from_license_to_revenue_months"`
	AnnualRevenueToLicenseeUSD     AssumptionRangeInt   `json:"annual_revenue_to_licensee_usd"`
	LicenseDurationYears           AssumptionRangeInt   `json:"license_duration_years"`
	PatentCostUSD                  AssumptionRangeInt   `json:"patent_cost_usd"`
}

type ScenarioOutput struct {
	NPVUSD            float64 `json:"npv_usd"`
	ExceedsPatentCost bool    `json:"exceeds_patent_cost"`
}

type SensitivityDriver struct {
	Assumption  string  `json:"assumption"`
	NPVDeltaUSD float64 `json:"npv_delta_usd"`
	Direction   string  `json:"direction"`
}

type Stage4ComputedOutput struct {
	Scenarios           map[string]ScenarioOutput `json:"scenarios"`
	PatentCostMidUSD    float64                   `json:"patent_cost_mid_usd"`
	SensitivityDrivers  []SensitivityDriver       `json:"sensitivity_drivers"`
	PathModelLimitation *string                   `json:"path_model_limitation"`
	RevenueRampYears    int                       `json:"revenue_ramp_years"`
}

type RecommendationDecision struct {
	Tier       RecommendationTier `json:"tier"`
	Confidence ConfidenceLevel    `json:"confidence"`
	Reason     string             `json:"reason"`
	Caveats    []string           `json:"caveats"`
}

type Stage5Output struct {
	ExecutiveSummary   string   `json:"executive_summary"`
	KeyDrivers         []string `json:"key_drivers"`
	DiligenceQuestions []string `json:"diligence_questions"`
	RecommendedActions []string `json:"recommended_actions"`
	NonPatentActions   []string `json:"non_patent_actions"`
	ModelLimitations   []string `json:"model_limitations"`
}

type PipelineResult struct {
	Request        RequestEnvelope
	Stage0         Stage0Output
	Stage1         *Stage1Output
	Stage2         *Stage2Output
	Stage3         *Stage3Output
	Stage4         *Stage4Output
	Stage4Computed *Stage4ComputedOutput
	Stage5         *Stage5Output
	Decision       RecommendationDecision
	Attempts       map[string]StageAttemptMetrics
	Metadata       PipelineMetadata
}
