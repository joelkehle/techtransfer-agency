package operator

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeBusServer creates an httptest server that mimics the bus API endpoints
// used by Bridge (agent register, list agents, send message, poll inbox, ack).
func fakeBusServer(t *testing.T, agents []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/agents/register":
			json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case r.URL.Path == "/v1/agents":
			json.NewEncoder(w).Encode(map[string]any{"agents": agents})
		case r.URL.Path == "/v1/messages":
			json.NewEncoder(w).Encode(map[string]any{"message_id": "msg-test-123"})
		case r.URL.Path == "/v1/inbox":
			json.NewEncoder(w).Encode(map[string]any{"events": []any{}, "cursor": "0"})
		case r.URL.Path == "/v1/acks":
			json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			w.WriteHeader(404)
		}
	}))
}

func setupServer(t *testing.T) (http.Handler, *SubmissionStore, *httptest.Server) {
	t.Helper()
	agents := []map[string]any{
		{"agent_id": "screener", "capabilities": []string{"patent-screen"}, "status": "active"},
		{"agent_id": "searcher", "capabilities": []string{"prior-art"}, "status": "active"},
	}
	bus := fakeBusServer(t, agents)
	t.Cleanup(bus.Close)

	store := NewSubmissionStore()
	bridge := NewBridge(bus.URL, "operator", "secret", store)

	uploadDir := t.TempDir()
	handler := NewServer(bridge, store, t.TempDir(), uploadDir)
	return handler, store, bus
}

func TestHandleWorkflows(t *testing.T) {
	handler, _, _ := setupServer(t)

	req := httptest.NewRequest(http.MethodGet, "/workflows", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	workflows, ok := resp["workflows"].([]any)
	if !ok {
		t.Fatal("expected workflows array in response")
	}
	if len(workflows) < 2 {
		t.Fatalf("expected at least 2 workflows, got %d", len(workflows))
	}

	var foundPriorArt bool
	for _, wf := range workflows {
		entry, ok := wf.(map[string]any)
		if !ok {
			continue
		}
		if entry["capability"] == "prior-art" && entry["label"] == "Prior Art Search" {
			foundPriorArt = true
			break
		}
	}
	if !foundPriorArt {
		t.Fatal("expected prior-art capability to be listed as Prior Art Search")
	}
}

func TestHandleWorkflowsMethodNotAllowed(t *testing.T) {
	handler, _, _ := setupServer(t)

	req := httptest.NewRequest(http.MethodPost, "/workflows", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestHandleSubmitValid(t *testing.T) {
	handler, store, _ := setupServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("workflows", "patent-screen,prior-art-search")

	fw, err := writer.CreateFormFile("file", "test.pdf")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte("fake pdf content"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/submit", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	token, ok := resp["token"].(string)
	if !ok || token == "" {
		t.Fatal("expected non-empty token in response")
	}
	if resp["status"] != "submitted" {
		t.Fatalf("expected status submitted, got %v", resp["status"])
	}

	sub := store.Get(token)
	if sub == nil {
		t.Fatal("expected submission to be in store")
	}
	if got, wantPrefix := sub.CaseID, "SUB-"; len(got) <= len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("expected generated case_id prefix %q, got %q", wantPrefix, got)
	}
}

func TestHandleSubmitMissingWorkflows(t *testing.T) {
	handler, _, _ := setupServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/submit", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 400 {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleSubmitNoFile(t *testing.T) {
	handler, _, _ := setupServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("workflows", "patent-screen")
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/submit", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should still succeed; file is optional.
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleSubmitGeneratesCaseIDWhenMissing(t *testing.T) {
	handler, store, _ := setupServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("workflows", "patent-screen")
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/submit", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	token, ok := resp["token"].(string)
	if !ok || token == "" {
		t.Fatal("expected non-empty token in response")
	}

	sub := store.Get(token)
	if sub == nil {
		t.Fatal("expected submission to be in store")
	}
	if sub.CaseID == "" {
		t.Fatal("expected generated case_id when missing")
	}
	if got, wantPrefix := sub.CaseID, "SUB-"; len(got) <= len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("expected generated case_id prefix %q, got %q", wantPrefix, got)
	}
}

func TestSubmitWithCaseNumber(t *testing.T) {
	handler, store, _ := setupServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("workflows", "patent-screen")
	writer.WriteField("case_number", "2023-107")
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/submit", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	token, ok := resp["token"].(string)
	if !ok || token == "" {
		t.Fatal("expected non-empty token in response")
	}

	sub := store.Get(token)
	if sub == nil {
		t.Fatal("expected submission to be in store")
	}
	if sub.CaseID != "2023-107" {
		t.Fatalf("expected case_id 2023-107, got %q", sub.CaseID)
	}
}

func TestSubmitWithoutCaseNumber(t *testing.T) {
	handler, store, _ := setupServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("workflows", "patent-screen")
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/submit", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	token, ok := resp["token"].(string)
	if !ok || token == "" {
		t.Fatal("expected non-empty token in response")
	}

	sub := store.Get(token)
	if sub == nil {
		t.Fatal("expected submission to be in store")
	}
	if !strings.HasPrefix(sub.CaseID, "SUB-") {
		t.Fatalf("expected generated case_id with SUB- prefix, got %q", sub.CaseID)
	}
}

func TestHandleStatusValid(t *testing.T) {
	handler, store, _ := setupServer(t)
	sub := store.Create("case-status", []string{"patent-screen", "prior-art-search"})
	store.SetWorkflowIDs(sub.Token, "patent-screen", "conv-ps", "req-ps")

	req := httptest.NewRequest(http.MethodGet, "/status/"+sub.Token, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["token"] != sub.Token {
		t.Fatalf("expected token %s, got %v", sub.Token, resp["token"])
	}

	workflows, ok := resp["workflows"].(map[string]any)
	if !ok {
		t.Fatal("expected workflows map in response")
	}
	ps, ok := workflows["patent-screen"].(map[string]any)
	if !ok {
		t.Fatal("expected patent-screen in workflows")
	}
	if ps["status"] != string(StatusExecuting) {
		t.Fatalf("expected patent-screen status=executing, got %v", ps["status"])
	}
}

func TestHandleStatusUnknownToken(t *testing.T) {
	handler, _, _ := setupServer(t)

	req := httptest.NewRequest(http.MethodGet, "/status/nonexistent", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 404 {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleStatusMethodNotAllowed(t *testing.T) {
	handler, _, _ := setupServer(t)

	req := httptest.NewRequest(http.MethodPost, "/status/some-token", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestHandleReportCompleted(t *testing.T) {
	handler, store, _ := setupServer(t)
	sub := store.Create("case-report", []string{"patent-screen"})
	store.SetWorkflowIDs(sub.Token, "patent-screen", "conv-report", "req-1")
	store.CompleteWorkflow("conv-report", "Final report text here")

	req := httptest.NewRequest(http.MethodGet, "/report/"+sub.Token+"/patent-screen", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("expected text/plain content-type, got %s", ct)
	}
	if rr.Body.String() != "Final report text here" {
		t.Fatalf("expected 'Final report text here', got %q", rr.Body.String())
	}
}

func TestHandleReportNotReady(t *testing.T) {
	handler, store, _ := setupServer(t)
	sub := store.Create("case-not-ready", []string{"patent-screen"})
	store.SetWorkflowIDs(sub.Token, "patent-screen", "conv-notready", "req-1")

	req := httptest.NewRequest(http.MethodGet, "/report/"+sub.Token+"/patent-screen", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 404 {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleReportUnknownToken(t *testing.T) {
	handler, _, _ := setupServer(t)

	req := httptest.NewRequest(http.MethodGet, "/report/bad-token/patent-screen", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 404 {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleReportUnknownWorkflow(t *testing.T) {
	handler, store, _ := setupServer(t)
	sub := store.Create("case-unknown-wf", []string{"patent-screen"})

	req := httptest.NewRequest(http.MethodGet, "/report/"+sub.Token+"/no-such-workflow", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 404 {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleReportBadPath(t *testing.T) {
	handler, _, _ := setupServer(t)

	req := httptest.NewRequest(http.MethodGet, "/report/only-one-segment", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 400 {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleReportBuildPatentScreen(t *testing.T) {
	handler, _, _ := setupServer(t)

	body := map[string]any{
		"workflow": "patent-screen",
		"envelope": map[string]any{
			"case_id":       "2026-124",
			"determination": "LIKELY_ELIGIBLE",
			"pathway":       "B1 — no judicial exception",
			"stage_outputs": map[string]any{
				"stage_1": map[string]any{
					"invention_title":       "Test Invention",
					"abstract":              "Test abstract",
					"problem_solved":        "Test problem",
					"invention_description": "Test description",
					"novel_elements":        []string{"Element A"},
					"technology_area":       "Optics",
					"claims_present":        false,
					"confidence_score":      0.9,
					"confidence_reason":     "ok",
				},
				"stage_2": map[string]any{
					"categories":        []string{"MANUFACTURE"},
					"explanation":       "fits category",
					"passes_step_1":     true,
					"confidence_score":  0.8,
					"confidence_reason": "ok",
				},
				"stage_3": map[string]any{
					"recites_exception": false,
					"reasoning":         "none",
					"mpep_reference":    "MPEP § 2106.04",
					"confidence_score":  0.8,
					"confidence_reason": "ok",
				},
				"stage_6": map[string]any{
					"novelty_concerns":          []string{"none"},
					"non_obviousness_concerns":  []string{"none"},
					"prior_art_search_priority": "HIGH",
					"reasoning":                 "reasoning",
					"confidence_score":          0.8,
					"confidence_reason":         "ok",
				},
			},
			"pipeline_metadata": map[string]any{},
			"disclaimer":        "test disclaimer",
		},
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/report-build", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["workflow"] != "patent-screen" {
		t.Fatalf("expected workflow patent-screen, got %v", resp["workflow"])
	}
	if markdown, _ := resp["report_markdown"].(string); strings.TrimSpace(markdown) == "" {
		t.Fatal("expected non-empty report_markdown")
	}
	if raw, _ := resp["report_raw"].(string); !strings.Contains(raw, "report_markdown") {
		t.Fatal("expected report_raw to include report_markdown field")
	}
}

func TestHandleReportBuildPriorArt(t *testing.T) {
	handler, _, _ := setupServer(t)

	body := map[string]any{
		"workflow": "prior-art-search",
		"envelope": map[string]any{
			"agent":         "prior-art-search",
			"case_id":       "SUB-123",
			"ucla_case_id":  "2026-124",
			"pi_name":       "Dr. Jane Smith",
			"determination": "CLEAR_FIELD",
			"structured_results": map[string]any{
				"search_strategy": map[string]any{
					"invention_title": "ROCIT",
					"novel_elements": []any{
						map[string]any{"id": "NE1", "description": "Long-read methylation classification"},
					},
				},
				"patents_found": map[string]any{
					"queries_executed": 1,
					"total_hits_by_query": map[string]any{
						"Q1": 2,
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
						"overlap_description":    "Similar objective",
						"novel_elements_covered": []any{"NE1"},
					},
				},
			},
			"metadata": map[string]any{
				"total_patents_retrieved": 10,
				"total_patents_assessed":  5,
			},
		},
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/report-build", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["workflow"] != "prior-art-search" {
		t.Fatalf("expected workflow prior-art-search, got %v", resp["workflow"])
	}
	md, _ := resp["report_markdown"].(string)
	if !strings.Contains(md, "## How Broad The Search Was") {
		t.Fatalf("expected rebuilt report markdown, got: %v", md)
	}
	if !strings.Contains(md, "- **UCLA Case ID:** 2026-124") || !strings.Contains(md, "- **PI:** Dr. Jane Smith") {
		t.Fatalf("expected case id and PI in report markdown, got: %v", md)
	}
	if strings.Contains(md, "- Case ID:") {
		t.Fatalf("did not expect inline case-id bullets in rebuilt markdown: %v", md)
	}
}

func TestHandleReportBuildUnsupportedWorkflow(t *testing.T) {
	handler, _, _ := setupServer(t)

	body := map[string]any{
		"workflow": "market-analysis",
		"envelope": map[string]any{"report_markdown": "x"},
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/report-build", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 400 {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleReportBuildPatentScreenInvalidEnvelope(t *testing.T) {
	handler, _, _ := setupServer(t)

	body := map[string]any{
		"workflow": "patent-screen",
		"envelope": map[string]any{
			"case_id":           "2026-124",
			"determination":     "LIKELY_ELIGIBLE",
			"pathway":           "B1 — no judicial exception",
			"stage_outputs":     map[string]any{},
			"pipeline_metadata": map[string]any{},
		},
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/report-build", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 400 {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleReportFormatsPriorArtStructuredReport(t *testing.T) {
	handler, store, _ := setupServer(t)
	sub := store.Create("case-prior-art-report", []string{"prior-art-search"})
	store.SetWorkflowIDs(sub.Token, "prior-art-search", "conv-prior-art", "req-1")

	env := map[string]any{
		"agent":           "prior-art-search",
		"case_id":         "SUB-123",
		"ucla_case_id":    "2026-124",
		"pi_name":         "Dr. Jane Smith",
		"determination":   "CLEAR_FIELD",
		"report_markdown": "# Prior Art Search Report\n\n- Case ID: SUB-123\n\nlegacy",
		"structured_results": map[string]any{
			"search_strategy": map[string]any{
				"invention_title": "ROCIT",
				"novel_elements": []any{
					map[string]any{"id": "NE1", "description": "Long-read methylation classification"},
				},
			},
			"patents_found": map[string]any{
				"queries_executed": 1,
				"total_hits_by_query": map[string]any{
					"Q1": 2,
				},
			},
			"assessments": []any{},
		},
		"metadata": map[string]any{
			"total_patents_retrieved": 2,
			"total_patents_assessed":  0,
		},
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	store.CompleteWorkflow("conv-prior-art", string(raw))

	req := httptest.NewRequest(http.MethodGet, "/report/"+sub.Token+"/prior-art-search", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var parsed map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	md := stringValue(parsed["report_markdown"])
	if !strings.Contains(md, "## How Broad The Search Was") {
		t.Fatalf("expected formatted prior-art markdown, got: %s", md)
	}
	if !strings.Contains(md, "- **UCLA Case ID:** 2026-124") || !strings.Contains(md, "- **PI:** Dr. Jane Smith") {
		t.Fatalf("expected case id and PI in formatted markdown, got: %s", md)
	}
	if strings.Contains(md, "- Case ID:") {
		t.Fatalf("did not expect inline case-id bullet after formatting: %s", md)
	}
}

type mockPDFRenderer struct {
	pdf []byte
	err error
}

func (m mockPDFRenderer) Render(_ context.Context, _ string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.pdf, nil
}

type capturePDFRenderer struct {
	pdf        []byte
	err        error
	lastReport string
}

func (m *capturePDFRenderer) Render(_ context.Context, report string) ([]byte, error) {
	m.lastReport = report
	if m.err != nil {
		return nil, m.err
	}
	return m.pdf, nil
}

func TestHandleReportPDFCompleted(t *testing.T) {
	agents := []map[string]any{
		{"agent_id": "screener", "capabilities": []string{"patent-screen"}, "status": "active"},
	}
	bus := fakeBusServer(t, agents)
	t.Cleanup(bus.Close)

	store := NewSubmissionStore()
	bridge := NewBridge(bus.URL, "operator", "secret", store)
	handler := newServer(bridge, store, t.TempDir(), t.TempDir(), mockPDFRenderer{pdf: []byte("%PDF-1.4\nmock\n")})

	sub := store.Create("case-pdf", []string{"patent-screen"})
	store.SetWorkflowIDs(sub.Token, "patent-screen", "conv-pdf", "req-1")
	store.CompleteWorkflow("conv-pdf", "Final report text here")

	req := httptest.NewRequest(http.MethodGet, "/report-pdf/"+sub.Token+"/patent-screen", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("expected application/pdf content-type, got %s", ct)
	}
	if !strings.Contains(rr.Header().Get("Content-Disposition"), ".pdf") {
		t.Fatalf("expected pdf content-disposition, got %q", rr.Header().Get("Content-Disposition"))
	}
	if got := rr.Body.String(); !strings.HasPrefix(got, "%PDF-1.4") {
		t.Fatalf("expected mock pdf body, got %q", got)
	}
}

func TestHandleReportPDFUnknownToken(t *testing.T) {
	agents := []map[string]any{
		{"agent_id": "screener", "capabilities": []string{"patent-screen"}, "status": "active"},
	}
	bus := fakeBusServer(t, agents)
	t.Cleanup(bus.Close)

	store := NewSubmissionStore()
	bridge := NewBridge(bus.URL, "operator", "secret", store)
	handler := newServer(bridge, store, t.TempDir(), t.TempDir(), mockPDFRenderer{pdf: []byte("%PDF-1.4\nmock\n")})

	req := httptest.NewRequest(http.MethodGet, "/report-pdf/bad-token/patent-screen", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 404 {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleReportPDFFormatsPriorArtStructuredReport(t *testing.T) {
	agents := []map[string]any{
		{"agent_id": "prior-art", "capabilities": []string{"prior-art-search"}, "status": "active"},
	}
	bus := fakeBusServer(t, agents)
	t.Cleanup(bus.Close)

	store := NewSubmissionStore()
	bridge := NewBridge(bus.URL, "operator", "secret", store)
	renderer := &capturePDFRenderer{pdf: []byte("%PDF-1.4\nmock\n")}
	handler := newServer(bridge, store, t.TempDir(), t.TempDir(), renderer)

	sub := store.Create("case-pdf-prior-art", []string{"prior-art-search"})
	store.SetWorkflowIDs(sub.Token, "prior-art-search", "conv-pdf-prior-art", "req-1")
	env := map[string]any{
		"agent":         "prior-art-search",
		"case_id":       "SUB-123",
		"ucla_case_id":  "2026-124",
		"pi_name":       "Dr. Jane Smith",
		"determination": "CLEAR_FIELD",
		"report_markdown": strings.Join([]string{
			"# Prior Art Search Report",
			"",
			"- Case ID: SUB-123",
			"- Invention: ROCIT",
			"- Date: 2026-03-04T02:48:53Z",
			"",
			"legacy",
		}, "\n"),
		"structured_results": map[string]any{
			"search_strategy": map[string]any{
				"invention_title":   "ROCIT",
				"invention_summary": "Classifies DNA reads from methylation patterns.",
				"novel_elements": []any{
					map[string]any{"id": "NE1", "description": "Long-read methylation classification"},
				},
				"query_strategies": []any{
					map[string]any{"id": "Q1"},
				},
			},
			"patents_found": map[string]any{
				"queries_executed": 1,
				"total_hits_by_query": map[string]any{
					"Q1": 2,
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
					"overlap_description":    "Similar objective with partial overlap.",
					"novel_elements_covered": []any{"NE1"},
				},
			},
		},
		"metadata": map[string]any{
			"total_patents_retrieved": 10,
			"total_patents_assessed":  5,
		},
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	store.CompleteWorkflow("conv-pdf-prior-art", string(raw))

	req := httptest.NewRequest(http.MethodGet, "/report-pdf/"+sub.Token+"/prior-art-search", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if renderer.lastReport == "" {
		t.Fatal("expected renderer input report to be captured")
	}
	if !strings.Contains(renderer.lastReport, "## How Broad The Search Was") {
		t.Fatalf("expected normalized prior-art report in renderer input, got: %s", renderer.lastReport)
	}
	if !strings.Contains(renderer.lastReport, "## Traceability Trail (Stage Inputs and Outputs)") {
		t.Fatalf("expected traceability section in renderer input, got: %s", renderer.lastReport)
	}
	if strings.Contains(renderer.lastReport, "- Case ID: SUB-123") {
		t.Fatalf("did not expect legacy header bullets in renderer input, got: %s", renderer.lastReport)
	}
}

func TestHandleReportPDFInline(t *testing.T) {
	agents := []map[string]any{
		{"agent_id": "screener", "capabilities": []string{"patent-screen"}, "status": "active"},
	}
	bus := fakeBusServer(t, agents)
	t.Cleanup(bus.Close)

	store := NewSubmissionStore()
	bridge := NewBridge(bus.URL, "operator", "secret", store)
	handler := newServer(bridge, store, t.TempDir(), t.TempDir(), mockPDFRenderer{pdf: []byte("%PDF-1.4\nmock\n")})

	req := httptest.NewRequest(http.MethodPost, "/report-pdf-inline", strings.NewReader("## Report\ncontent"))
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("expected application/pdf content-type, got %s", ct)
	}
}

func TestHandleSubmitCreatesUploadFile(t *testing.T) {
	agents := []map[string]any{
		{"agent_id": "screener", "capabilities": []string{"patent-screen"}, "status": "active"},
	}
	bus := fakeBusServer(t, agents)
	t.Cleanup(bus.Close)

	store := NewSubmissionStore()
	bridge := NewBridge(bus.URL, "operator", "secret", store)
	uploadDir := t.TempDir()
	handler := NewServer(bridge, store, t.TempDir(), uploadDir)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("workflows", "patent-screen")
	fw, _ := writer.CreateFormFile("file", "upload-test.txt")
	fw.Write([]byte("uploaded content"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/submit", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	// Check that the file was saved to the upload directory.
	data, err := os.ReadFile(uploadDir + "/upload-test.txt")
	if err != nil {
		t.Fatalf("expected upload file to exist: %v", err)
	}
	if string(data) != "uploaded content" {
		t.Fatalf("expected 'uploaded content', got %q", string(data))
	}
}

func TestHandleSubmitMethodNotAllowed(t *testing.T) {
	handler, _, _ := setupServer(t)

	req := httptest.NewRequest(http.MethodGet, "/submit", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestSubmitUIContractDoesNotReferenceRemovedCaseNumberField(t *testing.T) {
	indexPath := filepath.Join("..", "..", "web", "index.html")
	appPath := filepath.Join("..", "..", "web", "app.js")

	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	appBytes, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}

	index := string(indexBytes)
	app := string(appBytes)

	if strings.Contains(index, `id="case-number"`) {
		t.Fatal("unexpected case-number input in submit UI")
	}
	if strings.Contains(app, "case-number") {
		t.Fatal("app.js should not reference removed case-number input")
	}
	if !strings.Contains(app, `fetch("/submit"`) {
		t.Fatal("expected submit fetch call in app.js")
	}
	if !strings.Contains(app, `formData.append("file", selectedFile)`) {
		t.Fatal("expected file upload form data append in app.js")
	}
	if !strings.Contains(index, "<!DOCTYPE html>") ||
		!strings.Contains(index, `<div id="view-submit">`) ||
		!strings.Contains(index, `<script src="app.js`) {
		t.Fatal("index.html appears malformed or incomplete")
	}
}

func TestIndexHTMLNotCorruptedWithLineFragments(t *testing.T) {
	indexPath := filepath.Join("..", "..", "web", "index.html")
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	index := string(indexBytes)

	badFragments := []string{
		`27:        <div class="service-card live">`,
		`34:            <span class="service-badge live">Active</span>`,
		`50:          <h3>Prior Art Search</h3>`,
		`115:          <h3>Market Analysis</h3>`,
	}
	for _, frag := range badFragments {
		if strings.Contains(index, frag) {
			t.Fatalf("index.html appears corrupted; found fragment: %q", frag)
		}
	}
}

func TestHomepageShowsLiveChipsForPriorArtAndMarketAnalysis(t *testing.T) {
	indexPath := filepath.Join("..", "..", "web", "index.html")
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	index := string(indexBytes)

	if !strings.Contains(index, `<span class="strip-chip live">Prior Art Search</span>`) {
		t.Fatal("expected Prior Art Search chip to be marked live on homepage")
	}
	if !strings.Contains(index, `<span class="strip-chip live">Market Analysis</span>`) {
		t.Fatal("expected Market Analysis chip to be marked live on homepage")
	}
}

func TestVisionPageShowsActiveDemoForPatentEligibilityScreen(t *testing.T) {
	visionPath := filepath.Join("..", "..", "web", "vision.html")
	visionBytes, err := os.ReadFile(visionPath)
	if err != nil {
		t.Fatalf("read vision.html: %v", err)
	}
	vision := string(visionBytes)

	required := `<span class="service-badge live">Active</span>
            <span class="service-badge live">Demo</span>
          </div>
          <h3>Patent Eligibility Screen</h3>`
	if !strings.Contains(vision, required) {
		t.Fatal("expected Patent Eligibility Screen card to show Active and Demo badges")
	}
}
