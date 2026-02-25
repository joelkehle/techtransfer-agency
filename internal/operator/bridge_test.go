package operator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestDiscoverWorkflows(t *testing.T) {
	agentsResp := map[string]any{
		"agents": []map[string]any{
			{"agent_id": "screener", "capabilities": []string{"patent-screen", "prior-art"}, "status": "active"},
			{"agent_id": "analyzer", "capabilities": []string{"market-analysis"}, "status": "active"},
		},
	}
	bus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/agents" {
			json.NewEncoder(w).Encode(agentsResp)
			return
		}
		w.WriteHeader(404)
	}))
	defer bus.Close()

	store := NewSubmissionStore()
	bridge := NewBridge(bus.URL, "operator", "secret", store)

	agents, err := bridge.DiscoverWorkflows(context.Background())
	if err != nil {
		t.Fatalf("DiscoverWorkflows: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}
	if agents[0].AgentID != "screener" {
		t.Fatalf("expected first agent screener, got %s", agents[0].AgentID)
	}
	if len(agents[0].Capabilities) != 2 {
		t.Fatalf("expected 2 capabilities for screener, got %d", len(agents[0].Capabilities))
	}
}

func TestSubmitSendsMessageAndSetsIDs(t *testing.T) {
	var receivedPath string
	var receivedBody map[string]any
	bus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/agents":
			json.NewEncoder(w).Encode(map[string]any{
				"agents": []map[string]any{
					{"agent_id": "screener", "capabilities": []string{"patent-screen"}, "status": "active"},
				},
			})
		case "/v1/messages":
			receivedPath = r.URL.Path
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			receivedBody = body
			json.NewEncoder(w).Encode(map[string]any{"message_id": "msg-001"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer bus.Close()

	store := NewSubmissionStore()
	bridge := NewBridge(bus.URL, "operator", "secret", store)
	sub := store.Create("case-bridge", []string{"patent-screen"})

	err := bridge.Submit(context.Background(), sub.Token, "patent-screen", "case-bridge", nil)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if receivedPath != "/v1/messages" {
		t.Fatalf("expected message sent to /v1/messages, got %s", receivedPath)
	}
	if receivedBody["to"] != "screener" {
		t.Fatalf("expected to=screener, got %v", receivedBody["to"])
	}
	if receivedBody["type"] != "request" {
		t.Fatalf("expected type=request, got %v", receivedBody["type"])
	}

	ws := sub.Workflows["patent-screen"]
	if ws.Status != StatusExecuting {
		t.Fatalf("expected status executing after submit, got %s", ws.Status)
	}
	expectedConvID := "submission-" + sub.Token + "-patent-screen"
	if ws.ConversationID != expectedConvID {
		t.Fatalf("expected conversation_id=%s, got %s", expectedConvID, ws.ConversationID)
	}
}

func TestSubmitNoAgentsFoundReturnsError(t *testing.T) {
	bus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/agents":
			json.NewEncoder(w).Encode(map[string]any{"agents": []any{}})
		default:
			w.WriteHeader(404)
		}
	}))
	defer bus.Close()

	store := NewSubmissionStore()
	bridge := NewBridge(bus.URL, "operator", "secret", store)
	sub := store.Create("case-no-agent", []string{"patent-screen"})

	err := bridge.Submit(context.Background(), sub.Token, "patent-screen", "case-no-agent", nil)
	if err == nil {
		t.Fatal("expected error when no agents found")
	}
}

func TestPollMatchesResponseToSubmission(t *testing.T) {
	store := NewSubmissionStore()
	sub := store.Create("case-poll", []string{"patent-screen"})
	store.SetWorkflowIDs(sub.Token, "patent-screen", "conv-poll-1", "req-poll-1")

	var ackCount int32
	bus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/inbox":
			json.NewEncoder(w).Encode(map[string]any{
				"events": []map[string]any{
					{
						"message_id":      "msg-resp-1",
						"type":            "response",
						"from":            "screener",
						"conversation_id": "conv-poll-1",
						"body":            "Patent screen report",
					},
				},
				"cursor": "1",
			})
		case "/v1/acks":
			atomic.AddInt32(&ackCount, 1)
			json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			w.WriteHeader(404)
		}
	}))
	defer bus.Close()

	bridge := NewBridge(bus.URL, "operator", "secret", store)
	bridge.poll(context.Background())

	ws := sub.Workflows["patent-screen"]
	if ws.Status != StatusCompleted {
		t.Fatalf("expected status completed after poll, got %s", ws.Status)
	}
	if ws.Report != "Patent screen report" {
		t.Fatalf("expected report 'Patent screen report', got %q", ws.Report)
	}
	if !ws.Ready {
		t.Fatal("expected Ready=true after poll completion")
	}
	if atomic.LoadInt32(&ackCount) != 1 {
		t.Fatalf("expected 1 ack, got %d", ackCount)
	}
}

func TestPollMatchesErrorToSubmission(t *testing.T) {
	store := NewSubmissionStore()
	sub := store.Create("case-poll-err", []string{"prior-art"})
	store.SetWorkflowIDs(sub.Token, "prior-art", "conv-poll-err", "req-poll-err")

	bus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/inbox":
			json.NewEncoder(w).Encode(map[string]any{
				"events": []map[string]any{
					{
						"message_id":      "msg-err-1",
						"type":            "error",
						"from":            "searcher",
						"conversation_id": "conv-poll-err",
						"body":            "search failed",
					},
				},
				"cursor": "1",
			})
		case "/v1/acks":
			json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			w.WriteHeader(404)
		}
	}))
	defer bus.Close()

	bridge := NewBridge(bus.URL, "operator", "secret", store)
	bridge.poll(context.Background())

	ws := sub.Workflows["prior-art"]
	if ws.Status != StatusError {
		t.Fatalf("expected status error after poll, got %s", ws.Status)
	}
	if ws.Report != "search failed" {
		t.Fatalf("expected report 'search failed', got %q", ws.Report)
	}
}

func TestPollMatchesErrorResponseToSubmission(t *testing.T) {
	store := NewSubmissionStore()
	sub := store.Create("case-poll-err-resp", []string{"patent-screen"})
	store.SetWorkflowIDs(sub.Token, "patent-screen", "conv-poll-err-resp", "req-poll-err-resp")

	bus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/inbox":
			json.NewEncoder(w).Encode(map[string]any{
				"events": []map[string]any{
					{
						"message_id":      "msg-err-resp-1",
						"type":            "response",
						"from":            "patent-screen",
						"conversation_id": "conv-poll-err-resp",
						"body":            "disclosure text is insufficient for analysis",
						"meta": map[string]any{
							"status": "error",
						},
					},
				},
				"cursor": "1",
			})
		case "/v1/acks":
			json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			w.WriteHeader(404)
		}
	}))
	defer bus.Close()

	bridge := NewBridge(bus.URL, "operator", "secret", store)
	bridge.poll(context.Background())

	ws := sub.Workflows["patent-screen"]
	if ws.Status != StatusError {
		t.Fatalf("expected status error after poll, got %s", ws.Status)
	}
	if ws.Report != "disclosure text is insufficient for analysis" {
		t.Fatalf("expected error body to be stored, got %q", ws.Report)
	}
}

func TestPollCursorAdvances(t *testing.T) {
	store := NewSubmissionStore()

	bus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/inbox":
			json.NewEncoder(w).Encode(map[string]any{
				"events": []any{},
				"cursor": "42",
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer bus.Close()

	bridge := NewBridge(bus.URL, "operator", "secret", store)
	if bridge.cursor != 0 {
		t.Fatalf("expected initial cursor=0, got %d", bridge.cursor)
	}

	bridge.poll(context.Background())
	if bridge.cursor != 42 {
		t.Fatalf("expected cursor=42 after poll, got %d", bridge.cursor)
	}
}

func TestRegister(t *testing.T) {
	var registered bool
	bus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/agents/register" {
			registered = true
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["agent_id"] != "operator" {
				t.Errorf("expected agent_id=operator, got %v", body["agent_id"])
			}
			json.NewEncoder(w).Encode(map[string]any{"ok": true})
			return
		}
		w.WriteHeader(404)
	}))
	defer bus.Close()

	store := NewSubmissionStore()
	bridge := NewBridge(bus.URL, "operator", "secret", store)

	err := bridge.Register(context.Background())
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !registered {
		t.Fatal("expected register endpoint to be called")
	}
}

func TestNextRequestIDIsUnique(t *testing.T) {
	store := NewSubmissionStore()
	bridge := NewBridge("http://localhost:9999", "operator", "secret", store)

	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := bridge.nextRequestID("test")
		if ids[id] {
			t.Fatalf("duplicate request ID: %s", id)
		}
		ids[id] = true
	}
}
