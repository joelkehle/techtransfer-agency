//go:build integration

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joelkehle/techtransfer-agency/internal/bus"
	"github.com/joelkehle/techtransfer-agency/internal/busclient"
	"github.com/joelkehle/techtransfer-agency/internal/httpapi"
	"github.com/joelkehle/techtransfer-agency/internal/operator"
)

// minimalPDF returns a valid PDF that contains some text.
func minimalPDF() []byte {
	// A hand-crafted minimal valid PDF with text containing enough technical
	// keywords to trigger the "likely_eligible" branch in the evaluator.
	content := `%PDF-1.0
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]
   /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>
endobj
4 0 obj
<< /Length 280 >>
stream
BT
/F1 12 Tf
72 720 Td
(Novel Algorithm for Low Latency Signal Processing) Tj
0 -20 Td
(This disclosure describes a new model architecture for real-time sensor) Tj
0 -20 Td
(data processing with improved throughput and reduced latency.) Tj
0 -20 Td
(The protocol uses hardware encryption for secure inference at the edge.) Tj
0 -20 Td
(Training is performed on a custom dataset with a novel compiler backend.) Tj
ET
endstream
endobj
5 0 obj
<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>
endobj
xref
0 6
0000000000 65535 f
0000000009 00000 n
0000000058 00000 n
0000000115 00000 n
0000000266 00000 n
0000000598 00000 n
trailer
<< /Size 6 /Root 1 0 R >>
startxref
675
%%EOF`
	return []byte(content)
}

func TestE2EPatentScreening(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// --- 1. Start bus server in-process ---
	store := bus.NewStore(bus.Config{
		GracePeriod:            30 * time.Second,
		ProgressMinInterval:    100 * time.Millisecond,
		IdempotencyWindow:      1 * time.Hour,
		InboxWaitMax:           5 * time.Second,
		AckTimeout:             10 * time.Second,
		DefaultMessageTTL:      60 * time.Second,
		DefaultRegistrationTTL: 120 * time.Second,
		PushMaxAttempts:        1,
		PushBaseBackoff:        100 * time.Millisecond,
		MaxInboxEventsPerAgent: 1000,
		MaxObserveEvents:       1000,
	})
	busHandler := httpapi.NewServer(store)
	busLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen bus: %v", err)
	}
	busSrv := &http.Server{Handler: busHandler}
	go busSrv.Serve(busLn)
	defer busSrv.Close()

	busURL := "http://" + busLn.Addr().String()
	t.Logf("bus running at %s", busURL)

	// --- 2. Start operator bridge in-process ---
	operatorStore := operator.NewSubmissionStore()
	bridge := operator.NewBridge(busURL, "operator", "secret-operator", operatorStore)

	if err := bridge.Register(ctx); err != nil {
		t.Fatalf("register operator: %v", err)
	}
	pollCtx, pollCancel := context.WithCancel(ctx)
	defer pollCancel()
	go bridge.PollLoop(pollCtx)

	// Set up upload dir in a temp directory.
	uploadDir := t.TempDir()
	webDir := t.TempDir()

	operatorLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen operator: %v", err)
	}
	operatorHandler := operator.NewServer(bridge, operatorStore, webDir, uploadDir)
	operatorSrv := &http.Server{Handler: operatorHandler}
	go operatorSrv.Serve(operatorLn)
	defer operatorSrv.Close()

	operatorURL := "http://" + operatorLn.Addr().String()
	t.Logf("operator running at %s", operatorURL)

	// --- 3. Register a dummy agent with "patent-screen" capability ---
	dummyAgentID := "dummy-patent-screener"
	dummySecret := "secret-dummy"
	dummyClient := busclient.NewClient(busURL)

	if err := dummyClient.RegisterAgent(ctx, dummyAgentID, dummySecret, []string{"patent-screen"}); err != nil {
		t.Fatalf("register dummy agent: %v", err)
	}
	t.Log("dummy agent registered")

	// --- 4. Create test PDF fixture ---
	pdfPath := filepath.Join(t.TempDir(), "test-disclosure.pdf")
	if err := os.WriteFile(pdfPath, minimalPDF(), 0o644); err != nil {
		t.Fatalf("write test PDF: %v", err)
	}

	// --- 5. POST to /submit with the test PDF ---
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("workflows", "patent-screen")
	_ = writer.WriteField("case_id", "TEST-001")
	part, err := writer.CreateFormFile("file", "test-disclosure.pdf")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	pdfData := minimalPDF()
	if _, err := part.Write(pdfData); err != nil {
		t.Fatalf("write pdf to form: %v", err)
	}
	writer.Close()

	resp, err := http.Post(operatorURL+"/submit", writer.FormDataContentType(), &body)
	if err != nil {
		t.Fatalf("POST /submit: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /submit returned %d: %s", resp.StatusCode, string(respBody))
	}

	var submitResp struct {
		Token     string   `json:"token"`
		Workflows []string `json:"workflows"`
		Status    string   `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&submitResp); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	if submitResp.Token == "" {
		t.Fatal("submit response missing token")
	}
	t.Logf("submitted: token=%s workflows=%v", submitResp.Token, submitResp.Workflows)

	// --- 3b. Run dummy agent: poll inbox, ack, respond with canned report ---
	// The dummy agent acts as a simple request-response agent. It polls for the
	// request from operator, acknowledges it, and sends back a canned response.
	cannedReport := "Patent Eligibility Screening\nCase: TEST-001\nEligibility: likely_eligible\nConfidence: 0.85\n\nSummary:\nTest patent disclosure appears eligible.\n\nReasons:\n- Contains technical implementation details\n\nDisclaimer:\nAutomated screening only."

	dummyDone := make(chan error, 1)
	go func() {
		cursor := 0
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			events, next, err := dummyClient.PollInbox(ctx, dummyAgentID, dummySecret, cursor, 1)
			if err != nil {
				dummyDone <- fmt.Errorf("dummy poll: %w", err)
				return
			}
			cursor = next
			for _, evt := range events {
				if evt.Type != "request" {
					continue
				}
				// Ack the message.
				if err := dummyClient.Ack(ctx, dummyAgentID, dummySecret, evt.MessageID, "accepted", ""); err != nil {
					dummyDone <- fmt.Errorf("dummy ack: %w", err)
					return
				}
				// Post final event.
				if err := dummyClient.Event(ctx, dummyAgentID, dummySecret, evt.MessageID, "final", "done", nil); err != nil {
					dummyDone <- fmt.Errorf("dummy event: %w", err)
					return
				}
				// Send response back to operator.
				_, err := dummyClient.SendMessage(
					ctx,
					dummyAgentID,
					dummySecret,
					evt.From,
					evt.ConversationID,
					"dummy-response-1",
					"response",
					cannedReport,
					nil,
					nil,
				)
				if err != nil {
					dummyDone <- fmt.Errorf("dummy respond: %w", err)
					return
				}
				dummyDone <- nil
				return
			}
		}
		dummyDone <- fmt.Errorf("dummy agent: no request received within deadline")
	}()

	// Wait for dummy agent to process.
	select {
	case err := <-dummyDone:
		if err != nil {
			t.Fatalf("dummy agent error: %v", err)
		}
		t.Log("dummy agent processed request and sent response")
	case <-ctx.Done():
		t.Fatal("timeout waiting for dummy agent")
	}

	// --- 6. Poll /status/{token} until completed ---
	token := submitResp.Token
	var statusResult struct {
		Token     string         `json:"token"`
		Status    string         `json:"status"`
		Workflows map[string]any `json:"workflows"`
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		// Give the operator poll loop time to pick up the response.
		time.Sleep(500 * time.Millisecond)

		resp, err := http.Get(operatorURL + "/status/" + token)
		if err != nil {
			t.Fatalf("GET /status: %v", err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Fatalf("GET /status returned %d: %s", resp.StatusCode, string(respBody))
		}

		if err := json.Unmarshal(respBody, &statusResult); err != nil {
			t.Fatalf("decode status: %v", err)
		}
		t.Logf("status: %s", statusResult.Status)
		if statusResult.Status == "completed" {
			break
		}
		if statusResult.Status == "error" {
			t.Fatalf("workflow errored: %s", string(respBody))
		}
	}
	if statusResult.Status != "completed" {
		t.Fatalf("expected status 'completed', got %q", statusResult.Status)
	}

	// --- 7. GET /report/{token}/patent-screen ---
	resp, err = http.Get(operatorURL + "/report/" + token + "/patent-screen")
	if err != nil {
		t.Fatalf("GET /report: %v", err)
	}
	reportBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("GET /report returned %d: %s", resp.StatusCode, string(reportBody))
	}

	report := string(reportBody)
	t.Logf("report length: %d bytes", len(report))

	// Assert the report contains expected fields.
	expectedFields := []string{
		"Patent Eligibility Screening",
		"TEST-001",
		"likely_eligible",
		"Disclaimer",
	}
	for _, field := range expectedFields {
		if !bytes.Contains(reportBody, []byte(field)) {
			t.Errorf("report missing expected field %q", field)
		}
	}

	t.Log("E2E test passed: full patent screening workflow completed successfully")
}
