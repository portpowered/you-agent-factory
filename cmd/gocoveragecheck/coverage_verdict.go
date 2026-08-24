package main

import (
	"fmt"
	"slices"
	"strings"
)

// functionalCoverageSuite and unitCoverageSuite name the two lanes whose logs
// are rendered as a compact ordered verdict block instead of one raw coverage
// line per measured package.
const functionalCoverageSuite = "functional"
const unitCoverageSuite = "unit"

// nearFloorCoverageHeadroomPoints is the headroom in percentage points below
// which a passing gated package is reported as near its floor.
const nearFloorCoverageHeadroomPoints = 2.0

// packageCoverageVerdict is one measured package's coverage and, when present,
// its distance to the package floor. Headroom is negative when a measurable
// package is below its floor. A package without a numeric floor is report-only.
type packageCoverageVerdict struct {
	importPath      string
	hasFloor        bool
	held            bool
	floor           float64
	actual          float64
	headroom        float64
	measurable      int
}

// writeCoverageLaneReport prints a lane's per-package coverage result.
//
// The compact row is the default for both coverage lanes. The complete
// measurement remains in the JSON artifact when one is requested, while the
// row itself keeps the package, coverage, floor state, and outcome visible to a
// reader of an ordinary command log.
func writeCoverageLaneReport(cfg config, result coverageResult, failures []string) {
	writeCoverageVerdict(coverageLaneLabel(cfg.suite), result, failures)
}

// coverageLaneLabel names the lane in its verdict block so a job log that
// carries both reports stays attributable to the suite that produced it.
func coverageLaneLabel(suite string) string {
	switch strings.TrimSpace(suite) {
	case functionalCoverageSuite:
		return "Functional"
	case unitCoverageSuite, "":
		return "Unit"
	default:
		return strings.ToUpper(suite[:1]) + suite[1:]
	}
}

// coverageLaneNoun names the lane in prose. The timing capture notes are
// produced by code both suites share, so a hard-coded "functional" tells a
// reader of the unit lane's timing artifact that some other suite was
// interrupted.
func coverageLaneNoun(suite string) string {
	return strings.ToLower(coverageLaneLabel(suite))
}

// writePackageCoverageSummaries is retained for callers that explicitly need
// the legacy raw go-test listing while diagnosing a producer outside the
// normal lane report.
func writePackageCoverageSummaries(summaries []packageCoverageSummary) {
	for _, summary := range summaries {
		fmt.Fprintf(stdoutWriter, "%s\tcoverage: %.1f%% of statements\n", summary.importPath, summary.coverage)
	}
}

// writeCoverageVerdict renders one lane's ordered verdict block.
func writeCoverageVerdict(label string, result coverageResult, failures []string) {
	verdicts := collectPackageCoverageVerdicts(result)
	belowFloor, _, nearFloor := partitionPackageCoverageVerdicts(verdicts)
	slices.SortStableFunc(verdicts, comparePackageCoverageVerdicts)

	fmt.Fprintf(stdoutWriter, "%s package coverage verdict:\n", label)
	for _, verdict := range verdicts {
		writePackageCoverageVerdictLine(verdict, coverageLaneNoun(label), result)
	}
	fmt.Fprintf(
		stdoutWriter,
		"  tally: measured-packages=%d gated-packages=%d below-floor=%d near-floor=%d gate-failures=%d\n",
		len(result.packageSummaries),
		countGatedPackageCoverageVerdicts(verdicts),
		len(belowFloor),
		len(nearFloor),
		len(failures),
	)
}

func writePackageCoverageVerdictLine(verdict packageCoverageVerdict, lane string, result coverageResult) {
	if !verdict.hasFloor {
		fmt.Fprintf(
			stdoutWriter,
			"  package=%s coverage=%.1f%% floor=n/a status=report-only lane=%s\n",
			verdict.importPath,
			verdict.actual,
			lane,
		)
		return
	}

	status := "PASS"
	if verdict.held {
		status = "HOLD"
	} else {
		if verdict.measurable > 0 && verdict.headroom < 0 {
			status = "FAIL"
			if result.packageFloorPolicy == coverageFloorPolicyAdvisory {
				status = "WARN"
			}
		}
		if packageCoverageFinding(result.packageMinimumFailures, verdict.importPath) {
			status = "FAIL"
		} else if packageCoverageFinding(result.packageMinimumWarnings, verdict.importPath) {
			status = "WARN"
		} else if packageCoverageSummaryFinding(result.insufficientCoveragePackages, verdict.importPath) {
			status = "FAIL"
			if result.packageFloorPolicy == coverageFloorPolicyAdvisory {
				status = "WARN"
			}
		}
	}
	fmt.Fprintf(
		stdoutWriter,
		"  package=%s coverage=%.1f%% floor=%.1f%% delta=%+.1fpp status=%s lane=%s\n",
		verdict.importPath,
		verdict.actual,
		verdict.floor,
		verdict.headroom,
		status,
		lane,
	)
}

func packageCoverageFinding(diagnostics []string, importPath string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic, "package="+importPath+" ") {
			return true
		}
	}
	return false
}

func packageCoverageSummaryFinding(summaries []packageCoverageSummary, importPath string) bool {
	for _, summary := range summaries {
		if summary.importPath == importPath {
			return true
		}
	}
	return false
}

// collectPackageCoverageVerdicts reports one verdict per measured package.
// Numeric floors produce gate headroom; packages with no floor, including
// measurement exceptions, remain visible as report-only rows.
func collectPackageCoverageVerdicts(result coverageResult) []packageCoverageVerdict {
	verdicts := make([]packageCoverageVerdict, 0, len(result.packageSummaries))
	for _, summary := range result.packageSummaries {
		gate := result.packageGates[summary.importPath]
		totals := result.packageTotals[summary.importPath]
		verdict := packageCoverageVerdict{
			importPath: summary.importPath,
			actual:     summary.coverage,
			measurable: totals.totalStatements,
		}
		if gate.Floor != nil {
			verdict.hasFloor = true
			verdict.held = gate.FloorHold != nil
			verdict.floor = *gate.Floor
			verdict.headroom = verdict.actual - verdict.floor
		}
		verdicts = append(verdicts, verdict)
	}
	return verdicts
}

// partitionPackageCoverageVerdicts splits verdicts into active below-floor
// packages, staged floor holds, and passing packages within the near-floor
// band for the established diagnostics and tally. Report-only, held, and
// vacuously passing packages do not participate in the active failure or
// near-floor counts. All returned groups are ordered by headroom ascending
// with import path as the deterministic tiebreak.
//
// A zero floor is excluded from the near-floor band: coverage is never
// negative, so a package sitting on a 0% floor cannot regress through it and
// naming it would crowd out the packages that can.
func partitionPackageCoverageVerdicts(verdicts []packageCoverageVerdict) (belowFloor, heldFloor, nearFloor []packageCoverageVerdict) {
	belowFloor = make([]packageCoverageVerdict, 0)
	heldFloor = make([]packageCoverageVerdict, 0)
	nearFloor = make([]packageCoverageVerdict, 0)
	for _, verdict := range verdicts {
		if !verdict.hasFloor || verdict.measurable <= 0 {
			continue
		}
		switch {
		case verdict.headroom < 0:
			if verdict.held {
				heldFloor = append(heldFloor, verdict)
			} else {
				belowFloor = append(belowFloor, verdict)
			}
		case verdict.floor > 0 && verdict.headroom <= nearFloorCoverageHeadroomPoints:
			nearFloor = append(nearFloor, verdict)
		}
	}
	slices.SortStableFunc(belowFloor, comparePackageCoverageHeadroom)
	slices.SortStableFunc(heldFloor, comparePackageCoverageHeadroom)
	slices.SortStableFunc(nearFloor, comparePackageCoverageHeadroom)
	return belowFloor, heldFloor, nearFloor
}

func comparePackageCoverageVerdicts(left packageCoverageVerdict, right packageCoverageVerdict) int {
	if left.hasFloor != right.hasFloor {
		if left.hasFloor {
			return -1
		}
		return 1
	}
	if left.hasFloor {
		if comparison := comparePackageCoverageHeadroom(left, right); comparison != 0 {
			return comparison
		}
	}
	return strings.Compare(left.importPath, right.importPath)
}

func comparePackageCoverageHeadroom(left, right packageCoverageVerdict) int {
	if left.headroom < right.headroom {
		return -1
	}
	if left.headroom > right.headroom {
		return 1
	}
	return strings.Compare(left.importPath, right.importPath)
}

func countGatedPackageCoverageVerdicts(verdicts []packageCoverageVerdict) int {
	count := 0
	for _, verdict := range verdicts {
		if verdict.hasFloor {
			count++
		}
	}
	return count
}
