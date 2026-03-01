package patentscreen

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PipelineResultFromResponseEnvelope reconstructs a PipelineResult from a saved
// report envelope so markdown can be re-rendered without rerunning LLM stages.
func PipelineResultFromResponseEnvelope(env ResponseEnvelope) (PipelineResult, error) {
	res := PipelineResult{
		BaseDetermination:  env.Determination,
		FinalDetermination: env.Determination,
		Pathway:            Pathway(strings.TrimSpace(env.Pathway)),
		Request:            RequestEnvelope{CaseID: strings.TrimSpace(env.CaseID)},
		Metadata:           env.PipelineMetadata,
		Attempts:           map[string]StageAttemptMetrics{},
	}
	if err := decodeStageOutput(env.StageOutputs, "stage_1", &res.Stage1); err != nil {
		return PipelineResult{}, err
	}
	if err := decodeOptionalStageOutput(env.StageOutputs, "stage_2", &res.Stage2); err != nil {
		return PipelineResult{}, err
	}
	if err := decodeOptionalStageOutput(env.StageOutputs, "stage_3", &res.Stage3); err != nil {
		return PipelineResult{}, err
	}
	if err := decodeOptionalStageOutput(env.StageOutputs, "stage_4", &res.Stage4); err != nil {
		return PipelineResult{}, err
	}
	if err := decodeOptionalStageOutput(env.StageOutputs, "stage_5", &res.Stage5); err != nil {
		return PipelineResult{}, err
	}
	if err := decodeStageOutput(env.StageOutputs, "stage_6", &res.Stage6); err != nil {
		return PipelineResult{}, err
	}
	return res, nil
}

// RebuildResponseFromEnvelope regenerates report markdown from a saved envelope.
func RebuildResponseFromEnvelope(env ResponseEnvelope) (ResponseEnvelope, error) {
	res, err := PipelineResultFromResponseEnvelope(env)
	if err != nil {
		return ResponseEnvelope{}, err
	}
	return BuildResponse(res), nil
}

func decodeStageOutput(stageOutputs map[string]any, key string, out any) error {
	raw, ok := stageOutputs[key]
	if !ok || raw == nil {
		return fmt.Errorf("stage output %q is required", key)
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("stage output %q marshal: %w", key, err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("stage output %q decode: %w", key, err)
	}
	return nil
}

func decodeOptionalStageOutput[T any](stageOutputs map[string]any, key string, out **T) error {
	raw, ok := stageOutputs[key]
	if !ok || raw == nil {
		*out = nil
		return nil
	}
	var decoded T
	b, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("stage output %q marshal: %w", key, err)
	}
	if err := json.Unmarshal(b, &decoded); err != nil {
		return fmt.Errorf("stage output %q decode: %w", key, err)
	}
	*out = &decoded
	return nil
}
