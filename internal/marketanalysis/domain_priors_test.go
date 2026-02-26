package marketanalysis

import "testing"

func TestDefaultPriorsValidity(t *testing.T) {
	if _, ok := DefaultPriors["default"]; !ok {
		t.Fatal("default sector missing")
	}
	for name, p := range DefaultPriors {
		if p.RevenueRampYears <= 0 {
			t.Fatalf("%s: RevenueRampYears must be > 0", name)
		}
		if !(p.TypicalRoyaltyRangePct[0] < p.TypicalRoyaltyRangePct[1]) {
			t.Fatalf("%s: royalty low/high invalid", name)
		}
		if !(p.PLicense3yr[0] < p.PLicense3yr[1] && p.PLicense3yr[0] >= 0 && p.PLicense3yr[1] <= 1) {
			t.Fatalf("%s: p_license_3yr invalid", name)
		}
		if !(p.PCommercialSuccess[0] < p.PCommercialSuccess[1] && p.PCommercialSuccess[0] >= 0 && p.PCommercialSuccess[1] <= 1) {
			t.Fatalf("%s: p_commercial_success invalid", name)
		}
		if !(p.TimeToLicenseMonths[0] > 0 && p.TimeToLicenseMonths[0] < p.TimeToLicenseMonths[1]) {
			t.Fatalf("%s: time_to_license invalid", name)
		}
		if !(p.TimeFromLicenseToRevMonths[0] > 0 && p.TimeFromLicenseToRevMonths[0] < p.TimeFromLicenseToRevMonths[1]) {
			t.Fatalf("%s: time_from_license_to_revenue invalid", name)
		}
		if !(p.AnnualRevToLicenseeUSD[0] > 0 && p.AnnualRevToLicenseeUSD[0] < p.AnnualRevToLicenseeUSD[1]) {
			t.Fatalf("%s: annual revenue invalid", name)
		}
		if !(p.LicenseDurationYears[0] > 0 && p.LicenseDurationYears[0] < p.LicenseDurationYears[1]) {
			t.Fatalf("%s: license duration invalid", name)
		}
		if !(p.PatentCostRangeUSD[0] > 0 && p.PatentCostRangeUSD[0] < p.PatentCostRangeUSD[1]) {
			t.Fatalf("%s: patent cost invalid", name)
		}
	}
}
