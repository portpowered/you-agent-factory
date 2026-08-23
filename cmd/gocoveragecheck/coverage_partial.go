package main

import (
	"errors"
	"fmt"
	"strings"
)

func writePartialCoverageSnapshot(path, profilePath, repoRoot string, coverPackages []string, packageFloorPolicy string, reason string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	result, err := readPartialCoverageResult(profilePath, repoRoot, coverPackages)
	if err != nil {
		// A profile that has not reached a complete line yet is not a
		// trustworthy partial measurement. The timing artifact and runner
		// inventory name this coverage item as unavailable instead.
		return nil
	}
	result.packageFloorPolicy = packageFloorPolicy
	return writeCoverageSummaryJSONWithStatus(path, result, false, reason)
}

func readPartialCoverageResult(profilePath, repoRoot string, coverPackages []string) (coverageResult, error) {
	blocks, err := readCoverageProfileBlocks(profilePath, repoRoot)
	if err != nil {
		return coverageResult{}, err
	}
	if len(blocks) == 0 {
		return coverageResult{}, errors.New("partial go coverage profile contains no complete source blocks")
	}
	totals := coverageTotals(blocks)
	actual, _ := calculateTotalCoverage(totals, coverPackages)
	return coverageResult{
		actual:           actual,
		coverageBlocks:   blocks,
		packageTotals:    totals,
		packageSummaries: summarizePackageCoverageFromTotals(totals, coverPackages),
	}, nil
}

func partialCoverageReason(commandErr error) string {
	if commandErr == nil {
		return "go test coverage is still running; partial statements are diagnostic only"
	}
	return fmt.Sprintf("go test coverage did not complete: %s; partial statements are diagnostic only", compactDiagnosticError(commandErr))
}

func compactDiagnosticError(err error) string {
	if err == nil {
		return "unknown interruption"
	}
	reason := strings.Join(strings.Fields(err.Error()), " ")
	if len(reason) > 240 {
		return reason[:240] + "..."
	}
	return reason
}
