package priorartsearch

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
	go a.heartbeatLoop(ctx, 60*time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			events, next, err := a.client.PollInbox(ctx, a.cfg.AgentID, a.cfg.Secret, a.cursor, a.cfg.PollWaitSec)
			if err != nil {
				log.Printf("prior-art-search poll failed: %v", err)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			a.cursor = next
			for _, evt := range events {
				if err := a.handleEvent(ctx, evt); err != nil {
					log.Printf("prior-art-search handle event failed: %v", err)
				}
			}
		}
	}
}

func (a *Agent) heartbeatLoop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := a.register(ctx); err != nil {
				log.Printf("prior-art-search heartbeat register failed: %v", err)
			}
		}
	}
}

func (a *Agent) register(ctx context.Context) error {
	return a.client.RegisterAgent(ctx, a.cfg.AgentID, a.cfg.Secret, []string{CapabilityPriorArtSearch})
}

func (a *Agent) handleEvent(ctx context.Context, evt InboxEvent) error {
	started := time.Now()
	if err := a.client.Ack(ctx, a.cfg.AgentID, a.cfg.Secret, evt.MessageID, "accepted", "processing prior art search"); err != nil {
		return err
	}
	req, err := parseRequestEnvelope(evt.Body)
	if err != nil {
		_ = a.client.Event(ctx, a.cfg.AgentID, a.cfg.Secret, evt.MessageID, "error", "invalid request envelope", nil)
		_ = a.sendError(ctx, evt, "invalid request envelope")
		return err
	}
	log.Printf(
		"prior-art-search request_start message_id=%s conversation_id=%s case_id=%s disclosure_chars=%d",
		evt.MessageID,
		evt.ConversationID,
		req.CaseID,
		len(req.DisclosureText),
	)

	result, runErr := a.pipeline.RunWithProgress(ctx, req, func(stage, message string) {
		_ = a.client.Event(ctx, a.cfg.AgentID, a.cfg.Secret, evt.MessageID, "progress", message, map[string]any{"stage": stage})
	})
	if runErr != nil {
		stage := StageNameFromError(runErr)
		log.Printf(
			"prior-art-search request_error message_id=%s conversation_id=%s case_id=%s stage=%s elapsed_ms=%d err=%q",
			evt.MessageID,
			evt.ConversationID,
			req.CaseID,
			stage,
			time.Since(started).Milliseconds(),
			runErr.Error(),
		)
		_ = a.client.Event(ctx, a.cfg.AgentID, a.cfg.Secret, evt.MessageID, "error", runErr.Error(), map[string]any{"stage": stage})
		_ = a.sendError(ctx, evt, runErr.Error())
		return runErr
	}
	env := BuildResponse(result)
	blob, _ := json.Marshal(env)
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
		fmt.Sprintf("prior-art-search-response-%s", evt.MessageID),
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
	log.Printf(
		"prior-art-search request_done message_id=%s conversation_id=%s case_id=%s determination=%s degraded=%t elapsed_ms=%d",
		evt.MessageID,
		evt.ConversationID,
		req.CaseID,
		result.Determination,
		result.Metadata.Degraded,
		time.Since(started).Milliseconds(),
	)
	_ = a.client.Event(ctx, a.cfg.AgentID, a.cfg.Secret, evt.MessageID, "final", string(result.Determination), map[string]any{"degraded": result.Metadata.Degraded})
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
		fmt.Sprintf("prior-art-search-error-%s", evt.MessageID),
		"response",
		message,
		nil,
		map[string]any{"stage": "error", "status": "error"},
	)
	return err
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
		Metadata:       RequestMetadata{ExtractionMethod: legacy.ExtractionMethod, Truncated: legacy.Truncated},
	}, nil
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
