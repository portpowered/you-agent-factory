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
	covered         int
	measurable      int
	uncoveredBlocks int
}

// writeCoverageLaneReport prints a lane's per-package coverage result.
//
// A reporting lane replaces the raw "coverage: N% of statements" line per
// measured package — 300+ consecutive lines that bury the one actionable
// verdict — with a compact ordered block: the existing floor diagnostics, one
// complete package line per measured package, then a single tally line.
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
	belowFloor, heldFloor, nearFloor := partitionPackageCoverageVerdicts(verdicts)
	slices.SortStableFunc(verdicts, comparePackageCoverageVerdicts)

	fmt.Fprintf(stdoutWriter, "%s package coverage verdict:\n", label)
	writeBelowFloorCoverageLines(belowFloor)
	writeHeldCoverageLines(heldFloor)
	for _, verdict := range verdicts {
		writePackageCoverageVerdictLine(verdict, coverageLaneNoun(label))
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

func writeHeldCoverageLines(heldFloor []packageCoverageVerdict) {
	for _, verdict := range heldFloor {
		fmt.Fprintf(
			stdoutWriter,
			"  floor hold: package=%s floor=%.4f%% actual=%.4f%% delta=%+.4f percentage-points covered=%d/%d statements uncovered-blocks=%d\n",
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

func writePackageCoverageVerdictLine(verdict packageCoverageVerdict, lane string) {
	if !verdict.hasFloor {
		fmt.Fprintf(
			stdoutWriter,
			"  package=%s coverage=%.1f%% floor=none delta=n/a gate=report-only lane=%s\n",
			verdict.importPath,
			verdict.actual,
			lane,
		)
		return
	}

	gate := "pass"
	if verdict.measurable > 0 && verdict.headroom < 0 {
		gate = "fail"
		if verdict.held {
			gate = "hold"
		}
	}
	fmt.Fprintf(
		stdoutWriter,
		"  package=%s coverage=%.1f%% floor=%.1f%% delta=%+.1fpp gate=%s lane=%s\n",
		verdict.importPath,
		verdict.actual,
		verdict.floor,
		verdict.headroom,
		gate,
		lane,
	)
}

// collectPackageCoverageVerdicts reports one verdict per measured package.
// Numeric floors produce gate headroom; packages with no floor, including
// measurement exceptions, remain visible as report-only rows.
func collectPackageCoverageVerdicts(result coverageResult) []packageCoverageVerdict {
	uncovered := uncoveredCoverageBlockCounts(result.coverageBlocks)
	verdicts := make([]packageCoverageVerdict, 0, len(result.packageSummaries))
	for _, summary := range result.packageSummaries {
		gate := result.packageGates[summary.importPath]
		totals := result.packageTotals[summary.importPath]
		verdict := packageCoverageVerdict{
			importPath:      summary.importPath,
			actual:          summary.coverage,
			covered:         totals.coveredStatements,
			measurable:      totals.totalStatements,
			uncoveredBlocks: uncovered[summary.importPath],
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
