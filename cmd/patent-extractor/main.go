package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joelkehle/techtransfer-agency/internal/pdfextractor"
)

func main() {
	busURL := flag.String("bus-url", "http://localhost:8080", "Bus base URL")
	agentID := flag.String("agent-id", "patent-extractor", "Agent ID")
	capability := flag.String("capability", "patent-screen", "Capability to register on the bus")
	secretEnv := flag.String("secret-env", "PATENT_EXTRACTOR_AGENT_SECRET", "Environment variable name containing extractor secret")
	nextAgentID := flag.String("next-agent-id", "patent-screen", "Destination agent ID for patent screening")
	flag.Parse()

	secret := requiredEnv(*secretEnv)
	agent := pdfextractor.NewAgent(pdfextractor.AgentConfig{
		BusURL:      *busURL,
		AgentID:     *agentID,
		Capability:  *capability,
		Secret:      secret,
		NextAgentID: *nextAgentID,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Printf("starting patent-extractor agent (bus=%s, agent=%s, capability=%s, next=%s)", *busURL, *agentID, *capability, *nextAgentID)
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
