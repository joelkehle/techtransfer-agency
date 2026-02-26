package operator

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
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
	writer.WriteField("workflows", "patent-screen,prior-art")

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

func TestHandleStatusValid(t *testing.T) {
	handler, store, _ := setupServer(t)
	sub := store.Create("case-status", []string{"patent-screen", "prior-art"})
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
