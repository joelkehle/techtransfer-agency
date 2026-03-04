package operator

import (
	"strings"
	"testing"
)

func TestBuildPriorArtReportMarkdown(t *testing.T) {
	env := map[string]any{
		"agent":         "prior-art-search",
		"case_id":       "SUB-123",
		"ucla_case_id":  "2026-124",
		"pi_name":       "Dr. Jane Smith",
		"determination": "CLEAR_FIELD",
		"structured_results": map[string]any{
			"search_strategy": map[string]any{
				"invention_title":   "ROCIT",
				"invention_summary": "Classifies DNA reads from methylation patterns.",
				"novel_elements": []any{
					map[string]any{"id": "NE1", "description": "Long-read methylation classification"},
					map[string]any{"id": "NE2", "description": "Tumor/non-tumor origin model"},
				},
			},
			"patents_found": map[string]any{
				"queries_executed": 2,
				"total_hits_by_query": map[string]any{
					"Q1": 12,
					"Q2": 8,
				},
				"patents": []any{
					map[string]any{
						"patent_id":    "12345678",
						"patent_title": "DNA methylation classification",
						"patent_date":  "2025-01-01",
					},
				},
			},
			"assessments": []any{
				map[string]any{
					"patent_id":              "12345678",
					"relevance":              "MEDIUM",
					"overlap_description":    "Similar model objective but different data processing approach.",
					"novel_elements_covered": []any{"NE2"},
				},
			},
		},
		"metadata": map[string]any{
			"total_patents_retrieved": 50,
			"total_patents_assessed":  10,
			"degraded":                true,
			"degraded_reason":         "Stage 4 fallback",
		},
	}

	md, ok := buildPriorArtReportMarkdown(env)
	if !ok {
		t.Fatal("expected report markdown rebuild to succeed")
	}
	required := []string{
		"# Prior Art Search Report",
		"## Executive Summary",
		"- **UCLA Case ID:** 2026-124",
		"- **PI:** Dr. Jane Smith",
		"## How Broad The Search Was",
		"`Hits` means raw keyword matches before detailed reading.",
		"## Closest Patents to Review",
		"## Novel Element Coverage (Claim-Chart Style)",
		"## Recommended Next Steps",
		"## Traceability Trail (Stage Inputs and Outputs)",
		"Stage 1 — Search Strategy",
		"Stage 2 — Patent Retrieval",
		"Stage 3 — Relevance Assessment",
		"Stage 4 — Landscape Synthesis",
	}
	for _, want := range required {
		if !strings.Contains(md, want) {
			t.Fatalf("expected markdown to contain %q", want)
		}
	}
	if !strings.Contains(md, "[USPTO](") || !strings.Contains(md, "[Google](") {
		t.Fatalf("expected both USPTO and Google links in closest patents table: %s", md)
	}
	if strings.Contains(md, "<br>") {
		t.Fatalf("did not expect raw <br> tags in markdown table cells: %s", md)
	}
	ne2CoverageRow := ""
	for _, line := range strings.Split(md, "\n") {
		if strings.Contains(line, "| `NE2`") {
			ne2CoverageRow = line
			break
		}
	}
	if ne2CoverageRow == "" {
		t.Fatalf("expected claim-chart row for NE2 in markdown: %s", md)
	}
	if !strings.Contains(ne2CoverageRow, "[USPTO](") || !strings.Contains(ne2CoverageRow, "[Google](") {
		t.Fatalf("expected linked example patents in claim-chart row: %s", ne2CoverageRow)
	}
}

func TestBuildPriorArtReportMarkdownRequiresStructuredResults(t *testing.T) {
	env := map[string]any{
		"agent":         "prior-art-search",
		"determination": "CLEAR_FIELD",
	}
	if _, ok := buildPriorArtReportMarkdown(env); ok {
		t.Fatal("expected rebuild to fail without structured_results")
	}
}
