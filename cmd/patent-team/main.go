package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joelkehle/techtransfer-agency/internal/bus"
	"github.com/joelkehle/techtransfer-agency/internal/httpapi"
	"github.com/joelkehle/techtransfer-agency/internal/patentteam"
)

func main() {
	var (
		busURL       = flag.String("bus-url", "", "Existing bus base URL (default: start local in-process bus)")
		pdfPath      = flag.String("pdf", "", "Path to invention PDF")
		caseID       = flag.String("case-id", "CASE-001", "Case identifier")
		conversation = flag.String("conversation-id", "", "Conversation ID (optional)")
		timeout      = flag.Duration("timeout", 30*time.Second, "End-to-end run timeout")
	)
	flag.Parse()

	if strings.TrimSpace(*pdfPath) == "" {
		log.Fatal("--pdf is required")
	}

	finalBusURL := strings.TrimSpace(*busURL)
	var shutdown func(context.Context) error
	if finalBusURL == "" {
		url, stop, err := startLocalBus()
		if err != nil {
			log.Fatalf("start local bus: %v", err)
		}
		finalBusURL = url
		shutdown = stop
		log.Printf("started local bus at %s", finalBusURL)
	}
	if shutdown != nil {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = shutdown(ctx)
		}()
	}

	team := patentteam.NewTeam(patentteam.TeamConfig{
		CaseID:         *caseID,
		BusURL:         finalBusURL,
		PDFPath:        *pdfPath,
		Timeout:        *timeout,
		ConversationID: *conversation,
		Secrets: map[string]string{
			"coordinator":   requiredEnv("PATENT_TEAM_COORDINATOR_SECRET"),
			"intake":        requiredEnv("PATENT_TEAM_INTAKE_SECRET"),
			"pdf-extractor": requiredEnv("PATENT_TEAM_EXTRACTOR_SECRET"),
			"patent-agent":  requiredEnv("PATENT_TEAM_PATENT_AGENT_SECRET"),
			"reporter":      requiredEnv("PATENT_TEAM_REPORTER_SECRET"),
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	result, err := team.Run(ctx)
	if err != nil {
		log.Fatalf("team run failed: %v", err)
	}

	fmt.Printf("conversation_id=%s\n\n%s\n", result.ConversationID, result.FinalReport)
}

func requiredEnv(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		log.Fatalf("missing required env var %s", key)
	}
	return v
}

func startLocalBus() (string, func(context.Context) error, error) {
	cfg := bus.Config{
		GracePeriod:            30 * time.Second,
		ProgressMinInterval:    250 * time.Millisecond,
		IdempotencyWindow:      24 * time.Hour,
		InboxWaitMax:           1 * time.Second,
		AckTimeout:             10 * time.Second,
		DefaultMessageTTL:      600 * time.Second,
		DefaultRegistrationTTL: 120 * time.Second,
		PushMaxAttempts:        2,
		PushBaseBackoff:        100 * time.Millisecond,
		MaxInboxEventsPerAgent: 10000,
		MaxObserveEvents:       50000,
	}
	store := bus.NewStore(cfg)
	h := httpapi.NewServer(store)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	srv := &http.Server{Handler: h}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "bus server error: %v\n", err)
		}
	}()
	return "http://" + ln.Addr().String(), srv.Shutdown, nil
}
