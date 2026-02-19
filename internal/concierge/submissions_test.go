package concierge

import (
	"sync"
	"testing"
)

func TestCreateAndGet(t *testing.T) {
	store := NewSubmissionStore()
	sub := store.Create("case-1", []string{"patent-screen", "prior-art"})
	if sub.Token == "" {
		t.Fatal("expected non-empty token")
	}
	if sub.CaseID != "case-1" {
		t.Fatalf("expected case_id=case-1, got %s", sub.CaseID)
	}
	if len(sub.Workflows) != 2 {
		t.Fatalf("expected 2 workflows, got %d", len(sub.Workflows))
	}
	for _, name := range []string{"patent-screen", "prior-art"} {
		ws, ok := sub.Workflows[name]
		if !ok {
			t.Fatalf("missing workflow %s", name)
		}
		if ws.Status != StatusSubmitted {
			t.Fatalf("expected status submitted for %s, got %s", name, ws.Status)
		}
	}

	got := store.Get(sub.Token)
	if got == nil {
		t.Fatal("Get returned nil for valid token")
	}
	if got.Token != sub.Token {
		t.Fatalf("expected token %s, got %s", sub.Token, got.Token)
	}
}

func TestGetUnknownTokenReturnsNil(t *testing.T) {
	store := NewSubmissionStore()
	if got := store.Get("nonexistent"); got != nil {
		t.Fatalf("expected nil for unknown token, got %+v", got)
	}
}

func TestSetWorkflowIDs(t *testing.T) {
	store := NewSubmissionStore()
	sub := store.Create("case-2", []string{"patent-screen"})

	store.SetWorkflowIDs(sub.Token, "patent-screen", "conv-123", "req-456")

	ws := sub.Workflows["patent-screen"]
	if ws.ConversationID != "conv-123" {
		t.Fatalf("expected conversation_id=conv-123, got %s", ws.ConversationID)
	}
	if ws.RequestID != "req-456" {
		t.Fatalf("expected request_id=req-456, got %s", ws.RequestID)
	}
	if ws.Status != StatusExecuting {
		t.Fatalf("expected status executing, got %s", ws.Status)
	}
}

func TestSetWorkflowIDsUnknownToken(t *testing.T) {
	store := NewSubmissionStore()
	// Should not panic on unknown token.
	store.SetWorkflowIDs("bad-token", "wf", "c", "r")
}

func TestSetWorkflowIDsUnknownWorkflow(t *testing.T) {
	store := NewSubmissionStore()
	sub := store.Create("case-3", []string{"patent-screen"})
	// Should not panic on unknown workflow.
	store.SetWorkflowIDs(sub.Token, "unknown-wf", "c", "r")
}

func TestCompleteWorkflow(t *testing.T) {
	store := NewSubmissionStore()
	sub := store.Create("case-4", []string{"patent-screen"})
	store.SetWorkflowIDs(sub.Token, "patent-screen", "conv-complete", "req-1")

	ok := store.CompleteWorkflow("conv-complete", "the report")
	if !ok {
		t.Fatal("expected CompleteWorkflow to return true")
	}

	ws := sub.Workflows["patent-screen"]
	if ws.Status != StatusCompleted {
		t.Fatalf("expected status completed, got %s", ws.Status)
	}
	if ws.Report != "the report" {
		t.Fatalf("expected report 'the report', got %q", ws.Report)
	}
	if !ws.Ready {
		t.Fatal("expected Ready=true after completion")
	}
}

func TestCompleteWorkflowUnmatchedConversation(t *testing.T) {
	store := NewSubmissionStore()
	store.Create("case-5", []string{"patent-screen"})
	ok := store.CompleteWorkflow("no-such-conversation", "report")
	if ok {
		t.Fatal("expected CompleteWorkflow to return false for unmatched conversation")
	}
}

func TestErrorWorkflow(t *testing.T) {
	store := NewSubmissionStore()
	sub := store.Create("case-6", []string{"patent-screen"})
	store.SetWorkflowIDs(sub.Token, "patent-screen", "conv-err", "req-1")

	ok := store.ErrorWorkflow("conv-err", "something broke")
	if !ok {
		t.Fatal("expected ErrorWorkflow to return true")
	}

	ws := sub.Workflows["patent-screen"]
	if ws.Status != StatusError {
		t.Fatalf("expected status error, got %s", ws.Status)
	}
	if ws.Report != "something broke" {
		t.Fatalf("expected report 'something broke', got %q", ws.Report)
	}
}

func TestErrorWorkflowUnmatchedConversation(t *testing.T) {
	store := NewSubmissionStore()
	store.Create("case-7", []string{"patent-screen"})
	ok := store.ErrorWorkflow("no-such-conversation", "reason")
	if ok {
		t.Fatal("expected ErrorWorkflow to return false for unmatched conversation")
	}
}

func TestOverallStatus_AllSubmitted(t *testing.T) {
	sub := &Submission{
		Workflows: map[string]*WorkflowState{
			"a": {Status: StatusSubmitted},
			"b": {Status: StatusSubmitted},
		},
	}
	if s := sub.OverallStatus(); s != "submitted" {
		t.Fatalf("expected submitted, got %s", s)
	}
}

func TestOverallStatus_Partial(t *testing.T) {
	sub := &Submission{
		Workflows: map[string]*WorkflowState{
			"a": {Status: StatusExecuting},
			"b": {Status: StatusSubmitted},
		},
	}
	if s := sub.OverallStatus(); s != "partial" {
		t.Fatalf("expected partial, got %s", s)
	}
}

func TestOverallStatus_AllCompleted(t *testing.T) {
	sub := &Submission{
		Workflows: map[string]*WorkflowState{
			"a": {Status: StatusCompleted},
			"b": {Status: StatusCompleted},
		},
	}
	if s := sub.OverallStatus(); s != "completed" {
		t.Fatalf("expected completed, got %s", s)
	}
}

func TestOverallStatus_ErrorOverridesAll(t *testing.T) {
	sub := &Submission{
		Workflows: map[string]*WorkflowState{
			"a": {Status: StatusCompleted},
			"b": {Status: StatusError},
		},
	}
	if s := sub.OverallStatus(); s != "error" {
		t.Fatalf("expected error, got %s", s)
	}
}

func TestOverallStatus_ErrorOverridesPartial(t *testing.T) {
	sub := &Submission{
		Workflows: map[string]*WorkflowState{
			"a": {Status: StatusExecuting},
			"b": {Status: StatusError},
		},
	}
	if s := sub.OverallStatus(); s != "error" {
		t.Fatalf("expected error, got %s", s)
	}
}

func TestConcurrentAccess(t *testing.T) {
	store := NewSubmissionStore()
	var wg sync.WaitGroup
	const n = 100

	// Concurrent Creates.
	tokens := make([]string, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			sub := store.Create("case", []string{"wf"})
			tokens[idx] = sub.Token
		}(i)
	}
	wg.Wait()

	// All tokens should be unique.
	seen := map[string]bool{}
	for _, tok := range tokens {
		if tok == "" {
			t.Fatal("empty token from concurrent create")
		}
		if seen[tok] {
			t.Fatalf("duplicate token: %s", tok)
		}
		seen[tok] = true
	}

	// Concurrent Gets.
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			sub := store.Get(tokens[idx])
			if sub == nil {
				t.Errorf("nil Get for token %s", tokens[idx])
			}
		}(i)
	}
	wg.Wait()

	// Concurrent SetWorkflowIDs + CompleteWorkflow.
	sub := store.Create("concurrent-case", []string{"wf"})
	store.SetWorkflowIDs(sub.Token, "wf", "conv-concurrent", "req-1")
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			store.CompleteWorkflow("conv-concurrent", "report")
		}()
	}
	wg.Wait()
}
