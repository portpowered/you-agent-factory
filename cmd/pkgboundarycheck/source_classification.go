package main

import (
	"fmt"
	"io"
	"strings"
)

// boundarySourceClass identifies the kind of Go source that produced a
// dependency observation. Keep it on the finding so production and test-only
// edges remain distinct even when they have the same target.
type boundarySourceClass string

const (
	productionSourceClass boundarySourceClass = "production"
	testOnlySourceClass   boundarySourceClass = "test-only"
)

func classifyBoundarySource(filePath string) boundarySourceClass {
	if strings.HasSuffix(filePath, "_test.go") {
		return testOnlySourceClass
	}
	return productionSourceClass
}

func (class boundarySourceClass) valid() bool {
	return class == productionSourceClass || class == testOnlySourceClass
}

func effectiveBoundarySourceClass(class boundarySourceClass, filePath string) boundarySourceClass {
	if class.valid() {
		return class
	}
	return classifyBoundarySource(filePath)
}

// sourceClassFromBaseline keeps old exact production entries readable while
// allowing new class-bearing entries to distinguish otherwise identical edges.
// The source filename remains authoritative when a class is present.
func sourceClassFromBaseline(value string, filePath string) (boundarySourceClass, error) {
	expected := classifyBoundarySource(filePath)
	if strings.TrimSpace(value) == "" {
		return expected, nil
	}
	class := boundarySourceClass(value)
	if !class.valid() {
		return "", fmt.Errorf("baseline entry %s has invalid class %q", filePath, value)
	}
	if class != expected {
		return "", fmt.Errorf("baseline entry %s class = %q, want %q", filePath, class, expected)
	}
	return class, nil
}

type classifiedDependencyViolationCounts struct {
	production int
	testOnly   int
}

func (counts *classifiedDependencyViolationCounts) add(class boundarySourceClass) {
	switch class {
	case productionSourceClass:
		counts.production++
	case testOnlySourceClass:
		counts.testOnly++
	}
}

func countClassifiedDependencyViolations(findings scanResult) classifiedDependencyViolationCounts {
	var counts classifiedDependencyViolationCounts
	for _, finding := range findings.retiredPackageImportFindings {
		counts.add(effectiveBoundarySourceClass(finding.class, finding.filePath))
	}
	for _, finding := range findings.applicationGraphImportFindings {
		counts.add(effectiveBoundarySourceClass(finding.class, finding.filePath))
	}
	for _, finding := range findings.domainTransportFindings {
		counts.add(effectiveBoundarySourceClass(finding.class, finding.filePath))
	}
	for _, finding := range findings.peerServiceImportFindings {
		counts.add(effectiveBoundarySourceClass(finding.class, finding.filePath))
	}
	for _, finding := range findings.testServiceImportFindings {
		counts.add(effectiveBoundarySourceClass(finding.class, finding.filePath))
	}
	for _, finding := range findings.supportServiceImportFindings {
		counts.add(effectiveBoundarySourceClass(finding.class, finding.filePath))
	}
	for _, finding := range findings.serviceConstructionFindings {
		counts.add(effectiveBoundarySourceClass(finding.class, finding.filePath))
	}
	for _, finding := range findings.transportImplementationFindings {
		counts.add(effectiveBoundarySourceClass(finding.class, finding.filePath))
	}
	for _, finding := range findings.externalImplementationFindings {
		counts.add(effectiveBoundarySourceClass(finding.class, finding.filePath))
	}
	for _, entry := range findings.stalePeerServiceBaselineEntries {
		class, err := sourceClassFromBaseline(entry.Class, entry.FilePath)
		if err == nil {
			counts.add(class)
		}
	}
	for _, entry := range findings.staleTestServiceBaselineEntries {
		class, err := sourceClassFromBaseline(entry.Class, entry.FilePath)
		if err == nil {
			counts.add(class)
		}
	}
	for _, entry := range findings.staleSupportServiceBaselineEntries {
		class, err := sourceClassFromBaseline(entry.Class, entry.FilePath)
		if err == nil {
			counts.add(class)
		}
	}
	for _, entry := range findings.staleServiceConstructionEntries {
		class, err := sourceClassFromBaseline(entry.Class, entry.FilePath)
		if err == nil {
			counts.add(class)
		}
	}
	return counts
}

func writeClassifiedDependencyViolationCounts(writer io.Writer, counts classifiedDependencyViolationCounts) {
	fmt.Fprintf(
		writer,
		"[agent-factory:pkg-boundary] dependency violation counts: production=%d test-only=%d\n",
		counts.production,
		counts.testOnly,
	)
}

func countProductionBoundaryFindings[T any](findings []T, class func(T) boundarySourceClass, path func(T) string) int {
	count := 0
	for _, finding := range findings {
		if effectiveBoundarySourceClass(class(finding), path(finding)) == productionSourceClass {
			count++
		}
	}
	return count
}

func testOnlyDependencyFindings(findings scanResult) scanResult {
	result := scanResult{}
	result.retiredPackageImportFindings = filterRetiredPackageImportsByClass(findings.retiredPackageImportFindings, testOnlySourceClass)
	result.applicationGraphImportFindings = filterApplicationGraphImportsByClass(findings.applicationGraphImportFindings, testOnlySourceClass)
	result.domainTransportFindings = filterDomainTransportFindingsByClass(findings.domainTransportFindings, testOnlySourceClass)
	result.peerServiceImportFindings = filterPeerServiceImportsByClass(findings.peerServiceImportFindings, testOnlySourceClass)
	result.testServiceImportFindings = filterTestServiceImportsByClass(findings.testServiceImportFindings, testOnlySourceClass)
	result.supportServiceImportFindings = filterSupportServiceImportsByClass(findings.supportServiceImportFindings, testOnlySourceClass)
	result.serviceConstructionFindings = filterServiceConstructionFindingsByClass(findings.serviceConstructionFindings, testOnlySourceClass)
	result.transportImplementationFindings = filterTransportImplementationFindingsByClass(findings.transportImplementationFindings, testOnlySourceClass)
	result.externalImplementationFindings = filterTransportImplementationFindingsByClass(findings.externalImplementationFindings, testOnlySourceClass)
	result.stalePeerServiceBaselineEntries = filterPeerServiceBaselineEntriesByClass(findings.stalePeerServiceBaselineEntries, testOnlySourceClass)
	result.staleTestServiceBaselineEntries = filterTestServiceBaselineEntriesByClass(findings.staleTestServiceBaselineEntries, testOnlySourceClass)
	result.staleSupportServiceBaselineEntries = filterSupportServiceBaselineEntriesByClass(findings.staleSupportServiceBaselineEntries, testOnlySourceClass)
	result.staleServiceConstructionEntries = filterServiceConstructionBaselineEntriesByClass(findings.staleServiceConstructionEntries, testOnlySourceClass)
	return result
}

func filterRetiredPackageImportsByClass(findings []retiredPackageImportFinding, want boundarySourceClass) []retiredPackageImportFinding {
	return filterByClass(findings, want, func(finding retiredPackageImportFinding) boundarySourceClass { return finding.class }, func(finding retiredPackageImportFinding) string { return finding.filePath })
}

func filterApplicationGraphImportsByClass(findings []applicationGraphImportFinding, want boundarySourceClass) []applicationGraphImportFinding {
	return filterByClass(findings, want, func(finding applicationGraphImportFinding) boundarySourceClass { return finding.class }, func(finding applicationGraphImportFinding) string { return finding.filePath })
}

func filterDomainTransportFindingsByClass(findings []domainTransportImportFinding, want boundarySourceClass) []domainTransportImportFinding {
	return filterByClass(findings, want, func(finding domainTransportImportFinding) boundarySourceClass { return finding.class }, func(finding domainTransportImportFinding) string { return finding.filePath })
}

func filterPeerServiceImportsByClass(findings []peerServiceImportFinding, want boundarySourceClass) []peerServiceImportFinding {
	return filterByClass(findings, want, func(finding peerServiceImportFinding) boundarySourceClass { return finding.class }, func(finding peerServiceImportFinding) string { return finding.filePath })
}

func filterTestServiceImportsByClass(findings []testServiceImportFinding, want boundarySourceClass) []testServiceImportFinding {
	return filterByClass(findings, want, func(finding testServiceImportFinding) boundarySourceClass { return finding.class }, func(finding testServiceImportFinding) string { return finding.filePath })
}

func filterSupportServiceImportsByClass(findings []supportServiceImportFinding, want boundarySourceClass) []supportServiceImportFinding {
	return filterByClass(findings, want, func(finding supportServiceImportFinding) boundarySourceClass { return finding.class }, func(finding supportServiceImportFinding) string { return finding.filePath })
}

func filterServiceConstructionFindingsByClass(findings []serviceConstructionFinding, want boundarySourceClass) []serviceConstructionFinding {
	return filterByClass(findings, want, func(finding serviceConstructionFinding) boundarySourceClass { return finding.class }, func(finding serviceConstructionFinding) string { return finding.filePath })
}

func filterTransportImplementationFindingsByClass(findings []transportServiceImplementationFinding, want boundarySourceClass) []transportServiceImplementationFinding {
	return filterByClass(findings, want, func(finding transportServiceImplementationFinding) boundarySourceClass { return finding.class }, func(finding transportServiceImplementationFinding) string { return finding.filePath })
}

func filterPeerServiceBaselineEntriesByClass(findings []peerServiceImportBaselineEntry, want boundarySourceClass) []peerServiceImportBaselineEntry {
	return filterByClass(findings, want, func(finding peerServiceImportBaselineEntry) boundarySourceClass {
		class, _ := sourceClassFromBaseline(finding.Class, finding.FilePath)
		return class
	}, func(finding peerServiceImportBaselineEntry) string { return finding.FilePath })
}

func filterTestServiceBaselineEntriesByClass(findings []testServiceImportBaselineEntry, want boundarySourceClass) []testServiceImportBaselineEntry {
	return filterByClass(findings, want, func(finding testServiceImportBaselineEntry) boundarySourceClass {
		class, _ := sourceClassFromBaseline(finding.Class, finding.FilePath)
		return class
	}, func(finding testServiceImportBaselineEntry) string { return finding.FilePath })
}

func filterSupportServiceBaselineEntriesByClass(findings []supportServiceImportBaselineEntry, want boundarySourceClass) []supportServiceImportBaselineEntry {
	return filterByClass(findings, want, func(finding supportServiceImportBaselineEntry) boundarySourceClass {
		class, _ := sourceClassFromBaseline(finding.Class, finding.FilePath)
		return class
	}, func(finding supportServiceImportBaselineEntry) string { return finding.FilePath })
}

func filterServiceConstructionBaselineEntriesByClass(findings []serviceConstructionBaselineEntry, want boundarySourceClass) []serviceConstructionBaselineEntry {
	return filterByClass(findings, want, func(finding serviceConstructionBaselineEntry) boundarySourceClass {
		class, _ := sourceClassFromBaseline(finding.Class, finding.FilePath)
		return class
	}, func(finding serviceConstructionBaselineEntry) string { return finding.FilePath })
}

func filterByClass[T any](findings []T, want boundarySourceClass, class func(T) boundarySourceClass, path func(T) string) []T {
	filtered := make([]T, 0, len(findings))
	for _, finding := range findings {
		if effectiveBoundarySourceClass(class(finding), path(finding)) == want {
			filtered = append(filtered, finding)
		}
	}
	return filtered
}
