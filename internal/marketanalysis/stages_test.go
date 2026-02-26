package marketanalysis

import "testing"

func TestValidateStage1SecondaryPathRules(t *testing.T) {
	reason := "secondary rationale"
	secondary := PathNonExclusive
	s := Stage1Output{
		PrimaryPath:              PathNonExclusive,
		PrimaryPathReasoning:     "fits market",
		SecondaryPath:            &secondary,
		SecondaryPathReasoning:   &reason,
		ProductDefinition:        "Software SDK",
		HasPlausibleMonetization: true,
	}
	if err := validateStage1(s); err == nil {
		t.Fatal("expected validation error when primary and secondary are equal")
	}
}

func TestValidateStage3NonEstimableRequiresReason(t *testing.T) {
	s := Stage3Output{
		TAM: MarketRange{LowUSD: 1, HighUSD: 2, Unit: "annual revenue", Estimable: true, Assumptions: []Stage3Assumption{{Assumption: "x", Source: SourceEstimated}}},
		SAM: MarketRange{LowUSD: 1, HighUSD: 2, Unit: "annual revenue", Estimable: true, Assumptions: []Stage3Assumption{{Assumption: "x", Source: SourceEstimated}}},
		SOM: MarketRange{LowUSD: 0, HighUSD: 0, Unit: "annual revenue", Estimable: false, Assumptions: []Stage3Assumption{{Assumption: "x", Source: SourceEstimated}}},
	}
	if err := validateStage3(s); err == nil {
		t.Fatal("expected validation error for missing not_estimable_reason")
	}
}

func TestValidateOptionalFieldHandlesJSONArrayAsAny(t *testing.T) {
	reason := "unknown"
	f := NullableField{Value: []any{}, Confidence: ConfidenceLow, MissingReason: &reason}
	if err := validateOptionalField(f); err != nil {
		t.Fatalf("expected []any empty with reason to be valid, got %v", err)
	}
}
