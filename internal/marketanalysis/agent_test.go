package marketanalysis

import "testing"

func TestParseRequestEnvelopeStructured(t *testing.T) {
	body := `{"case_id":"CASE-1","disclosure_text":"A detailed disclosure text that is long enough to be meaningful and pass the minimum length requirement for this parser."}`
	req, err := parseRequestEnvelope(body)
	if err != nil {
		t.Fatalf("parseRequestEnvelope returned error: %v", err)
	}
	if req.CaseID != "CASE-1" {
		t.Fatalf("expected case_id CASE-1, got %q", req.CaseID)
	}
	if req.DisclosureText == "" {
		t.Fatal("expected disclosure_text to be present")
	}
}

func TestParseRequestEnvelopeLegacy(t *testing.T) {
	body := `{"case_id":"CASE-2","extracted_text":"Extracted PDF text with enough detail for screening and market analysis.","extraction_method":"pdftotext","truncated":true}`
	req, err := parseRequestEnvelope(body)
	if err != nil {
		t.Fatalf("parseRequestEnvelope returned error: %v", err)
	}
	if req.CaseID != "CASE-2" {
		t.Fatalf("expected case_id CASE-2, got %q", req.CaseID)
	}
	if req.Metadata.ExtractionMethod != "pdftotext" {
		t.Fatalf("expected extraction_method pdftotext, got %q", req.Metadata.ExtractionMethod)
	}
	if !req.Metadata.Truncated {
		t.Fatal("expected truncated=true")
	}
}
