package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/joelkehle/agent-bus-v2/internal/concierge"
)

func main() {
	var (
		busURL   = flag.String("bus-url", "http://localhost:8080", "Agent bus base URL")
		addr     = flag.String("addr", ":8090", "Concierge listen address")
		agentID  = flag.String("agent-id", "concierge", "Agent ID to register on the bus")
		secret   = flag.String("secret", "secret-concierge", "Agent secret for bus authentication")
		webDir   = flag.String("web-dir", "", "Directory containing web UI files (default: web/ relative to binary)")
		uploadDir = flag.String("upload-dir", "./uploads", "Directory for uploaded files")
	)
	flag.Parse()

	if strings.TrimSpace(*busURL) == "" {
		log.Fatal("--bus-url is required")
	}

	web := *webDir
	if web == "" {
		exe, _ := os.Executable()
		web = filepath.Join(filepath.Dir(exe), "..", "..", "web")
		if _, err := os.Stat(web); err != nil {
			web = "web"
		}
	}

	store := concierge.NewSubmissionStore()
	bridge := concierge.NewBridge(*busURL, *agentID, *secret, store)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := bridge.Register(ctx); err != nil {
		log.Printf("warning: initial bus registration failed: %v (will retry via heartbeat)", err)
	}

	go bridge.PollLoop(ctx)
	go bridge.Heartbeat(ctx)

	handler := concierge.NewServer(bridge, store, web, *uploadDir)

	log.Printf("concierge listening on %s (bus=%s, agent=%s)", *addr, *busURL, *agentID)
	srv := &http.Server{Addr: *addr, Handler: handler}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
