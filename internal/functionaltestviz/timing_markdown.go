package functionaltestviz

import (
	"fmt"
	"sort"
	"strings"
)

// RenderFunctionalTimingMarkdown renders total and per-functional-test-package
// timing from a decoded functional-timing-summary. Package elapsed durations
// can overlap because functional-test packages run concurrently, so their
// sum can exceed wall-clock duration; the rendered explanation calls this out
// explicitly. An incomplete summary is labeled as such rather than presented
// as a successful complete report.
func RenderFunctionalTimingMarkdown(summary FunctionalTimingSummary) string {
	packages := stableOrderedTimingPackages(summary.Packages)

	var b strings.Builder
	b.WriteString("## Functional test timings\n\n")
	if summary.Complete {
		b.WriteString("- Capture status: complete\n")
	} else {
		b.WriteString("- Capture status: incomplete — partial diagnostics only, not a successful complete capture\n")
	}
	fmt.Fprintf(&b, "- Total wall-clock duration: %.3fs\n", summary.WallSeconds)
	fmt.Fprintf(
		&b,
		"- Package elapsed total: %.3fs (functional-test packages run concurrently, so this total can exceed wall-clock duration)\n",
		summary.PackageElapsedSecondsSum,
	)
	if summary.ExpectedPackageCount > 0 {
		fmt.Fprintf(&b, "- Discovered functional packages: %d\n", summary.ExpectedPackageCount)
	}
	fmt.Fprintf(&b, "- Package count: %d\n\n", summary.PackageCount)
	if summary.TestCount > 0 || summary.InventoryTestCount > 0 {
		if summary.InventoryTestCount > 0 {
			fmt.Fprintf(&b, "- Top-level test inventory: %d\n", summary.InventoryTestCount)
		}
		fmt.Fprintf(&b, "- Top-level tests with observed outcomes: %d\n", summary.TestCount)
		fmt.Fprintf(&b, "- Top-level test outcomes: pass=%d, fail=%d, skip=%d\n\n", summary.TestPassCount, summary.TestFailCount, summary.TestSkipCount)
	}

	if len(packages) == 0 {
		b.WriteString("- _No functional-test packages in timing summary._\n")
		return b.String()
	}

	b.WriteString("| Package | Elapsed (s) | Outcome |\n")
	b.WriteString("| --- | ---: | --- |\n")
	for _, pkg := range packages {
		b.WriteString("| `")
		b.WriteString(pkg.Package)
		b.WriteString("` | ")
		fmt.Fprintf(&b, "%.3f", pkg.Seconds)
		b.WriteString(" | ")
		b.WriteString(pkg.Outcome)
		b.WriteString(" |\n")
	}

	renderFunctionalTestFailures(&b, summary.Tests)
	renderFunctionalTestsByElapsed(&b, summary.Tests)
	return b.String()
}

// renderFunctionalTestsByElapsed renders every top-level test the run observed,
// slowest first, from the per-test rows already captured alongside the package
// timings. Package totals answer "which package is slow"; this answers "how
// long did each test take". The job summary is the one surface with room for
// the complete list, so it is not truncated here -- the pull request comment
// renderer caps its own excerpt separately. Nothing is rendered when a run
// captured no per-test rows at all.
func renderFunctionalTestsByElapsed(output *strings.Builder, tests []FunctionalTestTiming) {
	if len(tests) == 0 {
		return
	}
	slowest := make([]FunctionalTestTiming, len(tests))
	copy(slowest, tests)
	sort.SliceStable(slowest, func(i, j int) bool {
		if slowest[i].Seconds != slowest[j].Seconds {
			return slowest[i].Seconds > slowest[j].Seconds
		}
		if slowest[i].Package != slowest[j].Package {
			return slowest[i].Package < slowest[j].Package
		}
		return slowest[i].Test < slowest[j].Test
	})

	output.WriteString("\n### Top-level tests by elapsed time\n\n")
	fmt.Fprintf(
		output,
		"- Showing all %d observed top-level tests by elapsed time, slowest first.\n\n",
		len(slowest),
	)
	output.WriteString("| Test | Package | Elapsed (s) | Outcome |\n")
	output.WriteString("| --- | --- | ---: | --- |\n")
	for _, test := range slowest {
		output.WriteString("| `")
		output.WriteString(test.Test)
		output.WriteString("` | `")
		output.WriteString(test.Package)
		output.WriteString("` | ")
		fmt.Fprintf(output, "%.3f", test.Seconds)
		output.WriteString(" | ")
		output.WriteString(test.Outcome)
		output.WriteString(" |\n")
	}
}

func renderFunctionalTestFailures(output *strings.Builder, tests []FunctionalTestTiming) {
	failed := make([]FunctionalTestTiming, 0)
	for _, test := range tests {
		if test.Outcome == timingOutcomeFail {
			failed = append(failed, test)
		}
	}
	if len(failed) == 0 {
		return
	}
	sort.SliceStable(failed, func(i, j int) bool {
		if failed[i].Package != failed[j].Package {
			return failed[i].Package < failed[j].Package
		}
		return failed[i].Test < failed[j].Test
	})

	output.WriteString("\n### Failed top-level tests\n\n")
	output.WriteString("| Package | Test | Elapsed (s) | Reason |\n")
	output.WriteString("| --- | --- | ---: | --- |\n")
	for _, test := range failed {
		output.WriteString("| `")
		output.WriteString(test.Package)
		output.WriteString("` | `")
		output.WriteString(test.Test)
		output.WriteString("` | ")
		fmt.Fprintf(output, "%.3f", test.Seconds)
		output.WriteString(" | ")
		if test.Reason == "" {
			output.WriteString("_no concise reason captured_")
		} else {
			output.WriteString(strings.ReplaceAll(test.Reason, "|", "\\|"))
		}
		output.WriteString(" |\n")
	}
}

func stableOrderedTimingPackages(packages []FunctionalPackageTiming) []FunctionalPackageTiming {
	ordered := make([]FunctionalPackageTiming, len(packages))
	copy(ordered, packages)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Package < ordered[j].Package
	})
	return ordered
}
