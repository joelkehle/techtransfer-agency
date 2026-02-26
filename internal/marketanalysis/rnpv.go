package marketanalysis

import (
	"fmt"
	"math"
	"sort"
)

const discountRate = 0.10

type scenarioInputs struct {
	royaltyRatePct                 float64
	pLicense3yr                    float64
	pCommercialSuccess             float64
	timeToLicenseMonths            int
	timeFromLicenseToRevenueMonths int
	annualRevenueToLicenseeUSD     int
	licenseDurationYears           int
	patentCostUSD                  int
}

func ComputeStage4Outputs(stage4 Stage4Output, priors DomainPriors) Stage4ComputedOutput {
	pess := scenarioFromRanges(stage4, false)
	opt := scenarioFromRanges(stage4, true)
	base := scenarioMid(stage4)

	patentCostMid := 0.5 * float64(stage4.PatentCostUSD.Low+stage4.PatentCostUSD.High)

	pessNPV := computeNPV(pess, priors.RevenueRampYears)
	baseNPV := computeNPV(base, priors.RevenueRampYears)
	optNPV := computeNPV(opt, priors.RevenueRampYears)

	sens := computeSensitivity(base, stage4, priors)
	if len(sens) > 3 {
		sens = sens[:3]
	}

	return Stage4ComputedOutput{
		Scenarios: map[string]ScenarioOutput{
			"pessimistic": {NPVUSD: pessNPV, ExceedsPatentCost: pessNPV > patentCostMid},
			"base":        {NPVUSD: baseNPV, ExceedsPatentCost: baseNPV > patentCostMid},
			"optimistic":  {NPVUSD: optNPV, ExceedsPatentCost: optNPV > patentCostMid},
		},
		PatentCostMidUSD:   patentCostMid,
		SensitivityDrivers: sens,
		RevenueRampYears:   priors.RevenueRampYears,
	}
}

func computeNPV(in scenarioInputs, rampYears int) float64 {
	timeToFirstRevenueMonths := in.timeToLicenseMonths + in.timeFromLicenseToRevenueMonths
	annualRoyalty := float64(in.annualRevenueToLicenseeUSD) * (in.royaltyRatePct / 100.0)
	riskAdjustedAnnual := annualRoyalty * in.pLicense3yr * in.pCommercialSuccess

	npv := 0.0
	for year := 1; year <= in.licenseDurationYears; year++ {
		monthsIntoDeal := year * 12
		if monthsIntoDeal < timeToFirstRevenueMonths {
			continue
		}
		yearsOfRevenue := float64(monthsIntoDeal-timeToFirstRevenueMonths) / 12.0
		yearRevenue := riskAdjustedAnnual
		if yearsOfRevenue <= float64(rampYears) {
			yearRevenue = riskAdjustedAnnual * 0.5
		}
		npv += yearRevenue / math.Pow(1+discountRate, float64(year))
	}
	return npv
}

func scenarioFromRanges(s Stage4Output, optimistic bool) scenarioInputs {
	if optimistic {
		return scenarioInputs{
			royaltyRatePct:                 s.RoyaltyRatePct.High,
			pLicense3yr:                    s.PLicense3yr.High,
			pCommercialSuccess:             s.PCommercialSuccess.High,
			timeToLicenseMonths:            s.TimeToLicenseMonths.Low,
			timeFromLicenseToRevenueMonths: s.TimeFromLicenseToRevenueMonths.Low,
			annualRevenueToLicenseeUSD:     s.AnnualRevenueToLicenseeUSD.High,
			licenseDurationYears:           s.LicenseDurationYears.High,
			patentCostUSD:                  s.PatentCostUSD.Low,
		}
	}
	return scenarioInputs{
		royaltyRatePct:                 s.RoyaltyRatePct.Low,
		pLicense3yr:                    s.PLicense3yr.Low,
		pCommercialSuccess:             s.PCommercialSuccess.Low,
		timeToLicenseMonths:            s.TimeToLicenseMonths.High,
		timeFromLicenseToRevenueMonths: s.TimeFromLicenseToRevenueMonths.High,
		annualRevenueToLicenseeUSD:     s.AnnualRevenueToLicenseeUSD.Low,
		licenseDurationYears:           s.LicenseDurationYears.Low,
		patentCostUSD:                  s.PatentCostUSD.High,
	}
}

func scenarioMid(s Stage4Output) scenarioInputs {
	return scenarioInputs{
		royaltyRatePct:                 midpointFloat(s.RoyaltyRatePct.Low, s.RoyaltyRatePct.High),
		pLicense3yr:                    midpointFloat(s.PLicense3yr.Low, s.PLicense3yr.High),
		pCommercialSuccess:             midpointFloat(s.PCommercialSuccess.Low, s.PCommercialSuccess.High),
		timeToLicenseMonths:            midpointInt(s.TimeToLicenseMonths.Low, s.TimeToLicenseMonths.High),
		timeFromLicenseToRevenueMonths: midpointInt(s.TimeFromLicenseToRevenueMonths.Low, s.TimeFromLicenseToRevenueMonths.High),
		annualRevenueToLicenseeUSD:     midpointInt(s.AnnualRevenueToLicenseeUSD.Low, s.AnnualRevenueToLicenseeUSD.High),
		licenseDurationYears:           midpointInt(s.LicenseDurationYears.Low, s.LicenseDurationYears.High),
		patentCostUSD:                  midpointInt(s.PatentCostUSD.Low, s.PatentCostUSD.High),
	}
}

func computeSensitivity(base scenarioInputs, stage4 Stage4Output, priors DomainPriors) []SensitivityDriver {
	type candidate struct {
		name      string
		low       func() float64
		high      func() float64
		direction string
	}

	cands := []candidate{
		{name: "royalty_rate_pct", low: func() float64 {
			b := base
			b.royaltyRatePct = stage4.RoyaltyRatePct.Low
			return computeNPV(b, priors.RevenueRampYears)
		}, high: func() float64 {
			b := base
			b.royaltyRatePct = stage4.RoyaltyRatePct.High
			return computeNPV(b, priors.RevenueRampYears)
		}, direction: "Higher royalty rate increases NPV"},
		{name: "p_license_3yr", low: func() float64 {
			b := base
			b.pLicense3yr = stage4.PLicense3yr.Low
			return computeNPV(b, priors.RevenueRampYears)
		}, high: func() float64 {
			b := base
			b.pLicense3yr = stage4.PLicense3yr.High
			return computeNPV(b, priors.RevenueRampYears)
		}, direction: "Higher licensing probability increases NPV"},
		{name: "p_commercial_success", low: func() float64 {
			b := base
			b.pCommercialSuccess = stage4.PCommercialSuccess.Low
			return computeNPV(b, priors.RevenueRampYears)
		}, high: func() float64 {
			b := base
			b.pCommercialSuccess = stage4.PCommercialSuccess.High
			return computeNPV(b, priors.RevenueRampYears)
		}, direction: "Higher commercial success probability increases NPV"},
		{name: "time_to_license_months", low: func() float64 {
			b := base
			b.timeToLicenseMonths = stage4.TimeToLicenseMonths.Low
			return computeNPV(b, priors.RevenueRampYears)
		}, high: func() float64 {
			b := base
			b.timeToLicenseMonths = stage4.TimeToLicenseMonths.High
			return computeNPV(b, priors.RevenueRampYears)
		}, direction: "Lower time to license increases NPV"},
		{name: "time_from_license_to_revenue_months", low: func() float64 {
			b := base
			b.timeFromLicenseToRevenueMonths = stage4.TimeFromLicenseToRevenueMonths.Low
			return computeNPV(b, priors.RevenueRampYears)
		}, high: func() float64 {
			b := base
			b.timeFromLicenseToRevenueMonths = stage4.TimeFromLicenseToRevenueMonths.High
			return computeNPV(b, priors.RevenueRampYears)
		}, direction: "Lower time from license to revenue increases NPV"},
		{name: "annual_revenue_to_licensee_usd", low: func() float64 {
			b := base
			b.annualRevenueToLicenseeUSD = stage4.AnnualRevenueToLicenseeUSD.Low
			return computeNPV(b, priors.RevenueRampYears)
		}, high: func() float64 {
			b := base
			b.annualRevenueToLicenseeUSD = stage4.AnnualRevenueToLicenseeUSD.High
			return computeNPV(b, priors.RevenueRampYears)
		}, direction: "Higher annual revenue to licensee increases NPV"},
		{name: "license_duration_years", low: func() float64 {
			b := base
			b.licenseDurationYears = stage4.LicenseDurationYears.Low
			return computeNPV(b, priors.RevenueRampYears)
		}, high: func() float64 {
			b := base
			b.licenseDurationYears = stage4.LicenseDurationYears.High
			return computeNPV(b, priors.RevenueRampYears)
		}, direction: "Longer license duration increases NPV"},
	}

	out := make([]SensitivityDriver, 0, len(cands))
	for _, c := range cands {
		nLow := c.low()
		nHigh := c.high()
		out = append(out, SensitivityDriver{
			Assumption:  c.name,
			NPVDeltaUSD: math.Abs(nHigh - nLow),
			Direction:   fmt.Sprintf("%s by $%.0f", c.direction, math.Abs(nHigh-nLow)),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NPVDeltaUSD > out[j].NPVDeltaUSD })
	return out
}

func midpointFloat(a, b float64) float64 { return (a + b) / 2.0 }
func midpointInt(a, b int) int           { return (a + b) / 2 }
