package patentscreen

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

type AgentConfig struct {
	BusURL      string
	AgentID     string
	Secret      string
	PollWaitSec int
}

type Agent struct {
	cfg      AgentConfig
	client   *Client
	pipeline *Pipeline
	cursor   int
}

func NewAgent(cfg AgentConfig, pipeline *Pipeline) *Agent {
	if cfg.PollWaitSec <= 0 {
		cfg.PollWaitSec = 5
	}
	return &Agent{cfg: cfg, client: NewClient(cfg.BusURL), pipeline: pipeline}
}

func (a *Agent) Run(ctx context.Context) error {
	if err := a.register(ctx); err != nil {
		return err
	}
	heartbeat := time.NewTicker(60 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-heartbeat.C:
			if err := a.register(ctx); err != nil {
				log.Printf("patent-screen heartbeat register failed: %v", err)
			}
		default:
			events, next, err := a.client.PollInbox(ctx, a.cfg.AgentID, a.cfg.Secret, a.cursor, a.cfg.PollWaitSec)
			if err != nil {
				log.Printf("patent-screen poll failed: %v", err)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			a.cursor = next
			for _, evt := range events {
				if err := a.handleEvent(ctx, evt); err != nil {
					log.Printf("patent-screen handle event failed: %v", err)
				}
			}
		}
	}
}

func (a *Agent) register(ctx context.Context) error {
	return a.client.RegisterAgent(ctx, a.cfg.AgentID, a.cfg.Secret, []string{CapabilityPatentEligibilityScreen})
}

func (a *Agent) handleEvent(ctx context.Context, evt InboxEvent) error {
	if err := a.client.Ack(ctx, a.cfg.AgentID, a.cfg.Secret, evt.MessageID, "accepted", "processing patent eligibility screen"); err != nil {
		return err
	}
	_ = a.client.Event(ctx, a.cfg.AgentID, a.cfg.Secret, evt.MessageID, "progress", "Stage 1: Extracting invention details...", map[string]any{"stage": "stage_1"})

	var req RequestEnvelope
	if err := json.Unmarshal([]byte(evt.Body), &req); err != nil {
		_ = a.client.Event(ctx, a.cfg.AgentID, a.cfg.Secret, evt.MessageID, "error", "invalid request envelope", nil)
		return err
	}

	result, err := a.pipeline.Run(ctx, req)
	if err != nil {
		_ = a.client.Event(ctx, a.cfg.AgentID, a.cfg.Secret, evt.MessageID, "error", err.Error(), map[string]any{"stage": "pipeline"})
		return err
	}

	envelope := BuildResponse(result)
	blob, _ := json.Marshal(envelope)
	replyTo := evt.From
	if rt := replyToFromMeta(evt.Meta); rt != "" {
		replyTo = rt
	}
	_, err = a.client.SendMessage(
		ctx,
		a.cfg.AgentID,
		a.cfg.Secret,
		replyTo,
		evt.ConversationID,
		fmt.Sprintf("patent-screen-response-%s", evt.MessageID),
		"response",
		string(blob),
		nil,
		map[string]any{"stage": "done"},
	)
	if err != nil {
		_ = a.client.Event(ctx, a.cfg.AgentID, a.cfg.Secret, evt.MessageID, "error", "failed to send response", nil)
		return err
	}

	_ = a.client.Event(ctx, a.cfg.AgentID, a.cfg.Secret, evt.MessageID, "final", string(result.FinalDetermination), map[string]any{"pathway": result.Pathway})
	return nil
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
