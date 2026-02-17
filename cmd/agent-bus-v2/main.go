package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joelkehle/agent-bus-v2/internal/bus"
	"github.com/joelkehle/agent-bus-v2/internal/httpapi"
)

func main() {
	addr := ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}

	cfg := bus.Config{
		GracePeriod:            30 * time.Second,
		ProgressMinInterval:    2 * time.Second,
		IdempotencyWindow:      24 * time.Hour,
		InboxWaitMax:           60 * time.Second,
		AckTimeout:             10 * time.Second,
		DefaultMessageTTL:      600 * time.Second,
		DefaultRegistrationTTL: 60 * time.Second,
		PushMaxAttempts:        3,
		PushBaseBackoff:        500 * time.Millisecond,
		MaxInboxEventsPerAgent: 10000,
		MaxObserveEvents:       50000,
	}

	var store bus.API
	backend := os.Getenv("STORE_BACKEND")
	if backend == "" {
		backend = "persistent"
	}

	switch backend {
	case "memory":
		store = bus.NewStore(cfg)
	default:
		statePath := os.Getenv("STATE_FILE")
		if statePath == "" {
			statePath = "./data/state.json"
		}
		ps, err := bus.NewPersistentStore(statePath, cfg)
		if err != nil {
			log.Fatalf("failed to initialize persistent store (%s): %v", statePath, err)
		}
		store = ps
		log.Printf("using persistent store at %s", statePath)
	}

	h := httpapi.NewServer(store)
	log.Printf("agent-bus-v2 listening on %s", addr)
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Fatal(err)
	}
}
