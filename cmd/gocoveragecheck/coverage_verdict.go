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

// nearFloorCoverageReportLimit caps how many passing near-floor packages the
// verdict block names. The block stays readable at a glance while the omitted
// count keeps the report honest about what it did not print.
const nearFloorCoverageReportLimit = 10

// nearFloorCoverageHeadroomPoints is the headroom in percentage points below
// which a passing gated package is reported as near its floor.
const nearFloorCoverageHeadroomPoints = 2.0

// packageCoverageVerdict is one gated package's measured distance to its floor.
// Headroom is negative exactly when the measurement is below the floor.
type packageCoverageVerdict struct {
	importPath      string
	floor           float64
	actual          float64
	headroom        float64
	covered         int
	measurable      int
	uncoveredBlocks int
}

// writeCoverageLaneReport prints a lane's per-package coverage result.
//
// A reporting lane replaces the raw "coverage: N% of statements" line per
// measured package — 300+ consecutive lines that bury the one actionable
// verdict — with a compact ordered block: floor violations first, then the
// packages closest to their floor, then a single tally line.
//
// Collapsing the listing is only safe when the complete per-package measurement
// survives somewhere a reader can still reach. Two lanes qualify: the
// functional lane, whose CI job always publishes the coverage-summary artifact,
// and any lane invoked with -json-output, which is that artifact. An invocation
// without one — an ordinary local `make test-unit-coverage`, or the
// malformed-manifest abort path that returns before the JSON is written — has
// the raw listing as the only copy of the measurement, so it keeps it.
func writeCoverageLaneReport(cfg config, result coverageResult, failures []string) {
	if cfg.suite != functionalCoverageSuite && strings.TrimSpace(cfg.jsonOutput) == "" {
		writePackageCoverageSummaries(result.packageSummaries)
		return
	}
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

// writePackageCoverageSummaries prints one raw coverage line per measured
// package. It remains the report for every non-functional lane and for the
// malformed-manifest abort path, which never reaches the JSON summary.
func writePackageCoverageSummaries(summaries []packageCoverageSummary) {
	for _, summary := range summaries {
		fmt.Fprintf(stdoutWriter, "%s\tcoverage: %.1f%% of statements\n", summary.importPath, summary.coverage)
	}
}

// writeCoverageVerdict renders one lane's ordered verdict block.
func writeCoverageVerdict(label string, result coverageResult, failures []string) {
	verdicts := collectPackageCoverageVerdicts(result)
	belowFloor, nearFloor := partitionPackageCoverageVerdicts(verdicts)

	fmt.Fprintf(stdoutWriter, "%s package coverage verdict:\n", label)
	writeBelowFloorCoverageLines(belowFloor)
	writeNearFloorCoverageLines(nearFloor)
	fmt.Fprintf(
		stdoutWriter,
		"  tally: measured-packages=%d gated-packages=%d below-floor=%d near-floor=%d gate-failures=%d\n",
		len(result.packageSummaries),
		len(verdicts),
		len(belowFloor),
		len(nearFloor),
		len(failures),
	)
}

func writeBelowFloorCoverageLines(belowFloor []packageCoverageVerdict) {
	if len(belowFloor) == 0 {
		fmt.Fprintln(stdoutWriter, "  floor violations: none")
		return
	}
	for _, verdict := range belowFloor {
		fmt.Fprintf(
			stdoutWriter,
			"  floor violation: package=%s floor=%.4f%% actual=%.4f%% delta=%+.4f percentage-points covered=%d/%d statements uncovered-blocks=%d\n",
			verdict.importPath,
			verdict.floor,
			verdict.actual,
			verdict.headroom,
			verdict.covered,
			verdict.measurable,
			verdict.uncoveredBlocks,
		)
	}
}

func writeNearFloorCoverageLines(nearFloor []packageCoverageVerdict) {
	if len(nearFloor) == 0 {
		fmt.Fprintf(
			stdoutWriter,
			"  near floor: none within %.4f percentage points\n",
			nearFloorCoverageHeadroomPoints,
		)
		return
	}
	reported := min(len(nearFloor), nearFloorCoverageReportLimit)
	for _, verdict := range nearFloor[:reported] {
		fmt.Fprintf(
			stdoutWriter,
			"  near floor: package=%s floor=%.4f%% actual=%.4f%% headroom=%+.4f percentage-points\n",
			verdict.importPath,
			verdict.floor,
			verdict.actual,
			verdict.headroom,
		)
	}
	if omitted := len(nearFloor) - reported; omitted > 0 {
		fmt.Fprintf(
			stdoutWriter,
			"  near floor: %d more package(s) within %.4f percentage points not shown\n",
			omitted,
			nearFloorCoverageHeadroomPoints,
		)
	}
}

// collectPackageCoverageVerdicts reports one verdict per package that carries a
// floor and has measurable statements. Packages without a floor, without a
// measurable denominator (a vacuous pass), or covered by a measurement
// exception have no distance to report and are excluded.
func collectPackageCoverageVerdicts(result coverageResult) []packageCoverageVerdict {
	uncovered := uncoveredCoverageBlockCounts(result.coverageBlocks)
	verdicts := make([]packageCoverageVerdict, 0, len(result.packageSummaries))
	for _, summary := range result.packageSummaries {
		gate := result.packageGates[summary.importPath]
		if gate.Floor == nil {
			continue
		}
		totals := result.packageTotals[summary.importPath]
		if totals.totalStatements <= 0 {
			continue
		}
		actual := float64(totals.coveredStatements) * 100 / float64(totals.totalStatements)
		verdicts = append(verdicts, packageCoverageVerdict{
			importPath:      summary.importPath,
			floor:           *gate.Floor,
			actual:          actual,
			headroom:        actual - *gate.Floor,
			covered:         totals.coveredStatements,
			measurable:      totals.totalStatements,
			uncoveredBlocks: uncovered[summary.importPath],
		})
	}
	return verdicts
}

// partitionPackageCoverageVerdicts splits verdicts into below-floor packages
// and passing packages within the near-floor band. Both are ordered by
// headroom ascending — the worst regression and the closest near-miss first —
// with import path as the deterministic tiebreak.
//
// A zero floor is excluded from the near-floor band: coverage is never
// negative, so a package sitting on a 0% floor cannot regress through it and
// naming it would crowd out the packages that can.
func partitionPackageCoverageVerdicts(verdicts []packageCoverageVerdict) (belowFloor, nearFloor []packageCoverageVerdict) {
	belowFloor = make([]packageCoverageVerdict, 0)
	nearFloor = make([]packageCoverageVerdict, 0)
	for _, verdict := range verdicts {
		switch {
		case verdict.headroom < 0:
			belowFloor = append(belowFloor, verdict)
		case verdict.floor > 0 && verdict.headroom <= nearFloorCoverageHeadroomPoints:
			nearFloor = append(nearFloor, verdict)
		}
	}
	slices.SortStableFunc(belowFloor, comparePackageCoverageHeadroom)
	slices.SortStableFunc(nearFloor, comparePackageCoverageHeadroom)
	return belowFloor, nearFloor
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

// uncoveredCoverageBlockCounts counts zero-execution coverage blocks per
// package in a single pass so the verdict block does not rescan every block
// once per reported package.
func uncoveredCoverageBlockCounts(blocks map[string]coverageBlock) map[string]int {
	counts := make(map[string]int, len(blocks))
	for _, block := range blocks {
		if block.executionCount != 0 {
			continue
		}
		counts[block.importPath]++
	}
	return counts
}
