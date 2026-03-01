package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/joelkehle/techtransfer-agency/internal/patentscreen"
)

func main() {
	inputPath := flag.String("input", "", "Path to saved patent-screen response envelope JSON")
	outputPath := flag.String("output", "", "Path to write rebuilt markdown (defaults to stdout)")
	jsonOutputPath := flag.String("json-output", "", "Optional path to write rebuilt response envelope JSON")
	flag.Parse()

	if *inputPath == "" {
		log.Fatal("missing required -input")
	}

	in, err := os.ReadFile(*inputPath)
	if err != nil {
		log.Fatalf("read input: %v", err)
	}

	var env patentscreen.ResponseEnvelope
	if err := json.Unmarshal(in, &env); err != nil {
		log.Fatalf("decode input JSON: %v", err)
	}

	rebuilt, err := patentscreen.RebuildResponseFromEnvelope(env)
	if err != nil {
		log.Fatalf("rebuild report: %v", err)
	}

	if err := writeMarkdown(*outputPath, rebuilt.ReportMarkdown); err != nil {
		log.Fatalf("write markdown: %v", err)
	}
	if *jsonOutputPath != "" {
		if err := writeEnvelopeJSON(*jsonOutputPath, rebuilt); err != nil {
			log.Fatalf("write json output: %v", err)
		}
	}
}

func writeMarkdown(outputPath, markdown string) error {
	if outputPath == "" {
		_, err := fmt.Print(markdown)
		return err
	}
	return os.WriteFile(outputPath, []byte(markdown), 0o644)
}

func writeEnvelopeJSON(path string, env patentscreen.ResponseEnvelope) error {
	b, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
