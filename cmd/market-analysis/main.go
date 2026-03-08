package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joelkehle/techtransfer-agency/internal/marketanalysis"
	"github.com/joelkehle/techtransfer-agency/internal/telemetry"
)

func main() {
	busURL := flag.String("bus-url", "http://localhost:8080", "Bus base URL")
	agentID := flag.String("agent-id", "market-analysis", "Agent ID")
	flag.Parse()

	secret := requiredEnv("MARKET_ANALYSIS_AGENT_SECRET")
	caller, err := marketanalysis.NewAnthropicCallerFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	exec := marketanalysis.NewStageExecutor(caller)
	runner := marketanalysis.NewLLMStageRunner(exec)
	pipeline := marketanalysis.NewPipeline(runner)
	agent := marketanalysis.NewAgent(marketanalysis.AgentConfig{
		BusURL:  *busURL,
		AgentID: *agentID,
		Secret:  secret,
	}, pipeline)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	shutdownTelemetry, err := telemetry.InitFromEnv(ctx, "market-analysis")
	if err != nil {
		log.Fatalf("init telemetry: %v", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := shutdownTelemetry(shutdownCtx); err != nil {
			log.Printf("telemetry shutdown error: %v", err)
		}
	}()

	log.Printf("starting market-analysis agent (bus=%s, agent=%s)", *busURL, *agentID)
	if err := agent.Run(ctx); err != nil && err != context.Canceled {
		log.Fatal(err)
	}
}

func requiredEnv(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		log.Fatalf("missing required env var %s", key)
	}
	return v
}
