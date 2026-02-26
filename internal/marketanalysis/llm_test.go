package marketanalysis

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
