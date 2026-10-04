package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

type config struct {
	root                            string
	packageRoot                     string
	all                             bool
	baseRef                         string
	baselineCacheDir                string
	writeProductionDefaultBaseline  bool
	writePetriPublicSurfaceBaseline bool
}

func parseConfig() config {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root to scan")
	flag.StringVar(&cfg.packageRoot, "package-root", defaultScanRoot, "repository-relative package root to scan")
	flag.BoolVar(&cfg.all, "all", false, "show recorded package-boundary diagnostics as well as unrecorded findings")
	flag.StringVar(&cfg.baseRef, "base-ref", "", "optional Git ref used to identify recorded package-boundary findings")
	flag.StringVar(&cfg.baselineCacheDir, "baseline-cache-dir", "", "memoize the base-tree scan per base commit and checker build in this directory (\"auto\" selects a per-user cache); empty disables the cache")
	flag.BoolVar(
		&cfg.writeProductionDefaultBaseline,
		"create-production-default-selection-baseline",
		false,
		"create the deletion-only production default-selection baseline; fails when the file already exists or no debt exists",
	)
	flag.BoolVar(
		&cfg.writePetriPublicSurfaceBaseline,
		"create-petri-public-surface-baseline",
		false,
		"create the exact deletion-only Petri public-surface baseline; fails when the file exists or no debt exists",
	)
	flag.Parse()
	return cfg
}

func run(cfg config, stdout io.Writer, stderr io.Writer) error {
	return runWithPolicy(cfg, defaultBoundaryPolicy(), stdout, stderr)
}

func runWithPolicy(cfg config, policy boundaryPolicy, stdout io.Writer, stderr io.Writer) error {
	if strings.TrimSpace(cfg.packageRoot) == "" {
		return fmt.Errorf("package root must not be empty")
	}

	if err := validatePolicy(policy); err != nil {
		return err
	}

	findings, err := scanBoundaryRepo(cfg, policy)
	if err != nil {
		return err
	}
	baseline, err := loadRecordedBoundaryBaseline(cfg, policy)
	if err != nil {
		return err
	}
	visibleFindings, _ := filterRecordedScanResult(findings, baseline)
	blockingViolationCount := countBlockingViolations(visibleFindings)
	classifiedDependencyCounts := countClassifiedDependencyViolations(visibleFindings)
	testOnlyFindings := testOnlyDependencyFindings(visibleFindings)
	if blockingViolationCount == 0 {
		if cfg.all {
			writeBoundaryFindings(stdout, findings)
			writeBaselineSummaries(stdout, findings)
		} else {
			writeBoundaryFindings(stdout, testOnlyFindings)
		}
		writeClassifiedDependencyViolationCounts(stdout, classifiedDependencyCounts)
		fmt.Fprintln(stdout, "[agent-factory:pkg-boundary] package boundary passed (no blocking package-boundary violations)")
		writeGeneratedCodeExceptionSummary(stdout, policy)
		return nil
	}

	reportFindings := visibleFindings
	if cfg.all {
		reportFindings = findings
	}
	writeBoundaryFindings(stderr, reportFindings)
	if cfg.all {
		writeBaselineSummaries(stderr, findings)
	}
	writeClassifiedDependencyViolationCounts(stderr, classifiedDependencyCounts)
	writeGeneratedCodeExceptionSummary(stderr, policy)
	return fmt.Errorf("[agent-factory:pkg-boundary] found %d package-boundary violation(s)", blockingViolationCount)
}

func countBlockingViolations(findings scanResult) int {
	return countAlwaysBlockingViolations(findings) +
		countProductionBoundaryViolations(findings)
}

func countAlwaysBlockingViolations(findings scanResult) int {
	return len(findings.rootPackageFindings) +
		len(findings.retiredPackageRootFindings) +
		len(findings.constructedServiceEdgesFindings) +
		len(findings.productionDefaultFindings) +
		len(findings.staleProductionDefaultEntries) +
		len(findings.petriPublicSurfaceFindings) +
		len(findings.stalePetriPublicSurfaceEntries)
}

func countProductionBoundaryViolations(findings scanResult) int {
	return countProductionBoundaryImports(findings) + countProductionBoundaryBaselines(findings)
}

func countProductionBoundaryImports(findings scanResult) int {
	count := 0
	count += countProductionBoundaryFindings(
		findings.serviceConstructionFindings,
		func(finding serviceConstructionFinding) boundarySourceClass { return finding.class },
		func(finding serviceConstructionFinding) string { return finding.filePath },
	)
	count += countProductionBoundaryFindings(
		findings.externalImplementationFindings,
		func(finding transportServiceImplementationFinding) boundarySourceClass { return finding.class },
		func(finding transportServiceImplementationFinding) string { return finding.filePath },
	)
	return count
}

func countProductionBoundaryBaselines(findings scanResult) int {
	count := 0
	count += countProductionBoundaryFindings(
		findings.staleServiceConstructionEntries,
		func(entry serviceConstructionBaselineEntry) boundarySourceClass {
			class, _ := sourceClassFromBaseline(entry.Class, entry.FilePath)
			return class
		},
		func(entry serviceConstructionBaselineEntry) string { return entry.FilePath },
	)
	return count
}
