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
	writeRetiredPackageImportFindings(writer, findings.retiredPackageImportFindings)
	writeMigrationShimBlockingFindings(writer, findings.migrationShimFindings)
	writeApplicationGraphImportFindings(writer, findings.applicationGraphImportFindings)
	writeHandwrittenGeneratedFindings(writer, findings.handwrittenGeneratedFindings)
	writeDomainTransportImportFindings(writer, findings.domainTransportFindings)
	writePeerServiceImportFindings(writer, findings.peerServiceImportFindings)
	writePeerServiceImportFindings(writer, findings.recordedPeerServiceImportFindings)
	writeStalePeerServiceBaselineEntries(writer, findings.stalePeerServiceBaselineEntries)
	writeTestServiceImportFindings(writer, findings.testServiceImportFindings)
	writeTestServiceImportFindings(writer, findings.recordedTestServiceImportFindings)
	writeStaleTestServiceBaselineEntries(writer, findings.staleTestServiceBaselineEntries)
	writeSupportServiceImportFindings(writer, findings.supportServiceImportFindings)
	writeSupportServiceImportFindings(writer, findings.recordedSupportServiceImportFindings)
	writeStaleSupportServiceBaselineEntries(writer, findings.staleSupportServiceBaselineEntries)
	writeServiceConstructionFindings(writer, findings.serviceConstructionFindings)
	writeServiceConstructionFindings(writer, findings.recordedServiceConstructionFindings)
	writeStaleServiceConstructionBaselineEntries(writer, findings.staleServiceConstructionEntries)
	writeTransportServiceImplementationFindings(writer, findings.transportImplementationFindings)
	writeExternalServiceImplementationFindings(writer, findings.externalImplementationFindings)
	writeTransportBehaviorFindings(writer, findings.transportBehaviorFindings)
	writeTransportBehaviorFindings(writer, findings.recordedTransportBehaviorFindings)
	writeStaleTransportBehaviorBaselineEntries(writer, findings.staleTransportBehaviorEntries)
	writeFunctionalProcessEdgeFindings(writer, findings.functionalProcessEdgeFindings)
	writeConstructedServiceEdgesFindings(writer, findings.constructedServiceEdgesFindings)
	writeTestWorkNormalizationFindings(writer, findings.testWorkNormalizationFindings)
	writeProductionDefaultFindings(writer, findings.productionDefaultFindings)
	writeProductionDefaultFindings(writer, findings.recordedProductionDefaultFindings)
	writeStaleProductionDefaultBaselineEntries(writer, findings.staleProductionDefaultEntries)
	writeInitializerBehaviorFindings(writer, findings.initializerBehaviorFindings)
	writeInitializerBehaviorFindings(writer, findings.recordedInitializerBehaviorFindings)
	writeStaleInitializerBehaviorBaselineEntries(writer, findings.staleInitializerBehaviorEntries)
	writeTestBehaviorFindings(writer, findings.testBehaviorFindings)
	writeTestBehaviorFindings(writer, findings.recordedTestBehaviorFindings)
	writeStaleTestBehaviorBaselineEntries(writer, findings.staleTestBehaviorEntries)
	writePetriPublicSurfaceFindings(writer, findings.petriPublicSurfaceFindings)
	writePetriPublicSurfaceFindings(writer, findings.recordedPetriPublicSurfaceFindings)
	writeStalePetriPublicSurfaceBaselineEntries(writer, findings.stalePetriPublicSurfaceEntries)
	writeProviderEffectOwnershipFindings(writer, findings.providerEffectOwnershipFindings)
}

func writeBaselineSummaries(writer io.Writer, findings scanResult) {
	writePeerServiceBaselineSummary(writer, findings.peerServiceBaselineCount)
	writeTestServiceBaselineSummary(writer, findings.testServiceBaselineCount)
	writeSupportServiceBaselineSummary(writer, findings.supportServiceBaselineCount)
	writeServiceConstructionBaselineSummary(writer, findings.serviceConstructionBaselineCount)
	writeTransportBehaviorBaselineSummary(writer, findings.transportBehaviorBaselineCount)
	writeProductionDefaultBaselineSummary(writer, findings.productionDefaultBaselineCount)
	writeInitializerBehaviorBaselineSummary(writer, findings.initializerBehaviorBaselineCount)
	writeTestBehaviorBaselineSummary(writer, findings.testBehaviorBaselineCount)
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

func writeRetiredPackageImportFindings(writer io.Writer, findings []retiredPackageImportFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited retired package import: %s (%s) [class=%s]\n", finding.importPath, finding.filePath, effectiveBoundarySourceClass(finding.class, finding.filePath))
		fmt.Fprintf(writer, "  canonical owner: %s\n", finding.canonicalOwner)
		fmt.Fprintf(writer, "  remediation: import %s directly; do not recreate or depend on %s.\n", finding.canonicalOwner, finding.packagePath)
	}
}

func generatedCodeExceptionDescriptions(policy boundaryPolicy) []string {
	descriptions := make([]string, 0, len(policy.generatedCodeExceptions))
	for _, exception := range policy.generatedCodeExceptions {
		descriptions = append(descriptions, fmt.Sprintf("%s (%s)", filepath.ToSlash(exception.packagePath), exception.scope))
	}
	return descriptions
}

func writeMigrationShimBlockingFindings(writer io.Writer, findings []migrationShimFinding) {
	if len(findings) == 0 {
		return
	}

	for _, finding := range findings {
		canonicalTarget := finding.canonicalTarget
		if canonicalTarget == "" {
			canonicalTarget = "not detected"
		}
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] blocked migration-only compatibility shim: %s\n", finding.packagePath)
		fmt.Fprintf(writer, "  marker: %s\n", finding.marker)
		fmt.Fprintf(writer, "  canonical target: %s\n", canonicalTarget)
		fmt.Fprintln(writer, "  remediation: import the canonical owner directly and do not recreate Batch 001 root compatibility shims.")
	}
}

func writeApplicationGraphImportFindings(writer io.Writer, findings []applicationGraphImportFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited application composition import: %s (%s) [class=%s]\n", finding.packagePath, finding.filePath, effectiveBoundarySourceClass(finding.class, finding.filePath))
		fmt.Fprintln(writer, "  reason: pkg/wire is the outward application composition root and must not be imported by domain or transport packages.")
		fmt.Fprintln(writer, "  remediation: depend on a narrow domain-owned contract and inject the collaborator through pkg/root or pkg/initializer.")
	}
}

func writeDomainTransportImportFindings(writer io.Writer, findings []domainTransportImportFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited domain transport import: %s (%s) [class=%s]\n", finding.importPath, finding.filePath, effectiveBoundarySourceClass(finding.class, finding.filePath))
		fmt.Fprintf(writer, "  domain owner: %s\n", finding.packagePath)
		fmt.Fprintln(writer, "  reason: protected domain packages must not consume transport contracts or adapters.")
		fmt.Fprintln(writer, "  remediation: define the input at its domain owner and map generated values under pkg/transports/mapping.")
	}
}

func writePeerServiceImportFindings(writer io.Writer, findings []peerServiceImportFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited peer service subpackage import: %s (%s) [class=%s]\n", finding.importPath, finding.filePath, effectiveBoundarySourceClass(finding.class, finding.filePath))
		fmt.Fprintf(writer, "  service owner: pkg/services/%s; peer owner: pkg/services/%s\n", finding.owner, finding.peer)
		fmt.Fprintf(writer, "  remediation: publish the required value or capability at pkg/services/%s and import only that peer root.\n", finding.peer)
	}
}

func writeStalePeerServiceBaselineEntries(writer io.Writer, entries []peerServiceImportBaselineEntry) {
	for _, entry := range entries {
		fmt.Fprintf(
			writer,
			"[agent-factory:pkg-boundary] stale peer service import baseline entry: %s -> %s [class=%s]\n",
			entry.FilePath,
			entry.ImportPath,
			func() boundarySourceClass {
				class, _ := sourceClassFromBaseline(entry.Class, entry.FilePath)
				return class
			}(),
		)
		fmt.Fprintln(writer, "  reason: the recorded bypass edge no longer exists.")
		fmt.Fprintln(writer, "  remediation: remove this entry from service-cross-import-baseline.json in the same change.")
	}
}

func writePeerServiceBaselineSummary(writer io.Writer, count int) {
	if count == 0 {
		return
	}
	fmt.Fprintf(
		writer,
		"[agent-factory:pkg-boundary] active peer-service root-contract migration baseline: %d edge(s)\n",
		count,
	)
	fmt.Fprintln(writer, "  deletion gate: migrate every edge to an exact pkg/services/<peer> root import, then delete the baseline.")
}

func writeTestServiceImportFindings(writer io.Writer, findings []testServiceImportFinding) {
	for _, finding := range findings {
		fmt.Fprintf(
			writer,
			"[agent-factory:pkg-boundary] prohibited test import of service internals: %s (%s) [class=%s]\n",
			finding.importPath,
			finding.filePath,
			effectiveBoundarySourceClass(finding.class, finding.filePath),
		)
		fmt.Fprintf(writer, "  service owner: pkg/services/%s\n", finding.owner)
		fmt.Fprintln(writer, "  remediation: use the service root contract, move the invariant to the owning service, or exercise cross-service behavior through root.BuildProcess.")
	}
}

func writeStaleTestServiceBaselineEntries(writer io.Writer, entries []testServiceImportBaselineEntry) {
	for _, entry := range entries {
		fmt.Fprintf(
			writer,
			"[agent-factory:pkg-boundary] stale test service import baseline entry: %s -> %s [class=%s]\n",
			entry.FilePath,
			entry.ImportPath,
			func() boundarySourceClass {
				class, _ := sourceClassFromBaseline(entry.Class, entry.FilePath)
				return class
			}(),
		)
		fmt.Fprintln(writer, "  reason: the concrete cross-owner test import no longer exists.")
		fmt.Fprintf(writer, "  remediation: remove this entry from %s in the same change.\n", testServiceImportBaselinePath)
	}
}

func writeTestServiceBaselineSummary(writer io.Writer, count int) {
	if count == 0 {
		return
	}
	fmt.Fprintf(
		writer,
		"[agent-factory:pkg-boundary] active test service-internal migration baseline: %d edge(s)\n",
		count,
	)
	fmt.Fprintln(writer, "  deletion gate: move each invariant to its service owner, use a service-root fake, or enter through root.BuildProcess; then delete the exact baseline entry.")
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

func writeTransportServiceImplementationFindings(writer io.Writer, findings []transportServiceImplementationFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited transport service implementation import: %s (%s) [class=%s]\n", finding.importPath, finding.filePath, effectiveBoundarySourceClass(finding.class, finding.filePath))
		fmt.Fprintln(writer, "  reason: transports may consume only service root contracts or explicitly public service subservices.")
		fmt.Fprintln(writer, "  remediation: publish the required capability at its service boundary and keep representation mapping in the transport.")
	}
}

func writeExternalServiceImplementationFindings(writer io.Writer, findings []transportServiceImplementationFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited external service subpackage import: %s (%s) [class=%s]\n", finding.importPath, finding.filePath, effectiveBoundarySourceClass(finding.class, finding.filePath))
		fmt.Fprintln(writer, "  reason: service subpackages are owner-internal for ordinary consumers; pkg/wire is the unrestricted composition-root exception.")
		fmt.Fprintln(writer, "  remediation: import the exact pkg/services/<service-name> root and use its published contract.")
	}
}

func writeHandwrittenGeneratedFindings(writer io.Writer, findings []handwrittenGeneratedFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] handwritten Go file in generated-only package: %s (%s)\n", finding.packagePath, finding.filePath)
		fmt.Fprintln(writer, "  reason: generated-only packages may contain only files with the standard Code generated ... DO NOT EDIT. marker.")
		fmt.Fprintln(writer, "  remediation: move handwritten mapping or policy to pkg/transports/http or pkg/transports/mapping.")
	}
}
