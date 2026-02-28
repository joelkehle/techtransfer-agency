package priorartsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var cpcSubclassRe = regexp.MustCompile(`^[A-HY][0-9]{2}[A-Z]$`)

type SearchConfig struct {
	APIKey             string
	BaseURL            string
	MaxPatents         int
	RateLimitPerMinute int
	HTTPClient         *http.Client
}

type Searcher struct {
	cfg       SearchConfig
	limiter   <-chan time.Time
	limiterMu sync.Mutex
}

func NewSearcher(cfg SearchConfig) (*Searcher, error) {
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	if cfg.APIKey == "" {
		return nil, errors.New("PATENTSVIEW_API_KEY not configured")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = PatentsViewBaseURL
	}
	if cfg.MaxPatents <= 0 {
		cfg.MaxPatents = DefaultMaxPatents
	}
	if cfg.RateLimitPerMinute <= 0 {
		cfg.RateLimitPerMinute = DefaultRateLimitPerMinute
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	interval := time.Minute / time.Duration(cfg.RateLimitPerMinute)
	ticker := time.NewTicker(interval)
	return &Searcher{cfg: cfg, limiter: ticker.C}, nil
}

type queryPlan struct {
	ID         string
	StrategyID string
	Body       map[string]any
}

type patentAPIResponse struct {
	Error     bool             `json:"error"`
	Count     int              `json:"count"`
	TotalHits int              `json:"total_hits"`
	Patents   []map[string]any `json:"patents"`
}

func (s *Searcher) Run(ctx context.Context, stage1 Stage1Output) (Stage2Output, error) {
	queries := buildQueryPlan(stage1)
	out := Stage2Output{TotalHitsByQuery: map[string]int{}, Patents: []PatentResult{}}
	if len(queries) == 0 {
		return out, errors.New("no valid patent queries generated")
	}

	patentByID := map[string]*PatentResult{}
	query400Count := 0
	limitStrategyID := ""

	for _, q := range queries {
		if limitStrategyID != "" && q.StrategyID != limitStrategyID {
			break
		}
		if err := s.waitRateLimit(ctx); err != nil {
			return out, err
		}
		resp, statusCode, attempts, err := s.executeWithRetry(ctx, q.Body)
		out.TotalAPICalls += attempts
		if err != nil {
			out.QueriesFailed++
			if statusCode == http.StatusForbidden {
				return out, errors.New("PatentsView API authentication failed. Check PATENTSVIEW_API_KEY")
			}
			if statusCode == http.StatusBadRequest {
				query400Count++
				if query400Count >= 3 {
					return out, errors.New("Multiple PatentsView query failures — likely a query builder bug. Check logs")
				}
			}
			log.Printf("prior-art-search query failed id=%s status=%d err=%v", q.ID, statusCode, err)
			continue
		}

		out.QueriesExecuted++
		out.TotalHitsByQuery[q.ID] = resp.TotalHits
		if strings.HasSuffix(q.ID, "_broad") && resp.TotalHits > 10000 {
			log.Printf("prior-art-search warning broad query id=%s total_hits=%d", q.ID, resp.TotalHits)
		}
		if strings.Contains(q.ID, "_narrow_") && resp.TotalHits == 0 {
			log.Printf("prior-art-search info narrow query id=%s returned 0 hits", q.ID)
		}

		for _, raw := range resp.Patents {
			pat := flattenPatent(raw)
			if pat.PatentID == "" {
				continue
			}
			existing := patentByID[pat.PatentID]
			if existing != nil {
				existing.MatchedQueries = appendIfMissing(existing.MatchedQueries, q.ID)
				continue
			}
			if len(patentByID) >= s.cfg.MaxPatents {
				continue
			}
			pat.MatchedQueries = []string{q.ID}
			copyPat := pat
			patentByID[pat.PatentID] = &copyPat
		}

		if len(patentByID) >= s.cfg.MaxPatents {
			limitStrategyID = q.StrategyID
		}
	}

	if out.QueriesFailed == len(queries) {
		return out, errors.New("PatentsView API unavailable")
	}

	ids := make([]string, 0, len(patentByID))
	for id := range patentByID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		pi := patentByID[ids[i]]
		pj := patentByID[ids[j]]
		if pi.GrantDate != pj.GrantDate {
			return pi.GrantDate > pj.GrantDate
		}
		return pi.PatentID < pj.PatentID
	})

	for _, id := range ids {
		p := patentByID[id]
		p.StrategyCount = strategyCount(p.MatchedQueries)
		out.Patents = append(out.Patents, *p)
	}
	return out, nil
}

func (s *Searcher) waitRateLimit(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.limiter:
		return nil
	}
}

func (s *Searcher) executeWithRetry(ctx context.Context, body map[string]any) (patentAPIResponse, int, int, error) {
	var lastErr error
	statusCode := 0
	attempts := 0
	timeoutRetried := false
	for attempt := 1; attempt <= 4; attempt++ {
		attempts++
		resp, code, retryAfter, err := s.executeOnce(ctx, body)
		statusCode = code
		if err == nil {
			return resp, statusCode, attempts, nil
		}
		lastErr = err

		if code == http.StatusBadRequest || code == http.StatusForbidden {
			return patentAPIResponse{}, statusCode, attempts, err
		}
		if code == http.StatusTooManyRequests {
			if attempt == 4 {
				break
			}
			sleep := retryAfter
			if sleep <= 0 {
				sleep = backoffDelay(attempt)
			}
			if err := sleepCtx(ctx, sleep); err != nil {
				return patentAPIResponse{}, statusCode, attempts, err
			}
			continue
		}
		if code >= 500 || errors.Is(err, context.DeadlineExceeded) {
			if isTimeoutError(err) {
				if timeoutRetried {
					break
				}
				timeoutRetried = true
			}
			if attempt == 4 {
				break
			}
			if err := sleepCtx(ctx, backoffDelay(attempt)); err != nil {
				return patentAPIResponse{}, statusCode, attempts, err
			}
			continue
		}
		return patentAPIResponse{}, statusCode, attempts, err
	}
	return patentAPIResponse{}, statusCode, attempts, lastErr
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (s *Searcher) executeOnce(ctx context.Context, body map[string]any) (patentAPIResponse, int, time.Duration, error) {
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.cfg.BaseURL, "/")+PatentsViewPatentPath, bytes.NewReader(payload))
	if err != nil {
		return patentAPIResponse{}, 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", s.cfg.APIKey)

	res, err := s.cfg.HTTPClient.Do(req)
	if err != nil {
		return patentAPIResponse{}, 0, 0, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))

	retryAfter := parseRetryAfter(res.Header.Get("Retry-After"))
	if res.StatusCode == http.StatusTooManyRequests {
		return patentAPIResponse{}, res.StatusCode, retryAfter, fmt.Errorf("status code: %d", res.StatusCode)
	}
	if res.StatusCode >= 400 {
		return patentAPIResponse{}, res.StatusCode, retryAfter, fmt.Errorf("status code: %d body=%s", res.StatusCode, string(b))
	}

	var parsed patentAPIResponse
	if err := json.Unmarshal(b, &parsed); err != nil {
		return patentAPIResponse{}, res.StatusCode, retryAfter, err
	}
	if parsed.Error {
		return patentAPIResponse{}, res.StatusCode, retryAfter, fmt.Errorf("patentsview error flag true body=%s", string(b))
	}
	return parsed, res.StatusCode, retryAfter, nil
}

func parseRetryAfter(v string) time.Duration {
	if strings.TrimSpace(v) == "" {
		return 0
	}
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err == nil {
		return time.Duration(secs) * time.Second
	}
	return 0
}

func buildQueryPlan(stage1 Stage1Output) []queryPlan {
	strategies := make([]QueryStrategy, len(stage1.QueryStrategies))
	copy(strategies, stage1.QueryStrategies)
	sort.SliceStable(strategies, func(i, j int) bool {
		return priorityRank(strategies[i].Priority) < priorityRank(strategies[j].Priority)
	})

	plans := make([]queryPlan, 0, len(strategies)*4)
	for _, st := range strategies {
		if strings.TrimSpace(st.ID) == "" {
			continue
		}
		cpcs := normalizeCPCs(st.CPCSubclasses)
		phrases := firstNonEmpty(st.Phrases, 3)
		if len(phrases) > 0 {
			for i, phrase := range phrases {
				plans = append(plans, queryPlan{ID: fmt.Sprintf("%s_narrow_p%d", st.ID, i+1), StrategyID: st.ID, Body: buildPhraseQuery(phrase, cpcs)})
			}
		} else {
			terms := topCanonicalTerms(st.TermFamilies, 2)
			if len(terms) == 0 {
				continue
			}
			plans = append(plans, queryPlan{ID: st.ID + "_narrow_all", StrategyID: st.ID, Body: buildAllTermsQuery(strings.Join(terms, " "), cpcs)})
		}

		tokens := normalizeTokens(st.TermFamilies)
		if len(cpcs) > 0 {
			if len(tokens) > 0 {
				plans = append(plans, queryPlan{ID: st.ID + "_broad", StrategyID: st.ID, Body: buildAnyTermsQuery(strings.Join(tokens, " "), cpcs)})
			}
		} else {
			terms := topCanonicalTerms(st.TermFamilies, 3)
			if len(terms) > 0 {
				plans = append(plans, queryPlan{ID: st.ID + "_broad", StrategyID: st.ID, Body: buildAllTermsQuery(strings.Join(terms, " "), nil)})
			}
		}
	}
	return plans
}

func buildPhraseQuery(phrase string, cpcs []string) map[string]any {
	text := map[string]any{"_or": []any{
		map[string]any{"_text_phrase": map[string]any{"patent_title": phrase}},
		map[string]any{"_text_phrase": map[string]any{"patent_abstract": phrase}},
	}}
	return queryBody(text, cpcs)
}

func buildAnyTermsQuery(tokens string, cpcs []string) map[string]any {
	text := map[string]any{"_or": []any{
		map[string]any{"_text_any": map[string]any{"patent_title": tokens}},
		map[string]any{"_text_any": map[string]any{"patent_abstract": tokens}},
	}}
	return queryBody(text, cpcs)
}

func buildAllTermsQuery(tokens string, cpcs []string) map[string]any {
	text := map[string]any{"_or": []any{
		map[string]any{"_text_all": map[string]any{"patent_title": tokens}},
		map[string]any{"_text_all": map[string]any{"patent_abstract": tokens}},
	}}
	return queryBody(text, cpcs)
}

func queryBody(textClause map[string]any, cpcs []string) map[string]any {
	q := any(textClause)
	if len(cpcs) > 0 {
		q = map[string]any{"_and": []any{textClause, map[string]any{"cpc_at_issue.cpc_subclass_id": cpcs}}}
	}
	return map[string]any{
		"q": q,
		"f": []string{
			"patent_id", "patent_title", "patent_abstract", "patent_date", "application.filing_date",
			"assignees.assignee_organization", "cpc_at_issue.cpc_subclass_id",
			"inventors.inventor_name_first", "inventors.inventor_name_last",
		},
		"s": []map[string]string{{"patent_date": "desc"}, {"patent_id": "asc"}},
		"o": map[string]int{"size": 200},
	}
}

func normalizeCPCs(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, code := range in {
		c := strings.ToUpper(strings.TrimSpace(code))
		if c == "" {
			continue
		}
		if !cpcSubclassRe.MatchString(c) {
			log.Printf("prior-art-search dropping invalid cpc_subclass=%q", code)
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

func normalizeTokens(families []TermFamily) []string {
	out := []string{}
	seen := map[string]struct{}{}
	acronymTokens := map[string]struct{}{}

	for _, tf := range families {
		for _, a := range tf.Acronyms {
			for _, t := range splitTokens(a) {
				acronymTokens[strings.ToLower(t)] = struct{}{}
			}
		}
	}

	for _, tf := range families {
		for _, source := range []string{tf.Canonical, strings.Join(tf.Synonyms, " "), strings.Join(tf.Acronyms, " ")} {
			for _, tok := range splitTokens(source) {
				tok = strings.ToLower(tok)
				if tok == "" {
					continue
				}
				_, isAcronym := acronymTokens[tok]
				if len(tok) < 3 && !isAcronym {
					continue
				}
				if _, stop := stopwords[tok]; stop {
					continue
				}
				if _, ok := seen[tok]; ok {
					continue
				}
				seen[tok] = struct{}{}
				out = append(out, tok)
				if len(out) >= 30 {
					return out
				}
			}
		}
	}
	return out
}

func splitTokens(s string) []string {
	replacer := strings.NewReplacer("-", " ")
	clean := replacer.Replace(s)
	return strings.Fields(clean)
}

func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func topCanonicalTerms(families []TermFamily, n int) []string {
	out := make([]string, 0, n)
	for _, tf := range families {
		c := strings.TrimSpace(tf.Canonical)
		if c == "" {
			continue
		}
		out = append(out, c)
		if len(out) == n {
			break
		}
	}
	return out
}

func firstNonEmpty(items []string, max int) []string {
	out := make([]string, 0, max)
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
		if len(out) == max {
			break
		}
	}
	return out
}

func priorityRank(p StrategyPriority) int {
	switch strings.ToUpper(string(p)) {
	case string(PriorityPrimary):
		return 0
	case string(PrioritySecondary):
		return 1
	default:
		return 2
	}
}

func flattenPatent(raw map[string]any) PatentResult {
	pat := PatentResult{
		PatentID:      strings.TrimSpace(str(raw["patent_id"])),
		Title:         strings.TrimSpace(str(raw["patent_title"])),
		Abstract:      strings.TrimSpace(str(raw["patent_abstract"])),
		GrantDate:     strings.TrimSpace(str(raw["patent_date"])),
		Assignees:     flattenAssignees(raw["assignees"]),
		CPCSubclasses: flattenCPC(raw["cpc_at_issue"]),
		Inventors:     flattenInventors(raw["inventors"]),
	}
	if fd := strings.TrimSpace(strFromPath(raw, "application", "filing_date")); fd != "" {
		pat.FilingDate = &fd
	}
	return pat
}

func flattenAssignees(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		m, _ := item.(map[string]any)
		name := normalizeAssignee(str(m["assignee_organization"]))
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func flattenCPC(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, item := range arr {
		m, _ := item.(map[string]any)
		c := strings.TrimSpace(str(m["cpc_subclass_id"]))
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

func flattenInventors(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, item := range arr {
		if len(out) >= 10 {
			break
		}
		m, _ := item.(map[string]any)
		first := strings.TrimSpace(str(m["inventor_name_first"]))
		last := strings.TrimSpace(str(m["inventor_name_last"]))
		name := strings.TrimSpace(first + " " + last)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func normalizeAssignee(s string) string {
	parts := strings.Fields(strings.TrimSpace(s))
	return strings.Join(parts, " ")
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func strFromPath(raw map[string]any, keys ...string) string {
	cur := any(raw)
	for _, key := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[key]
	}
	s, _ := cur.(string)
	return s
}

func appendIfMissing(items []string, v string) []string {
	for _, item := range items {
		if item == v {
			return items
		}
	}
	return append(items, v)
}

func strategyCount(queryIDs []string) int {
	seen := map[string]struct{}{}
	for _, id := range queryIDs {
		prefix := id
		if idx := strings.Index(id, "_"); idx > 0 {
			prefix = id[:idx]
		}
		seen[prefix] = struct{}{}
	}
	return len(seen)
}
