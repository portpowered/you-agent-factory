package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

type config struct {
	root                              string
	packageRoot                       string
	all                               bool
	baseRef                           string
	writeTestServiceImportBaseline    bool
	writeSupportServiceImportBaseline bool
	writeTransportBehaviorBaseline    bool
	writeProductionDefaultBaseline    bool
	writeTestBehaviorBaseline         bool
	writePetriPublicSurfaceBaseline   bool
}

func parseConfig() config {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root to scan")
	flag.StringVar(&cfg.packageRoot, "package-root", defaultScanRoot, "repository-relative package root to scan")
	flag.BoolVar(&cfg.all, "all", false, "show recorded package-boundary diagnostics as well as unrecorded findings")
	flag.StringVar(&cfg.baseRef, "base-ref", "", "optional Git ref used to identify recorded package-boundary findings")
	flag.BoolVar(
		&cfg.writeTestServiceImportBaseline,
		"create-test-service-import-baseline",
		false,
		"create the deletion-only test service import baseline; fails when the file already exists",
	)
	flag.BoolVar(
		&cfg.writeProductionDefaultBaseline,
		"create-production-default-selection-baseline",
		false,
		"create the deletion-only production default-selection baseline; fails when the file already exists or no debt exists",
	)
	flag.BoolVar(
		&cfg.writeSupportServiceImportBaseline,
		"create-support-service-import-baseline",
		false,
		"create the deletion-only reusable-support service import baseline; fails when the file already exists",
	)
	flag.BoolVar(
		&cfg.writeTransportBehaviorBaseline,
		"create-transport-behavior-baseline",
		false,
		"create the deletion-only transport behavior baseline; fails when the file already exists",
	)
	flag.BoolVar(
		&cfg.writeTestBehaviorBaseline,
		"create-test-behavior-boundary-baseline",
		false,
		"create the exact deletion-only test behavior baseline; fails when the file exists or no debt exists",
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

	findings, err := scanRepo(cfg, policy)
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
		countProductionBoundaryViolations(findings) +
		// Test-service imports are an intentional test-specific policy. They
		// remain blocking even though their source class is test-only.
		len(findings.testServiceImportFindings) +
		len(findings.staleTestServiceBaselineEntries)
}

func countAlwaysBlockingViolations(findings scanResult) int {
	return len(findings.rootPackageFindings) +
		len(findings.retiredPackageRootFindings) +
		len(findings.migrationShimFindings) +
		len(findings.handwrittenGeneratedFindings) +
		len(findings.transportBehaviorFindings) +
		len(findings.staleTransportBehaviorEntries) +
		len(findings.functionalProcessEdgeFindings) +
		len(findings.constructedServiceEdgesFindings) +
		len(findings.testWorkNormalizationFindings) +
		len(findings.productionDefaultFindings) +
		len(findings.staleProductionDefaultEntries) +
		len(findings.initializerBehaviorFindings) +
		len(findings.staleInitializerBehaviorEntries) +
		len(findings.testBehaviorFindings) +
		len(findings.staleTestBehaviorEntries) +
		len(findings.petriPublicSurfaceFindings) +
		len(findings.stalePetriPublicSurfaceEntries) +
		len(findings.providerEffectOwnershipFindings)
}

func countProductionBoundaryViolations(findings scanResult) int {
	return countProductionBoundaryImports(findings) + countProductionBoundaryBaselines(findings)
}

func countProductionBoundaryImports(findings scanResult) int {
	count := 0
	count += countProductionBoundaryFindings(
		findings.retiredPackageImportFindings,
		func(finding retiredPackageImportFinding) boundarySourceClass { return finding.class },
		func(finding retiredPackageImportFinding) string { return finding.filePath },
	)
	count += countProductionBoundaryFindings(
		findings.applicationGraphImportFindings,
		func(finding applicationGraphImportFinding) boundarySourceClass { return finding.class },
		func(finding applicationGraphImportFinding) string { return finding.filePath },
	)
	count += countProductionBoundaryFindings(
		findings.domainTransportFindings,
		func(finding domainTransportImportFinding) boundarySourceClass { return finding.class },
		func(finding domainTransportImportFinding) string { return finding.filePath },
	)
	count += countProductionBoundaryFindings(
		findings.peerServiceImportFindings,
		func(finding peerServiceImportFinding) boundarySourceClass { return finding.class },
		func(finding peerServiceImportFinding) string { return finding.filePath },
	)
	count += countProductionBoundaryFindings(
		findings.supportServiceImportFindings,
		func(finding supportServiceImportFinding) boundarySourceClass { return finding.class },
		func(finding supportServiceImportFinding) string { return finding.filePath },
	)
	count += countProductionBoundaryFindings(
		findings.serviceConstructionFindings,
		func(finding serviceConstructionFinding) boundarySourceClass { return finding.class },
		func(finding serviceConstructionFinding) string { return finding.filePath },
	)
	count += countProductionBoundaryFindings(
		findings.transportImplementationFindings,
		func(finding transportServiceImplementationFinding) boundarySourceClass { return finding.class },
		func(finding transportServiceImplementationFinding) string { return finding.filePath },
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
		findings.stalePeerServiceBaselineEntries,
		func(entry peerServiceImportBaselineEntry) boundarySourceClass {
			class, _ := sourceClassFromBaseline(entry.Class, entry.FilePath)
			return class
		},
		func(entry peerServiceImportBaselineEntry) string { return entry.FilePath },
	)
	count += countProductionBoundaryFindings(
		findings.staleSupportServiceBaselineEntries,
		func(entry supportServiceImportBaselineEntry) boundarySourceClass {
			class, _ := sourceClassFromBaseline(entry.Class, entry.FilePath)
			return class
		},
		func(entry supportServiceImportBaselineEntry) string { return entry.FilePath },
	)
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
