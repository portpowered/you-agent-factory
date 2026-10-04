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
	for _, finding := range findings.serviceConstructionFindings {
		counts.add(effectiveBoundarySourceClass(finding.class, finding.filePath))
	}
	for _, finding := range findings.externalImplementationFindings {
		counts.add(effectiveBoundarySourceClass(finding.class, finding.filePath))
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
	result.serviceConstructionFindings = filterServiceConstructionFindingsByClass(findings.serviceConstructionFindings, testOnlySourceClass)
	result.externalImplementationFindings = filterTransportImplementationFindingsByClass(findings.externalImplementationFindings, testOnlySourceClass)
	result.staleServiceConstructionEntries = filterServiceConstructionBaselineEntriesByClass(findings.staleServiceConstructionEntries, testOnlySourceClass)
	return result
}

func filterServiceConstructionFindingsByClass(findings []serviceConstructionFinding, want boundarySourceClass) []serviceConstructionFinding {
	return filterByClass(findings, want, func(finding serviceConstructionFinding) boundarySourceClass { return finding.class }, func(finding serviceConstructionFinding) string { return finding.filePath })
}

func filterTransportImplementationFindingsByClass(findings []transportServiceImplementationFinding, want boundarySourceClass) []transportServiceImplementationFinding {
	return filterByClass(findings, want, func(finding transportServiceImplementationFinding) boundarySourceClass { return finding.class }, func(finding transportServiceImplementationFinding) string { return finding.filePath })
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

func bytesContainGeneratedMarker(content []byte) bool {
	const marker = "Code generated "
	const suffix = " DO NOT EDIT."
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "package ") {
			return false
		}
		if strings.HasPrefix(trimmed, "// "+marker) && strings.Contains(trimmed, suffix) {
			return true
		}
	}
	return false
}
