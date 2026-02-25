package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joelkehle/techtransfer-agency/internal/patentscreen"
)

func main() {
	busURL := flag.String("bus-url", "http://localhost:8080", "Bus base URL")
	agentID := flag.String("agent-id", "patent-screen", "Agent ID")
	flag.Parse()

	secret := requiredEnv("PATENT_SCREEN_AGENT_SECRET")
	caller, err := patentscreen.NewAnthropicCallerFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	exec := patentscreen.NewStageExecutor(caller)
	runner := patentscreen.NewLLMStageRunner(exec)
	pipeline := patentscreen.NewPipeline(runner)
	agent := patentscreen.NewAgent(patentscreen.AgentConfig{
		BusURL:  *busURL,
		AgentID: *agentID,
		Secret:  secret,
	}, pipeline)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Printf("starting patent-screen agent (bus=%s, agent=%s)", *busURL, *agentID)
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
