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
	fmt.Fprintf(&b, "- Package count: %d\n\n", summary.PackageCount)

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
	return b.String()
}

func stableOrderedTimingPackages(packages []FunctionalPackageTiming) []FunctionalPackageTiming {
	ordered := make([]FunctionalPackageTiming, len(packages))
	copy(ordered, packages)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Package < ordered[j].Package
	})
	return ordered
}
