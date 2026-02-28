package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/joelkehle/techtransfer-agency/internal/priorartsearch"
)

func main() {
	busURL := flag.String("bus-url", "http://localhost:8080", "Bus base URL")
	agentID := flag.String("agent-id", "prior-art-search", "Agent ID")
	flag.Parse()

	secret := requiredEnv("PRIOR_ART_AGENT_SECRET")
	caller, err := priorartsearch.NewAnthropicCallerFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	exec := priorartsearch.NewStageExecutor(caller)
	runner := priorartsearch.NewLLMStageRunner(exec, envInt("PRIOR_ART_MAX_ASSESS", priorartsearch.DefaultMaxAssess), envInt("PRIOR_ART_BATCH_SIZE", priorartsearch.DefaultBatchSize))
	searcher, err := priorartsearch.NewSearcher(priorartsearch.SearchConfig{
		APIKey:             requiredEnv("PATENTSVIEW_API_KEY"),
		MaxPatents:         envInt("PRIOR_ART_MAX_PATENTS", priorartsearch.DefaultMaxPatents),
		RateLimitPerMinute: envInt("PRIOR_ART_RATE_LIMIT", priorartsearch.DefaultRateLimitPerMinute),
	})
	if err != nil {
		log.Fatal(err)
	}
	pipeline := priorartsearch.NewPipeline(runner, searcher)
	if err := pipeline.ValidateConfig(); err != nil {
		log.Fatal(err)
	}

	agent := priorartsearch.NewAgent(priorartsearch.AgentConfig{BusURL: *busURL, AgentID: *agentID, Secret: secret}, pipeline)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Printf("starting prior-art-search agent (bus=%s, agent=%s)", *busURL, *agentID)
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

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	if n <= 0 {
		return fallback
	}
	return n
}
