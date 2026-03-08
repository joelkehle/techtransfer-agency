package llmcost

import (
	"math"
	"testing"
)

func TestEstimateUSD(t *testing.T) {
	usage := Usage{
		InputTokens:              250_000,
		OutputTokens:             50_000,
		CacheCreationInputTokens: 10_000,
		CacheReadInputTokens:     20_000,
	}
	pricing := Pricing{
		InputPerMTokUSD:      3,
		OutputPerMTokUSD:     15,
		CacheWritePerMTokUSD: 3.75,
		CacheReadPerMTokUSD:  0.30,
	}
	got := EstimateUSD(usage, pricing)
	want := (250_000*3 + 50_000*15 + 10_000*3.75 + 20_000*0.30) / 1_000_000.0
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("EstimateUSD() = %f, want %f", got, want)
	}
}

func TestResolvePricingDefaultTable(t *testing.T) {
	t.Setenv("LLM_COST_INPUT_PER_MTOK_USD", "")
	t.Setenv("LLM_COST_OUTPUT_PER_MTOK_USD", "")
	p, source, ok := ResolvePricing("claude-sonnet-4-20250514")
	if !ok {
		t.Fatal("expected default pricing")
	}
	if source != "default_table" {
		t.Fatalf("source=%q want=default_table", source)
	}
	if p.InputPerMTokUSD <= 0 || p.OutputPerMTokUSD <= 0 {
		t.Fatalf("invalid pricing %+v", p)
	}
}

func TestResolvePricingGlobalEnvOverride(t *testing.T) {
	t.Setenv("LLM_COST_INPUT_PER_MTOK_USD", "1.25")
	t.Setenv("LLM_COST_OUTPUT_PER_MTOK_USD", "6.5")
	t.Setenv("LLM_COST_CACHE_WRITE_PER_MTOK_USD", "2.0")
	t.Setenv("LLM_COST_CACHE_READ_PER_MTOK_USD", "0.2")
	p, source, ok := ResolvePricing("claude-sonnet-4-20250514")
	if !ok {
		t.Fatal("expected env pricing")
	}
	if source != "env_global" {
		t.Fatalf("source=%q want=env_global", source)
	}
	if p.InputPerMTokUSD != 1.25 || p.OutputPerMTokUSD != 6.5 || p.CacheWritePerMTokUSD != 2.0 || p.CacheReadPerMTokUSD != 0.2 {
		t.Fatalf("pricing mismatch %+v", p)
	}
}
