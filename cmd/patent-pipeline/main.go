package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joelkehle/techtransfer-agency/internal/patentteam"
)

func main() {
	busURL := flag.String("bus-url", "http://localhost:8080", "Bus base URL")
	flag.Parse()

	intakeSecret := requiredEnv("PATENT_PIPELINE_INTAKE_SECRET")
	extractorSecret := requiredEnv("PATENT_PIPELINE_EXTRACTOR_SECRET")
	evaluatorSecret := requiredEnv("PATENT_PIPELINE_EVALUATOR_SECRET")
	reporterSecret := requiredEnv("PATENT_PIPELINE_REPORTER_SECRET")

	cfg := patentteam.PipelineConfig{
		BusURL: *busURL,
		Intake: patentteam.AgentConfig{
			ID:           "patent-intake",
			Secret:       intakeSecret,
			Capabilities: []string{"patent-screen"},
		},
		Extractor: patentteam.AgentConfig{
			ID:           "patent-pdf-extractor",
			Secret:       extractorSecret,
			Capabilities: []string{"pdf-extract"},
		},
		Evaluator: patentteam.AgentConfig{
			ID:           "patent-evaluator",
			Secret:       evaluatorSecret,
			Capabilities: []string{"patent-eligibility"},
		},
		Reporter: patentteam.AgentConfig{
			ID:           "patent-reporter",
			Secret:       reporterSecret,
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

func requiredEnv(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		log.Fatalf("missing required env var %s", key)
	}
	return v
}
