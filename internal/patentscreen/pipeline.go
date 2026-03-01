package patentscreen

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type StageError struct {
	Stage string
	Err   error
}

func (e *StageError) Error() string {
	return fmt.Sprintf("%s: %v", e.Stage, e.Err)
}

func (e *StageError) Unwrap() error { return e.Err }

type StageProgressFn func(stage, message string)

type Pipeline struct {
	runner StageRunner
}

func NewPipeline(runner StageRunner) *Pipeline {
	return &Pipeline{runner: runner}
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
		Metadata: PipelineMetadata{StartedAt: time.Now()},
	}

	if strings.TrimSpace(req.CaseID) == "" {
		return res, fmt.Errorf("case_id is required")
	}
	if len(strings.TrimSpace(req.DisclosureText)) < MinDisclosureChars {
		return res, fmt.Errorf("disclosure text is insufficient for analysis")
	}
	if len(req.DisclosureText) > MaxDisclosureChars {
		req.DisclosureText = req.DisclosureText[:MaxDisclosureChars]
		res.Metadata.InputTruncated = true
	}
	res.Request = req

	emit(progress, "stage_1", "Stage 1: Extracting invention details...")
	stageStarted := time.Now()
	s1, m1, err := p.runner.RunStage1(ctx, req)
	if err != nil {
		return res, &StageError{Stage: "stage_1", Err: err}
	}
	emit(progress, "stage_1", fmt.Sprintf("Stage 1 complete in %s", time.Since(stageStarted).Round(time.Millisecond)))
	res.Stage1 = s1
	res.Attempts["stage_1"] = m1
	res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_1")

	// Advisory track always runs after stage 1.
	emit(progress, "stage_6", "Stage 6: Running §102/§103 preliminary flags...")
	stageStarted = time.Now()
	s6, m6, err := p.runner.RunStage6(ctx, s1)
	if err != nil {
		return res, &StageError{Stage: "stage_6", Err: err}
	}
	emit(progress, "stage_6", fmt.Sprintf("Stage 6 complete in %s", time.Since(stageStarted).Round(time.Millisecond)))
	res.Stage6 = s6
	res.Attempts["stage_6"] = m6
	res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_6")

	emit(progress, "stage_2", "Stage 2: Classifying statutory category...")
	stageStarted = time.Now()
	s2, m2, err := p.runner.RunStage2(ctx, s1)
	if err != nil {
		return res, &StageError{Stage: "stage_2", Err: err}
	}
	emit(progress, "stage_2", fmt.Sprintf("Stage 2 complete in %s", time.Since(stageStarted).Round(time.Millisecond)))
	res.Stage2 = &s2
	res.Attempts["stage_2"] = m2
	res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_2")

	if !s2.PassesStep1 {
		res.BaseDetermination = DeterminationLikelyNotEligible
		res.Pathway = PathwayA
		res.Metadata.StagesSkipped = append(res.Metadata.StagesSkipped, "stage_3", "stage_4", "stage_5")
		return p.finalize(res), nil
	}

	emit(progress, "stage_3", "Stage 3: Evaluating judicial exception (Step 2A Prong 1)...")
	stageStarted = time.Now()
	s3, m3, err := p.runner.RunStage3(ctx, s1, s2)
	if err != nil {
		return res, &StageError{Stage: "stage_3", Err: err}
	}
	emit(progress, "stage_3", fmt.Sprintf("Stage 3 complete in %s", time.Since(stageStarted).Round(time.Millisecond)))
	res.Stage3 = &s3
	res.Attempts["stage_3"] = m3
	res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_3")

	if !s3.RecitesException {
		res.BaseDetermination = DeterminationLikelyEligible
		res.Pathway = PathwayB1
		res.Metadata.StagesSkipped = append(res.Metadata.StagesSkipped, "stage_4", "stage_5")
		return p.finalize(res), nil
	}

	emit(progress, "stage_4", "Stage 4: Evaluating practical application (Step 2A Prong 2)...")
	stageStarted = time.Now()
	s4, m4, err := p.runner.RunStage4(ctx, s1, s3)
	if err != nil {
		return res, &StageError{Stage: "stage_4", Err: err}
	}
	emit(progress, "stage_4", fmt.Sprintf("Stage 4 complete in %s", time.Since(stageStarted).Round(time.Millisecond)))
	res.Stage4 = &s4
	res.Attempts["stage_4"] = m4
	res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_4")

	if s4.IntegratesPracticalApplication {
		res.BaseDetermination = DeterminationLikelyEligible
		res.Pathway = PathwayB2
		res.Metadata.StagesSkipped = append(res.Metadata.StagesSkipped, "stage_5")
		return p.finalize(res), nil
	}

	emit(progress, "stage_5", "Stage 5: Evaluating inventive concept (Step 2B)...")
	stageStarted = time.Now()
	s5First, m5First, err := p.runner.RunStage5(ctx, s1, s3, s4)
	if err != nil {
		return res, &StageError{Stage: "stage_5", Err: err}
	}
	s5Second, m5Second, secondErr := p.runner.RunStage5(ctx, s1, s3, s4)
	emit(progress, "stage_5", fmt.Sprintf("Stage 5 complete in %s", time.Since(stageStarted).Round(time.Millisecond)))
	res.Stage5 = &s5First
	res.Attempts["stage_5"] = StageAttemptMetrics{
		Attempts:       m5First.Attempts + m5Second.Attempts,
		ContentRetries: m5First.ContentRetries + m5Second.ContentRetries,
	}
	res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_5")
	if secondErr != nil {
		if res.Metadata.DecisionTrace == nil {
			res.Metadata.DecisionTrace = map[string]any{}
		}
		res.Metadata.DecisionTrace["stage_5"] = map[string]any{
			"verification_error": secondErr.Error(),
			"run_1":              s5First,
		}
	}
	if secondErr == nil {
		agreement := s5First.HasInventiveConcept == s5Second.HasInventiveConcept
		res.Metadata.Stage5BooleanAgreement = &agreement
		if !agreement {
			if res.Metadata.DecisionTrace == nil {
				res.Metadata.DecisionTrace = map[string]any{}
			}
			res.Metadata.DecisionTrace["stage_5"] = map[string]any{
				"disagreement": true,
				"run_1":        s5First,
				"run_2":        s5Second,
			}
		}
	}

	if s5First.HasInventiveConcept {
		res.BaseDetermination = DeterminationLikelyEligible
		res.Pathway = PathwayC
	} else {
		res.BaseDetermination = DeterminationLikelyNotEligible
		res.Pathway = PathwayD
	}

	return p.finalize(res), nil
}

func emit(progress StageProgressFn, stage, message string) {
	if progress != nil {
		progress(stage, message)
	}
}

func (p *Pipeline) finalize(res PipelineResult) PipelineResult {
	res.Metadata.NeedsReviewReasons = computeNeedsReviewReasons(res)
	if len(res.Metadata.NeedsReviewReasons) > 0 {
		res.FinalDetermination = DeterminationNeedsFurtherReview
	} else {
		res.FinalDetermination = res.BaseDetermination
	}
	res.Metadata.CompletedAt = time.Now()
	res.Metadata.StageAttempts = map[string]int{}
	res.Metadata.StageContentRetries = map[string]int{}
	for _, m := range res.Attempts {
		res.Metadata.TotalLLMCalls += m.Attempts
		if m.Attempts > 1 {
			res.Metadata.TotalRetries += m.Attempts - 1
		}
	}
	for stage, m := range res.Attempts {
		res.Metadata.StageAttempts[stage] = m.Attempts
		res.Metadata.StageContentRetries[stage] = m.ContentRetries
	}
	return res
}

func computeNeedsReviewReasons(res PipelineResult) []string {
	var reasons []string
	for stage, m := range res.Attempts {
		if m.ContentRetries > 0 {
			reasons = append(reasons, fmt.Sprintf("%s: required content retries (%d)", stage, m.ContentRetries))
		}
	}
	for stage, c := range stageConfidences(res) {
		if c.ConfidenceScore < NeedsReviewConfidenceThreshold {
			reasons = append(reasons, fmt.Sprintf("%s: low confidence (%.2f) — %s", stage, c.ConfidenceScore, c.ConfidenceReason))
		}
		if c.InsufficientInformation {
			reasons = append(reasons, fmt.Sprintf("%s: insufficient information — %s", stage, c.ConfidenceReason))
		}
	}
	if res.Metadata.InputTruncated {
		reasons = append(reasons, fmt.Sprintf("Input disclosure was truncated to %d characters", MaxDisclosureChars))
	}
	if res.Metadata.Stage5BooleanAgreement != nil && !*res.Metadata.Stage5BooleanAgreement {
		reasons = append(reasons, "stage_5: verification disagreement between repeated boolean outputs")
	}
	if trace, ok := res.Metadata.DecisionTrace["stage_5"].(map[string]any); ok {
		if errText, ok := trace["verification_error"].(string); ok && strings.TrimSpace(errText) != "" {
			reasons = append(reasons, fmt.Sprintf("stage_5: verification retry failed (%s)", errText))
		}
	}
	return reasons
}

func StageNameFromError(err error) string {
	var se *StageError
	if errors.As(err, &se) {
		return se.Stage
	}
	return "pipeline"
}

func stageConfidences(res PipelineResult) map[string]StageConfidence {
	out := map[string]StageConfidence{"stage_1": res.Stage1.StageConfidence, "stage_6": res.Stage6.StageConfidence}
	if res.Stage2 != nil {
		out["stage_2"] = res.Stage2.StageConfidence
	}
	if res.Stage3 != nil {
		out["stage_3"] = res.Stage3.StageConfidence
	}
	if res.Stage4 != nil {
		out["stage_4"] = res.Stage4.StageConfidence
	}
	if res.Stage5 != nil {
		out["stage_5"] = res.Stage5.StageConfidence
	}
	return out
}
