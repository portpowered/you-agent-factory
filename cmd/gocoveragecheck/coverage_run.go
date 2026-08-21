package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

func runCoverageProfile(cfg config, targetOS string, logicalCPUs int, profilePath string) (coverageResult, error) {
	repoRoot, err := repoRootDir()
	if err != nil {
		return coverageResult{}, err
	}
	selectorVerification := startFunctionalQuarantineSelectorVerification(cfg, targetOS, logicalCPUs, repoRoot)
	if selectorVerification != nil {
		selectorVerification.ratchet = startFunctionalQuarantineRatchetVerification(
			selectorVerification.manifest,
			cfg.timeout,
			cfg.short,
			repoRoot,
		)
		defer func() {
			_ = selectorVerification.waitAll()
		}()
	}
	coverPackages, err := resolveCoverPackages(cfg)
	if err != nil {
		if selectorVerification != nil {
			_ = selectorVerification.waitAll()
		}
		return coverageResult{}, err
	}
	testPackages, functionalMetadata, functionalDiscoveryStarted, err := resolveCoverageTestPackages(cfg, repoRoot, selectorVerification)
	if err != nil {
		return coverageResult{}, err
	}
	testPackages, functionalSelection, err := prepareCoverageTestPackagesWithVerification(
		cfg,
		testPackages,
		targetOS,
		logicalCPUs,
		repoRoot,
		functionalMetadata,
		functionalDiscoveryStarted,
		selectorVerification,
	)
	if err != nil {
		return coverageResult{}, err
	}
	coverageTestArgs, testPackageArgs := buildCoverageTestArguments(cfg, targetOS, logicalCPUs, coverPackages, testPackages)
	var runErr error
	if functionalSelection == nil {
		runErr = runGoTestCoverageLane(cfg, coverageTestArgs, testPackages, profilePath, repoRoot, coverPackages, targetOS, "run go test coverage lane", testPackageArgs)
	} else {
		runErr = runGoTestCoverageLaneWithSelection(cfg, coverageTestArgs, testPackages, profilePath, repoRoot, coverPackages, targetOS, "run go test coverage lane", *functionalSelection)
	}
	if runErr != nil {
		return coverageResult{}, runErr
	}
	if err := canonicalizeCoverageProfile(profilePath, repoRoot, coverPackages); err != nil {
		return coverageResult{}, err
	}

	legacyPackageGateEnabled := !cfg.totalOnly && cfg.generateManifest == "" && cfg.updateManifest == "" && strings.TrimSpace(cfg.packageManifest) == ""
	baselinePackages := map[string]struct{}{}
	if legacyPackageGateEnabled {
		baselinePackages, err = packageCoverageBaselinePackages(cfg, repoRoot)
		if err != nil {
			return coverageResult{}, err
		}
	}

	result, totalLine, err := evaluateCoverage("", "", profilePath, repoRoot, coverPackages, cfg.packageCoverageMin(), baselinePackages, legacyPackageGateEnabled)
	if err != nil {
		return coverageResult{}, err
	}
	fmt.Fprintln(stdoutWriter, totalLine)
	if !cfg.totalOnly && strings.TrimSpace(cfg.packageManifest) != "" {
		manifestPath := cfg.packageManifest
		if !filepath.IsAbs(manifestPath) {
			manifestPath = filepath.Join(repoRoot, manifestPath)
		}
		manifest, err := readCoverageManifestFileWithTotals(manifestPath, cfg.suite, packageImportPaths(result.packageSummaries), result.packageTotals)
		if err != nil {
			var validationErr *coverageManifestValidationError
			if errors.As(err, &validationErr) {
				return result, err
			}
			return coverageResult{}, err
		}
		result.packageMinimumFailures, result.packageMinimumWarnings = checkCoverageManifestWithEpsilonAndBlocks(manifest, result.packageTotals, cfg.packageManifest, cfg.packageFloorEpsilon, result.coverageBlocks)
		result.unmeasuredPackageDiagnostics = formatUnmeasuredCoverageManifestDiagnostics(manifest, result.packageTotals)
		result.packageGates = coverageManifestGatedPackages(manifest, result.packageTotals)
	} else if legacyPackageGateEnabled {
		result.packageGates = packageGatesFromLegacyMin(result.packageSummaries, cfg.packageCoverageMin(), baselinePackages)
	}
	return result, nil
}

func buildCoverageTestArguments(cfg config, targetOS string, logicalCPUs int, coverPackages, testPackages []string) ([]string, []string) {
	coverPackageArgument := strings.Join(coverPackages, ",")
	if targetOS == "windows" && strings.TrimSpace(cfg.coverpkg) == "" {
		// A fully expanded backend package list exceeds Windows' command-line
		// limit. A package pattern keeps the invocation to one logical coverage
		// pass; the resolved list above remains authoritative for profile
		// filtering, reporting, and package gates.
		coverPackageArgument = modulePath + "/pkg/..."
	}
	coverageTestArgs := []string{
		"test",
		fmt.Sprintf("-coverpkg=%s", coverPackageArgument),
		fmt.Sprintf("-p=%d", cfg.testJobs(targetOS, logicalCPUs)),
		// Coverage is an authoritative measurement, so every package must run
		// even when a prior non-instrumented invocation is cached.
		"-count=1",
	}
	if cfg.short {
		coverageTestArgs = append(coverageTestArgs, "-short")
	}
	coverageTestArgs = append(coverageTestArgs,
		fmt.Sprintf("-covermode=%s", cfg.covermode),
		fmt.Sprintf("-timeout=%s", cfg.timeout),
	)
	return coverageTestArgs, compactUnitTestPackageArgs(cfg, testPackages, targetOS)
}
