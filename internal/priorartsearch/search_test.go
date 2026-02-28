package priorartsearch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNormalizeTokensPreservesTwoLetterAcronyms(t *testing.T) {
	families := []TermFamily{{
		Canonical: "federated learning",
		Synonyms:  []string{"distributed", "method"},
		Acronyms:  []string{"AI", "ML"},
	}}
	toks := normalizeTokens(families)
	joined := map[string]bool{}
	for _, tok := range toks {
		joined[tok] = true
	}
	if !joined["ai"] || !joined["ml"] {
		t.Fatalf("expected ai/ml preserved, got %v", toks)
	}
	if joined["method"] {
		t.Fatalf("expected stopword removed, got %v", toks)
	}
}

func TestStrategyCountByPrefix(t *testing.T) {
	got := strategyCount([]string{"Q1_narrow_p1", "Q1_broad", "Q2_broad"})
	if got != 2 {
		t.Fatalf("expected 2, got %d", got)
	}
}

func TestExecuteOnceErrorFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":true,"count":0,"total_hits":0,"patents":[]}`))
	}))
	defer srv.Close()

	s, err := NewSearcher(SearchConfig{APIKey: "x", BaseURL: srv.URL, HTTPClient: srv.Client(), RateLimitPerMinute: 60000})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = s.executeOnce(context.Background(), map[string]any{"q": map[string]any{}, "s": []map[string]string{{"patent_date": "desc"}}})
	if err == nil {
		t.Fatal("expected error when error=true")
	}
}

func TestSearcherRunHappyPathFlattenAndDedup(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		if idx == 1 {
			_, _ = w.Write([]byte(`{"error":false,"count":1,"total_hits":10,"patents":[{"patent_id":"123","patent_title":"T1","patent_abstract":"A","patent_date":"2024-01-01","application":{"filing_date":"2020-01-01"},"assignees":[{"assignee_organization":"  Org   Name  "}],"cpc_at_issue":[{"cpc_subclass_id":"G06N"},{"cpc_subclass_id":"G06N"}],"inventors":[{"inventor_name_first":"A","inventor_name_last":"B"}]}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"error":false,"count":1,"total_hits":12,"patents":[{"patent_id":"123","patent_title":"T1","patent_abstract":"A","patent_date":"2024-01-01","application":{"filing_date":"2020-01-01"},"assignees":[{"assignee_organization":"Org Name"}],"cpc_at_issue":[{"cpc_subclass_id":"G06N"}],"inventors":[{"inventor_name_first":"A","inventor_name_last":"B"}]}]}`))
	}))
	defer srv.Close()

	s, err := NewSearcher(SearchConfig{APIKey: "x", BaseURL: srv.URL, HTTPClient: srv.Client(), RateLimitPerMinute: 60000})
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Run(context.Background(), testStage1OneStrategy())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Patents) != 1 {
		t.Fatalf("expected dedup to 1 patent, got %d", len(out.Patents))
	}
	if out.Patents[0].StrategyCount != 1 {
		t.Fatalf("expected strategy count 1, got %d", out.Patents[0].StrategyCount)
	}
	if len(out.Patents[0].MatchedQueries) < 2 {
		t.Fatalf("expected matched_queries from multiple calls, got %v", out.Patents[0].MatchedQueries)
	}
	if out.Patents[0].Assignees[0] != "Org Name" {
		t.Fatalf("expected normalized assignee, got %q", out.Patents[0].Assignees[0])
	}
}

func TestSearcherRunAuthFailure403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":true}`))
	}))
	defer srv.Close()

	s, _ := NewSearcher(SearchConfig{APIKey: "x", BaseURL: srv.URL, HTTPClient: srv.Client(), RateLimitPerMinute: 60000})
	_, err := s.Run(context.Background(), testStage1OneStrategy())
	if err == nil || err.Error() != "PatentsView API authentication failed. Check PATENTSVIEW_API_KEY" {
		t.Fatalf("expected auth error, got %v", err)
	}
}

func TestSearcherRunThree400HardFail(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":true}`))
	}))
	defer srv.Close()

	s, _ := NewSearcher(SearchConfig{APIKey: "x", BaseURL: srv.URL, HTTPClient: srv.Client(), RateLimitPerMinute: 60000})
	_, err := s.Run(context.Background(), testStage1TwoStrategies())
	if err == nil || err.Error() != "Multiple PatentsView query failures — likely a query builder bug. Check logs" {
		t.Fatalf("expected 400 hard fail, got %v", err)
	}
	if calls < 3 {
		t.Fatalf("expected at least 3 calls, got %d", calls)
	}
}

func TestSearcherRunPartial400Continues(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := atomic.AddInt32(&calls, 1)
		if idx == 1 || idx == 2 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":true}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":false,"count":1,"total_hits":1,"patents":[{"patent_id":"321","patent_title":"T","patent_abstract":"A","patent_date":"2024-01-01","assignees":[],"cpc_at_issue":[],"inventors":[]}]}`))
	}))
	defer srv.Close()

	s, _ := NewSearcher(SearchConfig{APIKey: "x", BaseURL: srv.URL, HTTPClient: srv.Client(), RateLimitPerMinute: 60000})
	out, err := s.Run(context.Background(), testStage1TwoStrategies())
	if err != nil {
		t.Fatalf("expected continue on partial 400 failures, got %v", err)
	}
	if out.QueriesFailed < 2 || len(out.Patents) == 0 {
		t.Fatalf("expected failures with continued results, failed=%d patents=%d", out.QueriesFailed, len(out.Patents))
	}
}

func TestSearcherRunAll500Unavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":true}`))
	}))
	defer srv.Close()

	s, _ := NewSearcher(SearchConfig{APIKey: "x", BaseURL: srv.URL, HTTPClient: srv.Client(), RateLimitPerMinute: 60000})
	_, err := s.Run(context.Background(), testStage1OneStrategy())
	if err == nil || err.Error() != "PatentsView API unavailable" {
		t.Fatalf("expected API unavailable, got %v", err)
	}
}

func TestSearcherRunResultCapCompletesCurrentStrategy(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error":false,"count":1,"total_hits":1,"patents":[{"patent_id":"%d","patent_title":"T","patent_abstract":"A","patent_date":"2024-01-01","assignees":[],"cpc_at_issue":[],"inventors":[]}]}`, idx)))
	}))
	defer srv.Close()

	s, _ := NewSearcher(SearchConfig{APIKey: "x", BaseURL: srv.URL, HTTPClient: srv.Client(), RateLimitPerMinute: 60000, MaxPatents: 1})
	_, err := s.Run(context.Background(), testStage1TwoStrategies())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("expected only first strategy queries after cap (3 calls), got %d", calls)
	}
}

func TestNormalizeCPCsDropsInvalid(t *testing.T) {
	out := normalizeCPCs([]string{"G06N3/08", "g06n", "H04L", "", "H04L"})
	if len(out) != 2 || out[0] != "G06N" || out[1] != "H04L" {
		t.Fatalf("unexpected cpc normalization: %v", out)
	}
}

func TestExecuteWithRetry429ThenSuccess(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := atomic.AddInt32(&calls, 1)
		if idx == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":true}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":false,"count":0,"total_hits":0,"patents":[]}`))
	}))
	defer srv.Close()
	s, _ := NewSearcher(SearchConfig{APIKey: "x", BaseURL: srv.URL, HTTPClient: srv.Client(), RateLimitPerMinute: 60000})
	_, _, attempts, err := s.executeWithRetry(context.Background(), map[string]any{"q": map[string]any{}, "s": []map[string]string{{"patent_date": "desc"}}})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || calls != 2 {
		t.Fatalf("expected 2 attempts/calls, got attempts=%d calls=%d", attempts, calls)
	}
}

func TestExecuteWithRetryTimeoutRetriesOnce(t *testing.T) {
	timeoutClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		}),
	}
	s, _ := NewSearcher(SearchConfig{APIKey: "x", BaseURL: "http://example.invalid", HTTPClient: timeoutClient, RateLimitPerMinute: 60000})
	start := nowMilli()
	_, _, attempts, err := s.executeWithRetry(context.Background(), map[string]any{"q": map[string]any{}, "s": []map[string]string{{"patent_date": "desc"}}})
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected one retry on timeout (2 attempts), got %d", attempts)
	}
	if nowMilli()-start < 900 {
		t.Fatalf("expected backoff delay on retry")
	}
}

func testStage1OneStrategy() Stage1Output {
	return Stage1Output{
		QueryStrategies: []QueryStrategy{{
			ID:            "Q1",
			Description:   "desc desc desc desc desc",
			Priority:      PriorityPrimary,
			TermFamilies:  []TermFamily{{Canonical: "federated", Synonyms: []string{"distributed"}, Acronyms: []string{"AI"}}},
			Phrases:       []string{"federated learning"},
			CPCSubclasses: []string{"G06N"},
		}},
	}
}

func testStage1TwoStrategies() Stage1Output {
	s := testStage1OneStrategy()
	s.QueryStrategies[0].Phrases = []string{"p1", "p2"}
	s.QueryStrategies = append(s.QueryStrategies, QueryStrategy{
		ID:            "Q2",
		Description:   "desc desc desc desc desc",
		Priority:      PrioritySecondary,
		TermFamilies:  []TermFamily{{Canonical: "privacy", Synonyms: []string{"secure"}, Acronyms: []string{"ML"}}},
		Phrases:       []string{"federated privacy"},
		CPCSubclasses: []string{"H04L"},
	})
	return s
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func nowMilli() int64 { return time.Now().UnixMilli() }
