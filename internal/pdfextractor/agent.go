package pdfextractor

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/joelkehle/techtransfer-agency/internal/patentteam"
)

type AgentConfig struct {
	BusURL      string
	AgentID     string
	Capability  string
	Secret      string
	NextAgentID string
	PollWaitSec int
}

type Agent struct {
	cfg    AgentConfig
	client *Client
	cursor int
}

func NewAgent(cfg AgentConfig) *Agent {
	if cfg.PollWaitSec <= 0 {
		cfg.PollWaitSec = 5
	}
	if cfg.Capability == "" {
		cfg.Capability = "patent-screen"
	}
	if cfg.NextAgentID == "" {
		cfg.NextAgentID = "patent-screen"
	}
	return &Agent{cfg: cfg, client: NewClient(cfg.BusURL)}
}

func (a *Agent) Run(ctx context.Context) error {
	if err := a.register(ctx); err != nil {
		return err
	}
	log.Printf("%s registered capability=%s next=%s", a.cfg.AgentID, a.cfg.Capability, a.cfg.NextAgentID)

	heartbeat := time.NewTicker(60 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-heartbeat.C:
			if err := a.register(ctx); err != nil {
				log.Printf("patent-extractor heartbeat register failed: %v", err)
			} else {
				log.Printf("%s heartbeat renewed capability=%s", a.cfg.AgentID, a.cfg.Capability)
			}
		default:
			events, next, err := a.client.PollInbox(ctx, a.cfg.AgentID, a.cfg.Secret, a.cursor, a.cfg.PollWaitSec)
			if err != nil {
				log.Printf("patent-extractor poll failed: %v", err)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			a.cursor = next
			for _, evt := range events {
				log.Printf("%s received message_id=%s from=%s conversation=%s", a.cfg.AgentID, evt.MessageID, evt.From, evt.ConversationID)
				if err := a.handleEvent(ctx, evt); err != nil {
					log.Printf("patent-extractor handle event failed: %v", err)
				}
			}
		}
	}
}

func (a *Agent) register(ctx context.Context) error {
	return a.client.RegisterAgent(ctx, a.cfg.AgentID, a.cfg.Secret, []string{a.cfg.Capability})
}

func (a *Agent) handleEvent(ctx context.Context, evt InboxEvent) error {
	if err := patentteam.HandleExtractorMessage(ctx, a.client, a.cfg.AgentID, a.cfg.Secret, evt, a.cfg.NextAgentID); err != nil {
		_ = a.sendError(ctx, evt, err.Error())
		return err
	}
	return nil
}

func (a *Agent) sendError(ctx context.Context, evt InboxEvent, message string) error {
	replyTo := evt.From
	if rt := replyToFromMeta(evt.Meta); rt != "" {
		replyTo = rt
	}
	_, err := a.client.SendMessage(
		ctx,
		a.cfg.AgentID,
		a.cfg.Secret,
		replyTo,
		evt.ConversationID,
		fmt.Sprintf("patent-extractor-error-%s", evt.MessageID),
		"response",
		message,
		nil,
		map[string]any{"stage": "extract", "status": "error"},
	)
	return err
}

func replyToFromMeta(meta any) string {
	if meta == nil {
		return ""
	}
	m, ok := meta.(map[string]any)
	if !ok {
		return ""
	}
	rt, _ := m["reply_to"].(string)
	return strings.TrimSpace(rt)
}
