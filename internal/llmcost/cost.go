package llmcost

import (
	"os"
	"strconv"
	"strings"
)

const perMillion = 1_000_000.0

type Usage struct {
	InputTokens              int64
	OutputTokens             int64
	CacheCreationInputTokens int64
	CacheReadInputTokens     int64
}

type Pricing struct {
	InputPerMTokUSD      float64
	OutputPerMTokUSD     float64
	CacheWritePerMTokUSD float64
	CacheReadPerMTokUSD  float64
}

func (u Usage) TotalInputTokens() int64 {
	return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

func EstimateUSD(u Usage, p Pricing) float64 {
	if p.InputPerMTokUSD < 0 || p.OutputPerMTokUSD < 0 || p.CacheWritePerMTokUSD < 0 || p.CacheReadPerMTokUSD < 0 {
		return 0
	}
	return (float64(u.InputTokens)*p.InputPerMTokUSD +
		float64(u.OutputTokens)*p.OutputPerMTokUSD +
		float64(u.CacheCreationInputTokens)*p.CacheWritePerMTokUSD +
		float64(u.CacheReadInputTokens)*p.CacheReadPerMTokUSD) / perMillion
}

// ResolvePricing returns pricing and source.
// Source is one of: "env_global", "default_table".
func ResolvePricing(model string) (Pricing, string, bool) {
	if p, ok := pricingFromGlobalEnv(); ok {
		return p, "env_global", true
	}
	if p, ok := defaultPricing(model); ok {
		return p, "default_table", true
	}
	return Pricing{}, "", false
}

func pricingFromGlobalEnv() (Pricing, bool) {
	input, okIn := parseFloatEnv("LLM_COST_INPUT_PER_MTOK_USD")
	output, okOut := parseFloatEnv("LLM_COST_OUTPUT_PER_MTOK_USD")
	cacheWrite, okCW := parseFloatEnv("LLM_COST_CACHE_WRITE_PER_MTOK_USD")
	cacheRead, okCR := parseFloatEnv("LLM_COST_CACHE_READ_PER_MTOK_USD")
	if !okIn || !okOut {
		return Pricing{}, false
	}
	if !okCW {
		cacheWrite = 0
	}
	if !okCR {
		cacheRead = 0
	}
	return Pricing{
		InputPerMTokUSD:      input,
		OutputPerMTokUSD:     output,
		CacheWritePerMTokUSD: cacheWrite,
		CacheReadPerMTokUSD:  cacheRead,
	}, true
}

func parseFloatEnv(key string) (float64, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 {
		return 0, false
	}
	return v, true
}

func defaultPricing(model string) (Pricing, bool) {
	n := normalizeModel(model)
	switch {
	case n == "claude-sonnet-4-20250514", n == "claude-sonnet-4-6", strings.HasPrefix(n, "claude-sonnet-4"):
		return Pricing{
			InputPerMTokUSD:      3.00,
			OutputPerMTokUSD:     15.00,
			CacheWritePerMTokUSD: 3.75,
			CacheReadPerMTokUSD:  0.30,
		}, true
	default:
		return Pricing{}, false
	}
}

func normalizeModel(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}
