package patentteam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

type TeamConfig struct {
	CaseID         string
	BusURL         string
	PDFPath        string
	Timeout        time.Duration
	ConversationID string
}

type TeamResult struct {
	ConversationID string
	FinalReport    string
	Assessment     PatentAssessment
}

type Team struct {
	cfg     TeamConfig
	client  *Client
	secrets map[string]string
	cursors map[string]int
	seq     int64
}

func NewTeam(cfg TeamConfig) *Team {
	return &Team{
		cfg:    cfg,
		client: NewClient(cfg.BusURL),
		secrets: map[string]string{
			"coordinator":   "secret-coordinator",
			"intake":        "secret-intake",
			"pdf-extractor": "secret-pdf-extractor",
			"patent-agent":  "secret-patent-agent",
			"reporter":      "secret-reporter",
		},
		cursors: map[string]int{},
	}
}

func (t *Team) nextRequestID(prefix string) string {
	n := atomic.AddInt64(&t.seq, 1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

func (t *Team) registerAgents(ctx context.Context) error {
	registry := []struct {
		id   string
		caps []string
	}{
		{id: "coordinator", caps: []string{"orchestrator"}},
		{id: "intake", caps: []string{"intake"}},
		{id: "pdf-extractor", caps: []string{"pdf-extract"}},
		{id: "patent-agent", caps: []string{"patent-eligibility"}},
		{id: "reporter", caps: []string{"report"}},
	}
	for _, a := range registry {
		if err := t.client.RegisterAgent(ctx, a.id, t.secrets[a.id], a.caps); err != nil {
			return fmt.Errorf("register %s: %w", a.id, err)
		}
	}
	return nil
}

func (t *Team) Run(ctx context.Context) (TeamResult, error) {
	if strings.TrimSpace(t.cfg.CaseID) == "" {
		return TeamResult{}, errors.New("case id required")
	}
	if strings.TrimSpace(t.cfg.PDFPath) == "" {
		return TeamResult{}, errors.New("pdf path required")
	}

	pdfAbs, err := filepath.Abs(t.cfg.PDFPath)
	if err != nil {
		return TeamResult{}, err
	}
	conversationID := t.cfg.ConversationID
	if strings.TrimSpace(conversationID) == "" {
		conversationID = "conv-" + strings.ReplaceAll(strings.ToLower(t.cfg.CaseID), " ", "-")
	}

	if err := t.registerAgents(ctx); err != nil {
		return TeamResult{}, err
	}

	bodyMap := map[string]any{
		"task":    "screen patent eligibility",
		"case_id": t.cfg.CaseID,
		"note":    "start pipeline",
	}
	bodyBlob, _ := json.Marshal(bodyMap)
	_, err = t.client.SendMessage(
		ctx,
		"coordinator",
		t.secrets["coordinator"],
		"intake",
		conversationID,
		t.nextRequestID("init"),
		"request",
		string(bodyBlob),
		[]Attachment{{URL: "file://" + pdfAbs, Name: filepath.Base(pdfAbs), ContentType: "application/pdf"}},
		map[string]any{"stage": "intake"},
	)
	if err != nil {
		return TeamResult{}, err
	}

	deadline := time.Now().Add(t.cfg.Timeout)
	for time.Now().Before(deadline) {
		if err := t.processInbox(ctx, "intake"); err != nil {
			return TeamResult{}, err
		}
		if err := t.processInbox(ctx, "pdf-extractor"); err != nil {
			return TeamResult{}, err
		}
		if err := t.processInbox(ctx, "patent-agent"); err != nil {
			return TeamResult{}, err
		}
		if err := t.processInbox(ctx, "reporter"); err != nil {
			return TeamResult{}, err
		}

		report, assessment, done, err := t.readCoordinatorResult(ctx)
		if err != nil {
			return TeamResult{}, err
		}
		if done {
			return TeamResult{ConversationID: conversationID, FinalReport: report, Assessment: assessment}, nil
		}
		time.Sleep(150 * time.Millisecond)
	}

	return TeamResult{}, fmt.Errorf("team run timed out after %s", t.cfg.Timeout)
}

func (t *Team) processInbox(ctx context.Context, agentID string) error {
	secret := t.secrets[agentID]
	cursor := t.cursors[agentID]
	events, next, err := t.client.PollInbox(ctx, agentID, secret, cursor, 0)
	if err != nil {
		return fmt.Errorf("poll inbox %s: %w", agentID, err)
	}
	t.cursors[agentID] = next
	for _, evt := range events {
		switch agentID {
		case "intake":
			if err := t.handleIntake(ctx, evt); err != nil {
				return err
			}
		case "pdf-extractor":
			if err := t.handleExtractor(ctx, evt); err != nil {
				return err
			}
		case "patent-agent":
			if err := t.handlePatentAgent(ctx, evt); err != nil {
				return err
			}
		case "reporter":
			if err := t.handleReporter(ctx, evt); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *Team) handleIntake(ctx context.Context, evt InboxEvent) error {
	if err := t.client.Ack(ctx, "intake", t.secrets["intake"], evt.MessageID, "accepted", "validated"); err != nil {
		return err
	}
	_ = t.client.Event(ctx, "intake", t.secrets["intake"], evt.MessageID, "progress", "validated inbound disclosure payload", map[string]any{"stage": "intake"})

	if len(evt.Attachments) == 0 {
		_ = t.client.Event(ctx, "intake", t.secrets["intake"], evt.MessageID, "error", "missing attachment", nil)
		return errors.New("intake: missing attachment")
	}

	var payload map[string]any
	_ = json.Unmarshal([]byte(evt.Body), &payload)
	caseID, _ := payload["case_id"].(string)
	forwardBody, _ := json.Marshal(map[string]any{
		"case_id": caseID,
		"task":    "extract text",
	})

	_, err := t.client.SendMessage(
		ctx,
		"intake",
		t.secrets["intake"],
		"pdf-extractor",
		evt.ConversationID,
		t.nextRequestID("extract"),
		"request",
		string(forwardBody),
		evt.Attachments,
		map[string]any{"stage": "extract"},
	)
	if err != nil {
		_ = t.client.Event(ctx, "intake", t.secrets["intake"], evt.MessageID, "error", "failed to forward to extractor", nil)
		return err
	}

	_ = t.client.Event(ctx, "intake", t.secrets["intake"], evt.MessageID, "final", "forwarded to pdf-extractor", map[string]any{"stage": "intake"})
	return nil
}

func (t *Team) handleExtractor(ctx context.Context, evt InboxEvent) error {
	if err := t.client.Ack(ctx, "pdf-extractor", t.secrets["pdf-extractor"], evt.MessageID, "accepted", "extracting"); err != nil {
		return err
	}
	_ = t.client.Event(ctx, "pdf-extractor", t.secrets["pdf-extractor"], evt.MessageID, "progress", "extracting PDF text", map[string]any{"stage": "extract"})

	if len(evt.Attachments) == 0 {
		_ = t.client.Event(ctx, "pdf-extractor", t.secrets["pdf-extractor"], evt.MessageID, "error", "missing attachment", nil)
		return errors.New("extractor: missing attachment")
	}
	path, err := AttachmentFilePath(evt.Attachments[0])
	if err != nil {
		_ = t.client.Event(ctx, "pdf-extractor", t.secrets["pdf-extractor"], evt.MessageID, "error", err.Error(), nil)
		return err
	}

	extracted, err := ExtractPDFText(ctx, path)
	if err != nil {
		_ = t.client.Event(ctx, "pdf-extractor", t.secrets["pdf-extractor"], evt.MessageID, "error", err.Error(), nil)
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

	_, err = t.client.SendMessage(
		ctx,
		"pdf-extractor",
		t.secrets["pdf-extractor"],
		"patent-agent",
		evt.ConversationID,
		t.nextRequestID("patent"),
		"request",
		string(forwardBody),
		nil,
		map[string]any{"stage": "patent"},
	)
	if err != nil {
		_ = t.client.Event(ctx, "pdf-extractor", t.secrets["pdf-extractor"], evt.MessageID, "error", "failed to forward to patent-agent", nil)
		return err
	}

	_ = t.client.Event(ctx, "pdf-extractor", t.secrets["pdf-extractor"], evt.MessageID, "final", "extracted and forwarded to patent-agent", map[string]any{"method": extracted.Method})
	return nil
}

func (t *Team) handlePatentAgent(ctx context.Context, evt InboxEvent) error {
	if err := t.client.Ack(ctx, "patent-agent", t.secrets["patent-agent"], evt.MessageID, "accepted", "analyzing"); err != nil {
		return err
	}
	_ = t.client.Event(ctx, "patent-agent", t.secrets["patent-agent"], evt.MessageID, "progress", "running patent eligibility analysis", map[string]any{"stage": "analysis"})

	var payload struct {
		CaseID        string `json:"case_id"`
		ExtractedText string `json:"extracted_text"`
	}
	if err := json.Unmarshal([]byte(evt.Body), &payload); err != nil {
		_ = t.client.Event(ctx, "patent-agent", t.secrets["patent-agent"], evt.MessageID, "error", "invalid payload", nil)
		return err
	}

	assessment := EvaluatePatentEligibility(payload.CaseID, payload.ExtractedText)
	blob, _ := json.Marshal(assessment)

	_, err := t.client.SendMessage(
		ctx,
		"patent-agent",
		t.secrets["patent-agent"],
		"reporter",
		evt.ConversationID,
		t.nextRequestID("report"),
		"request",
		string(blob),
		nil,
		map[string]any{"stage": "report"},
	)
	if err != nil {
		_ = t.client.Event(ctx, "patent-agent", t.secrets["patent-agent"], evt.MessageID, "error", "failed to forward to reporter", nil)
		return err
	}

	_ = t.client.Event(ctx, "patent-agent", t.secrets["patent-agent"], evt.MessageID, "final", "assessment completed", map[string]any{"eligibility": assessment.Eligibility})
	return nil
}

func (t *Team) handleReporter(ctx context.Context, evt InboxEvent) error {
	if err := t.client.Ack(ctx, "reporter", t.secrets["reporter"], evt.MessageID, "accepted", "rendering"); err != nil {
		return err
	}

	var assessment PatentAssessment
	if err := json.Unmarshal([]byte(evt.Body), &assessment); err != nil {
		_ = t.client.Event(ctx, "reporter", t.secrets["reporter"], evt.MessageID, "error", "invalid assessment payload", nil)
		return err
	}
	finalReport := renderFinalReport(assessment)

	_, err := t.client.SendMessage(
		ctx,
		"reporter",
		t.secrets["reporter"],
		"coordinator",
		evt.ConversationID,
		t.nextRequestID("final"),
		"response",
		finalReport,
		nil,
		map[string]any{"stage": "done"},
	)
	if err != nil {
		_ = t.client.Event(ctx, "reporter", t.secrets["reporter"], evt.MessageID, "error", "failed to send final report", nil)
		return err
	}

	_ = t.client.Event(ctx, "reporter", t.secrets["reporter"], evt.MessageID, "final", "report delivered", nil)
	return nil
}

func (t *Team) readCoordinatorResult(ctx context.Context) (string, PatentAssessment, bool, error) {
	cursor := t.cursors["coordinator"]
	events, next, err := t.client.PollInbox(ctx, "coordinator", t.secrets["coordinator"], cursor, 0)
	if err != nil {
		return "", PatentAssessment{}, false, err
	}
	t.cursors["coordinator"] = next
	for _, evt := range events {
		if evt.From != "reporter" || evt.Type != "response" {
			continue
		}
		assessment := extractAssessmentFromReport(evt.Body)
		return evt.Body, assessment, true, nil
	}
	return "", PatentAssessment{}, false, nil
}

func extractAssessmentFromReport(report string) PatentAssessment {
	assessment := PatentAssessment{}
	idx := strings.Index(report, "\n\nRAW_JSON:\n")
	if idx < 0 {
		return assessment
	}
	raw := strings.TrimSpace(report[idx+len("\n\nRAW_JSON:\n"):])
	_ = json.Unmarshal([]byte(raw), &assessment)
	return assessment
}

func renderFinalReport(a PatentAssessment) string {
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
