package marketanalysis

import (
	"context"
	"strings"
	"testing"
)

type queueCaller struct {
	responses []string
	prompts   []string
}

func (q *queueCaller) GenerateJSON(_ context.Context, prompt string) (string, error) {
	q.prompts = append(q.prompts, prompt)
	if len(q.responses) == 0 {
		return "{}", nil
	}
	out := q.responses[0]
	q.responses = q.responses[1:]
	return out, nil
}

func TestRunStage0HappyRetryFailure(t *testing.T) {
	valid := `{"invention_title":{"value":"Title","confidence":"HIGH","missing_reason":null},"problem_solved":{"value":"Problem","confidence":"HIGH","missing_reason":null},"solution_description":{"value":"Solution","confidence":"HIGH","missing_reason":null},"claimed_advantages":{"value":["adv"],"confidence":"MEDIUM","missing_reason":null},"target_user":{"value":"Lab user","confidence":"MEDIUM","missing_reason":null},"target_buyer":{"value":"Hospital","confidence":"MEDIUM","missing_reason":null},"application_domains":{"value":["domain"],"confidence":"MEDIUM","missing_reason":null},"evidence_level":"PROTOTYPE","competing_approaches":{"value":["alt"],"confidence":"LOW","missing_reason":null},"dependencies":{"value":["device"],"confidence":"LOW","missing_reason":null},"sector":"software"}`

	t.Run("happy", func(t *testing.T) {
		q := &queueCaller{responses: []string{valid}}
		r := NewLLMStageRunner(NewStageExecutor(q))
		_, m, err := r.RunStage0(context.Background(), baseReq())
		if err != nil {
			t.Fatalf("RunStage0: %v", err)
		}
		if m.Attempts != 1 {
			t.Fatalf("expected 1 attempt, got %d", m.Attempts)
		}
		if len(q.prompts) != 1 || !strings.Contains(q.prompts[0], "Required JSON schema") {
			t.Fatal("expected schema prompt in stage 0")
		}
	})

	t.Run("retry", func(t *testing.T) {
		q := &queueCaller{responses: []string{"not-json", valid}}
		r := NewLLMStageRunner(NewStageExecutor(q))
		_, m, err := r.RunStage0(context.Background(), baseReq())
		if err != nil {
			t.Fatalf("RunStage0 retry: %v", err)
		}
		if m.Attempts != 2 || m.ContentRetries != 1 {
			t.Fatalf("expected attempts=2 content_retries=1, got %+v", m)
		}
	})

	t.Run("failure", func(t *testing.T) {
		q := &queueCaller{responses: []string{"{}", "{}", "{}"}}
		r := NewLLMStageRunner(NewStageExecutor(q))
		_, _, err := r.RunStage0(context.Background(), baseReq())
		if err == nil {
			t.Fatal("expected failure")
		}
	})
}

func TestRunStage1To5HappyAndPromptBlocks(t *testing.T) {
	stageCases := []struct {
		name         string
		run          func(*LLMStageRunner) (StageAttemptMetrics, error)
		validJSON    string
		promptMarker string
	}{
		{
			name:         "stage1",
			validJSON:    `{"primary_path":"NON_EXCLUSIVE_LICENSE","primary_path_reasoning":"fit","secondary_path":null,"secondary_path_reasoning":null,"product_definition":"SDK","has_plausible_monetization":true,"no_monetization_reasoning":null,"non_patent_monetization":false,"non_patent_monetization_reasoning":null}`,
			promptMarker: "OPEN_SOURCE_PLUS_SERVICES",
			run: func(r *LLMStageRunner) (StageAttemptMetrics, error) {
				_, m, err := r.RunStage1(context.Background(), baseStage0())
				return m, err
			},
		},
		{
			name:         "stage2",
			validJSON:    `{"scores":{"market_pain":{"score":3,"reasoning":"x"},"differentiation":{"score":3,"reasoning":"x"},"adoption_friction":{"score":3,"reasoning":"x"},"development_burden":{"score":3,"reasoning":"x"},"partner_density":{"score":3,"reasoning":"x"},"ip_leverage":{"score":3,"reasoning":"x"}},"confidence":"MEDIUM","confidence_reasoning":"limited but usable","unknown_key_factors":["pricing"]}`,
			promptMarker: "MARKET_PAIN (1-5)",
			run: func(r *LLMStageRunner) (StageAttemptMetrics, error) {
				_, m, err := r.RunStage2(context.Background(), baseStage0(), baseStage1())
				return m, err
			},
		},
		{
			name:         "stage3",
			validJSON:    `{"tam":{"low_usd":100,"high_usd":200,"unit":"annual revenue","assumptions":[{"assumption":"x","source":"ESTIMATED"}],"estimable":true,"not_estimable_reason":null},"sam":{"low_usd":80,"high_usd":160,"unit":"annual revenue","assumptions":[{"assumption":"x","source":"INFERRED"}],"estimable":true,"not_estimable_reason":null},"som":{"low_usd":20,"high_usd":60,"unit":"annual revenue","assumptions":[{"assumption":"x","source":"DISCLOSURE_DERIVED"}],"estimable":true,"not_estimable_reason":null},"tam_som_ratio_warning":null}`,
			promptMarker: "SOM (Serviceable Obtainable Market)",
			run: func(r *LLMStageRunner) (StageAttemptMetrics, error) {
				_, m, err := r.RunStage3(context.Background(), baseStage0(), baseStage1(), baseStage2())
				return m, err
			},
		},
		{
			name:         "stage4",
			validJSON:    `{"royalty_rate_pct":{"low":2,"high":5,"source":"DOMAIN_DEFAULT","reasoning":"default"},"p_license_3yr":{"low":0.1,"high":0.2,"source":"DOMAIN_DEFAULT","reasoning":"default"},"p_commercial_success":{"low":0.1,"high":0.2,"source":"DOMAIN_DEFAULT","reasoning":"default"},"time_to_license_months":{"low":6,"high":18,"source":"DOMAIN_DEFAULT","reasoning":"default"},"time_from_license_to_revenue_months":{"low":3,"high":12,"source":"DOMAIN_DEFAULT","reasoning":"default"},"annual_revenue_to_licensee_usd":{"low":500000,"high":1000000,"source":"DOMAIN_DEFAULT","reasoning":"default"},"license_duration_years":{"low":5,"high":10,"source":"DOMAIN_DEFAULT","reasoning":"default"},"patent_cost_usd":{"low":20000,"high":50000,"source":"DOMAIN_DEFAULT","reasoning":"default"}}`,
			promptMarker: "You must output assumption ranges, NOT computed financial results.",
			run: func(r *LLMStageRunner) (StageAttemptMetrics, error) {
				_, m, err := r.RunStage4(context.Background(), baseStage0(), baseStage1(), baseStage2(), baseStage3())
				return m, err
			},
		},
		{
			name:         "stage5",
			validJSON:    `{"executive_summary":"summary","key_drivers":["a","b","c"],"diligence_questions":["a","b","c"],"recommended_actions":["meet inventor"],"non_patent_actions":[],"model_limitations":[]}`,
			promptMarker: "Do not contradict the computed recommendation tier.",
			run: func(r *LLMStageRunner) (StageAttemptMetrics, error) {
				in := Stage5Input{Stage0: baseStage0(), Stage1: baseStage1(), Stage2: baseStage2(), Stage3: ptrStage3(baseStage3()), Stage4: ptrStage4(baseStage4()), Stage4Computed: &Stage4ComputedOutput{Scenarios: map[string]ScenarioOutput{"base": {}}, SensitivityDrivers: []SensitivityDriver{{Assumption: "royalty_rate_pct", NPVDeltaUSD: 10}}}, Decision: RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceLow, Reason: "x"}}
				_, m, err := r.RunStage5(context.Background(), in)
				return m, err
			},
		},
	}

	for _, tc := range stageCases {
		t.Run(tc.name, func(t *testing.T) {
			q := &queueCaller{responses: []string{tc.validJSON}}
			r := NewLLMStageRunner(NewStageExecutor(q))
			m, err := tc.run(r)
			if err != nil {
				t.Fatalf("run failed: %v", err)
			}
			if m.Attempts != 1 {
				t.Fatalf("expected 1 attempt, got %d", m.Attempts)
			}
			if len(q.prompts) != 1 || !strings.Contains(q.prompts[0], tc.promptMarker) {
				t.Fatalf("expected prompt marker %q", tc.promptMarker)
			}
		})
	}
}

func TestRunStage1To5RetryAndFailure(t *testing.T) {
	cases := []struct {
		name      string
		validJSON string
		run       func(*LLMStageRunner) (StageAttemptMetrics, error)
	}{
		{
			name:      "stage1",
			validJSON: `{"primary_path":"NON_EXCLUSIVE_LICENSE","primary_path_reasoning":"fit","secondary_path":null,"secondary_path_reasoning":null,"product_definition":"SDK","has_plausible_monetization":true,"no_monetization_reasoning":null,"non_patent_monetization":false,"non_patent_monetization_reasoning":null}`,
			run: func(r *LLMStageRunner) (StageAttemptMetrics, error) {
				_, m, err := r.RunStage1(context.Background(), baseStage0())
				return m, err
			},
		},
		{
			name:      "stage2",
			validJSON: `{"scores":{"market_pain":{"score":3,"reasoning":"x"},"differentiation":{"score":3,"reasoning":"x"},"adoption_friction":{"score":3,"reasoning":"x"},"development_burden":{"score":3,"reasoning":"x"},"partner_density":{"score":3,"reasoning":"x"},"ip_leverage":{"score":3,"reasoning":"x"}},"confidence":"MEDIUM","confidence_reasoning":"usable","unknown_key_factors":[]}`,
			run: func(r *LLMStageRunner) (StageAttemptMetrics, error) {
				_, m, err := r.RunStage2(context.Background(), baseStage0(), baseStage1())
				return m, err
			},
		},
		{
			name:      "stage3",
			validJSON: `{"tam":{"low_usd":100,"high_usd":200,"unit":"annual revenue","assumptions":[{"assumption":"x","source":"ESTIMATED"}],"estimable":true,"not_estimable_reason":null},"sam":{"low_usd":80,"high_usd":160,"unit":"annual revenue","assumptions":[{"assumption":"x","source":"ESTIMATED"}],"estimable":true,"not_estimable_reason":null},"som":{"low_usd":20,"high_usd":60,"unit":"annual revenue","assumptions":[{"assumption":"x","source":"ESTIMATED"}],"estimable":true,"not_estimable_reason":null},"tam_som_ratio_warning":null}`,
			run: func(r *LLMStageRunner) (StageAttemptMetrics, error) {
				_, m, err := r.RunStage3(context.Background(), baseStage0(), baseStage1(), baseStage2())
				return m, err
			},
		},
		{
			name:      "stage4",
			validJSON: `{"royalty_rate_pct":{"low":2,"high":5,"source":"DOMAIN_DEFAULT","reasoning":"default"},"p_license_3yr":{"low":0.1,"high":0.2,"source":"DOMAIN_DEFAULT","reasoning":"default"},"p_commercial_success":{"low":0.1,"high":0.2,"source":"DOMAIN_DEFAULT","reasoning":"default"},"time_to_license_months":{"low":6,"high":18,"source":"DOMAIN_DEFAULT","reasoning":"default"},"time_from_license_to_revenue_months":{"low":3,"high":12,"source":"DOMAIN_DEFAULT","reasoning":"default"},"annual_revenue_to_licensee_usd":{"low":500000,"high":1000000,"source":"DOMAIN_DEFAULT","reasoning":"default"},"license_duration_years":{"low":5,"high":10,"source":"DOMAIN_DEFAULT","reasoning":"default"},"patent_cost_usd":{"low":20000,"high":50000,"source":"DOMAIN_DEFAULT","reasoning":"default"}}`,
			run: func(r *LLMStageRunner) (StageAttemptMetrics, error) {
				_, m, err := r.RunStage4(context.Background(), baseStage0(), baseStage1(), baseStage2(), baseStage3())
				return m, err
			},
		},
		{
			name:      "stage5",
			validJSON: `{"executive_summary":"summary","key_drivers":["a","b","c"],"diligence_questions":["a","b","c"],"recommended_actions":["meet inventor"],"non_patent_actions":[],"model_limitations":[]}`,
			run: func(r *LLMStageRunner) (StageAttemptMetrics, error) {
				in := Stage5Input{Stage0: baseStage0(), Stage1: baseStage1(), Stage2: baseStage2(), Stage3: ptrStage3(baseStage3()), Stage4: ptrStage4(baseStage4()), Stage4Computed: &Stage4ComputedOutput{Scenarios: map[string]ScenarioOutput{"base": {}}}, Decision: RecommendationDecision{Tier: RecommendationDefer, Confidence: ConfidenceLow, Reason: "x"}}
				_, m, err := r.RunStage5(context.Background(), in)
				return m, err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+"_retry", func(t *testing.T) {
			q := &queueCaller{responses: []string{"not-json", tc.validJSON}}
			r := NewLLMStageRunner(NewStageExecutor(q))
			m, err := tc.run(r)
			if err != nil {
				t.Fatalf("expected retry success, got %v", err)
			}
			if m.Attempts != 2 || m.ContentRetries != 1 {
				t.Fatalf("unexpected metrics: %+v", m)
			}
		})
		t.Run(tc.name+"_failure", func(t *testing.T) {
			q := &queueCaller{responses: []string{"not-json", "not-json", "not-json"}}
			r := NewLLMStageRunner(NewStageExecutor(q))
			_, err := tc.run(r)
			if err == nil {
				t.Fatal("expected stage failure")
			}
		})
	}
}

func ptrStage3(v Stage3Output) *Stage3Output { return &v }
func ptrStage4(v Stage4Output) *Stage4Output { return &v }
