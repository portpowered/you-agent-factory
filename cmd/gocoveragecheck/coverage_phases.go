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

// Functional packages combine package-level workers with tests marked
// t.Parallel. Keep the in-package worker count bounded while the lane-level
// -p value remains caller-controlled; otherwise the two levels multiply into
// an excessive number of instrumented functional runtimes on CI hosts.
const functionalCoverageTestParallelism = 2

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
	if cfg.suite == functionalCoverageSuite {
		coverageTestArgs = append(coverageTestArgs, fmt.Sprintf("-parallel=%d", functionalCoverageTestParallelism))
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
	if !cfg.totalOnly && strings.TrimSpace(cfg.packageManifest) != "" {
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
	return result, nil
}
