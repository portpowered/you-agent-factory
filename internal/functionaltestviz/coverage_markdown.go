package functionaltestviz

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// RenderPackageCoverageMarkdown renders overall statement totals and a
// stable-ordered per-production-package table from coverage-summary JSON
// fields only (no profile recomputation).
func RenderPackageCoverageMarkdown(summary CoverageSummary) string {
	packages := StableOrderedPackages(summary.Packages)

	var b strings.Builder
	b.WriteString("## Package coverage\n\n")
	b.WriteString(fmt.Sprintf("- Covered statements: %d\n", summary.CoveredStatements))
	b.WriteString(fmt.Sprintf("- Measurable statements: %d\n", summary.MeasurableStatements))
	b.WriteString(fmt.Sprintf("- Coverage percent: %s%%\n\n", formatCoveragePercent(summary.CoveragePercent)))

	if len(packages) == 0 {
		b.WriteString("- _No production packages in coverage summary._\n")
		return b.String()
	}

	b.WriteString("| Package | Covered | Measurable | Coverage % | Floor | Measurement exception |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: | --- |\n")
	for _, pkg := range packages {
		b.WriteString("| `")
		b.WriteString(pkg.Package)
		b.WriteString("` | ")
		b.WriteString(strconv.Itoa(pkg.CoveredStatements))
		b.WriteString(" | ")
		b.WriteString(strconv.Itoa(pkg.MeasurableStatements))
		b.WriteString(" | ")
		b.WriteString(formatCoveragePercent(pkg.CoveragePercent))
		b.WriteString(" | ")
		b.WriteString(formatOptionalFloor(pkg.PackageFloor))
		b.WriteString(" | ")
		b.WriteString(formatMeasurementException(pkg.MeasurementException))
		b.WriteString(" |\n")
	}
	return b.String()
}

// StableOrderedPackages returns a copy of packages sorted by import path.
func StableOrderedPackages(packages []PackageCoverage) []PackageCoverage {
	ordered := make([]PackageCoverage, len(packages))
	copy(ordered, packages)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Package < ordered[j].Package
	})
	return ordered
}

func formatCoveragePercent(value float64) string {
	return strconv.FormatFloat(value, 'f', 1, 64)
}

func formatOptionalFloor(floor *float64) string {
	if floor == nil {
		return "—"
	}
	return strconv.FormatFloat(*floor, 'f', -1, 64)
}

func formatMeasurementException(exception *MeasurementException) string {
	if exception == nil {
		return "—"
	}
	parts := make([]string, 0, 4)
	if kind := strings.TrimSpace(exception.Kind); kind != "" {
		parts = append(parts, kind)
	}
	if justification := strings.TrimSpace(exception.Justification); justification != "" {
		parts = append(parts, justification)
	}
	meta := make([]string, 0, 3)
	if owner := strings.TrimSpace(exception.Owner); owner != "" {
		meta = append(meta, "owner="+owner)
	}
	if deadline := strings.TrimSpace(exception.Deadline); deadline != "" {
		meta = append(meta, "deadline="+deadline)
	}
	if removalGate := strings.TrimSpace(exception.RemovalGate); removalGate != "" {
		meta = append(meta, "removalGate="+removalGate)
	}
	body := strings.Join(parts, ": ")
	if len(meta) == 0 {
		if body == "" {
			return "—"
		}
		return escapeTableCell(body)
	}
	if body == "" {
		return escapeTableCell(strings.Join(meta, "; "))
	}
	return escapeTableCell(body + " (" + strings.Join(meta, "; ") + ")")
}

func escapeTableCell(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}
