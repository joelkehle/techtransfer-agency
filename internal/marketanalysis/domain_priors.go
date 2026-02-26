package marketanalysis

type DomainPriors struct {
	Sector                     string
	TypicalRoyaltyRangePct     [2]float64
	PLicense3yr                [2]float64
	PCommercialSuccess         [2]float64
	TimeToLicenseMonths        [2]int
	TimeFromLicenseToRevMonths [2]int
	AnnualRevToLicenseeUSD     [2]int
	LicenseDurationYears       [2]int
	TypicalDealType            string
	PatentCostRangeUSD         [2]int
	TypicalTRL                 [2]int
	RevenueRampYears           int
}

var DefaultPriors = map[string]DomainPriors{
	"software": {
		Sector:                     "software",
		TypicalRoyaltyRangePct:     [2]float64{1.0, 5.0},
		PLicense3yr:                [2]float64{0.05, 0.20},
		PCommercialSuccess:         [2]float64{0.10, 0.40},
		TimeToLicenseMonths:        [2]int{6, 24},
		TimeFromLicenseToRevMonths: [2]int{3, 12},
		AnnualRevToLicenseeUSD:     [2]int{500000, 10000000},
		LicenseDurationYears:       [2]int{5, 15},
		TypicalDealType:            "non-exclusive",
		PatentCostRangeUSD:         [2]int{15000, 50000},
		TypicalTRL:                 [2]int{3, 6},
		RevenueRampYears:           2,
	},
	"biotech_therapeutic": {
		Sector:                     "biotech_therapeutic",
		TypicalRoyaltyRangePct:     [2]float64{3.0, 8.0},
		PLicense3yr:                [2]float64{0.02, 0.10},
		PCommercialSuccess:         [2]float64{0.05, 0.20},
		TimeToLicenseMonths:        [2]int{12, 48},
		TimeFromLicenseToRevMonths: [2]int{36, 96},
		AnnualRevToLicenseeUSD:     [2]int{5000000, 200000000},
		LicenseDurationYears:       [2]int{10, 20},
		TypicalDealType:            "exclusive",
		PatentCostRangeUSD:         [2]int{30000, 100000},
		TypicalTRL:                 [2]int{1, 4},
		RevenueRampYears:           3,
	},
	"biotech_diagnostic": {
		Sector:                     "biotech_diagnostic",
		TypicalRoyaltyRangePct:     [2]float64{3.0, 7.0},
		PLicense3yr:                [2]float64{0.05, 0.15},
		PCommercialSuccess:         [2]float64{0.10, 0.30},
		TimeToLicenseMonths:        [2]int{12, 36},
		TimeFromLicenseToRevMonths: [2]int{12, 36},
		AnnualRevToLicenseeUSD:     [2]int{2000000, 50000000},
		LicenseDurationYears:       [2]int{8, 17},
		TypicalDealType:            "exclusive",
		PatentCostRangeUSD:         [2]int{25000, 80000},
		TypicalTRL:                 [2]int{2, 5},
		RevenueRampYears:           2,
	},
	"medical_device": {
		Sector:                     "medical_device",
		TypicalRoyaltyRangePct:     [2]float64{3.0, 7.0},
		PLicense3yr:                [2]float64{0.05, 0.15},
		PCommercialSuccess:         [2]float64{0.10, 0.30},
		TimeToLicenseMonths:        [2]int{12, 36},
		TimeFromLicenseToRevMonths: [2]int{12, 36},
		AnnualRevToLicenseeUSD:     [2]int{2000000, 50000000},
		LicenseDurationYears:       [2]int{8, 17},
		TypicalDealType:            "exclusive",
		PatentCostRangeUSD:         [2]int{25000, 80000},
		TypicalTRL:                 [2]int{2, 5},
		RevenueRampYears:           2,
	},
	"semiconductor": {
		Sector:                     "semiconductor",
		TypicalRoyaltyRangePct:     [2]float64{1.0, 4.0},
		PLicense3yr:                [2]float64{0.03, 0.12},
		PCommercialSuccess:         [2]float64{0.10, 0.30},
		TimeToLicenseMonths:        [2]int{12, 36},
		TimeFromLicenseToRevMonths: [2]int{12, 30},
		AnnualRevToLicenseeUSD:     [2]int{5000000, 100000000},
		LicenseDurationYears:       [2]int{8, 17},
		TypicalDealType:            "exclusive",
		PatentCostRangeUSD:         [2]int{25000, 80000},
		TypicalTRL:                 [2]int{2, 5},
		RevenueRampYears:           2,
	},
	"materials": {
		Sector:                     "materials",
		TypicalRoyaltyRangePct:     [2]float64{2.0, 5.0},
		PLicense3yr:                [2]float64{0.03, 0.12},
		PCommercialSuccess:         [2]float64{0.10, 0.25},
		TimeToLicenseMonths:        [2]int{12, 36},
		TimeFromLicenseToRevMonths: [2]int{12, 36},
		AnnualRevToLicenseeUSD:     [2]int{1000000, 30000000},
		LicenseDurationYears:       [2]int{8, 17},
		TypicalDealType:            "exclusive",
		PatentCostRangeUSD:         [2]int{20000, 70000},
		TypicalTRL:                 [2]int{2, 5},
		RevenueRampYears:           2,
	},
	"clean_energy": {
		Sector:                     "clean_energy",
		TypicalRoyaltyRangePct:     [2]float64{2.0, 6.0},
		PLicense3yr:                [2]float64{0.03, 0.10},
		PCommercialSuccess:         [2]float64{0.05, 0.25},
		TimeToLicenseMonths:        [2]int{12, 48},
		TimeFromLicenseToRevMonths: [2]int{18, 48},
		AnnualRevToLicenseeUSD:     [2]int{2000000, 80000000},
		LicenseDurationYears:       [2]int{10, 20},
		TypicalDealType:            "exclusive",
		PatentCostRangeUSD:         [2]int{25000, 80000},
		TypicalTRL:                 [2]int{2, 5},
		RevenueRampYears:           3,
	},
	"mechanical_engineering": {
		Sector:                     "mechanical_engineering",
		TypicalRoyaltyRangePct:     [2]float64{2.0, 5.0},
		PLicense3yr:                [2]float64{0.05, 0.15},
		PCommercialSuccess:         [2]float64{0.10, 0.30},
		TimeToLicenseMonths:        [2]int{12, 30},
		TimeFromLicenseToRevMonths: [2]int{6, 24},
		AnnualRevToLicenseeUSD:     [2]int{1000000, 30000000},
		LicenseDurationYears:       [2]int{8, 17},
		TypicalDealType:            "exclusive",
		PatentCostRangeUSD:         [2]int{20000, 60000},
		TypicalTRL:                 [2]int{3, 6},
		RevenueRampYears:           2,
	},
	"default": {
		Sector:                     "default",
		TypicalRoyaltyRangePct:     [2]float64{2.0, 5.0},
		PLicense3yr:                [2]float64{0.05, 0.12},
		PCommercialSuccess:         [2]float64{0.10, 0.25},
		TimeToLicenseMonths:        [2]int{12, 36},
		TimeFromLicenseToRevMonths: [2]int{12, 30},
		AnnualRevToLicenseeUSD:     [2]int{1000000, 30000000},
		LicenseDurationYears:       [2]int{8, 17},
		TypicalDealType:            "exclusive",
		PatentCostRangeUSD:         [2]int{20000, 70000},
		TypicalTRL:                 [2]int{2, 5},
		RevenueRampYears:           2,
	},
}

func PriorForSector(sector string) DomainPriors {
	if p, ok := DefaultPriors[sector]; ok {
		return p
	}
	return DefaultPriors["default"]
}
