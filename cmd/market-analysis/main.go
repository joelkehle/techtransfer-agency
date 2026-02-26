package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joelkehle/techtransfer-agency/internal/marketanalysis"
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
