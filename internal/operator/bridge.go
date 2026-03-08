package operator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joelkehle/techtransfer-agency/pkg/busclient"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type Bridge struct {
	client   *busclient.Client
	agentID  string
	secret   string
	store    *SubmissionStore
	cursorMu sync.Mutex
	cursor   int
	seq      int64
	flowMu   sync.Mutex
	flows    map[string]flowState
}

type flowState struct {
	Token          string
	Workflow       string
	TargetAgent    string
	CaseID         string
	RequestID      string
	AttachmentCnt  int
	SubmittedAtUTC time.Time
}

func NewBridge(busURL, agentID, secret string, store *SubmissionStore) *Bridge {
	return &Bridge{
		client:  busclient.NewClient(busURL),
		agentID: agentID,
		secret:  secret,
		store:   store,
		flows:   make(map[string]flowState),
	}
}

func (b *Bridge) Client() *busclient.Client {
	return b.client
}

func (b *Bridge) Register(ctx context.Context) error {
	return b.client.RegisterAgent(ctx, b.agentID, b.secret, []string{"submission-portal"})
}

func (b *Bridge) nextRequestID(prefix string) string {
	n := atomic.AddInt64(&b.seq, 1)
	return fmt.Sprintf("%s-%s-%d-%d", b.agentID, prefix, time.Now().UTC().UnixNano(), n)
}

// DiscoverWorkflows queries the bus for all registered agents and returns them.
func (b *Bridge) DiscoverWorkflows(ctx context.Context) ([]busclient.AgentInfo, error) {
	return b.client.ListAgents(ctx, "")
}

// Submit sends a request message to the appropriate agent for a given workflow capability.
func (b *Bridge) Submit(ctx context.Context, token, workflow, caseID string, attachments []busclient.Attachment) error {
	ctx, span := otel.Tracer("techtransfer-agency/operator").Start(ctx, "workflow.submit")
	defer span.End()
	span.SetAttributes(
		attribute.String("workflow", workflow),
		attribute.String("token", token),
		attribute.String("case_id", caseID),
		attribute.Int("attachment_count", len(attachments)),
	)

	agents, err := b.client.ListAgents(ctx, workflow)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("discover agents for %s: %w", workflow, err)
	}
	if len(agents) == 0 {
		err := fmt.Errorf("no agents found for capability %q", workflow)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("no agents found for capability %q", workflow)
	}

	target := selectTargetAgent(workflow, agents)
	conversationID := fmt.Sprintf("submission-%s-%s", token, workflow)
	requestID := b.nextRequestID(workflow)
	span.SetAttributes(
		attribute.String("target_agent", target),
		attribute.String("conversation_id", conversationID),
		attribute.String("request_id", requestID),
	)

	bodyMap := map[string]any{
		"task":    workflow,
		"case_id": caseID,
	}
	bodyBlob, _ := json.Marshal(bodyMap)

	// Register IDs before sending so the poll loop can match the response
	// even if the pipeline replies before SendMessage returns.
	b.store.SetWorkflowIDs(token, workflow, conversationID, requestID)

	_, err = b.client.SendMessage(
		ctx,
		b.agentID,
		b.secret,
		target,
		conversationID,
		requestID,
		"request",
		string(bodyBlob),
		attachments,
		map[string]any{"source": "operator", "token": token},
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("send message for %s: %w", workflow, err)
	}
	b.trackFlow(conversationID, flowState{
		Token:          token,
		Workflow:       workflow,
		TargetAgent:    target,
		CaseID:         caseID,
		RequestID:      requestID,
		AttachmentCnt:  len(attachments),
		SubmittedAtUTC: time.Now().UTC(),
	})
	return nil
}

func selectTargetAgent(workflow string, agents []busclient.AgentInfo) string {
	preferredByWorkflow := map[string]string{
		"patent-screen":    envOrDefault("WORKFLOW_TARGET_PATENT_SCREEN", "patent-extractor"),
		"market-analysis":  envOrDefault("WORKFLOW_TARGET_MARKET_ANALYSIS", "market-extractor"),
		"prior-art":        envOrDefault("WORKFLOW_TARGET_PRIOR_ART_SEARCH", "prior-art-extractor"),
		"prior-art-search": envOrDefault("WORKFLOW_TARGET_PRIOR_ART_SEARCH", "prior-art-extractor"),
	}
	preferred, ok := preferredByWorkflow[workflow]
	if !ok {
		return agents[0].AgentID
	}
	for _, agent := range agents {
		if agent.AgentID == preferred {
			return agent.AgentID
		}
	}
	return agents[0].AgentID
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

// PollLoop runs the inbox poll loop, matching responses back to submissions.
// It blocks until the context is cancelled.
func (b *Bridge) PollLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.poll(ctx)
		}
	}
}

func (b *Bridge) Cursor() int {
	b.cursorMu.Lock()
	defer b.cursorMu.Unlock()
	return b.cursor
}

func (b *Bridge) SetCursor(cursor int) {
	b.cursorMu.Lock()
	b.cursor = cursor
	b.cursorMu.Unlock()
}

// SyncCursorToLatest advances cursor without processing returned events.
// Useful on cold start to avoid replaying historical backlog when no local state exists.
func (b *Bridge) SyncCursorToLatest(ctx context.Context) error {
	cursor := b.Cursor()
	_, next, err := b.client.PollInbox(ctx, b.agentID, b.secret, cursor, 0)
	if err != nil {
		if isUnauthorizedPollError(err) {
			if regErr := b.Register(ctx); regErr != nil {
				return fmt.Errorf("re-register before cursor sync: %w", regErr)
			}
			_, next, err = b.client.PollInbox(ctx, b.agentID, b.secret, cursor, 0)
			if err != nil {
				return fmt.Errorf("cursor sync poll after re-register: %w", err)
			}
		} else {
			return fmt.Errorf("cursor sync poll: %w", err)
		}
	}
	b.SetCursor(next)
	return nil
}

func (b *Bridge) poll(ctx context.Context) {
	cursor := b.Cursor()
	events, next, err := b.client.PollInbox(ctx, b.agentID, b.secret, cursor, 0)
	if err != nil {
		if isUnauthorizedPollError(err) {
			log.Printf("operator poll unauthorized; attempting re-register")
			if regErr := b.Register(ctx); regErr != nil {
				log.Printf("operator re-register failed after unauthorized poll: %v", regErr)
				return
			}
			events, next, err = b.client.PollInbox(ctx, b.agentID, b.secret, cursor, 0)
			if err != nil {
				log.Printf("operator poll error after re-register: %v", err)
				return
			}
		} else {
			log.Printf("operator poll error: %v", err)
			return
		}
	}
	b.SetCursor(next)

	for _, evt := range events {
		b.observeResult(ctx, evt)
		switch evt.Type {
		case "response":
			if isErrorResponse(evt.Meta) {
				if !b.store.ErrorWorkflow(evt.ConversationID, evt.Body) {
					log.Printf("operator: unmatched error response for conversation %s", evt.ConversationID)
				}
				break
			}
			if !b.store.CompleteWorkflow(evt.ConversationID, evt.Body) {
				log.Printf("operator: unmatched response for conversation %s", evt.ConversationID)
			}
		case "error":
			if !b.store.ErrorWorkflow(evt.ConversationID, evt.Body) {
				log.Printf("operator: unmatched error for conversation %s", evt.ConversationID)
			}
		}
		// Ack all messages.
		_ = b.client.Ack(ctx, b.agentID, b.secret, evt.MessageID, "accepted", "")
	}
}

func (b *Bridge) trackFlow(conversationID string, state flowState) {
	b.flowMu.Lock()
	b.flows[conversationID] = state
	b.flowMu.Unlock()
}

func (b *Bridge) popFlow(conversationID string) (flowState, bool) {
	b.flowMu.Lock()
	defer b.flowMu.Unlock()
	state, ok := b.flows[conversationID]
	if ok {
		delete(b.flows, conversationID)
	}
	return state, ok
}

func (b *Bridge) observeResult(ctx context.Context, evt busclient.InboxEvent) {
	state, ok := b.popFlow(evt.ConversationID)
	if !ok {
		return
	}
	_, span := otel.Tracer("techtransfer-agency/operator").Start(ctx, "workflow.result")
	defer span.End()

	status := "completed"
	if evt.Type == "error" || isErrorResponse(evt.Meta) {
		status = "error"
	}
	span.SetAttributes(
		attribute.String("workflow", state.Workflow),
		attribute.String("token", state.Token),
		attribute.String("case_id", state.CaseID),
		attribute.String("target_agent", state.TargetAgent),
		attribute.String("conversation_id", evt.ConversationID),
		attribute.String("request_id", state.RequestID),
		attribute.String("event_type", evt.Type),
		attribute.String("result_status", status),
		attribute.String("from_agent", evt.From),
		attribute.String("message_id", evt.MessageID),
		attribute.Int("attachment_count", state.AttachmentCnt),
		attribute.Float64("workflow.elapsed_ms", float64(time.Since(state.SubmittedAtUTC).Milliseconds())),
	)
	if status == "error" {
		span.SetStatus(codes.Error, "workflow failed")
	}
}

func isErrorResponse(meta any) bool {
	if meta == nil {
		return false
	}
	m, ok := meta.(map[string]any)
	if !ok {
		return false
	}
	status, _ := m["status"].(string)
	return status == "error"
}

func isUnauthorizedPollError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "status=401") || strings.Contains(msg, "unauthorized")
}

// Heartbeat re-registers the agent periodically to keep it alive.
func (b *Bridge) Heartbeat(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := b.Register(ctx); err != nil {
				log.Printf("operator heartbeat error: %v", err)
			}
		}
	}
}
