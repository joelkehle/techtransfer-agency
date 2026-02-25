package concierge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/joelkehle/techtransfer-agency/internal/busclient"
)

type Bridge struct {
	client  *busclient.Client
	agentID string
	secret  string
	store   *SubmissionStore
	cursor  int
	seq     int64
}

func NewBridge(busURL, agentID, secret string, store *SubmissionStore) *Bridge {
	return &Bridge{
		client:  busclient.NewClient(busURL),
		agentID: agentID,
		secret:  secret,
		store:   store,
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
	return fmt.Sprintf("%s-%s-%d", b.agentID, prefix, n)
}

// DiscoverWorkflows queries the bus for all registered agents and returns them.
func (b *Bridge) DiscoverWorkflows(ctx context.Context) ([]busclient.AgentInfo, error) {
	return b.client.ListAgents(ctx, "")
}

// Submit sends a request message to the appropriate agent for a given workflow capability.
func (b *Bridge) Submit(ctx context.Context, token, workflow, caseID string, attachments []busclient.Attachment) error {
	agents, err := b.client.ListAgents(ctx, workflow)
	if err != nil {
		return fmt.Errorf("discover agents for %s: %w", workflow, err)
	}
	if len(agents) == 0 {
		return fmt.Errorf("no agents found for capability %q", workflow)
	}

	target := agents[0].AgentID
	conversationID := fmt.Sprintf("submission-%s-%s", token, workflow)
	requestID := b.nextRequestID(workflow)

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
		map[string]any{"source": "concierge", "token": token},
	)
	if err != nil {
		return fmt.Errorf("send message for %s: %w", workflow, err)
	}
	return nil
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

func (b *Bridge) poll(ctx context.Context) {
	events, next, err := b.client.PollInbox(ctx, b.agentID, b.secret, b.cursor, 0)
	if err != nil {
		log.Printf("concierge poll error: %v", err)
		return
	}
	b.cursor = next

	for _, evt := range events {
		switch evt.Type {
		case "response":
			if !b.store.CompleteWorkflow(evt.ConversationID, evt.Body) {
				log.Printf("concierge: unmatched response for conversation %s", evt.ConversationID)
			}
		case "error":
			if !b.store.ErrorWorkflow(evt.ConversationID, evt.Body) {
				log.Printf("concierge: unmatched error for conversation %s", evt.ConversationID)
			}
		}
		// Ack all messages.
		_ = b.client.Ack(ctx, b.agentID, b.secret, evt.MessageID, "accepted", "")
	}
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
				log.Printf("concierge heartbeat error: %v", err)
			}
		}
	}
}
