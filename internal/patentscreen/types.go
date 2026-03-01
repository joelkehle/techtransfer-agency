package patentscreen

import "time"

const Disclaimer = "This is a preliminary automated screen, not a legal opinion. " +
	"It is not intended for patent filing, prosecution, or as legal advice. " +
	"Consult qualified patent counsel for formal evaluation."

const (
	CapabilityPatentEligibilityScreen = "patent-eligibility-screen"
	MaxDisclosureChars                = 100000
	MinDisclosureChars                = 100
	NeedsReviewConfidenceThreshold    = 0.65
)

type Determination string

const (
	DeterminationLikelyEligible     Determination = "LIKELY_ELIGIBLE"
	DeterminationLikelyNotEligible  Determination = "LIKELY_NOT_ELIGIBLE"
	DeterminationNeedsFurtherReview Determination = "NEEDS_FURTHER_REVIEW"
)

type Pathway string

const (
	PathwayA  Pathway = "A — not a statutory category"
	PathwayB1 Pathway = "B1 — no judicial exception"
	PathwayB2 Pathway = "B2 — judicial exception integrated into practical application"
	PathwayC  Pathway = "C — inventive concept provides significantly more"
	PathwayD  Pathway = "D — no inventive concept beyond judicial exception"
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
	StagesExecuted         []string       `json:"stages_executed"`
	StagesSkipped          []string       `json:"stages_skipped"`
	TotalLLMCalls          int            `json:"total_llm_calls"`
	TotalRetries           int            `json:"total_retries"`
	StageAttempts          map[string]int `json:"stage_attempts,omitempty"`
	StageContentRetries    map[string]int `json:"stage_content_retries,omitempty"`
	Stage5BooleanAgreement *bool          `json:"stage_5_boolean_agreement,omitempty"`
	DecisionTrace          map[string]any `json:"decision_trace,omitempty"`
	StartedAt              time.Time      `json:"started_at"`
	CompletedAt            time.Time      `json:"completed_at"`
	InputTruncated         bool           `json:"input_truncated"`
	NeedsReviewReasons     []string       `json:"needs_review_reasons"`
}

type ResponseEnvelope struct {
	CaseID           string           `json:"case_id"`
	Determination    Determination    `json:"determination"`
	Pathway          string           `json:"pathway"`
	ReportMarkdown   string           `json:"report_markdown"`
	StageOutputs     map[string]any   `json:"stage_outputs"`
	PipelineMetadata PipelineMetadata `json:"pipeline_metadata"`
	Disclaimer       string           `json:"disclaimer"`
}

type StageConfidence struct {
	ConfidenceScore         float64 `json:"confidence_score"`
	ConfidenceReason        string  `json:"confidence_reason"`
	InsufficientInformation bool    `json:"insufficient_information"`
}

type StageAttemptMetrics struct {
	Attempts       int
	ContentRetries int
}

type Stage1Output struct {
	InventionTitle       string   `json:"invention_title"`
	Abstract             string   `json:"abstract"`
	ProblemSolved        string   `json:"problem_solved"`
	InventionDescription string   `json:"invention_description"`
	NovelElements        []string `json:"novel_elements"`
	TechnologyArea       string   `json:"technology_area"`
	ClaimsPresent        bool     `json:"claims_present"`
	ClaimsSummary        *string  `json:"claims_summary"`
	StageConfidence
}

type Stage2Category string

const (
	CategoryProcess             Stage2Category = "PROCESS"
	CategoryMachine             Stage2Category = "MACHINE"
	CategoryManufacture         Stage2Category = "MANUFACTURE"
	CategoryCompositionOfMatter Stage2Category = "COMPOSITION_OF_MATTER"
)

type Stage2Output struct {
	Categories  []Stage2Category `json:"categories"`
	Explanation string           `json:"explanation"`
	PassesStep1 bool             `json:"passes_step_1"`
	StageConfidence
}

type ExceptionType string

const (
	ExceptionAbstractIdea      ExceptionType = "ABSTRACT_IDEA"
	ExceptionLawOfNature       ExceptionType = "LAW_OF_NATURE"
	ExceptionNaturalPhenomenon ExceptionType = "NATURAL_PHENOMENON"
)

type AbstractIdeaSubcategory string

const (
	SubcategoryMathematicalConcept     AbstractIdeaSubcategory = "MATHEMATICAL_CONCEPT"
	SubcategoryOrganizingHumanActivity AbstractIdeaSubcategory = "ORGANIZING_HUMAN_ACTIVITY"
	SubcategoryMentalProcess           AbstractIdeaSubcategory = "MENTAL_PROCESS"
)

type Stage3Output struct {
	RecitesException        bool                     `json:"recites_exception"`
	ExceptionType           *ExceptionType           `json:"exception_type"`
	AbstractIdeaSubcategory *AbstractIdeaSubcategory `json:"abstract_idea_subcategory"`
	Reasoning               string                   `json:"reasoning"`
	MPEPReference           string                   `json:"mpep_reference"`
	StageConfidence
}

type Stage4Output struct {
	AdditionalElements             []string `json:"additional_elements"`
	IntegratesPracticalApplication bool     `json:"integrates_practical_application"`
	ConsiderationsFor              []string `json:"considerations_for"`
	ConsiderationsAgainst          []string `json:"considerations_against"`
	Reasoning                      string   `json:"reasoning"`
	MPEPReference                  string   `json:"mpep_reference"`
	StageConfidence
}

type Stage5Output struct {
	HasInventiveConcept      bool   `json:"has_inventive_concept"`
	Reasoning                string `json:"reasoning"`
	BerkheimerConsiderations string `json:"berkheimer_considerations"`
	MPEPReference            string `json:"mpep_reference"`
	StageConfidence
}

type PriorArtSearchPriority string

const (
	PriorityHigh   PriorArtSearchPriority = "HIGH"
	PriorityMedium PriorArtSearchPriority = "MEDIUM"
	PriorityLow    PriorArtSearchPriority = "LOW"
)

type Stage6Output struct {
	NoveltyConcerns        []string               `json:"novelty_concerns"`
	NonObviousnessConcerns []string               `json:"non_obviousness_concerns"`
	PriorArtSearchPriority PriorArtSearchPriority `json:"prior_art_search_priority"`
	Reasoning              string                 `json:"reasoning"`
	StageConfidence
}

type PipelineResult struct {
	BaseDetermination  Determination
	FinalDetermination Determination
	Pathway            Pathway
	Request            RequestEnvelope
	Stage1             Stage1Output
	Stage2             *Stage2Output
	Stage3             *Stage3Output
	Stage4             *Stage4Output
	Stage5             *Stage5Output
	Stage6             Stage6Output
	Attempts           map[string]StageAttemptMetrics
	Metadata           PipelineMetadata
}

type LLMStageInput struct {
	Name       string
	PromptBody string
}
