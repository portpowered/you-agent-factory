package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

func writeBoundaryFindings(writer io.Writer, findings scanResult) {
	for _, finding := range findings.rootPackageFindings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] unapproved root package family: %s\n", finding.packagePath)
		fmt.Fprintf(writer, "  reason: %s is outside the approved package-family allowlist.\n", finding.packagePath)
		fmt.Fprintln(writer, "  remediation: move the code under an approved owner or deliberately update the allowlist with ownership rationale.")
	}
	writeRetiredPackageRootFindings(writer, findings.retiredPackageRootFindings)
	writeServiceConstructionFindings(writer, findings.serviceConstructionFindings)
	writeServiceConstructionFindings(writer, findings.recordedServiceConstructionFindings)
	writeStaleServiceConstructionBaselineEntries(writer, findings.staleServiceConstructionEntries)
	writeExternalServiceImplementationFindings(writer, findings.externalImplementationFindings)
	writeProductionDefaultFindings(writer, findings.productionDefaultFindings)
	writeProductionDefaultFindings(writer, findings.recordedProductionDefaultFindings)
	writeStaleProductionDefaultBaselineEntries(writer, findings.staleProductionDefaultEntries)
	writePetriPublicSurfaceFindings(writer, findings.petriPublicSurfaceFindings)
	writePetriPublicSurfaceFindings(writer, findings.recordedPetriPublicSurfaceFindings)
	writeStalePetriPublicSurfaceBaselineEntries(writer, findings.stalePetriPublicSurfaceEntries)
}

func writeBaselineSummaries(writer io.Writer, findings scanResult) {
	writeServiceConstructionBaselineSummary(writer, findings.serviceConstructionBaselineCount)
	writeProductionDefaultBaselineSummary(writer, findings.productionDefaultBaselineCount)
	writePetriPublicSurfaceBaselineSummary(writer, findings.petriPublicSurfaceBaselineCount)
}

func writeGeneratedCodeExceptionSummary(writer io.Writer, policy boundaryPolicy) {
	exceptions := generatedCodeExceptionDescriptions(policy)
	if len(exceptions) == 0 {
		return
	}
	fmt.Fprintf(writer, "[agent-factory:pkg-boundary] active generated-code exceptions: %s\n", strings.Join(exceptions, ", "))
}

func writeRetiredPackageRootFindings(writer io.Writer, findings []retiredPackageRootFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited retired package root: %s\n", finding.packagePath)
		fmt.Fprintf(writer, "  canonical owner: %s\n", finding.canonicalOwner)
		fmt.Fprintf(writer, "  remediation: move the code to %s and delete the retired root.\n", finding.canonicalOwner)
	}
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

func writeExternalServiceImplementationFindings(writer io.Writer, findings []transportServiceImplementationFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited external service subpackage import: %s (%s) [class=%s]\n", finding.importPath, finding.filePath, effectiveBoundarySourceClass(finding.class, finding.filePath))
		fmt.Fprintln(writer, "  reason: service subpackages are owner-internal for ordinary consumers; pkg/wire is the unrestricted composition-root exception.")
		fmt.Fprintln(writer, "  remediation: import the exact pkg/services/<service-name> root and use its published contract.")
	}
}
