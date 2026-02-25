package operator

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type WorkflowStatus string

const (
	StatusSubmitted WorkflowStatus = "submitted"
	StatusExecuting WorkflowStatus = "executing"
	StatusCompleted WorkflowStatus = "completed"
	StatusError     WorkflowStatus = "error"
)

type WorkflowState struct {
	Status         WorkflowStatus `json:"status"`
	ConversationID string         `json:"conversation_id"`
	RequestID      string         `json:"request_id"`
	Report         string         `json:"-"`
	Ready          bool           `json:"ready"`
}

type Submission struct {
	Token     string                    `json:"token"`
	CaseID    string                    `json:"case_id"`
	CreatedAt time.Time                 `json:"created_at"`
	Workflows map[string]*WorkflowState `json:"workflows"`
}

// OverallStatus returns the aggregate status across all workflows.
func (s *Submission) OverallStatus() string {
	allCompleted := true
	for _, ws := range s.Workflows {
		if ws.Status == StatusError {
			return "error"
		}
		if ws.Status != StatusCompleted {
			allCompleted = false
		}
	}
	if allCompleted {
		return "completed"
	}
	// If any are executing or submitted, we're partial.
	for _, ws := range s.Workflows {
		if ws.Status == StatusExecuting {
			return "partial"
		}
	}
	return "submitted"
}

type SubmissionStore struct {
	mu          sync.RWMutex
	submissions map[string]*Submission
}

func NewSubmissionStore() *SubmissionStore {
	return &SubmissionStore{
		submissions: make(map[string]*Submission),
	}
}

func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *SubmissionStore) Create(caseID string, workflows []string) *Submission {
	token := generateToken()
	wf := make(map[string]*WorkflowState, len(workflows))
	for _, w := range workflows {
		wf[w] = &WorkflowState{Status: StatusSubmitted}
	}
	sub := &Submission{
		Token:     token,
		CaseID:    caseID,
		CreatedAt: time.Now(),
		Workflows: wf,
	}
	s.mu.Lock()
	s.submissions[token] = sub
	s.mu.Unlock()
	return sub
}

func (s *SubmissionStore) Get(token string) *Submission {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.submissions[token]
}

// SetWorkflowIDs records the conversation_id and request_id for a workflow after sending the bus message.
func (s *SubmissionStore) SetWorkflowIDs(token, workflow, conversationID, requestID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.submissions[token]
	if !ok {
		return
	}
	ws, ok := sub.Workflows[workflow]
	if !ok {
		return
	}
	ws.ConversationID = conversationID
	ws.RequestID = requestID
	ws.Status = StatusExecuting
}

// CompleteWorkflow stores a report for a workflow matched by conversation_id.
func (s *SubmissionStore) CompleteWorkflow(conversationID, report string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sub := range s.submissions {
		for _, ws := range sub.Workflows {
			if ws.ConversationID == conversationID {
				ws.Report = report
				ws.Status = StatusCompleted
				ws.Ready = true
				return true
			}
		}
	}
	return false
}

// ErrorWorkflow marks a workflow as errored by conversation_id.
func (s *SubmissionStore) ErrorWorkflow(conversationID, reason string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sub := range s.submissions {
		for _, ws := range sub.Workflows {
			if ws.ConversationID == conversationID {
				ws.Status = StatusError
				ws.Report = reason
				return true
			}
		}
	}
	return false
}
