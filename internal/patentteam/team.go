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
	return HandleIntakeMessage(ctx, t.client, "intake", t.secrets["intake"], evt, "pdf-extractor")
}

func (t *Team) handleExtractor(ctx context.Context, evt InboxEvent) error {
	return HandleExtractorMessage(ctx, t.client, "pdf-extractor", t.secrets["pdf-extractor"], evt, "patent-agent")
}

func (t *Team) handlePatentAgent(ctx context.Context, evt InboxEvent) error {
	return HandlePatentAgentMessage(ctx, t.client, "patent-agent", t.secrets["patent-agent"], evt, "reporter")
}

func (t *Team) handleReporter(ctx context.Context, evt InboxEvent) error {
	return HandleReporterMessageTo(ctx, t.client, "reporter", t.secrets["reporter"], evt, "coordinator")
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

