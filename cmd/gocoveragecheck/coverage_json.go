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
	CoveredStatements    int                   `json:"coveredStatements"`
	MeasurableStatements int                   `json:"measurableStatements"`
	CoveragePercent      float64               `json:"coveragePercent"`
	Packages             []packageCoverageJSON `json:"packages"`
}

// packageCoverageJSON reports one measured production package, including the
// package-gate floor or measurement exception used for that run.
type packageCoverageJSON struct {
	Package              string                     `json:"package"`
	CoveredStatements    int                        `json:"coveredStatements"`
	MeasurableStatements int                        `json:"measurableStatements"`
	CoveragePercent      float64                    `json:"coveragePercent"`
	PackageFloor         *float64                   `json:"packageFloor"`
	MeasurementException *coverageManifestException `json:"measurementException"`
}

// packageCoverageGate is the package-local floor or exception policy applied
// during a coverage run. Exactly one of Floor or Exception is set when a gate
// applies; both remain nil when the package is ungated for that run.
type packageCoverageGate struct {
	Floor     *float64
	Exception *coverageManifestException
}

func buildCoverageSummaryJSON(result coverageResult) coverageSummaryJSON {
	covered, measurable := overallStatementTotals(result)
	return coverageSummaryJSON{
		CoveredStatements:    covered,
		MeasurableStatements: measurable,
		CoveragePercent:      roundCoveragePercent(result.actual),
		Packages:             buildPackageCoverageJSON(result),
	}
}

func buildPackageCoverageJSON(result coverageResult) []packageCoverageJSON {
	packages := make([]packageCoverageJSON, 0, len(result.packageSummaries))
	for _, summary := range result.packageSummaries {
		totals := result.packageTotals[summary.importPath]
		gate := result.packageGates[summary.importPath]
		entry := packageCoverageJSON{
			Package:              summary.importPath,
			CoveredStatements:    totals.coveredStatements,
			MeasurableStatements: totals.totalStatements,
			CoveragePercent:      roundCoveragePercent(summary.coverage),
			PackageFloor:         gate.Floor,
			MeasurementException: cloneCoverageManifestException(gate.Exception),
		}
		packages = append(packages, entry)
	}
	return packages
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

func cloneCoverageManifestException(exception *coverageManifestException) *coverageManifestException {
	if exception == nil {
		return nil
	}
	cloned := *exception
	return &cloned
}

func packageGatesFromManifest(manifest coverageManifest) map[string]packageCoverageGate {
	gates := make(map[string]packageCoverageGate, len(manifest.Packages))
	for _, entry := range manifest.Packages {
		if entry.Exception != nil {
			gates[entry.Package] = packageCoverageGate{
				Exception: cloneCoverageManifestException(entry.Exception),
			}
			continue
		}
		floor, err := parseCoverageFloor(entry.Minimum)
		if err != nil {
			gates[entry.Package] = packageCoverageGate{}
			continue
		}
		percent := float64(floor) / 100
		gates[entry.Package] = packageCoverageGate{Floor: &percent}
	}
	return gates
}

func packageGatesFromLegacyMin(summaries []packageCoverageSummary, packageMin float64, baselinePackages map[string]struct{}) map[string]packageCoverageGate {
	gates := make(map[string]packageCoverageGate, len(summaries))
	for _, summary := range summaries {
		if _, exempt := baselinePackages[summary.importPath]; exempt {
			gates[summary.importPath] = packageCoverageGate{}
			continue
		}
		floor := packageMin
		gates[summary.importPath] = packageCoverageGate{Floor: &floor}
	}
	return gates
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
