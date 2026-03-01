package operator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

type ChromiumPDFRenderer struct {
	webDir     string
	chromePath string
	styleOnce  sync.Once
	styleCSS   string
	styleErr   error
}

func NewChromiumPDFRenderer(webDir string) *ChromiumPDFRenderer {
	return &ChromiumPDFRenderer{
		webDir:     webDir,
		chromePath: detectChromePath(),
	}
}

func (r *ChromiumPDFRenderer) Render(ctx context.Context, report string) ([]byte, error) {
	htmlDoc, err := r.buildHTML(report)
	if err != nil {
		return nil, err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoSandbox,
		chromedp.DisableGPU,
		chromedp.Flag("disable-dev-shm-usage", true),
	}
	if r.chromePath != "" {
		opts = append(opts, chromedp.ExecPath(r.chromePath))
	}
	allocCtx, allocCancel := chromedp.NewExecAllocator(timeoutCtx, append(chromedp.DefaultExecAllocatorOptions[:], opts...)...)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	var pdf []byte
	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(htmlDoc))
	if err := chromedp.Run(taskCtx,
		chromedp.Navigate(dataURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			footer := `<div style="width:100%;text-align:center;font-size:9px;color:#666;padding-right:8px;">` +
				`Page <span class="pageNumber"></span> of <span class="totalPages"></span></div>`
			out, _, err := page.PrintToPDF().
				WithPrintBackground(true).
				WithDisplayHeaderFooter(true).
				WithHeaderTemplate(`<div></div>`).
				WithFooterTemplate(footer).
				WithPaperWidth(8.27).
				WithPaperHeight(11.69).
				WithMarginTop(0.5).
				WithMarginBottom(0.75).
				WithMarginLeft(0.45).
				WithMarginRight(0.45).
				Do(ctx)
			if err != nil {
				return err
			}
			pdf = out
			return nil
		}),
	); err != nil {
		return nil, err
	}
	return pdf, nil
}

func (r *ChromiumPDFRenderer) buildHTML(report string) (string, error) {
	metaHTML := ""
	badgeHTML := ""
	markdown := report

	var envelope map[string]any
	if json.Unmarshal([]byte(report), &envelope) == nil {
		if s, ok := envelope["report_markdown"].(string); ok && strings.TrimSpace(s) != "" {
			markdown = s
		}
		metaHTML = buildMetaHTML(envelope)
		badgeHTML = buildBadgeHTML(envelope)
	}

	var content strings.Builder
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	if err := md.Convert([]byte(markdown), &content); err != nil {
		return "", fmt.Errorf("markdown convert: %w", err)
	}
	contentHTML := applyPrintLayoutHooks(content.String())

	styleCSS, err := r.loadStyleCSS()
	if err != nil {
		return "", err
	}
	return "<!doctype html><html><head><meta charset='utf-8'><title>Patent Report</title>" +
		"<style>" + styleCSS + "\n" +
		"html,body,*{-webkit-print-color-adjust:exact !important;print-color-adjust:exact !important;} " +
		"body{background:#fff !important;padding:0.6rem;} .pdf-wrap{max-width:1000px;margin:0 auto;} .pdf-gutter{border-left:3px solid #92400e !important;border-right:3px solid #92400e !important;padding:0 0.65rem;} .report-tools{display:block;} " +
		".report-viewer{background:#f9f7f3 !important;border:0 !important;} " +
		".report-badge{background:#fef3c7 !important;color:#78350f !important;border:1px solid #fcd34d !important;} " +
		".report-meta strong{color:#1c1917 !important;} .report-meta{color:#44403c !important;} " +
		".report-html a{color:#1d4ed8 !important;text-decoration:underline !important;} " +
		".report-html h2[data-stage-heading='true']{font-family:var(--font-body) !important;font-weight:700 !important;letter-spacing:0.01em;} " +
		".report-html table{width:100% !important;border-collapse:collapse !important;border:1px solid #a8a29e !important;font-size:0.8rem !important;} " +
		".report-html th,.report-html td{border:1px solid #a8a29e !important;padding:0.35rem 0.45rem !important;text-align:left !important;vertical-align:top !important;} " +
		".report-html thead th{background:#f1f5f9 !important;font-weight:700 !important;} " +
		`h2[data-page-break-before="true"]{break-before:page;page-break-before:always;} ` +
		"@media print{ @page{size:auto;margin:12mm;} body{background:#fff !important;padding:0;} .pdf-wrap{max-width:none;} .report-viewer{box-shadow:none !important;} }" +
		"</style></head><body>" +
		"<div class='pdf-wrap'><div class='pdf-gutter'><section class='report-viewer'><div class='report-header'>" +
		"<div class='report-meta'>" + metaHTML + "</div>" +
		"<div class='report-tools'><div class='report-badges'>" + badgeHTML + "</div></div>" +
		"</div><div class='report-html'>" + contentHTML + "</div></section></div></div>" +
		"</body></html>", nil
}

func applyPrintLayoutHooks(contentHTML string) string {
	reHowItWorks := regexp.MustCompile(`(?i)<h2([^>]*)>\s*How This Report Works\s*</h2>`)
	out := reHowItWorks.ReplaceAllString(contentHTML, `<h2$1 data-page-break-before="true">How This Report Works</h2>`)

	// Ensure stage headings in the PDF remain visually prominent and scannable.
	reStageHeading := regexp.MustCompile(`(?i)<h2([^>]*)>\s*(Stage\s+[0-9]+:[^<]*)\s*</h2>`)
	out = reStageHeading.ReplaceAllString(out, `<h2$1 data-stage-heading="true">$2</h2>`)

	return out
}

func (r *ChromiumPDFRenderer) loadStyleCSS() (string, error) {
	r.styleOnce.Do(func() {
		b, err := os.ReadFile(filepath.Join(r.webDir, "style.css"))
		if err != nil {
			r.styleErr = fmt.Errorf("read style.css: %w", err)
			return
		}
		r.styleCSS = string(b)
	})
	return r.styleCSS, r.styleErr
}

func buildMetaHTML(env map[string]any) string {
	var out strings.Builder
	ref := formatCaseReference(stringValue(env["case_id"]))
	if ref != "" {
		out.WriteString("<div><strong>Reference:</strong> " + html.EscapeString(ref) + "</div>")
	}
	if title := lookupString(env, "stage_outputs", "stage_1", "invention_title"); title != "" {
		out.WriteString("<div><strong>Invention:</strong> " + html.EscapeString(title) + "</div>")
	}
	if completed := lookupString(env, "pipeline_metadata", "completed_at"); completed != "" {
		if ts, err := time.Parse(time.RFC3339Nano, completed); err == nil {
			out.WriteString("<div><strong>Date:</strong> " + html.EscapeString(ts.In(time.Local).Format("January 2, 2006 at 3:04 PM MST")) + "</div>")
		} else {
			out.WriteString("<div><strong>Date:</strong> " + html.EscapeString(completed) + "</div>")
		}
	}
	return out.String()
}

func buildBadgeHTML(env map[string]any) string {
	var out strings.Builder
	if d := stringValue(env["determination"]); d != "" {
		out.WriteString("<span class='report-badge'>" + html.EscapeString(d) + "</span>")
	}
	if p := lookupString(env, "stage_outputs", "stage_6", "prior_art_search_priority"); p != "" {
		out.WriteString("<span class='report-badge'>Prior Art Priority: " + html.EscapeString(p) + "</span>")
	}
	return out.String()
}

func lookupString(root map[string]any, path ...string) string {
	var cur any = root
	for _, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[p]
	}
	return stringValue(cur)
}

func stringValue(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func formatCaseReference(caseID string) string {
	id := strings.TrimSpace(caseID)
	if id == "" {
		return ""
	}
	if len(id) == 8 && id[4] == '-' {
		allDigits := true
		for i := 0; i < len(id); i++ {
			if i == 4 {
				continue
			}
			if id[i] < '0' || id[i] > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return "UCLA Case #" + id
		}
	}
	return id
}

func detectChromePath() string {
	candidates := []string{
		"/usr/bin/chromium-browser",
		"/usr/bin/chromium",
		"/usr/bin/google-chrome",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
