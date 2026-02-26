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
	log.Printf("%s registered capability=%s", a.cfg.AgentID, CapabilityPatentEligibilityScreen)
	go a.heartbeatLoop(ctx, 60*time.Second)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			events, next, err := a.client.PollInbox(ctx, a.cfg.AgentID, a.cfg.Secret, a.cursor, a.cfg.PollWaitSec)
			if err != nil {
				log.Printf("patent-screen poll failed: %v", err)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			a.cursor = next
			for _, evt := range events {
				log.Printf("%s received message_id=%s from=%s conversation=%s", a.cfg.AgentID, evt.MessageID, evt.From, evt.ConversationID)
				go func(ev InboxEvent) {
					if err := a.handleEvent(ctx, ev); err != nil {
						log.Printf("patent-screen handle event failed: %v", err)
					}
				}(evt)
			}
		}
	}
}

func (a *Agent) heartbeatLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.register(ctx); err != nil {
				log.Printf("patent-screen heartbeat register failed: %v", err)
			} else {
				log.Printf("%s heartbeat renewed capability=%s", a.cfg.AgentID, CapabilityPatentEligibilityScreen)
			}
		}
	}
}

func (a *Agent) register(ctx context.Context) error {
	return a.client.RegisterAgent(ctx, a.cfg.AgentID, a.cfg.Secret, []string{CapabilityPatentEligibilityScreen})
}

func (a *Agent) handleEvent(ctx context.Context, evt InboxEvent) error {
	startedAt := time.Now()
	if err := a.client.Ack(ctx, a.cfg.AgentID, a.cfg.Secret, evt.MessageID, "accepted", "processing patent eligibility screen"); err != nil {
		return err
	}

	req, err := parseRequestEnvelope(evt.Body)
	if err != nil {
		_ = a.client.Event(ctx, a.cfg.AgentID, a.cfg.Secret, evt.MessageID, "error", "invalid request envelope", nil)
		_ = a.sendError(ctx, evt, "invalid request envelope")
		return err
	}
	log.Printf("%s accepted message_id=%s case_id=%s chars=%d extraction_method=%s truncated=%t", a.cfg.AgentID, evt.MessageID, req.CaseID, len(req.DisclosureText), req.Metadata.ExtractionMethod, req.Metadata.Truncated)

	result, err := a.pipeline.RunWithProgress(ctx, req, func(stage, message string) {
		log.Printf("%s stage=%s message_id=%s progress=%s", a.cfg.AgentID, stage, evt.MessageID, message)
		_ = a.client.Event(ctx, a.cfg.AgentID, a.cfg.Secret, evt.MessageID, "progress", message, map[string]any{"stage": stage})
	})
	if err != nil {
		stage := StageNameFromError(err)
		_ = a.client.Event(ctx, a.cfg.AgentID, a.cfg.Secret, evt.MessageID, "error", err.Error(), map[string]any{"stage": stage})
		_ = a.sendError(ctx, evt, err.Error())
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
		_ = a.sendError(ctx, evt, "failed to send response")
		return err
	}
	log.Printf("%s completed message_id=%s case_id=%s determination=%s pathway=%s elapsed=%s", a.cfg.AgentID, evt.MessageID, req.CaseID, result.FinalDetermination, result.Pathway, time.Since(startedAt).Round(time.Millisecond))

	_ = a.client.Event(ctx, a.cfg.AgentID, a.cfg.Secret, evt.MessageID, "final", string(result.FinalDetermination), map[string]any{"pathway": result.Pathway})
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
		fmt.Sprintf("patent-screen-error-%s", evt.MessageID),
		"response",
		message,
		nil,
		map[string]any{"stage": "error", "status": "error"},
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

func parseRequestEnvelope(body string) (RequestEnvelope, error) {
	var req RequestEnvelope
	if err := json.Unmarshal([]byte(body), &req); err == nil && strings.TrimSpace(req.DisclosureText) != "" {
		return req, nil
	}

	var legacy struct {
		CaseID           string `json:"case_id"`
		ExtractedText    string `json:"extracted_text"`
		ExtractionMethod string `json:"extraction_method"`
		Truncated        bool   `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(body), &legacy); err != nil {
		return RequestEnvelope{}, err
	}
	if strings.TrimSpace(legacy.ExtractedText) == "" {
		return RequestEnvelope{}, fmt.Errorf("missing disclosure_text or extracted_text")
	}
	return RequestEnvelope{
		CaseID:         legacy.CaseID,
		DisclosureText: legacy.ExtractedText,
		Metadata: RequestMetadata{
			ExtractionMethod: legacy.ExtractionMethod,
			Truncated:        legacy.Truncated,
		},
	}, nil
}
