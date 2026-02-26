package marketanalysis

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type StageError struct {
	Stage string
	Err   error
}

func (e *StageError) Error() string { return fmt.Sprintf("%s: %v", e.Stage, e.Err) }
func (e *StageError) Unwrap() error { return e.Err }

type StageProgressFn func(stage, message string)

type Pipeline struct {
	runner StageRunner
}

func NewPipeline(runner StageRunner) *Pipeline { return &Pipeline{runner: runner} }

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
		Metadata: PipelineMetadata{StartedAt: time.Now(), Mode: ReportModeComplete},
		Decision: RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceLow, Reason: "Analysis incomplete"},
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

	if err := p.runStage0(ctx, &res, progress); err != nil {
		return res, err
	}
	if err := p.runStage1(ctx, &res, progress); err != nil {
		return p.finalizeDegraded(res, "stage_1", err), nil
	}
	if !res.Stage1.HasPlausibleMonetization {
		res.Metadata.StagesSkipped = append(res.Metadata.StagesSkipped, "stage_2", "stage_3", "stage_4", "stage_5")
		res.Metadata.EarlyExitReason = "No plausible commercialization path identified."
		res.Decision = RecommendationDecision{Tier: RecommendationNoGo, Confidence: ConfidenceHigh, Reason: res.Metadata.EarlyExitReason}
		return p.finalize(res), nil
	}

	if err := p.runStage2(ctx, &res, progress); err != nil {
		return p.finalizeDegraded(res, "stage_2", err), nil
	}
	if res.Stage2.WeightedScore < 2.0 {
		reason := fmt.Sprintf("Weighted score %.2f below threshold; lowest dimensions: %s", res.Stage2.WeightedScore, strings.Join(lowestDimensions(res.Stage2), ", "))
		res.Metadata.StagesSkipped = append(res.Metadata.StagesSkipped, "stage_3", "stage_4", "stage_5")
		res.Metadata.EarlyExitReason = reason
		res.Decision = RecommendationDecision{Tier: RecommendationNoGo, Confidence: ConfidenceHigh, Reason: reason}
		return p.finalize(res), nil
	}
	if res.Stage2.WeightedScore >= 2.0 && res.Stage2.WeightedScore < 2.5 && res.Stage2.Confidence == ConfidenceLow {
		reason := fmt.Sprintf("Insufficient information for confident triage. Recommend inventor meeting to clarify %s.", topUnknown(res.Stage2))
		res.Metadata.StagesSkipped = append(res.Metadata.StagesSkipped, "stage_3", "stage_4", "stage_5")
		res.Metadata.EarlyExitReason = reason
		res.Decision = RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceLow, Reason: reason}
		return p.finalize(res), nil
	}
	if res.Stage2.Confidence == ConfidenceLow && len(res.Stage2.UnknownKeyFactors) >= 2 {
		reason := fmt.Sprintf("Key commercial factors unknown: %s. Recommend inventor meeting before further analysis.", strings.Join(res.Stage2.UnknownKeyFactors, ", "))
		res.Metadata.StagesSkipped = append(res.Metadata.StagesSkipped, "stage_3", "stage_4", "stage_5")
		res.Metadata.EarlyExitReason = reason
		res.Decision = RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceLow, Reason: reason}
		return p.finalize(res), nil
	}

	if err := p.runStage3(ctx, &res, progress); err != nil {
		return p.finalizeDegraded(res, "stage_3", err), nil
	}
	if !res.Stage3.SOM.Estimable {
		reason := fmt.Sprintf("Cannot estimate serviceable obtainable market. %s. Recommend inventor meeting to clarify target customer, pricing basis, and competitive positioning before economic analysis.", stringPtr(res.Stage3.SOM.NotEstimableReason))
		res.Metadata.StagesSkipped = append(res.Metadata.StagesSkipped, "stage_4", "stage_5")
		res.Metadata.EarlyExitReason = reason
		res.Decision = RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceLow, Reason: reason}
		return p.finalize(res), nil
	}

	if err := p.runStage4(ctx, &res, progress); err != nil {
		return p.finalizeDegraded(res, "stage_4", err), nil
	}
	dec := determineRecommendation(res)
	res.Decision = dec

	if err := p.runStage5(ctx, &res, progress); err != nil {
		return p.finalizeDegraded(res, "stage_5", err), nil
	}

	return p.finalize(res), nil
}

func (p *Pipeline) runStage0(ctx context.Context, res *PipelineResult, progress StageProgressFn) error {
	emit(progress, "stage_0", "Stage 0: Structured extraction...")
	s0, m0, err := p.runner.RunStage0(ctx, res.Request)
	if err != nil {
		return &StageError{Stage: "stage_0", Err: err}
	}
	res.Stage0 = s0
	res.Attempts["stage_0"] = m0
	res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_0")
	return nil
}

func (p *Pipeline) runStage1(ctx context.Context, res *PipelineResult, progress StageProgressFn) error {
	emit(progress, "stage_1", "Stage 1: Commercialization path selection...")
	s1, m, err := p.runner.RunStage1(ctx, res.Stage0)
	if err != nil {
		return &StageError{Stage: "stage_1", Err: err}
	}
	res.Stage1 = &s1
	res.Attempts["stage_1"] = m
	res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_1")
	return nil
}

func (p *Pipeline) runStage2(ctx context.Context, res *PipelineResult, progress StageProgressFn) error {
	emit(progress, "stage_2", "Stage 2: Triage scorecard...")
	s2, m, err := p.runner.RunStage2(ctx, res.Stage0, *res.Stage1)
	if err != nil {
		return &StageError{Stage: "stage_2", Err: err}
	}
	res.Stage2 = &s2
	res.Attempts["stage_2"] = m
	res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_2")
	return nil
}

func (p *Pipeline) runStage3(ctx context.Context, res *PipelineResult, progress StageProgressFn) error {
	emit(progress, "stage_3", "Stage 3: Market sizing (TAM/SAM/SOM)...")
	s3, m, err := p.runner.RunStage3(ctx, res.Stage0, *res.Stage1, *res.Stage2)
	if err != nil {
		return &StageError{Stage: "stage_3", Err: err}
	}
	res.Stage3 = &s3
	res.Attempts["stage_3"] = m
	res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_3")
	return nil
}

func (p *Pipeline) runStage4(ctx context.Context, res *PipelineResult, progress StageProgressFn) error {
	emit(progress, "stage_4", "Stage 4: Economic viability assumptions and rNPV...")
	s4, m, err := p.runner.RunStage4(ctx, res.Stage0, *res.Stage1, *res.Stage2, *res.Stage3)
	if err != nil {
		return &StageError{Stage: "stage_4", Err: err}
	}
	computed := ComputeStage4Outputs(s4, PriorForSector(res.Stage0.Sector))
	if res.Stage1.PrimaryPath == PathStartup {
		msg := "Royalty-based NPV model undervalues startup paths where equity dominates. Treat NPV as a lower bound. Recommend separate startup valuation if GO or DEFER."
		computed.PathModelLimitation = &msg
	}
	if res.Stage1.PrimaryPath == PathOpenSourceServices {
		msg := "Royalty-based NPV model does not capture open source economics (services, support, complementary products). Patent investment may not be primary value driver."
		computed.PathModelLimitation = &msg
	}
	res.Stage4 = &s4
	res.Stage4Computed = &computed
	res.Attempts["stage_4"] = m
	res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_4")
	return nil
}

func (p *Pipeline) runStage5(ctx context.Context, res *PipelineResult, progress StageProgressFn) error {
	emit(progress, "stage_5", "Stage 5: Recommendation narrative and diligence questions...")
	in := Stage5Input{Stage0: res.Stage0, Stage1: *res.Stage1, Stage2: *res.Stage2, Stage3: res.Stage3, Stage4: res.Stage4, Stage4Computed: res.Stage4Computed, Decision: res.Decision}
	s5, m, err := p.runner.RunStage5(ctx, in)
	if err != nil {
		return &StageError{Stage: "stage_5", Err: err}
	}
	res.Stage5 = &s5
	res.Attempts["stage_5"] = m
	res.Metadata.StagesExecuted = append(res.Metadata.StagesExecuted, "stage_5")
	return nil
}

func determineRecommendation(res PipelineResult) RecommendationDecision {
	c := RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceLow, Reason: "Analysis incomplete"}
	if res.Stage1 != nil && res.Stage1.PrimaryPath == PathResearchUseOnly {
		return RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceLow, Reason: "Research-use-only path reached economic stage unexpectedly."}
	}
	if res.Stage1 != nil && res.Stage1.PrimaryPath == PathOpenSourceServices {
		if res.Stage2.Scores.IPLeverage.Score < 4 {
			return RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceLow, Reason: stringPtr(res.Stage4Computed.PathModelLimitation) + " IP leverage insufficient for patent investment in open source context."}
		}
		if res.Stage4Computed.Scenarios["base"].ExceedsPatentCost {
			return RecommendationDecision{Tier: RecommendationGO, Confidence: ConfidenceLow, Reason: "Base scenario exceeds patent cost with open-source caveat.", Caveats: []string{stringPtr(res.Stage4Computed.PathModelLimitation)}}
		}
		return RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceLow, Reason: stringPtr(res.Stage4Computed.PathModelLimitation) + " Recommend specialized valuation."}
	}
	if res.Stage4Computed.PathModelLimitation != nil {
		if res.Stage4Computed.Scenarios["base"].ExceedsPatentCost {
			c = RecommendationDecision{Tier: RecommendationGO, Confidence: ConfidenceLow, Reason: "Base scenario exceeds patent cost with model limitation caveat.", Caveats: []string{*res.Stage4Computed.PathModelLimitation}}
		} else {
			c = RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceLow, Reason: *res.Stage4Computed.PathModelLimitation + " Recommend specialized valuation."}
		}
	} else if res.Stage4Computed.Scenarios["base"].ExceedsPatentCost && res.Stage4Computed.Scenarios["pessimistic"].ExceedsPatentCost {
		c = RecommendationDecision{Tier: RecommendationGO, Confidence: ConfidenceHigh, Reason: "Base and pessimistic scenarios exceed patent cost."}
	} else if res.Stage4Computed.Scenarios["base"].ExceedsPatentCost && !res.Stage4Computed.Scenarios["pessimistic"].ExceedsPatentCost {
		c = RecommendationDecision{Tier: RecommendationGO, Confidence: ConfidenceMedium, Reason: "Base scenario exceeds patent cost, pessimistic does not."}
	} else if res.Stage4Computed.Scenarios["optimistic"].ExceedsPatentCost && !res.Stage4Computed.Scenarios["base"].ExceedsPatentCost {
		c = RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceLow, Reason: "Viable only under optimistic assumptions."}
	} else {
		c = RecommendationDecision{Tier: RecommendationNoGo, Confidence: ConfidenceHigh, Reason: "Expected value does not exceed patent costs under any scenario."}
	}

	if res.Stage2 != nil && res.Stage2.Confidence == ConfidenceLow && len(res.Stage2.UnknownKeyFactors) >= 2 && c.Tier == RecommendationGO {
		c.Tier = RecommendationDefer
		c.Confidence = ConfidenceLow
		c.Reason = "Key commercial factors unknown: " + strings.Join(res.Stage2.UnknownKeyFactors, ", ") + ". NPV analysis is unreliable without these inputs."
	}
	return c
}

func (p *Pipeline) finalizeDegraded(res PipelineResult, failedStage string, err error) PipelineResult {
	res.Metadata.Mode = ReportModeDegraded
	res.Metadata.StageFailed = failedStage
	res.Metadata.EarlyExitReason = err.Error()
	if res.Decision.Tier == RecommendationGO {
		res.Decision.Tier = RecommendationDefer
		res.Decision.Confidence = ConfidenceLow
	}
	if res.Decision.Tier == "" {
		res.Decision = RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceLow, Reason: "Analysis incomplete due to stage failure. " + failedStage + " could not be completed."}
	}
	if strings.TrimSpace(res.Decision.Reason) == "" {
		res.Decision.Reason = "Analysis incomplete due to stage failure. " + failedStage + " could not be completed."
	}
	return p.finalize(res)
}

func (p *Pipeline) finalize(res PipelineResult) PipelineResult {
	res.Metadata.CompletedAt = time.Now()
	if res.Metadata.Mode == "" {
		res.Metadata.Mode = ReportModeComplete
	}
	res.Metadata.StageAttempts = map[string]int{}
	res.Metadata.StageContentRetries = map[string]int{}
	for stage, m := range res.Attempts {
		res.Metadata.StageAttempts[stage] = m.Attempts
		res.Metadata.StageContentRetries[stage] = m.ContentRetries
		res.Metadata.TotalLLMCalls += m.Attempts
		if m.Attempts > 1 {
			res.Metadata.TotalRetries += (m.Attempts - 1)
		}
	}
	return res
}

func emit(progress StageProgressFn, stage, message string) {
	if progress != nil {
		progress(stage, message)
	}
}

func lowestDimensions(s *Stage2Output) []string {
	pairs := []struct {
		name  string
		score int
	}{
		{"market_pain", s.Scores.MarketPain.Score},
		{"differentiation", s.Scores.Differentiation.Score},
		{"adoption_friction", s.Scores.AdoptionFriction.Score},
		{"development_burden", s.Scores.DevelopmentBurden.Score},
		{"partner_density", s.Scores.PartnerDensity.Score},
		{"ip_leverage", s.Scores.IPLeverage.Score},
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].score < pairs[j].score })
	return []string{pairs[0].name, pairs[1].name}
}

func topUnknown(s *Stage2Output) string {
	if len(s.UnknownKeyFactors) == 0 {
		return "buyer and pricing"
	}
	return s.UnknownKeyFactors[0]
}

func stringPtr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func StageNameFromError(err error) string {
	var se *StageError
	if errors.As(err, &se) {
		return se.Stage
	}
	return "pipeline"
}
