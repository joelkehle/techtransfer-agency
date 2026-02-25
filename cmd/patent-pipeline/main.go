package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joelkehle/techtransfer-agency/internal/patentteam"
)

func main() {
	busURL := flag.String("bus-url", "http://localhost:8080", "Bus base URL")
	flag.Parse()

	cfg := patentteam.PipelineConfig{
		BusURL: *busURL,
		Intake: patentteam.AgentConfig{
			ID:           "patent-intake",
			Secret:       "secret-patent-intake",
			Capabilities: []string{"patent-screen"},
		},
		Extractor: patentteam.AgentConfig{
			ID:           "patent-pdf-extractor",
			Secret:       "secret-patent-pdf-extractor",
			Capabilities: []string{"pdf-extract"},
		},
		Evaluator: patentteam.AgentConfig{
			ID:           "patent-evaluator",
			Secret:       "secret-patent-evaluator",
			Capabilities: []string{"patent-eligibility"},
		},
		Reporter: patentteam.AgentConfig{
			ID:           "patent-reporter",
			Secret:       "secret-patent-reporter",
			Capabilities: []string{"patent-report"},
		},
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	svc := patentteam.NewPipelineService(cfg)
	log.Printf("starting patent pipeline (bus=%s)", *busURL)
	if err := svc.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("pipeline: %v", err)
	}
	log.Println("patent pipeline stopped")
}
