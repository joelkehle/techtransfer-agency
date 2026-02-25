package pdfextractor

import (
	"context"
	"log"
	"time"

	"github.com/joelkehle/techtransfer-agency/internal/patentteam"
)

const CapabilityPatentScreen = "patent-screen"

type AgentConfig struct {
	BusURL      string
	AgentID     string
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
	if cfg.NextAgentID == "" {
		cfg.NextAgentID = "patent-screen"
	}
	return &Agent{cfg: cfg, client: NewClient(cfg.BusURL)}
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
				log.Printf("patent-extractor heartbeat register failed: %v", err)
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
				if err := a.handleEvent(ctx, evt); err != nil {
					log.Printf("patent-extractor handle event failed: %v", err)
				}
			}
		}
	}
}

func (a *Agent) register(ctx context.Context) error {
	return a.client.RegisterAgent(ctx, a.cfg.AgentID, a.cfg.Secret, []string{CapabilityPatentScreen})
}

func (a *Agent) handleEvent(ctx context.Context, evt InboxEvent) error {
	return patentteam.HandleExtractorMessage(ctx, a.client, a.cfg.AgentID, a.cfg.Secret, evt, a.cfg.NextAgentID)
}
