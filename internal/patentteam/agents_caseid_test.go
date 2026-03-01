package patentteam

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleExtractorMessageUsesExtractedCaseNumberForSUBIDs(t *testing.T) {
	t.Setenv("DOC_CACHE_PATH", filepath.Join(t.TempDir(), "missing-doc-cache"))

	var sentPayload map[string]any
	bus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/acks":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case "/v1/events":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case "/v1/messages":
			if err := json.NewDecoder(r.Body).Decode(&sentPayload); err != nil {
				t.Fatalf("decode sent payload: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"message_id": "msg-1"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(bus.Close)

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.pdf")
	body := []byte("%PDF-1.4\nUC Case Number: 2023-107\nThis invention includes a novel algorithm.\n%%EOF")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	evtBody, _ := json.Marshal(map[string]any{"case_id": "SUB-123456"})
	evt := InboxEvent{
		MessageID:      "evt-1",
		From:           "operator",
		ConversationID: "conv-1",
		Body:           string(evtBody),
		Attachments:    []Attachment{{URL: "file://" + path}},
	}

	client := NewClient(bus.URL)
	if err := HandleExtractorMessage(context.Background(), client, "extractor", "secret", evt, "patent-screen"); err != nil {
		t.Fatalf("HandleExtractorMessage error: %v", err)
	}

	rawInnerBody, ok := sentPayload["body"].(string)
	if !ok || rawInnerBody == "" {
		t.Fatalf("expected forwarded message body string, got %#v", sentPayload["body"])
	}

	var inner map[string]any
	if err := json.Unmarshal([]byte(rawInnerBody), &inner); err != nil {
		t.Fatalf("decode forwarded inner body: %v", err)
	}
	if inner["case_id"] != "2023-107" {
		t.Fatalf("expected extracted case_id 2023-107, got %v", inner["case_id"])
	}
}
