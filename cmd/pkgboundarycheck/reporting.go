package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

func writeBoundaryFindings(writer io.Writer, findings scanResult) {
	writeServiceConstructionFindings(writer, findings.serviceConstructionFindings)
	writeServiceConstructionFindings(writer, findings.recordedServiceConstructionFindings)
	writeStaleServiceConstructionBaselineEntries(writer, findings.staleServiceConstructionEntries)
	writeProductionDefaultFindings(writer, findings.productionDefaultFindings)
	writeProductionDefaultFindings(writer, findings.recordedProductionDefaultFindings)
	writeStaleProductionDefaultBaselineEntries(writer, findings.staleProductionDefaultEntries)
}

func writeBaselineSummaries(writer io.Writer, findings scanResult) {
	writeServiceConstructionBaselineSummary(writer, findings.serviceConstructionBaselineCount)
	writeProductionDefaultBaselineSummary(writer, findings.productionDefaultBaselineCount)
}

func writeGeneratedCodeExceptionSummary(writer io.Writer, policy boundaryPolicy) {
	exceptions := generatedCodeExceptionDescriptions(policy)
	if len(exceptions) == 0 {
		return
	}
	fmt.Fprintf(writer, "[agent-factory:pkg-boundary] active generated-code exceptions: %s\n", strings.Join(exceptions, ", "))
}

func generatedCodeExceptionDescriptions(policy boundaryPolicy) []string {
	descriptions := make([]string, 0, len(policy.generatedCodeExceptions))
	for _, exception := range policy.generatedCodeExceptions {
		descriptions = append(descriptions, fmt.Sprintf("%s (%s)", filepath.ToSlash(exception.packagePath), exception.scope))
	}
	return descriptions
}

func writeServiceConstructionFindings(writer io.Writer, findings []serviceConstructionFinding) {
	for _, finding := range findings {
		fmt.Fprintf(
			writer,
			"[agent-factory:pkg-boundary] prohibited product-service construction: %s.%s (%s:%d, %d selection(s), class=%s)\n",
			finding.importPath,
			finding.symbol,
			finding.filePath,
			finding.line,
			finding.count,
			effectiveBoundarySourceClass(finding.class, finding.filePath),
		)
		fmt.Fprintf(writer, "  service owner: pkg/services/%s\n", finding.owner)
		fmt.Fprintln(writer, "  remediation: construct the collaborator in pkg/wire and inject its service-root role; owner-local invariants may construct it inside the owning service.")
	}
}

func writeStaleServiceConstructionBaselineEntries(writer io.Writer, entries []serviceConstructionBaselineEntry) {
	for _, entry := range entries {
		fmt.Fprintf(
			writer,
			"[agent-factory:pkg-boundary] stale service construction baseline entry: %s -> %s.%s [class=%s]\n",
			entry.FilePath,
			entry.ImportPath,
			entry.Symbol,
			func() boundarySourceClass {
				class, _ := sourceClassFromBaseline(entry.Class, entry.FilePath)
				return class
			}(),
		)
		fmt.Fprintln(writer, "  reason: the recorded construction selection no longer exists.")
		fmt.Fprintf(writer, "  remediation: remove this entry from %s in the same change.\n", serviceConstructionBaselinePath)
	}
}

func writeServiceConstructionBaselineSummary(writer io.Writer, count int) {
	if count == 0 {
		return
	}
	fmt.Fprintf(
		writer,
		"[agent-factory:pkg-boundary] active product-service construction migration baseline: %d exact file/symbol edge(s)\n",
		count,
	)
	fmt.Fprintln(writer, "  deletion gate: inject each service role from pkg/wire or move the invariant to its owning service, then delete the exact baseline entry.")
}
