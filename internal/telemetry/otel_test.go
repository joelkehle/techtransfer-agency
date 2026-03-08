package telemetry

import "testing"

func TestParseHeaders(t *testing.T) {
	got := parseHeaders("Authorization=Basic%20abc%3D%3D, x-test = 123 ,bad")
	if got["Authorization"] != "Basic abc==" {
		t.Fatalf("Authorization header mismatch: %q", got["Authorization"])
	}
	if got["x-test"] != "123" {
		t.Fatalf("x-test header mismatch: %q", got["x-test"])
	}
	if _, ok := got["bad"]; ok {
		t.Fatal("did not expect invalid header entry")
	}
}

func TestTraceSamplingRatio(t *testing.T) {
	t.Setenv("OTEL_TRACE_SAMPLING_RATIO", "")
	if traceSamplingRatio() != 1.0 {
		t.Fatal("expected default ratio 1.0")
	}

	t.Setenv("OTEL_TRACE_SAMPLING_RATIO", "0.25")
	if traceSamplingRatio() != 0.25 {
		t.Fatal("expected ratio 0.25")
	}

	t.Setenv("OTEL_TRACE_SAMPLING_RATIO", "-2")
	if traceSamplingRatio() != 0 {
		t.Fatal("expected ratio clamp to 0")
	}

	t.Setenv("OTEL_TRACE_SAMPLING_RATIO", "2")
	if traceSamplingRatio() != 1 {
		t.Fatal("expected ratio clamp to 1")
	}
}
