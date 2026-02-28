package priorartsearch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type StageProgressFn func(stage, message string)

type SearchRunner interface {
	Run(ctx context.Context, stage1 Stage1Output) (Stage2Output, error)
}

type Pipeline struct {
	runner   StageRunner
	searcher SearchRunner
}

func NewPipeline(runner StageRunner, searcher SearchRunner) *Pipeline {
	return &Pipeline{runner: runner, searcher: searcher}
}

func (p *Pipeline) Run(ctx context.Context, req RequestEnvelope) (PipelineResult, error) {
	return p.runWithProgress(ctx, req, nil)
}

func (p *Pipeline) RunWithProgress(ctx context.Context, req RequestEnvelope, progress StageProgressFn) (PipelineResult, error) {
	return p.runWithProgress(ctx, req, progress)
}

func (p *Pipeline) runWithProgress(ctx context.Context, req RequestEnvelope, progress StageProgressFn) (PipelineResult, error) {
	res := PipelineResult{
		Request:  req,
		Attempts: map[string]StageAttemptMetrics{},
		Metadata: PipelineMetadata{StartedAt: time.Now(), Model: p.modelName(), Temperature: 0.0},
	}
	if strings.TrimSpace(req.CaseID) == "" {
		return res, errors.New("case_id is required")
	}
	if len(strings.TrimSpace(req.DisclosureText)) < MinDisclosureChars {
		return res, errors.New("disclosure_text is insufficient for analysis")
	}
	if len(req.DisclosureText) > MaxDisclosureChars {
		req.DisclosureText = req.DisclosureText[:MaxDisclosureChars]
		res.Metadata.InputTruncated = true
	}
	res.Request = req

	emit(progress, "stage_1", "Stage 1: Building search strategy...")
	s1, m1, err := p.runner.RunStage1(ctx, req)
	if err != nil {
		return res, &StageError{Stage: "stage_1", Err: err}
	}
	res.Stage1 = s1
	res.Attempts["stage_1"] = m1
	res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_1")

	emit(progress, "stage_2", "Stage 2: Executing PatentsView search...")
	s2, err := p.searcher.Run(ctx, s1)
	if err != nil {
		return res, &StageError{Stage: "stage_2", Err: err}
	}
	res.Stage2 = s2
	res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_2")

	emit(progress, "stage_3", "Stage 3: Assessing relevance...")
	s3, s3Meta, m3, err := p.runner.RunStage3(ctx, s1, s2)
	res.Attempts["stage_3"] = m3
	if err != nil {
		reason := "Stage 3 failed. Report generated in degraded mode with raw results only."
		res.Metadata.Degraded = true
		res.Metadata.DegradedReason = &reason
		res.Metadata.StagesFailed = append(res.Metadata.StagesFailed, "stage_3")
		res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_3")
		res.Stage3 = nil
		res.Stage4 = nil
		res.Determination = fallbackDetermination(nil, len(s2.Patents))
		stats := computeLandscapeStats(s1, s2, Stage3Output{})
		res.AssigneeFrequency = stats.AssigneeFrequency
		res.CPCHistogram = stats.CPCHistogram
		res.NovelCoverage = stats.NovelCoverage
		finalizeMetadata(&res, s3Meta)
		return res, nil
	}
	res.Stage3 = &s3
	res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_3")

	emit(progress, "stage_4", "Stage 4: Building landscape analysis...")
	s4, m4, err := p.runner.RunStage4(ctx, s1, s2, s3)
	res.Attempts["stage_4"] = m4
	stats := computeLandscapeStats(s1, s2, s3)
	res.AssigneeFrequency = stats.AssigneeFrequency
	res.CPCHistogram = stats.CPCHistogram
	res.NovelCoverage = stats.NovelCoverage
	if err != nil {
		reason := "Stage 4 failed. Report generated in degraded mode with code-generated landscape statistics."
		res.Metadata.Degraded = true
		res.Metadata.DegradedReason = &reason
		res.Metadata.StagesFailed = append(res.Metadata.StagesFailed, "stage_4")
		res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_4")
		res.Stage4 = nil
		res.Determination = fallbackDetermination(s3.Assessments, len(s2.Patents))
		finalizeMetadata(&res, s3Meta)
		return res, nil
	}
	res.Stage4 = &s4
	res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_4")
	res.Determination = s4.Determination
	if s4.BlockingRisk.Level == BlockingRiskHigh {
		res.Determination = DeterminationBlockingArt
	}
	finalizeMetadata(&res, s3Meta)
	return res, nil
}

func (p *Pipeline) modelName() string {
	if llmRunner, ok := p.runner.(*LLMStageRunner); ok && llmRunner.exec != nil {
		return llmRunner.exec.ModelName()
	}
	return DefaultLLMModel
}

func fallbackDetermination(assessments []PatentAssessment, totalRetrieved int) Determination {
	high := 0
	medium := 0
	for _, a := range assessments {
		if a.Relevance == RelevanceHigh {
			high++
		}
		if a.Relevance == RelevanceMedium {
			medium++
		}
	}
	switch {
	case high > 0:
		return DeterminationBlockingArt
	case medium >= 5:
		return DeterminationCrowdedField
	case totalRetrieved == 0:
		return DeterminationInconclusive
	default:
		return DeterminationClearField
	}
}

func finalizeMetadata(res *PipelineResult, s3 Stage3RunMetadata) {
	res.Metadata.AbstractsMissing = s3.AbstractsMissing
	res.Metadata.AssessmentTruncated = s3.AssessmentTruncated
	res.Metadata.AssessedNone = s3.AssessedNone
	res.Metadata.TotalPatentsRetrieved = len(res.Stage2.Patents)
	if res.Stage3 != nil {
		res.Metadata.TotalPatentsAssessed = len(res.Stage3.Assessments)
	}
	res.Metadata.APICallsMade = res.Stage2.TotalAPICalls
	res.Metadata.CompletedAt = time.Now()
	res.Metadata.DurationMS = res.Metadata.CompletedAt.Sub(res.Metadata.StartedAt).Milliseconds()
}

func emit(progress StageProgressFn, stage, message string) {
	if progress != nil {
		progress(stage, message)
	}
}

func StageNameFromError(err error) string {
	var se *StageError
	if errors.As(err, &se) {
		return se.Stage
	}
	return "pipeline"
}

func BuildResponse(result PipelineResult) ResponseEnvelope {
	env := ResponseEnvelope{
		CaseID:        result.Request.CaseID,
		Agent:         "prior-art-search",
		Version:       AgentVersion,
		Determination: result.Determination,
		Metadata:      result.Metadata,
		StructuredResults: StructuredResults{
			SearchStrategy: result.Stage1,
			PatentsFound:   result.Stage2,
			Landscape:      result.Stage4,
		},
	}
	if result.Stage3 != nil {
		env.StructuredResults.Assessments = result.Stage3.Assessments
	}
	env.ReportMarkdown = BuildReportMarkdown(result)
	return env
}

func (p *Pipeline) ValidateConfig() error {
	if p.searcher == nil {
		return fmt.Errorf("searcher is required")
	}
	if p.runner == nil {
		return fmt.Errorf("runner is required")
	}
	return nil
}
