package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type preparedCoverageRun struct {
	plan                        coverageInvocationPlan
	repoRoot                    string
	testPackages                []string
	expectedFunctionalInventory *functionalTestInventory
}

func prepareCoverageRunWithFunctionalMetadata(
	cfg config,
	targetOS string,
	logicalCPUs int,
	profilePath string,
	coverPackages []string,
	testPackages []string,
	packageUniverse []string,
	listedPackages []functionalGoListPackage,
	discoveryStarted time.Time,
	selectorVerification *functionalQuarantineSelectorVerification,
	unitPackageFiles []coveragePackageListing,
) (preparedCoverageRun, error) {
	repoRoot, err := repoRootDir()
	if err != nil {
		return preparedCoverageRun{}, err
	}

	var functionalSelection *functionalCoverageSelection
	var expectedFunctionalInventory *functionalTestInventory
	if strings.TrimSpace(cfg.functionalQuarantine) != "" {
		var selection functionalCoverageSelection
		var selectedPackages []string
		if discoveryStarted.IsZero() {
			selection, selectedPackages, err = prepareFunctionalCoverageRun(cfg, testPackages, targetOS, logicalCPUs, repoRoot)
		} else {
			selectedPackages, functionalSelection, err = prepareCoverageTestPackagesWithVerification(
				cfg,
				testPackages,
				targetOS,
				logicalCPUs,
				repoRoot,
				listedPackages,
				discoveryStarted,
				selectorVerification,
			)
			if err == nil {
				selection = *functionalSelection
			}
		}
		selectionErr := err
		if selectionErr != nil {
			return preparedCoverageRun{}, selectionErr
		}
		if functionalSelection == nil {
			functionalSelection = &selection
		}
		expected := selectedFunctionalTestInventory(selection)
		expectedFunctionalInventory = &expected
		testPackages = selectedPackages
	}

	coverPackageArgument := strings.Join(coverPackages, ",")
	if targetOS == "windows" && strings.TrimSpace(cfg.coverpkg) == "" {
		// A fully expanded backend package list exceeds Windows' command-line
		// limit. A package pattern keeps the invocation to one logical coverage
		// pass; the resolved list remains authoritative for filtering, reporting,
		// and package gates.
		coverPackageArgument = modulePath + "/pkg/..."
	}
	coverageTestArgs := []string{
		"test",
		fmt.Sprintf("-coverpkg=%s", coverPackageArgument),
		fmt.Sprintf("-p=%d", cfg.testJobs(targetOS, logicalCPUs)),
	}
	// Required Backend Lint runs vet across the repository. Repeating it on the
	// instrumented graph adds static-analysis work to every test binary without
	// exercising another behavior. Coverage evidence retains Go's stack symbols
	// and source coordinates; debugger-only DWARF data is unnecessary, and
	// omitting it shortens the link of every instrumented test binary.
	coverageTestArgs = append(coverageTestArgs, "-vet=off", "-ldflags=-w")
	if cfg.suite == functionalCoverageSuite {
		// Consolidated suites contain independent, session-owned IO journeys.
		// Use the lane budget for their concurrency as well as package builds;
		// Go's CPU-count default otherwise serializes those waits after a merge.
		coverageTestArgs = append(coverageTestArgs, "-count=1", fmt.Sprintf("-parallel=%d", cfg.testJobs(targetOS, logicalCPUs)))
	}
	if cfg.short {
		coverageTestArgs = append(coverageTestArgs, "-short")
	}
	coverageTestArgs = append(coverageTestArgs,
		fmt.Sprintf("-covermode=%s", cfg.covermode),
		fmt.Sprintf("-timeout=%s", cfg.timeout),
	)
	testPackageArgs := compactUnitTestPackageArgs(cfg, testPackages, targetOS, packageUniverse)
	plan, err := planCoverageInvocationWithUnitImports(cfg, coverageTestArgs, testPackages, profilePath, targetOS, testPackageArgs, functionalSelection, unitPackageFiles)
	if err != nil {
		return preparedCoverageRun{}, err
	}
	if cfg.functionalMonolith {
		if err := consolidateFunctionalCoveragePlan(cfg, &plan, testPackages, repoRoot, listedPackages, functionalSelection); err != nil {
			return preparedCoverageRun{}, errors.Join(err, plan.cleanup())
		}
	}
	return preparedCoverageRun{
		plan:                        plan,
		repoRoot:                    repoRoot,
		testPackages:                testPackages,
		expectedFunctionalInventory: expectedFunctionalInventory,
	}, nil
}

func planCoverageInvocationWithUnitImports(
	cfg config,
	coverageTestArgs []string,
	testPackages []string,
	profilePath string,
	targetOS string,
	testPackageArgs []string,
	functionalSelection *functionalCoverageSelection,
	unitPackageFiles []coveragePackageListing,
) (coverageInvocationPlan, error) {
	unitCoverageImportCleanup := func() error { return nil }
	if len(unitPackageFiles) > 0 && strings.TrimSpace(cfg.packages) == "" && strings.TrimSpace(cfg.coverpkg) == "" && (cfg.suite == "" || cfg.suite == unitCoverageSuite) {
		var err error
		unitCoverageImportCleanup, err = prepareUnitCoverageImportFile(testPackages, unitPackageFiles)
		if err != nil {
			if unitCoverageImportCleanup != nil {
				err = errors.Join(err, unitCoverageImportCleanup())
			}
			return coverageInvocationPlan{}, err
		}
	}

	var plan coverageInvocationPlan
	var err error
	if functionalSelection == nil {
		plan, err = planGoTestCoverageLane(coverageTestArgs, testPackages, profilePath, cfg, targetOS, testPackageArgs)
	} else {
		plan, err = planGoTestCoverageLaneWithSelection(coverageTestArgs, profilePath, cfg, targetOS, *functionalSelection)
	}
	if err != nil {
		return coverageInvocationPlan{}, errors.Join(err, unitCoverageImportCleanup())
	}
	planCleanup := plan.cleanup
	plan.cleanup = func() error {
		return errors.Join(planCleanup(), unitCoverageImportCleanup())
	}
	return plan, nil
}

type evaluatedCoverageRun struct {
	result           coverageResult
	baselinePackages map[string]struct{}
}

func evaluateCoverageRun(cfg config, profilePath string, repoRoot string, coverPackages []string, canonicalBlocks map[string]coverageBlock) (evaluatedCoverageRun, error) {
	baselinePackages := map[string]struct{}{}
	if legacyPackageGateEnabled(cfg) {
		var err error
		baselinePackages, err = packageCoverageBaselinePackages(cfg, repoRoot)
		if err != nil {
			return evaluatedCoverageRun{}, err
		}
	}

	var result coverageResult
	var totalLine string
	var err error
	if canonicalBlocks == nil {
		result, totalLine, err = evaluateCoverage("", "", profilePath, repoRoot, coverPackages, cfg.packageCoverageMin(), baselinePackages, legacyPackageGateEnabled(cfg))
	} else {
		result, totalLine, err = evaluateCoverageBlocks(canonicalBlocks, coverPackages, cfg.packageCoverageMin(), baselinePackages, legacyPackageGateEnabled(cfg))
	}
	if err != nil {
		return evaluatedCoverageRun{}, err
	}
	fmt.Fprintln(stdoutWriter, totalLine)
	return evaluatedCoverageRun{result: result, baselinePackages: baselinePackages}, nil
}

func legacyPackageGateEnabled(cfg config) bool {
	return !cfg.totalOnly && cfg.generateManifest == "" && cfg.updateManifest == "" && strings.TrimSpace(cfg.packageManifest) == ""
}

func applyCoverageManifestGate(cfg config, result coverageResult, repoRoot string, baselinePackages map[string]struct{}) (coverageResult, error) {
	if (!cfg.totalOnly || cfg.suite == functionalCoverageSuite) && strings.TrimSpace(cfg.packageManifest) != "" {
		manifestPath := cfg.packageManifest
		if !filepath.IsAbs(manifestPath) {
			manifestPath = filepath.Join(repoRoot, manifestPath)
		}
		measuredPackages := packageImportPaths(result.packageSummaries)
		manifest, err := readCoverageManifestFileWithTotalsAtMode(
			manifestPath,
			cfg.suite,
			measuredPackages,
			result.packageTotals,
			!cfg.packageFloorPolicyIsAdvisory(),
		)
		if err != nil {
			var validationErr *coverageManifestValidationError
			if errors.As(err, &validationErr) {
				return result, err
			}
			return coverageResult{}, err
		}
		result.packageMinimumFailures, result.packageMinimumWarnings = checkCoverageManifestWithEpsilonAndBlocks(manifest, result.packageTotals, cfg.packageManifest, cfg.packageFloorEpsilon, result.coverageBlocks)
		retainedFailures := checkRetainedFunctionalFloors(manifest, result.packageTotals, result.coverageBlocks, repoRoot)
		result.packageMinimumFailures = append(filterDispositionFloorFindings(manifest, result.packageMinimumFailures, retainedFailures), retainedFailures...)
		result.packageMinimumWarnings = filterDispositionFloorFindings(manifest, result.packageMinimumWarnings, retainedFailures)
		result.packageMinimumWarnings = append(result.packageMinimumWarnings, retainedDispositionEligibilityDiagnostics(manifest, result.packageMinimumFailures)...)
		if cfg.packageFloorPolicyIsAdvisory() {
			result.packageMinimumWarnings = append(result.packageMinimumWarnings, result.packageMinimumFailures...)
			result.packageMinimumFailures = nil
			result.manifestCompletenessWarnings = formatMissingCoverageManifestServiceRootDiagnostics(
				cfg.suite,
				measuredPackages,
				manifestPackageSet(manifest),
				result.packageTotals,
			)
		}
		result.unmeasuredPackageDiagnostics = formatUnmeasuredCoverageManifestDiagnostics(manifest, result.packageTotals)
		result.packageGates = coverageManifestGatedPackages(manifest, result.packageTotals)
	} else if legacyPackageGateEnabled(cfg) {
		result.packageGates = packageGatesFromLegacyMin(result.packageSummaries, cfg.packageCoverageMin(), baselinePackages)
	}
	if cfg.suite == functionalCoverageSuite && strings.TrimSpace(cfg.packageManifest) == "" && cfg.generateManifest == "" {
		result.packageMinimumFailures = append(result.packageMinimumFailures, checkRetainedFunctionalFloors(coverageManifest{Lane: functionalCoverageSuite}, result.packageTotals, result.coverageBlocks, repoRoot)...)
	}
	return result, nil
}
