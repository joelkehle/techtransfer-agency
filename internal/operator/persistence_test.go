package operator

import (
	"path/filepath"
	"testing"
)

func TestLoadStateMissingFileReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-state.json")
	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState missing file: %v", err)
	}
	if st.Cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", st.Cursor)
	}
	if len(st.Submissions) != 0 {
		t.Fatalf("expected empty submissions, got %d", len(st.Submissions))
	}
}

func TestSaveLoadStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operator-state.json")
	st := PersistedState{
		Cursor: 12,
		Submissions: map[string]*Submission{
			"tok-1": {
				Token:  "tok-1",
				CaseID: "2023-107",
				Workflows: map[string]*WorkflowState{
					"patent-screen": {
						Status:         StatusCompleted,
						ConversationID: "conv-1",
						RequestID:      "req-1",
						Report:         "report body",
						Ready:          true,
					},
				},
			},
		},
	}
	if err := SaveState(path, st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got.Cursor != 12 {
		t.Fatalf("expected cursor 12, got %d", got.Cursor)
	}
	sub := got.Submissions["tok-1"]
	if sub == nil {
		t.Fatal("expected submission tok-1")
	}
	ws := sub.Workflows["patent-screen"]
	if ws == nil {
		t.Fatal("expected workflow patent-screen")
	}
	if ws.Report != "report body" {
		t.Fatalf("expected report body, got %q", ws.Report)
	}
	if !ws.Ready {
		t.Fatal("expected ready true")
	}
}
