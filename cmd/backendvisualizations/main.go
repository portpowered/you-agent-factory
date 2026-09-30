package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/portpowered/infinite-you/internal/backendvisualizations"
)

func main() {
	var cfg backendvisualizations.Config
	flag.StringVar(&cfg.Root, "root", ".", "repository root")
	flag.StringVar(&cfg.OutputDir, "output-dir", "docs/architecture/visualizations", "generated Markdown directory")
	flag.StringVar(&cfg.GoBinary, "go", "go", "Go command")
	flag.StringVar(&cfg.SourceCommit, "source-commit", "", "commit measured by supplied coverage summaries")
	flag.StringVar(&cfg.UnitSummary, "unit-summary", "", "complete unit coverage-summary JSON")
	flag.StringVar(&cfg.FunctionalSummary, "functional-summary", "", "complete functional coverage-summary JSON")
	flag.BoolVar(&cfg.RequireCoverage, "require-coverage", false, "fail if either complete coverage summary is unavailable")
	flag.Parse()
	if err := backendvisualizations.Generate(context.Background(), cfg); err != nil {
		fmt.Fprintln(os.Stderr, "backend visualizations:", err)
		os.Exit(1)
	}
}
