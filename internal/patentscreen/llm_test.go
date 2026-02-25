package patentscreen

import "testing"

func TestStripCodeFences(t *testing.T) {
	in := "```json\n{\"a\":1}\n```"
	got := stripCodeFences(in)
	if got != "{\"a\":1}" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestBackoffDelay(t *testing.T) {
	if backoffDelay(1).Seconds() != 1 {
		t.Fatal("attempt 1 should be 1s")
	}
	if backoffDelay(2).Seconds() != 2 {
		t.Fatal("attempt 2 should be 2s")
	}
}

func TestClassifyTransportErrorAvoidsBroadNumericMatch(t *testing.T) {
	err := classifyTransportError(assertErr("failed after 5 retries while waiting 4 seconds"))
	if err != failureServer {
		t.Fatalf("expected default server classification, got %v", err)
	}
	err = classifyTransportError(assertErr("status code: 400 bad request"))
	if err != failureClient {
		t.Fatalf("expected client failure classification, got %v", err)
	}
	err = classifyTransportError(assertErr("status=500 upstream error"))
	if err != failureServer {
		t.Fatalf("expected server failure classification, got %v", err)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
