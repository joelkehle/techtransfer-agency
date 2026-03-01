package patentscreen

import (
	"os"
	"testing"
)

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

func TestNewAnthropicCallerFromEnvDisabled(t *testing.T) {
	t.Setenv("PATENT_SCREEN_NO_LLM", "1")
	t.Setenv("ANTHROPIC_API_KEY", "ignored")
	_, err := NewAnthropicCallerFromEnv()
	if err == nil {
		t.Fatal("expected error when PATENT_SCREEN_NO_LLM is enabled")
	}
}

func TestEnvEnabled(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{value: "", want: false},
		{value: "0", want: false},
		{value: "false", want: false},
		{value: "1", want: true},
		{value: "TRUE", want: true},
		{value: "yes", want: true},
		{value: "on", want: true},
	} {
		if tc.value == "" {
			_ = os.Unsetenv("X_FLAG")
		} else {
			t.Setenv("X_FLAG", tc.value)
		}
		if got := envEnabled("X_FLAG"); got != tc.want {
			t.Fatalf("envEnabled(%q) got %v, want %v", tc.value, got, tc.want)
		}
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
