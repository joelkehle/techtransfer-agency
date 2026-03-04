package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joelkehle/techtransfer-agency/internal/patentscreen"
)

func main() {
	workflow := flag.String("workflow", "patent-screen", "Workflow name: patent-screen or prior-art-search")
	inputPath := flag.String("input", "", "Path to saved workflow envelope JSON")
	outputPath := flag.String("output", "", "Path to write report markdown (defaults to stdout)")
	jsonOutputPath := flag.String("json-output", "", "Optional path to write normalized/rebuilt envelope JSON")
	flag.Parse()

	if strings.TrimSpace(*inputPath) == "" {
		log.Fatal("missing required -input")
	}

	in, err := os.ReadFile(*inputPath)
	if err != nil {
		log.Fatalf("read input: %v", err)
	}

	out, err := buildReport(strings.TrimSpace(*workflow), in)
	if err != nil {
		log.Fatalf("build report: %v", err)
	}

	if err := writeMarkdown(*outputPath, out.Markdown); err != nil {
		log.Fatalf("write markdown: %v", err)
	}
	if strings.TrimSpace(*jsonOutputPath) != "" {
		if err := os.WriteFile(*jsonOutputPath, out.RawJSON, 0o644); err != nil {
			log.Fatalf("write json output: %v", err)
		}
	}
}

type buildResult struct {
	Markdown string
	RawJSON  []byte
}

func buildReport(workflow string, input []byte) (buildResult, error) {
	switch normalizeWorkflow(workflow) {
	case "patent-screen":
		var env patentscreen.ResponseEnvelope
		if err := json.Unmarshal(input, &env); err != nil {
			return buildResult{}, fmt.Errorf("decode patent-screen envelope: %w", err)
		}
		rebuilt, err := patentscreen.RebuildResponseFromEnvelope(env)
		if err != nil {
			return buildResult{}, fmt.Errorf("rebuild patent-screen report: %w", err)
		}
		raw, err := json.MarshalIndent(rebuilt, "", "  ")
		if err != nil {
			return buildResult{}, fmt.Errorf("encode patent-screen envelope: %w", err)
		}
		return buildResult{
			Markdown: rebuilt.ReportMarkdown,
			RawJSON:  raw,
		}, nil
	case "prior-art-search":
		var env map[string]any
		if err := json.Unmarshal(input, &env); err != nil {
			return buildResult{}, fmt.Errorf("decode prior-art-search envelope: %w", err)
		}
		reportMarkdown, _ := env["report_markdown"].(string)
		if strings.TrimSpace(reportMarkdown) == "" {
			return buildResult{}, fmt.Errorf("prior-art-search envelope missing report_markdown")
		}
		raw, err := json.MarshalIndent(env, "", "  ")
		if err != nil {
			return buildResult{}, fmt.Errorf("encode prior-art-search envelope: %w", err)
		}
		return buildResult{
			Markdown: reportMarkdown,
			RawJSON:  raw,
		}, nil
	default:
		return buildResult{}, fmt.Errorf("unsupported workflow %q", workflow)
	}
}

func normalizeWorkflow(workflow string) string {
	switch strings.TrimSpace(workflow) {
	case "patent-screen", "patent-eligibility-screen":
		return "patent-screen"
	case "prior-art-search", "prior-art":
		return "prior-art-search"
	default:
		return strings.TrimSpace(workflow)
	}
}

func writeMarkdown(outputPath, markdown string) error {
	if strings.TrimSpace(outputPath) == "" {
		_, err := fmt.Print(markdown)
		return err
	}
	return os.WriteFile(outputPath, []byte(markdown), 0o644)
}
