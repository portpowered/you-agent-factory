package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
)

// coverageSummaryJSON is the machine-readable coverage summary owned by
// gocoveragecheck. Downstream visualizers consume this artifact instead of
// re-parsing coverage profiles.
type coverageSummaryJSON struct {
	CoveredStatements    int     `json:"coveredStatements"`
	MeasurableStatements int     `json:"measurableStatements"`
	CoveragePercent      float64 `json:"coveragePercent"`
}

func buildCoverageSummaryJSON(result coverageResult) coverageSummaryJSON {
	covered, measurable := overallStatementTotals(result)
	return coverageSummaryJSON{
		CoveredStatements:    covered,
		MeasurableStatements: measurable,
		CoveragePercent:      roundCoveragePercent(result.actual),
	}
}

func overallStatementTotals(result coverageResult) (covered int, measurable int) {
	for _, summary := range result.packageSummaries {
		totals := result.packageTotals[summary.importPath]
		covered += totals.coveredStatements
		measurable += totals.totalStatements
	}
	return covered, measurable
}

func roundCoveragePercent(value float64) float64 {
	return math.Round(value*10) / 10
}

func renderCoverageSummaryJSON(summary coverageSummaryJSON) ([]byte, error) {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("render go coverage summary json: %w", err)
	}
	return append(data, '\n'), nil
}

func writeCoverageSummaryJSON(path string, result coverageResult) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	data, err := renderCoverageSummaryJSON(buildCoverageSummaryJSON(result))
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write go coverage summary json: %w", err)
	}
	return nil
}
