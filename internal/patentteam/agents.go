package patentteam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var handlerSeq int64

func nextHandlerRequestID(prefix string) string {
	n := atomic.AddInt64(&handlerSeq, 1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

// AgentConfig holds configuration for a single pipeline agent.
type AgentConfig struct {
	ID           string
	Secret       string
	Capabilities []string
}

// PipelineConfig holds configuration for the long-lived patent pipeline.
type PipelineConfig struct {
	BusURL    string
	Intake    AgentConfig
	Extractor AgentConfig
	Evaluator AgentConfig
	Reporter  AgentConfig
}

// PipelineService runs all patent pipeline agents as long-lived goroutines.
type PipelineService struct {
	cfg    PipelineConfig
	client *Client
}

// NewPipelineService creates a new PipelineService.
func NewPipelineService(cfg PipelineConfig) *PipelineService {
	return &PipelineService{
		cfg:    cfg,
		client: NewClient(cfg.BusURL),
	}
}

// extractMeta extracts the meta field from an InboxEvent as a map.
func extractMeta(evt InboxEvent) map[string]any {
	if evt.Meta == nil {
		return nil
	}
	if m, ok := evt.Meta.(map[string]any); ok {
		return m
	}
	blob, err := json.Marshal(evt.Meta)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(blob, &m) != nil {
		return nil
	}
	return m
}

// replyTo reads the reply_to field from event metadata, falling back to evt.From.
func replyTo(evt InboxEvent) string {
	meta := extractMeta(evt)
	if meta != nil {
		if rt, ok := meta["reply_to"].(string); ok && rt != "" {
			return rt
		}
	}
	return evt.From
}

// HandleIntakeMessage validates attachments and forwards to the next pipeline agent.
// It captures evt.From as reply_to in outgoing metadata.
func HandleIntakeMessage(ctx context.Context, client *Client, agentID, secret string, evt InboxEvent, nextAgent string) error {
	if err := client.Ack(ctx, agentID, secret, evt.MessageID, "accepted", "validated"); err != nil {
		return err
	}
	_ = client.Event(ctx, agentID, secret, evt.MessageID, "progress", "validated inbound disclosure payload", map[string]any{"stage": "intake"})

	if len(evt.Attachments) == 0 {
		_ = client.Event(ctx, agentID, secret, evt.MessageID, "error", "missing attachment", nil)
		return errors.New("intake: missing attachment")
	}

	var payload map[string]any
	_ = json.Unmarshal([]byte(evt.Body), &payload)
	caseID, _ := payload["case_id"].(string)
	forwardBody, _ := json.Marshal(map[string]any{
		"case_id": caseID,
		"task":    "extract text",
	})

	_, err := client.SendMessage(
		ctx,
		agentID,
		secret,
		nextAgent,
		evt.ConversationID,
		nextHandlerRequestID("extract"),
		"request",
		string(forwardBody),
		evt.Attachments,
		map[string]any{"stage": "extract", "reply_to": evt.From},
	)
	if err != nil {
		_ = client.Event(ctx, agentID, secret, evt.MessageID, "error", "failed to forward to extractor", nil)
		return err
	}

	_ = client.Event(ctx, agentID, secret, evt.MessageID, "final", "forwarded to "+nextAgent, map[string]any{"stage": "intake"})
	return nil
}

// HandleExtractorMessage extracts text from a PDF and forwards to the next pipeline agent.
// It propagates reply_to from incoming metadata.
func HandleExtractorMessage(ctx context.Context, client *Client, agentID, secret string, evt InboxEvent, nextAgent string) error {
	if err := client.Ack(ctx, agentID, secret, evt.MessageID, "accepted", "extracting"); err != nil {
		return err
	}
	_ = client.Event(ctx, agentID, secret, evt.MessageID, "progress", "extracting PDF text", map[string]any{"stage": "extract"})

	if len(evt.Attachments) == 0 {
		_ = client.Event(ctx, agentID, secret, evt.MessageID, "error", "missing attachment", nil)
		return errors.New("extractor: missing attachment")
	}
	path, err := AttachmentFilePath(evt.Attachments[0])
	if err != nil {
		_ = client.Event(ctx, agentID, secret, evt.MessageID, "error", err.Error(), nil)
		return err
	}

	extracted, err := ExtractPDFText(ctx, path)
	if err != nil {
		_ = client.Event(ctx, agentID, secret, evt.MessageID, "error", err.Error(), nil)
		return err
	}

	var payload map[string]any
	_ = json.Unmarshal([]byte(evt.Body), &payload)
	caseID, _ := payload["case_id"].(string)
	forwardBody, _ := json.Marshal(map[string]any{
		"case_id":           caseID,
		"task":              "patent eligibility",
		"extracted_text":    extracted.Text,
		"extraction_method": extracted.Method,
		"truncated":         extracted.Truncated,
	})

	_, err = client.SendMessage(
		ctx,
		agentID,
		secret,
		nextAgent,
		evt.ConversationID,
		nextHandlerRequestID("patent"),
		"request",
		string(forwardBody),
		nil,
		map[string]any{"stage": "patent", "reply_to": replyTo(evt)},
	)
	if err != nil {
		_ = client.Event(ctx, agentID, secret, evt.MessageID, "error", "failed to forward to patent-agent", nil)
		return err
	}

	_ = client.Event(ctx, agentID, secret, evt.MessageID, "final", "extracted and forwarded to "+nextAgent, map[string]any{"method": extracted.Method})
	return nil
}

// HandlePatentAgentMessage runs eligibility evaluation and forwards to the next pipeline agent.
// It propagates reply_to from incoming metadata.
func HandlePatentAgentMessage(ctx context.Context, client *Client, agentID, secret string, evt InboxEvent, nextAgent string) error {
	if err := client.Ack(ctx, agentID, secret, evt.MessageID, "accepted", "analyzing"); err != nil {
		return err
	}
	_ = client.Event(ctx, agentID, secret, evt.MessageID, "progress", "running patent eligibility analysis", map[string]any{"stage": "analysis"})

	var payload struct {
		CaseID        string `json:"case_id"`
		ExtractedText string `json:"extracted_text"`
	}
	if err := json.Unmarshal([]byte(evt.Body), &payload); err != nil {
		_ = client.Event(ctx, agentID, secret, evt.MessageID, "error", "invalid payload", nil)
		return err
	}

	assessment := EvaluatePatentEligibility(payload.CaseID, payload.ExtractedText)
	blob, _ := json.Marshal(assessment)

	_, err := client.SendMessage(
		ctx,
		agentID,
		secret,
		nextAgent,
		evt.ConversationID,
		nextHandlerRequestID("report"),
		"request",
		string(blob),
		nil,
		map[string]any{"stage": "report", "reply_to": replyTo(evt)},
	)
	if err != nil {
		_ = client.Event(ctx, agentID, secret, evt.MessageID, "error", "failed to forward to reporter", nil)
		return err
	}

	_ = client.Event(ctx, agentID, secret, evt.MessageID, "final", "assessment completed", map[string]any{"eligibility": assessment.Eligibility})
	return nil
}

// HandleReporterMessageTo renders the final report and sends it to a specific destination.
func HandleReporterMessageTo(ctx context.Context, client *Client, agentID, secret string, evt InboxEvent, destination string) error {
	if err := client.Ack(ctx, agentID, secret, evt.MessageID, "accepted", "rendering"); err != nil {
		return err
	}

	var assessment PatentAssessment
	if err := json.Unmarshal([]byte(evt.Body), &assessment); err != nil {
		_ = client.Event(ctx, agentID, secret, evt.MessageID, "error", "invalid assessment payload", nil)
		return err
	}
	finalReport := RenderFinalReport(assessment)

	_, err := client.SendMessage(
		ctx,
		agentID,
		secret,
		destination,
		evt.ConversationID,
		nextHandlerRequestID("final"),
		"response",
		finalReport,
		nil,
		map[string]any{"stage": "done"},
	)
	if err != nil {
		_ = client.Event(ctx, agentID, secret, evt.MessageID, "error", "failed to send final report", nil)
		return err
	}

	_ = client.Event(ctx, agentID, secret, evt.MessageID, "final", "report delivered", nil)
	return nil
}

// HandleReporterMessage renders the final report and sends it to the reply_to destination.
func HandleReporterMessage(ctx context.Context, client *Client, agentID, secret string, evt InboxEvent) error {
	return HandleReporterMessageTo(ctx, client, agentID, secret, evt, replyTo(evt))
}

// RenderFinalReport formats a PatentAssessment into a human-readable report.
func RenderFinalReport(a PatentAssessment) string {
	blob, _ := json.MarshalIndent(a, "", "  ")
	reasons := strings.Join(a.EligibilityReason, "\n- ")
	questions := strings.Join(a.Questions, "\n- ")
	if reasons == "" {
		reasons = "(none)"
	}
	if questions == "" {
		questions = "(none)"
	}
	return fmt.Sprintf(
		"Patent Eligibility Screening\nCase: %s\nEligibility: %s\nConfidence: %.2f\n\nSummary:\n%s\n\nReasons:\n- %s\n\nQuestions for Inventors:\n- %s\n\nDisclaimer:\n%s\n\nRAW_JSON:\n%s",
		a.CaseID,
		a.Eligibility,
		a.Confidence,
		a.Summary,
		reasons,
		questions,
		a.Disclaimer,
		string(blob),
	)
}

// Run registers all pipeline agents and starts poll loops and heartbeats.
// It blocks until ctx is cancelled.
func (ps *PipelineService) Run(ctx context.Context) error {
	agents := []struct {
		cfg     AgentConfig
		handler func(context.Context, InboxEvent) error
	}{
		{
			cfg: ps.cfg.Intake,
			handler: func(ctx context.Context, evt InboxEvent) error {
				return HandleIntakeMessage(ctx, ps.client, ps.cfg.Intake.ID, ps.cfg.Intake.Secret, evt, ps.cfg.Extractor.ID)
			},
		},
		{
			cfg: ps.cfg.Extractor,
			handler: func(ctx context.Context, evt InboxEvent) error {
				return HandleExtractorMessage(ctx, ps.client, ps.cfg.Extractor.ID, ps.cfg.Extractor.Secret, evt, ps.cfg.Evaluator.ID)
			},
		},
		{
			cfg: ps.cfg.Evaluator,
			handler: func(ctx context.Context, evt InboxEvent) error {
				return HandlePatentAgentMessage(ctx, ps.client, ps.cfg.Evaluator.ID, ps.cfg.Evaluator.Secret, evt, ps.cfg.Reporter.ID)
			},
		},
		{
			cfg: ps.cfg.Reporter,
			handler: func(ctx context.Context, evt InboxEvent) error {
				return HandleReporterMessage(ctx, ps.client, ps.cfg.Reporter.ID, ps.cfg.Reporter.Secret, evt)
			},
		},
	}

	for _, a := range agents {
		if err := ps.client.RegisterAgent(ctx, a.cfg.ID, a.cfg.Secret, a.cfg.Capabilities); err != nil {
			return fmt.Errorf("register %s: %w", a.cfg.ID, err)
		}
		log.Printf("registered agent %s with capabilities %v", a.cfg.ID, a.cfg.Capabilities)
	}

	var wg sync.WaitGroup
	for _, a := range agents {
		wg.Add(2)
		go func() {
			defer wg.Done()
			ps.pollLoop(ctx, a.cfg, a.handler)
		}()
		go func() {
			defer wg.Done()
			ps.heartbeat(ctx, a.cfg)
		}()
	}

	<-ctx.Done()
	wg.Wait()
	return ctx.Err()
}

func (ps *PipelineService) pollLoop(ctx context.Context, cfg AgentConfig, handler func(context.Context, InboxEvent) error) {
	cursor := 0
	for {
		if ctx.Err() != nil {
			return
		}
		events, next, err := ps.client.PollInbox(ctx, cfg.ID, cfg.Secret, cursor, 5)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("%s poll error: %v", cfg.ID, err)
			time.Sleep(2 * time.Second)
			continue
		}
		cursor = next
		for _, evt := range events {
			if err := handler(ctx, evt); err != nil {
				log.Printf("%s handler error: %v", cfg.ID, err)
			}
		}
	}
}

func (ps *PipelineService) heartbeat(ctx context.Context, cfg AgentConfig) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := ps.client.RegisterAgent(ctx, cfg.ID, cfg.Secret, cfg.Capabilities); err != nil {
				log.Printf("%s heartbeat error: %v", cfg.ID, err)
			}
		}
	}
}
