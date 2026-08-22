package main

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

func parseTotalCoverage(report string) (float64, string, error) {
	matches := totalCoveragePattern.FindStringSubmatch(report)
	if len(matches) != 2 {
		return 0, "", errors.New("parse go coverage total: missing total statements line")
	}
	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, "", fmt.Errorf("parse go coverage percentage %q: %w", matches[1], err)
	}
	for _, line := range strings.Split(report, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "total:") {
			return value, fmt.Sprintf("total: (statements) %.1f%%", value), nil
		}
	}
	return value, fmt.Sprintf("total: (statements) %.1f%%", value), nil
}

func evaluateCoverage(_ string, _ string, profilePath string, repoRoot string, coverPackages []string, minCoverage float64, baselinePackages map[string]struct{}, packageGate ...bool) (coverageResult, string, error) {
	coverageBlocks, err := readCoverageProfileBlocks(profilePath, repoRoot)
	if err != nil {
		return coverageResult{}, "", err
	}
	return evaluateCoverageBlocks(coverageBlocks, coverPackages, minCoverage, baselinePackages, packageGate...)
}

func evaluateCoverageBlocks(coverageBlocks map[string]coverageBlock, coverPackages []string, minCoverage float64, baselinePackages map[string]struct{}, packageGate ...bool) (coverageResult, string, error) {
	packageTotals := coverageTotals(coverageBlocks)
	actual, totalLine := calculateTotalCoverage(packageTotals, coverPackages)
	packageGateEnabled := len(packageGate) == 0 || packageGate[0]
	packageSummaries := summarizePackageCoverageFromTotals(packageTotals, coverPackages)
	var insufficientCoveragePackages []packageCoverageSummary
	if packageGateEnabled {
		insufficientCoveragePackages = findInsufficientCoveragePackages(packageSummaries, minCoverage, baselinePackages)
	}
	zeroCoveragePackages := findZeroCoveragePackagesFromSummaries(packageSummaries, baselinePackages)

	return coverageResult{
		actual:                       actual,
		insufficientCoveragePackages: insufficientCoveragePackages,
		coverageBlocks:               coverageBlocks,
		packageTotals:                packageTotals,
		packageSummaries:             packageSummaries,
		zeroCoveragePackages:         zeroCoveragePackages,
	}, totalLine, nil
}

func readPackageCoverageBaseline(path string) (map[string]struct{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read go coverage package baseline: %w", err)
	}

	packages := make(map[string]struct{})
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		packages[line] = struct{}{}
	}
	return packages, nil
}

func readCoverageProfileTotals(profilePath string, repoRoot string) (map[string]packageCoverageTotals, error) {
	coverageBlocks, err := readCoverageProfileBlocks(profilePath, repoRoot)
	if err != nil {
		return nil, err
	}
	return coverageTotals(coverageBlocks), nil
}

func readCoverageProfileBlocks(profilePath string, repoRoot string) (map[string]coverageBlock, error) {
	profile, err := os.Open(profilePath)
	if err != nil {
		return nil, fmt.Errorf("read go coverage profile: %w", err)
	}
	defer profile.Close()

	_, coverageBlocks, err := scanCoverageProfile(profile, repoRoot)
	if err != nil {
		return nil, err
	}
	return coverageBlocks, nil
}

func calculateTotalCoverage(packageTotals map[string]packageCoverageTotals, coverPackages []string) (float64, string) {
	totalCovered, totalStatements := summedCoverageTotals(packageTotals, coverPackages)
	if totalStatements == 0 {
		return 0, "total: (statements) 0.0%"
	}
	actual := float64(totalCovered) * 100 / float64(totalStatements)
	return actual, fmt.Sprintf("total: (statements) %.1f%%", actual)
}

func summedCoverageTotals(packageTotals map[string]packageCoverageTotals, coverPackages []string) (int, int) {
	selectedPackages := selectedCoveragePackages(coverPackages)
	totalCovered := 0
	totalStatements := 0
	for _, coverPackage := range selectedPackages {
		totals := packageTotals[coverPackage]
		totalCovered += totals.coveredStatements
		totalStatements += totals.totalStatements
	}
	return totalCovered, totalStatements
}

func selectedCoveragePackages(coverPackages []string) []string {
	seen := make(map[string]struct{}, len(coverPackages))
	selected := make([]string, 0, len(coverPackages))
	for _, coverPackage := range coverPackages {
		if !isBackendCoveragePackage(coverPackage) {
			continue
		}
		if _, ok := seen[coverPackage]; ok {
			continue
		}
		seen[coverPackage] = struct{}{}
		selected = append(selected, coverPackage)
	}
	slices.Sort(selected)
	return selected
}

func summarizePackageCoverageFromTotals(packageTotals map[string]packageCoverageTotals, coverPackages []string) []packageCoverageSummary {
	selectedPackages := selectedCoveragePackages(coverPackages)
	summaries := make([]packageCoverageSummary, 0, len(selectedPackages))
	for _, coverPackage := range selectedPackages {
		totals := packageTotals[coverPackage]
		coverage := 0.0
		if totals.totalStatements > 0 {
			coverage = float64(totals.coveredStatements) * 100 / float64(totals.totalStatements)
		}
		summaries = append(summaries, packageCoverageSummary{
			importPath: coverPackage,
			coverage:   coverage,
		})
	}
	return summaries
}

func findInsufficientCoveragePackages(summaries []packageCoverageSummary, minCoverage float64, baselinePackages map[string]struct{}) []packageCoverageSummary {
	packages := make([]packageCoverageSummary, 0)
	for _, summary := range summaries {
		if summary.coverage >= minCoverage {
			continue
		}
		if _, ok := baselinePackages[summary.importPath]; ok {
			continue
		}
		packages = append(packages, summary)
	}
	return packages
}

func findZeroCoveragePackagesFromSummaries(summaries []packageCoverageSummary, baselinePackages map[string]struct{}) []string {
	zeroCoveragePackages := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		if summary.coverage != 0 {
			continue
		}
		if _, ok := baselinePackages[summary.importPath]; ok {
			continue
		}
		zeroCoveragePackages = append(zeroCoveragePackages, summary.importPath)
	}
	slices.Sort(zeroCoveragePackages)
	return zeroCoveragePackages
}
