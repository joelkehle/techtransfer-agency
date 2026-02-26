package marketanalysis

import "testing"

func sampleStage4() Stage4Output {
	return Stage4Output{
		RoyaltyRatePct:                 AssumptionRangeFloat{Low: 2, High: 6, Source: SourceDomainDefault, Reasoning: "default"},
		PLicense3yr:                    AssumptionRangeFloat{Low: 0.1, High: 0.3, Source: SourceDomainDefault, Reasoning: "default"},
		PCommercialSuccess:             AssumptionRangeFloat{Low: 0.1, High: 0.4, Source: SourceDomainDefault, Reasoning: "default"},
		TimeToLicenseMonths:            AssumptionRangeInt{Low: 12, High: 24, Source: SourceDomainDefault, Reasoning: "default"},
		TimeFromLicenseToRevenueMonths: AssumptionRangeInt{Low: 6, High: 18, Source: SourceDomainDefault, Reasoning: "default"},
		AnnualRevenueToLicenseeUSD:     AssumptionRangeInt{Low: 1000000, High: 10000000, Source: SourceDomainDefault, Reasoning: "default"},
		LicenseDurationYears:           AssumptionRangeInt{Low: 6, High: 12, Source: SourceDomainDefault, Reasoning: "default"},
		PatentCostUSD:                  AssumptionRangeInt{Low: 25000, High: 75000, Source: SourceDomainDefault, Reasoning: "default"},
	}
}

func TestComputeStage4OutputsScenarioOrdering(t *testing.T) {
	out := ComputeStage4Outputs(sampleStage4(), PriorForSector("default"))
	if out.Scenarios["optimistic"].NPVUSD < out.Scenarios["base"].NPVUSD {
		t.Fatal("expected optimistic >= base")
	}
	if out.Scenarios["base"].NPVUSD < out.Scenarios["pessimistic"].NPVUSD {
		t.Fatal("expected base >= pessimistic")
	}
	if len(out.SensitivityDrivers) == 0 {
		t.Fatal("expected sensitivity drivers")
	}
}

func TestComputeNPVTimeToRevenuePenalty(t *testing.T) {
	fast := scenarioInputs{royaltyRatePct: 5, pLicense3yr: 0.2, pCommercialSuccess: 0.3, timeToLicenseMonths: 6, timeFromLicenseToRevenueMonths: 6, annualRevenueToLicenseeUSD: 5000000, licenseDurationYears: 10}
	slow := fast
	slow.timeToLicenseMonths = 36
	nFast := computeNPV(fast, 2)
	nSlow := computeNPV(slow, 2)
	if nFast <= nSlow {
		t.Fatalf("expected faster time-to-revenue to have higher NPV; fast=%f slow=%f", nFast, nSlow)
	}
}

func TestComputeNPVExactKnownValue(t *testing.T) {
	in := scenarioInputs{
		royaltyRatePct:                 10,
		pLicense3yr:                    1.0,
		pCommercialSuccess:             1.0,
		timeToLicenseMonths:            0,
		timeFromLicenseToRevenueMonths: 0,
		annualRevenueToLicenseeUSD:     1000,
		licenseDurationYears:           3,
	}
	got := computeNPV(in, 0)
	// 100/1.1 + 100/1.21 + 100/1.331 = 248.6852
	want := 248.6852
	if diff(got, want) > 0.001 {
		t.Fatalf("unexpected NPV: got=%f want=%f", got, want)
	}
}

func TestComputeNPVRevenueRampStepFunction(t *testing.T) {
	in := scenarioInputs{
		royaltyRatePct:                 10,
		pLicense3yr:                    1.0,
		pCommercialSuccess:             1.0,
		timeToLicenseMonths:            0,
		timeFromLicenseToRevenueMonths: 0,
		annualRevenueToLicenseeUSD:     1000,
		licenseDurationYears:           3,
	}
	got := computeNPV(in, 1)
	// year1 half revenue, year2+ full
	want := 203.2306
	if diff(got, want) > 0.001 {
		t.Fatalf("unexpected ramped NPV: got=%f want=%f", got, want)
	}
}

func TestComputeSensitivityUsesActualBounds(t *testing.T) {
	s4 := sampleStage4()
	base := scenarioMid(s4)
	sens := computeSensitivity(base, s4, PriorForSector("default"))
	if len(sens) == 0 {
		t.Fatal("expected sensitivity output")
	}
	found := false
	for _, d := range sens {
		if d.Assumption == "royalty_rate_pct" {
			found = true
			low := base
			high := base
			low.royaltyRatePct = s4.RoyaltyRatePct.Low
			high.royaltyRatePct = s4.RoyaltyRatePct.High
			want := computeNPV(high, PriorForSector("default").RevenueRampYears) - computeNPV(low, PriorForSector("default").RevenueRampYears)
			if want < 0 {
				want = -want
			}
			if diff(d.NPVDeltaUSD, want) > 0.001 {
				t.Fatalf("unexpected delta for royalty_rate_pct: got=%f want=%f", d.NPVDeltaUSD, want)
			}
		}
	}
	if !found {
		t.Fatal("expected royalty_rate_pct sensitivity driver")
	}
}

func TestPatentCostComparisonFlag(t *testing.T) {
	s4 := sampleStage4()
	// Make value small and cost huge so base scenario should not exceed cost.
	s4.AnnualRevenueToLicenseeUSD = AssumptionRangeInt{Low: 1, High: 2, Source: SourceDomainDefault, Reasoning: "tiny"}
	s4.PatentCostUSD = AssumptionRangeInt{Low: 1000000, High: 2000000, Source: SourceDomainDefault, Reasoning: "large"}
	out := ComputeStage4Outputs(s4, PriorForSector("default"))
	if out.Scenarios["base"].ExceedsPatentCost {
		t.Fatal("expected base scenario not to exceed patent cost")
	}
}

func diff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
