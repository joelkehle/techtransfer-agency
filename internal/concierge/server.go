package concierge

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/joelkehle/agent-bus-v2/internal/busclient"
)

// WorkflowLabel provides a human-readable name for a capability.
type WorkflowLabel struct {
	Capability  string `json:"capability"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type Server struct {
	bridge    *Bridge
	store     *SubmissionStore
	labels    map[string]WorkflowLabel
	webDir    string
	uploadDir string
}

func NewServer(bridge *Bridge, store *SubmissionStore, webDir, uploadDir string) http.Handler {
	s := &Server{
		bridge:    bridge,
		store:     store,
		webDir:    webDir,
		uploadDir: uploadDir,
		labels: map[string]WorkflowLabel{
			// Prior Art & Patentability
			"patent-screen":    {Capability: "patent-screen", Label: "Patent Eligibility Screen", Description: "Assess patentability of an invention disclosure"},
			"prior-art-search": {Capability: "prior-art-search", Label: "Prior Art Search", Description: "Search USPTO, EPO, WIPO and academic literature for prior art"},
			"patent-opinion":   {Capability: "patent-opinion", Label: "Patentability Opinion", Description: "Formal patentability opinion and claim drafting guidance"},
			// Technical Assessment
			"technical-review":      {Capability: "technical-review", Label: "Technical Domain Review", Description: "Verify technical claims, assess feasibility and TRL"},
			"competitive-landscape": {Capability: "competitive-landscape", Label: "Competitive Landscape", Description: "Map competing technologies and patent landscape"},
			// Commercial Viability
			"market-analysis":        {Capability: "market-analysis", Label: "Market Analysis", Description: "Total addressable market sizing and commercial potential"},
			"licensee-identification": {Capability: "licensee-identification", Label: "Licensee Identification", Description: "Find potential licensees, partners, and industry players"},
			"financial-valuation":    {Capability: "financial-valuation", Label: "Financial Valuation", Description: "Estimate IP value, royalty rates, and cost-to-patent ROI"},
			// Regulatory & Legal
			"regulatory-pathway": {Capability: "regulatory-pathway", Label: "Regulatory Pathway", Description: "FDA/EPA/FCC pathway assessment, timeline, and risks"},
			"ip-compliance":      {Capability: "ip-compliance", Label: "IP Ownership & Compliance", Description: "Inventorship analysis, Bayh-Dole compliance, conflict of interest"},
			// Strategic
			"commercialization-strategy": {Capability: "commercialization-strategy", Label: "Commercialization Strategy", Description: "Licensing vs. startup vs. partnership recommendation"},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/workflows", s.handleWorkflows)
	mux.HandleFunc("/submit", s.handleSubmit)
	mux.HandleFunc("/status/", s.handleStatus)
	mux.HandleFunc("/report/", s.handleReport)
	mux.HandleFunc("/", s.handleRoot)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" || r.URL.Path == "/index.html" {
		http.ServeFile(w, r, filepath.Join(s.webDir, "index.html"))
		return
	}
	// Serve static files from web directory.
	path := filepath.Join(s.webDir, filepath.Clean(r.URL.Path))
	if _, err := fs.Stat(os.DirFS(s.webDir), strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")); err == nil {
		http.ServeFile(w, r, path)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleWorkflows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	agents, err := s.bridge.DiscoverWorkflows(r.Context())
	if err != nil {
		log.Printf("discover workflows: %v", err)
		writeError(w, 502, "failed to query bus agent registry")
		return
	}

	// Collect unique capabilities from active agents.
	seen := map[string]bool{}
	var workflows []WorkflowLabel
	for _, a := range agents {
		if a.AgentID == s.bridge.agentID {
			continue
		}
		for _, cap := range a.Capabilities {
			if seen[cap] {
				continue
			}
			seen[cap] = true
			if label, ok := s.labels[cap]; ok {
				workflows = append(workflows, label)
			}
			// Skip capabilities without a label — they are internal pipeline steps.
		}
	}
	writeJSON(w, 200, map[string]any{"workflows": workflows})
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, 400, "invalid multipart form")
		return
	}

	workflowsRaw := r.FormValue("workflows")
	if strings.TrimSpace(workflowsRaw) == "" {
		writeError(w, 400, "workflows field is required")
		return
	}
	workflows := strings.Split(workflowsRaw, ",")
	for i := range workflows {
		workflows[i] = strings.TrimSpace(workflows[i])
	}

	caseID := r.FormValue("case_id")

	// Handle file upload.
	var attachments []busclient.Attachment
	file, header, err := r.FormFile("file")
	if err == nil {
		defer file.Close()
		_ = os.MkdirAll(s.uploadDir, 0o755)
		dst := filepath.Join(s.uploadDir, header.Filename)
		out, err := os.Create(dst)
		if err != nil {
			writeError(w, 500, "failed to save uploaded file")
			return
		}
		if _, err := io.Copy(out, file); err != nil {
			out.Close()
			writeError(w, 500, "failed to write uploaded file")
			return
		}
		out.Close()
		abs, _ := filepath.Abs(dst)
		attachments = append(attachments, busclient.Attachment{
			URL:         "file://" + abs,
			Name:        header.Filename,
			ContentType: header.Header.Get("Content-Type"),
			Size:        header.Size,
		})
	}

	sub := s.store.Create(caseID, workflows)

	// Send a bus message for each workflow.
	for _, wf := range workflows {
		if err := s.bridge.Submit(r.Context(), sub.Token, wf, caseID, attachments); err != nil {
			log.Printf("submit workflow %s: %v", wf, err)
			// Mark as error but continue with other workflows.
			s.store.ErrorWorkflow(fmt.Sprintf("submission-%s-%s", sub.Token, wf), err.Error())
		}
	}

	writeJSON(w, 200, map[string]any{
		"token":     sub.Token,
		"workflows": workflows,
		"status":    "submitted",
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/status/")
	token = strings.TrimSuffix(token, "/")
	if token == "" {
		writeError(w, 400, "token is required")
		return
	}
	sub := s.store.Get(token)
	if sub == nil {
		writeError(w, 404, "submission not found")
		return
	}

	wfStatus := make(map[string]any)
	for name, ws := range sub.Workflows {
		wfStatus[name] = map[string]any{
			"status": ws.Status,
			"ready":  ws.Ready,
		}
	}
	writeJSON(w, 200, map[string]any{
		"token":     sub.Token,
		"status":    sub.OverallStatus(),
		"workflows": wfStatus,
	})
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	// Path: /report/{token}/{workflow}
	path := strings.TrimPrefix(r.URL.Path, "/report/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, 400, "path must be /report/{token}/{workflow}")
		return
	}
	token := parts[0]
	workflow := parts[1]

	sub := s.store.Get(token)
	if sub == nil {
		writeError(w, 404, "submission not found")
		return
	}
	ws, ok := sub.Workflows[workflow]
	if !ok {
		writeError(w, 404, "workflow not found")
		return
	}
	if !ws.Ready {
		writeError(w, 404, "report not ready")
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	_, _ = w.Write([]byte(ws.Report))
}
