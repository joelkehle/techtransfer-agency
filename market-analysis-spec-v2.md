# Market Analysis Agent — Spec for Codex (v2)

## What This Agent Does

The Market Analysis agent takes an invention disclosure as input and produces a commercial viability assessment for tech transfer officers. It answers the question: "Is this invention worth investing in patenting and licensing, and if so, what should we do next?"

The agent applies structured decision analysis — commercialization path selection, triage scorecard, market sizing, and economic viability — using a deterministic pipeline. The Go code controls stage sequencing, validates outputs, computes all math, and assembles the final report. The LLM provides judgment within stages (reading the disclosure, classifying markets, generating assumption ranges), but never controls flow or computes financial outputs.

This is a Speed 1 (triage) agent. It processes every disclosure quickly and produces a go/no-go/defer recommendation with the key drivers and next diligence actions. Speed 2 (full decision tree + Monte Carlo + EVPI) is a future capability for disclosures that clear triage.

## Architecture

```
Disclosure In
    │
    ▼
┌──────────────────────────────────────────┐
│ Stage 0: Structured Extraction           │  LLM reads disclosure, code validates schema
│                                          │  Nullable fields with missing_reason to
│                                          │  prevent hallucination on sparse disclosures
├──────────────────────────────────────────┤
│ Stage 1: Commercialization Path          │  LLM classifies path, code validates enum
│          Selection                       │  SHORT-CIRCUIT if no plausible monetization
├──────────────────────────────────────────┤
│ Stage 2: Triage Scorecard                │  LLM scores dimensions, code aggregates
│                                          │  SHORT-CIRCUIT if below threshold
├──────────────────────────────────────────┤
│ Stage 3: Market Sizing                   │  LLM estimates TAM/SAM/SOM ranges,
│          (TAM/SAM/SOM)                   │  code validates consistency
│                                          │  HARD GATE: SOM not estimable → DEFER
├──────────────────────────────────────────┤
│ Stage 4: Quick Economic Viability        │  LLM adjusts from domain priors,
│          (Simplified rNPV)               │  CODE computes rNPV with pessimistic/
│                                          │  base/optimistic scenarios
├──────────────────────────────────────────┤
│ Stage 5: Recommendation &                │  Code determines tier. LLM writes
│          Diligence Questions             │  narrative anchored to computed outputs.
├──────────────────────────────────────────┤
│ Stage 6: Report Assembly                 │  Code only — no LLM.
│                                          │  Supports COMPLETE and DEGRADED modes.
└──────────────────────────────────────────┘
    │
    ▼
Structured Report Out
```

The pipeline is sequential. Every stage must complete before the next begins. Short-circuits at Stages 1 and 2 skip to Stage 6 (Report Assembly) with an early termination reason.

If a stage fails validation after retries, the pipeline produces a DEGRADED report containing all successfully completed stages, an explicit incompleteness warning, and a recommendation of DEFER or NO_GO only (never GO). A degraded report must never be mistaken for a complete assessment.

## Domain Default Priors

The agent includes a curated table of domain-default priors embedded in Go code (not LLM-generated). These are conservative starting points that the LLM adjusts with reasoning. Every assumption in the final output is tagged with its source.

```go
// domain_priors.go

type DomainPriors struct {
    Sector                    string
    TypicalRoyaltyRangePct    [2]float64 // low, high as percentages
    PLicense3yr               [2]float64 // P(license within 3 years), low/high
    PCommercialSuccess        [2]float64 // P(commercial success given license), low/high
    TimeToLicenseMonths       [2]int     // low, high
    TimeFromLicenseToRevMonths [2]int    // low, high (additional delay after license)
    AnnualRevToLicenseeUSD    [2]int     // low, high (licensee's annual revenue from product)
    LicenseDurationYears      [2]int     // low, high
    TypicalDealType           string     // "exclusive", "non-exclusive", "equity+royalty"
    PatentCostRangeUSD        [2]int     // provisional through national phase
    TypicalTRL                [2]int     // range of TRL at disclosure stage
    RevenueRampYears          int        // years at half-revenue before full (step ramp)
}

var DefaultPriors = map[string]DomainPriors{
    "software": {
        Sector:                     "software",
        TypicalRoyaltyRangePct:     [2]float64{1.0, 5.0},
        PLicense3yr:                [2]float64{0.05, 0.20},
        PCommercialSuccess:         [2]float64{0.10, 0.40},
        TimeToLicenseMonths:        [2]int{6, 24},
        TimeFromLicenseToRevMonths: [2]int{3, 12},
        AnnualRevToLicenseeUSD:     [2]int{500_000, 10_000_000},
        LicenseDurationYears:       [2]int{5, 15},
        TypicalDealType:            "non-exclusive",
        PatentCostRangeUSD:         [2]int{15_000, 50_000},
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
        AnnualRevToLicenseeUSD:     [2]int{5_000_000, 200_000_000},
        LicenseDurationYears:       [2]int{10, 20},
        TypicalDealType:            "exclusive",
        PatentCostRangeUSD:         [2]int{30_000, 100_000},
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
        AnnualRevToLicenseeUSD:     [2]int{2_000_000, 50_000_000},
        LicenseDurationYears:       [2]int{8, 17},
        TypicalDealType:            "exclusive",
        PatentCostRangeUSD:         [2]int{25_000, 80_000},
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
        AnnualRevToLicenseeUSD:     [2]int{2_000_000, 50_000_000},
        LicenseDurationYears:       [2]int{8, 17},
        TypicalDealType:            "exclusive",
        PatentCostRangeUSD:         [2]int{25_000, 80_000},
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
        AnnualRevToLicenseeUSD:     [2]int{5_000_000, 100_000_000},
        LicenseDurationYears:       [2]int{8, 17},
        TypicalDealType:            "exclusive",
        PatentCostRangeUSD:         [2]int{25_000, 80_000},
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
        AnnualRevToLicenseeUSD:     [2]int{1_000_000, 30_000_000},
        LicenseDurationYears:       [2]int{8, 17},
        TypicalDealType:            "exclusive",
        PatentCostRangeUSD:         [2]int{20_000, 70_000},
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
        AnnualRevToLicenseeUSD:     [2]int{2_000_000, 80_000_000},
        LicenseDurationYears:       [2]int{10, 20},
        TypicalDealType:            "exclusive",
        PatentCostRangeUSD:         [2]int{25_000, 80_000},
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
        AnnualRevToLicenseeUSD:     [2]int{1_000_000, 30_000_000},
        LicenseDurationYears:       [2]int{8, 17},
        TypicalDealType:            "exclusive",
        PatentCostRangeUSD:         [2]int{20_000, 60_000},
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
        AnnualRevToLicenseeUSD:     [2]int{1_000_000, 30_000_000},
        LicenseDurationYears:       [2]int{8, 17},
        TypicalDealType:            "exclusive",
        PatentCostRangeUSD:         [2]int{20_000, 70_000},
        TypicalTRL:                 [2]int{2, 5},
        RevenueRampYears:           2,
    },
}
```

These priors are conservative estimates based on published university licensing data (AUTM surveys, public licensing rate analyses). They will be refined with UCLA TDG-specific data over time. The LLM may adjust from these priors with explicit reasoning, but every adjustment must be tagged.

## Stage Definitions

### Stage 0: Structured Extraction

Purpose: Extract the key commercial elements of the invention from the disclosure text. This parallels the patent screen's extraction but focuses on commercial rather than legal elements.

Input: Raw disclosure text.

LLM task: Read the disclosure and extract the following fields. For fields where the disclosure does not provide sufficient information, return null for the value and provide a missing_reason. Do not fabricate information to fill fields — an explicit "unknown" is always preferred over a plausible guess.

Required output schema:
```json
{
  "invention_title": {
    "value": "string",
    "confidence": "LOW | MEDIUM | HIGH",
    "missing_reason": null
  },
  "problem_solved": {
    "value": "string (what problem does this invention address, for whom)",
    "confidence": "LOW | MEDIUM | HIGH",
    "missing_reason": null
  },
  "solution_description": {
    "value": "string (how the invention works, key technical approach)",
    "confidence": "LOW | MEDIUM | HIGH",
    "missing_reason": null
  },
  "claimed_advantages": {
    "value": ["string (each advantage the inventor claims over existing approaches)"] | null,
    "confidence": "LOW | MEDIUM | HIGH",
    "missing_reason": "string or null"
  },
  "target_user": {
    "value": "string (who would use this — be specific, e.g., 'clinical laboratory technicians' not 'healthcare')" | null,
    "confidence": "LOW | MEDIUM | HIGH",
    "missing_reason": "string or null"
  },
  "target_buyer": {
    "value": "string (who pays — may differ from user, e.g., 'hospital procurement departments')" | null,
    "confidence": "LOW | MEDIUM | HIGH",
    "missing_reason": "string or null"
  },
  "application_domains": {
    "value": ["string (specific application areas, e.g., 'point-of-care diagnostics for sepsis')"] | null,
    "confidence": "LOW | MEDIUM | HIGH",
    "missing_reason": "string or null"
  },
  "evidence_level": "CONCEPT_ONLY | IN_VITRO | ANIMAL | PROTOTYPE | PILOT | CLINICAL",
  "competing_approaches": {
    "value": ["string (alternatives mentioned or implied in the disclosure)"] | null,
    "confidence": "LOW | MEDIUM | HIGH",
    "missing_reason": "string or null"
  },
  "dependencies": {
    "value": ["string (special equipment, materials, regulatory requirements, standards)"] | null,
    "confidence": "LOW | MEDIUM | HIGH",
    "missing_reason": "string or null"
  },
  "sector": "string (map to one of: software, biotech_therapeutic, biotech_diagnostic, medical_device, semiconductor, materials, clean_energy, mechanical_engineering, or default)"
}
```

Validation (code):
- Required non-null fields: `invention_title.value`, `problem_solved.value`, `solution_description.value`. These three fields are always extractable — if the disclosure doesn't contain them, it's not a usable disclosure.
- All other `.value` fields may be null if and only if `.missing_reason` is non-empty.
- `evidence_level` must be one of the six enums.
- `sector` must match a key in the DefaultPriors table.
- All `confidence` fields must be one of the three enums.
- If validation fails, retry up to 2 times (3 attempts total).

### Stage 1: Commercialization Path Selection

Purpose: Determine the most likely route to market. This must happen before market sizing because TAM/SAM/SOM depend on what the "product" is and who buys it.

Input: Stage 0 output.

LLM prompt context (include verbatim):
```
Classify the most likely commercialization path(s) for this invention.
Select one primary path and optionally one secondary path.

EXCLUSIVE_LICENSE_INCUMBENT
  The invention fits into an existing company's product line. The university
  licenses exclusively to one company in exchange for upfront fees, milestones,
  and royalties. Most common for medical devices, therapeutics, and specialty
  chemicals.

STARTUP_FORMATION
  The invention is the core of a new company. The university takes equity
  and/or royalties. Common for platform technologies, novel therapeutics
  with no obvious incumbent, and deep tech.

NON_EXCLUSIVE_LICENSE
  The invention is a tool, method, or enabling technology useful to many
  players. Licensed non-exclusively to multiple companies at lower royalty
  rates but broader adoption. Common for research tools, software libraries,
  and materials/coatings.

OPEN_SOURCE_PLUS_SERVICES
  The invention is best distributed as open source software or open
  standards, with revenue from services, support, or complementary products.
  Common for software infrastructure and developer tools.

RESEARCH_USE_ONLY
  The invention has academic value but no clear commercial path. Best
  distributed for research use, potentially generating sponsored research
  relationships but not license revenue.

For each selected path, explain why it fits this invention. If the invention
does not have a plausible monetization path, say so explicitly.

Note: if the invention has commercial value but NOT via patents (e.g.,
copyright, know-how licensing, data licensing, services), identify the
primary path and note "non_patent_monetization": true with an explanation.
```

Required output schema:
```json
{
  "primary_path": "EXCLUSIVE_LICENSE_INCUMBENT | STARTUP_FORMATION | NON_EXCLUSIVE_LICENSE | OPEN_SOURCE_PLUS_SERVICES | RESEARCH_USE_ONLY",
  "primary_path_reasoning": "string",
  "secondary_path": "string enum or null",
  "secondary_path_reasoning": "string or null",
  "product_definition": "string (what the actual product/offering would be — not the technique, the thing someone buys or uses)",
  "has_plausible_monetization": true | false,
  "no_monetization_reasoning": "string or null",
  "non_patent_monetization": true | false,
  "non_patent_monetization_reasoning": "string or null"
}
```

Validation (code):
- primary_path must be one of the five enums.
- primary_path_reasoning must be non-empty.
- secondary_path must be one of the five enums or null.
- If secondary_path is non-null, secondary_path_reasoning must be non-empty.
- secondary_path must not equal primary_path.
- product_definition must be non-empty.
- If has_plausible_monetization is false, no_monetization_reasoning must be non-empty.
- If non_patent_monetization is true, non_patent_monetization_reasoning must be non-empty.

Flow control (code):
- If has_plausible_monetization is false → pipeline skips to Stage 6 with NO_GO determination and reason "No plausible commercialization path identified." Report includes recommended non-patent actions (publish, open dissemination, sponsored research).

### Stage 2: Triage Scorecard

Purpose: Fast structured assessment across six dimensions. Produces a composite score that determines whether deeper analysis (Stages 3-5) is warranted.

Input: Stage 0 output + Stage 1 output.

LLM task: Score each of the six dimensions on a 1-5 scale with brief reasoning.

LLM prompt context (include verbatim):
```
Score each dimension from 1 (very weak) to 5 (very strong). Provide a
1-2 sentence justification for each score. Be conservative — default to
3 unless the disclosure provides clear evidence for a higher or lower score.

MARKET_PAIN (1-5)
  How severe is the problem this invention solves? Is there demonstrated
  willingness to pay for solutions? Score 5 if the problem causes significant
  measurable cost/harm and buyers actively seek solutions. Score 1 if the
  problem is minor, theoretical, or no one is looking for a fix.

DIFFERENTIATION (1-5)
  How clearly does this invention improve on existing approaches? Score 5
  if the advantage is large, measurable, and hard to replicate. Score 1
  if the improvement is marginal or many alternatives exist with similar
  performance.

ADOPTION_FRICTION (1-5, where 5 = LOW friction = good)
  How easy is it for the target customer to adopt this invention? Consider:
  workflow changes required, integration complexity, switching costs,
  training needed, infrastructure requirements. Score 5 if drop-in
  replacement. Score 1 if requires fundamental workflow change or new
  infrastructure.

DEVELOPMENT_BURDEN (1-5, where 5 = LOW burden = good)
  How much work remains to reach a commercially viable product? Consider:
  current TRL/evidence level, regulatory requirements, manufacturing
  complexity, timeline to market. Score 5 if near-ready. Score 1 if
  10+ years and heavy regulatory path.

PARTNER_DENSITY (1-5)
  How many plausible licensees, acquirers, or partners exist for this
  technology? Score 5 if 10+ identifiable companies with clear product-
  line fit. Score 1 if no obvious buyer or highly fragmented market with
  no natural licensee.

IP_LEVERAGE (1-5)
  Based on the disclosure alone (not a formal patent analysis), how strong
  is the likely IP position? Consider: is the invention specific and
  concrete vs. a general idea? Are design-arounds obvious? Would a
  competitor need this specific approach? Score 5 if strong, specific,
  hard to design around. Score 1 if the invention is a general concept
  with obvious alternatives.
```

Required output schema:
```json
{
  "scores": {
    "market_pain": {"score": 1-5, "reasoning": "string"},
    "differentiation": {"score": 1-5, "reasoning": "string"},
    "adoption_friction": {"score": 1-5, "reasoning": "string"},
    "development_burden": {"score": 1-5, "reasoning": "string"},
    "partner_density": {"score": 1-5, "reasoning": "string"},
    "ip_leverage": {"score": 1-5, "reasoning": "string"}
  },
  "confidence": "LOW | MEDIUM | HIGH",
  "confidence_reasoning": "string (what information is missing that limits confidence)",
  "unknown_key_factors": ["string (list any of: buyer, pricing, regulatory, competitive_landscape, adoption_requirements that are unknown)"]
}
```

Validation (code):
- All six scores must be integers 1-5.
- All reasoning strings must be non-empty.
- confidence must be one of the three enums.
- unknown_key_factors must be an array (may be empty).

Aggregation (code — not LLM):
```
composite_score = (market_pain + differentiation + adoption_friction +
                   development_burden + partner_density + ip_leverage) / 6.0

// Weighted version gives more weight to market and differentiation:
weighted_score = (market_pain * 2 + differentiation * 2 + adoption_friction * 1.5 +
                  development_burden * 1 + partner_density * 1.5 + ip_leverage * 1) / 9.0
```

Flow control (code):
- If weighted_score < 2.0 → pipeline skips to Stage 6 with NO_GO determination. Reason cites the lowest-scoring dimensions.
- If weighted_score >= 2.0 AND weighted_score < 2.5 AND confidence == "LOW" → pipeline skips to Stage 6 with DEFER determination. Reason: "Insufficient information for confident triage. Recommend inventor meeting to clarify [top uncertainty]."
- If confidence == "LOW" AND len(unknown_key_factors) >= 2 → pipeline skips to Stage 6 with DEFER determination. Reason: "Key commercial factors unknown: [factors]. Recommend inventor meeting before further analysis."
- Otherwise → pipeline continues to Stage 3.

### Stage 3: Market Sizing (TAM/SAM/SOM)

Purpose: Estimate market size ranges for the defined product (from Stage 1) in the target segments (from Stage 0). Uses ranges, not point estimates.

Input: Stage 0 output + Stage 1 output + Stage 2 output.

LLM prompt context (include verbatim):
```
Estimate market size ranges for the defined product. Use ranges (low/high),
not single numbers. Be explicit about what you're measuring (revenue,
units, patients, etc.).

TAM (Total Addressable Market):
  The total annual revenue opportunity if every possible customer in every
  possible segment adopted this type of solution. This is the broadest
  reasonable definition of the market.

SAM (Serviceable Addressable Market):
  The subset of TAM that this specific product could realistically serve,
  given geographic, regulatory, channel, and technical constraints.

SOM (Serviceable Obtainable Market):
  The subset of SAM that a licensee could credibly capture within 5 years,
  given competitive dynamics, adoption friction, and market penetration
  rates for comparable technologies.

For each estimate, provide:
- The range (low and high, in USD annual revenue)
- The key assumptions driving the estimate
- The source type for each assumption: DISCLOSURE_DERIVED (stated in the
  disclosure), INFERRED (reasonable inference from disclosure), or
  ESTIMATED (general knowledge, not from disclosure)

If you cannot estimate a credible range because the disclosure lacks
sufficient information, set estimable to false and explain what's missing.

IMPORTANT: SOM is required for downstream economic analysis. If you truly
cannot estimate SOM, the pipeline will defer the disclosure for further
information gathering rather than proceed with unreliable numbers.
```

Required output schema:
```json
{
  "tam": {
    "low_usd": 0,
    "high_usd": 0,
    "unit": "string (e.g., 'annual revenue' or 'annual unit sales x ASP')",
    "assumptions": [{"assumption": "string", "source": "DISCLOSURE_DERIVED | INFERRED | ESTIMATED"}],
    "estimable": true | false,
    "not_estimable_reason": "string or null"
  },
  "sam": {
    "low_usd": 0,
    "high_usd": 0,
    "unit": "string",
    "assumptions": [{"assumption": "string", "source": "DISCLOSURE_DERIVED | INFERRED | ESTIMATED"}],
    "estimable": true | false,
    "not_estimable_reason": "string or null"
  },
  "som": {
    "low_usd": 0,
    "high_usd": 0,
    "unit": "string",
    "assumptions": [{"assumption": "string", "source": "DISCLOSURE_DERIVED | INFERRED | ESTIMATED"}],
    "estimable": true | false,
    "not_estimable_reason": "string or null"
  },
  "tam_som_ratio_warning": "string or null (if TAM is huge but SOM is tiny, explain why)"
}
```

Validation (code):
- If estimable is true: low_usd and high_usd must be > 0, and low_usd <= high_usd.
- If estimable is false: not_estimable_reason must be non-empty; low_usd and high_usd must be 0.
- If estimable is true: not_estimable_reason must be null.
- SAM must be <= TAM (both low and high).
- SOM must be <= SAM (both low and high).
- Each assumptions array must have at least one entry.

Flow control (code — hard gate):
- If som.estimable is false → pipeline skips to Stage 6 with DEFER determination. Reason: "Cannot estimate serviceable obtainable market. [som.not_estimable_reason]. Recommend inventor meeting to clarify target customer, pricing basis, and competitive positioning before economic analysis." Diligence questions are generated from the not_estimable_reason.
- If som.estimable is true → pipeline continues to Stage 4.

### Stage 4: Quick Economic Viability (Simplified rNPV)

Purpose: Compute a simplified risk-adjusted NPV to determine whether the expected value of licensing exceeds the cost of patenting. This is the "is it worth the option purchase?" test.

Input: Stage 0 output (sector) + Stage 1 output (primary_path) + Stage 2 output (scores) + Stage 3 output (SOM) + domain priors for the sector.

LLM task: Adjust the domain-default priors based on the specific disclosure. The LLM proposes adjustments; the code computes the math.

LLM prompt context (include verbatim):
```
Given the invention details and the domain default assumptions below,
propose adjustments where the disclosure provides evidence for deviation
from defaults. For each adjustment, explain why.

You must output assumption ranges, NOT computed financial results. The
financial computation is done by the system, not by you.

For each assumption, provide:
- Your adjusted range (low, high)
- Whether you adjusted from the default and why
- Source: DOMAIN_DEFAULT (unchanged from the prior provided),
  ADJUSTED (changed from default with reasoning),
  or DISCLOSURE_DERIVED (directly supported by disclosure text)

IMPORTANT: Every variable has a domain default provided below. If you
have no disclosure-specific reason to adjust, keep the domain default
and mark source as DOMAIN_DEFAULT. Do not invent adjustments.
```

The LLM receives the domain priors for the matched sector and the disclosure details. It outputs:

Required output schema:
```json
{
  "royalty_rate_pct": {
    "low": 0.0, "high": 0.0,
    "source": "DOMAIN_DEFAULT | ADJUSTED | DISCLOSURE_DERIVED",
    "reasoning": "string"
  },
  "p_license_3yr": {
    "low": 0.0, "high": 0.0,
    "source": "DOMAIN_DEFAULT | ADJUSTED | DISCLOSURE_DERIVED",
    "reasoning": "string"
  },
  "p_commercial_success": {
    "low": 0.0, "high": 0.0,
    "source": "DOMAIN_DEFAULT | ADJUSTED | DISCLOSURE_DERIVED",
    "reasoning": "string"
  },
  "time_to_license_months": {
    "low": 0, "high": 0,
    "source": "DOMAIN_DEFAULT | ADJUSTED | DISCLOSURE_DERIVED",
    "reasoning": "string"
  },
  "time_from_license_to_revenue_months": {
    "low": 0, "high": 0,
    "source": "DOMAIN_DEFAULT | ADJUSTED | DISCLOSURE_DERIVED",
    "reasoning": "string"
  },
  "annual_revenue_to_licensee_usd": {
    "low": 0, "high": 0,
    "source": "DOMAIN_DEFAULT | ADJUSTED | DISCLOSURE_DERIVED",
    "reasoning": "string"
  },
  "license_duration_years": {
    "low": 0, "high": 0,
    "source": "DOMAIN_DEFAULT | ADJUSTED | DISCLOSURE_DERIVED",
    "reasoning": "string"
  },
  "patent_cost_usd": {
    "low": 0, "high": 0,
    "source": "DOMAIN_DEFAULT | ADJUSTED | DISCLOSURE_DERIVED",
    "reasoning": "string"
  }
}
```

Validation (code):
- All probabilities must be in [0.0, 1.0].
- All monetary values must be >= 0.
- All time values must be > 0.
- For every ranged assumption, low must be <= high.
- royalty_rate_pct must be in [0.0, 25.0] (sanity ceiling).
- Each source must be one of the three enums.
- Each reasoning must be non-empty.
- time_to_license + time_from_license_to_revenue > 0.

Path-specific adjustments (code, applied after LLM output):
- If primary_path is STARTUP_FORMATION: set `path_model_limitation = "Royalty-based NPV model undervalues startup paths where equity dominates. Treat NPV as a lower bound. Recommend separate startup valuation if GO or DEFER."` Force confidence to LOW if not already LOW.
- If primary_path is OPEN_SOURCE_PLUS_SERVICES: set `path_model_limitation = "Royalty-based NPV model does not capture open source economics (services, support, complementary products). Patent investment may not be primary value driver."` Force confidence to LOW. Bias recommendation toward DEFER unless IP leverage score >= 4.
- If primary_path is RESEARCH_USE_ONLY: this path should have been caught by Stage 1 short-circuit. If it reaches Stage 4, force DEFER.

Scenario construction (code — never LLM):

Each assumption has an inherent directionality — whether a higher value produces a better or worse financial outcome.

```go
// Directionality per assumption:
// "higher is better":  royalty_rate_pct, p_license_3yr, p_commercial_success,
//                      annual_revenue_to_licensee_usd, license_duration_years
// "lower is better":   time_to_license_months, time_from_license_to_revenue_months,
//                      patent_cost_usd

// Pessimistic scenario: worst-case for each assumption
//   "higher is better" variables → use LOW value
//   "lower is better" variables → use HIGH value

// Optimistic scenario: best-case for each assumption
//   "higher is better" variables → use HIGH value
//   "lower is better" variables → use LOW value

// Base scenario: midpoint of each range
```

Computation (code — never LLM):
```
// For each scenario (pessimistic / base / optimistic):

// 1. Compute time-to-first-revenue
time_to_first_revenue_months = time_to_license_months + time_from_license_to_revenue_months

// 2. Compute annual royalty
annual_royalty = annual_revenue_to_licensee_usd * (royalty_rate_pct / 100)

// 3. Risk-adjust
risk_adjusted_annual = annual_royalty * p_license_3yr * p_commercial_success

// 4. Apply revenue ramp (step function from domain priors)
//    For the first RevenueRampYears after revenue starts: half revenue
//    After that: full revenue
ramp_years = sector_priors.RevenueRampYears

// 5. Discount to present value
discount_rate = 0.10  // standard for university tech transfer
npv = 0
for year in range(1, license_duration_years + 1):
    months_into_deal = year * 12
    if months_into_deal < time_to_first_revenue_months:
        continue  // no revenue yet
    years_of_revenue = (months_into_deal - time_to_first_revenue_months) / 12
    if years_of_revenue <= ramp_years:
        year_revenue = risk_adjusted_annual * 0.5  // ramp period
    else:
        year_revenue = risk_adjusted_annual         // full revenue
    npv += year_revenue / (1 + discount_rate) ^ year

// 6. Compare against patent cost
cost_mid = (patent_cost_low + patent_cost_high) / 2
viable = npv > cost_mid
```

Sensitivity analysis (code):
```
// For each input assumption:
//   Hold all others at base (midpoint)
//   Set target to its low bound → compute NPV_low
//   Set target to its high bound → compute NPV_high
//   delta = abs(NPV_high - NPV_low)
// Rank by delta descending. Report top 3.
```

Output (computed by code):
```json
{
  "scenarios": {
    "pessimistic": {"npv_usd": 0, "exceeds_patent_cost": false},
    "base": {"npv_usd": 0, "exceeds_patent_cost": false},
    "optimistic": {"npv_usd": 0, "exceeds_patent_cost": false}
  },
  "patent_cost_mid_usd": 0,
  "sensitivity_drivers": [
    {"assumption": "string", "npv_delta_usd": 0, "direction": "string (e.g., 'Higher royalty rate increases NPV by $X')"}
  ],
  "path_model_limitation": "string or null",
  "revenue_ramp_years": 0
}
```

### Stage 5: Recommendation & Diligence Questions

Purpose: Produce a human-readable recommendation anchored to the computed outputs from prior stages. The LLM writes the narrative, but the recommendation tier is determined by code.

Determination logic (code):
```
IF pipeline is in DEGRADED mode (a stage failed after retries):
    recommendation = DEFER
    reason = "Analysis incomplete due to stage failure. [failed_stage] could not be completed."
    // DEGRADED reports can only be DEFER or NO_GO, never GO.

ELSE IF Stage 1 short-circuited (no monetization):
    recommendation = NO_GO
    reason = "No plausible commercialization path"

ELSE IF Stage 2 short-circuited (low scorecard or high uncertainty):
    recommendation = NO_GO or DEFER (per Stage 2 logic)
    reason = per Stage 2

ELSE IF Stage 3 short-circuited (SOM not estimable):
    recommendation = DEFER
    reason = per Stage 3

ELSE IF primary_path == OPEN_SOURCE_PLUS_SERVICES:
    IF stage2.scores.ip_leverage.score < 4:
        recommendation = DEFER
        confidence = LOW
        reason = path_model_limitation + " IP leverage insufficient for patent investment in open source context."
    ELSE IF scenarios.base.exceeds_patent_cost:
        recommendation = GO
        confidence = LOW
        caveat = path_model_limitation
    ELSE:
        recommendation = DEFER
        confidence = LOW
        reason = path_model_limitation + " Recommend specialized valuation."

ELSE IF path_model_limitation is set (startup or open source path):
    // Model acknowledges it can't properly value these paths
    IF scenarios.base.exceeds_patent_cost:
        recommendation = GO
        confidence = LOW
        caveat = path_model_limitation
    ELSE:
        recommendation = DEFER
        confidence = LOW
        reason = path_model_limitation + "Recommend specialized valuation."

ELSE IF scenarios.base.exceeds_patent_cost AND scenarios.pessimistic.exceeds_patent_cost:
    recommendation = GO
    confidence = HIGH

ELSE IF scenarios.base.exceeds_patent_cost AND NOT scenarios.pessimistic.exceeds_patent_cost:
    recommendation = GO
    confidence = MEDIUM

ELSE IF scenarios.optimistic.exceeds_patent_cost AND NOT scenarios.base.exceeds_patent_cost:
    recommendation = DEFER
    confidence = LOW
    reason = "Viable only under optimistic assumptions"

ELSE:
    recommendation = NO_GO
    reason = "Expected value does not exceed patent costs under any scenario"

// Final override: if Stage 2 confidence is LOW and 2+ unknown_key_factors,
// cap recommendation at DEFER regardless of NPV outcome.
IF stage2.confidence == LOW AND len(stage2.unknown_key_factors) >= 2:
    IF recommendation == GO:
        recommendation = DEFER
        reason = "Key commercial factors unknown: " + factors + ". NPV analysis is unreliable without these inputs."
```

Input to LLM: All prior stage outputs + the computed recommendation tier + the sensitivity drivers.

LLM task: Write a narrative recommendation that is anchored to the numbers. Do not contradict the computed recommendation tier.

Required output schema:
```json
{
  "executive_summary": "string (2-3 sentences: what the invention is, the recommendation, and the primary reason)",
  "key_drivers": ["string (top 3-5 factors driving the recommendation, tied to scorecard and sensitivity)"],
  "diligence_questions": ["string (top 3-5 questions that would most change the assessment — tied to sensitivity drivers and low-confidence assumptions)"],
  "recommended_actions": ["string (specific next steps for the TTO officer)"],
  "non_patent_actions": ["string or empty (if NO_GO: alternative actions like publish, open disseminate, sponsored research)"],
  "model_limitations": ["string or empty (any path_model_limitation or confidence caveats)"]
}
```

Validation (code):
- executive_summary must be non-empty.
- key_drivers must have 3-5 entries.
- diligence_questions must have 3-5 entries.
- recommended_actions must have at least 1 entry.

### Stage 6: Report Assembly

Purpose: Assemble the final structured report from all stage outputs. This stage is pure code — no LLM call.

Input: Outputs from all completed stages + the pipeline flow path (which stages executed, which short-circuited, whether pipeline is in DEGRADED mode).

Report modes:
- COMPLETE: All stages executed successfully (including short-circuit paths, which are intentional).
- DEGRADED: One or more stages failed after retries. Report contains all successfully completed stages plus an explicit incompleteness warning. Recommendation is capped at DEFER or NO_GO.

The report must contain:

1. Header: Case ID, invention title, date, report mode (COMPLETE or DEGRADED), disclaimer ("This is a preliminary automated market assessment, not a valuation or investment recommendation. Estimates are based on limited disclosure information and domain default assumptions.")

2. If DEGRADED: prominent warning block: "INCOMPLETE ANALYSIS: [failed_stage] could not be completed. This report contains partial results only. Do not treat this as a complete assessment."

3. Executive Summary: From Stage 5 (or generated by code if pipeline short-circuited early).

4. Recommendation: One of three values:
   - `GO` (with confidence HIGH, MEDIUM, or LOW + caveats)
   - `DEFER` (with conditions and diligence questions)
   - `NO_GO` (with reason and non-patent alternatives)

5. Commercialization Path: From Stage 1. Includes product definition, primary/secondary paths, and any non-patent monetization notes.

6. Triage Scorecard: All six dimensions with scores and reasoning. Composite and weighted scores.

7. Market Sizing: TAM/SAM/SOM ranges with assumptions. Warnings about TAM/SOM ratio if applicable. If SOM was not estimable, the section says so and explains why.

8. Economic Viability: Three scenarios (pessimistic/base/optimistic) with NPV, patent cost comparison, and viability flag. Revenue ramp applied. Sensitivity drivers (top 3). Path model limitations if applicable.

9. Diligence Questions: Ranked by expected impact on the decision.

10. Recommended Next Steps: From Stage 5.

11. Model Limitations: Any path-specific limitations, confidence caveats, or degraded-mode warnings collected in one place.

12. Assumption Audit Trail: Every numeric assumption with its value, source type (DOMAIN_DEFAULT / ADJUSTED / DISCLOSURE_DERIVED / INFERRED / ESTIMATED), and reasoning. This is the auditability layer.

13. Appendix: Full structured data from each stage (JSON) for machine consumption.

Output format: Report body is plain text/markdown. Appendix is JSON. Both included in the bus message body.

## LLM Integration

Identical pattern to the Patent Eligibility Screen agent:

- Model: Claude (via Anthropic API, ANTHROPIC_API_KEY from environment).
- System prompt: "You are a technology commercialization analyst conducting a preliminary market assessment for a university tech transfer office. You provide structured, evidence-based analysis. You are conservative in your estimates and explicit about uncertainty. You never fabricate market data — if you don't know, you say so and explain what information would be needed."
- Temperature: 0.
- All outputs are JSON with schema validation.
- Maximum 3 attempts per stage (1 initial + 2 retries).
- Stage-specific context blocks included verbatim in prompts.

## Bus Integration

- Registers with capability: `market-analysis`
- Receives requests containing disclosure text in message body
- Posts progress events at each stage transition
- Posts final event with complete report (marked COMPLETE or DEGRADED)
- Sends response message back to sender

## Error Handling

- Stage 0 failure after retries → immediate error, no report. DEGRADED mode applies only to failures in Stages 1-5.
- Stage failure after retries → pipeline enters DEGRADED mode. Completes remaining stages if possible, otherwise assembles report from completed stages. Recommendation capped at DEFER or NO_GO.
- Empty/too-short disclosure → immediate error, no report.
- API unreachable → exponential backoff, 3 failures → DEGRADED mode or error if no stages completed.
- A DEGRADED report is always better than no report, but a DEGRADED report must never recommend GO.

## File Structure

```
internal/marketanalysis/
    agent.go              // Bus registration, inbox polling, message handling
    pipeline.go           // Stage orchestrator with short-circuit + degraded mode
    stages.go             // Individual stage implementations
    stages_test.go        // Unit tests for each stage (mock LLM responses)
    llm.go                // Anthropic API client wrapper
    llm_test.go           // LLM client tests
    domain_priors.go      // Curated prior tables (all sectors + new fields)
    domain_priors_test.go // Prior table validation tests
    rnpv.go               // rNPV computation, scenario builder, sensitivity analysis
    rnpv_test.go          // Financial computation tests (deterministic, no LLM)
    report.go             // Stage 6 report assembly (COMPLETE + DEGRADED modes)
    report_test.go        // Report assembly tests
    types.go              // All type definitions, schemas, enums
```

cmd/market-analysis/main.go: Binary entry point.

## Testing Strategy

Each stage: unit tests with mocked LLM responses (happy path, retry, failure, schema validation).

Pipeline: integration tests covering:
- Full pipeline with all stages (COMPLETE mode)
- Short-circuit at Stage 1 (no monetization)
- Short-circuit at Stage 2 (low scorecard)
- Short-circuit at Stage 2 (low confidence + 2+ unknown factors)
- Short-circuit at Stage 3 (SOM not estimable)
- DEGRADED mode: Stage 3 fails → report contains Stages 0-2 + DEFER
- DEGRADED mode: Stage 4 fails → report contains Stages 0-3 + DEFER
- Path-specific behavior: STARTUP_FORMATION → confidence forced LOW, limitation noted
- Path-specific behavior: OPEN_SOURCE_PLUS_SERVICES → confidence forced LOW, DEFER unless high IP leverage
- Stage 5 override: GO downgraded to DEFER when Stage 2 has LOW confidence + 2 unknown factors

Financial computations (rnpv.go): separate test file with known inputs and expected outputs. These tests must not involve any LLM calls — they validate:
- Pessimistic/base/optimistic scenario construction (correct directionality)
- Revenue ramp step function
- Time-to-first-revenue = time-to-license + time-from-license-to-revenue
- NPV discounting
- Sensitivity analysis using actual assumption bounds
- Patent cost comparison

Domain priors: test that all sectors have valid ranges:
- low < high for all range fields
- probabilities in [0,1]
- costs > 0
- time values > 0
- RevenueRampYears > 0
- All sectors present in map including "default"

## Future: Speed 2 (not built now, design for it)

The Speed 1 pipeline outputs enough structured data that Speed 2 can consume it later:
- Decision tree with abandon/continue nodes at each patent investment stage (provisional, conversion, national phase)
- Full Monte Carlo with correlated distributions
- EVPI/EVSI for each diligence action
- Comparable deal analysis against a curated database
- Path-specific financial models (startup equity valuation, platform multi-license, open source services)

Speed 2 would be triggered by the TTO officer explicitly requesting deeper analysis on a GO or DEFER disclosure. The architecture supports this because all Speed 1 outputs are structured JSON that Speed 2 can ingest as priors.
