package main

import (
	"fmt"
	"io"
	"strings"
)

// boundarySourceClass identifies whether a Go dependency observation came
// from production code or from a test-only source file. The class is part of
// the finding identity so the same boundary relationship can be ratcheted
// independently in each source class.
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

func baselineSourceClass(entry baselineEntry) (boundarySourceClass, error) {
	expected := classifyBoundarySource(entry.FilePath)
	if strings.TrimSpace(entry.Class) == "" {
		if expected == testOnlySourceClass {
			return "", fmt.Errorf(
				"%s entry %s requires an explicit class=%q",
				baselineFile,
				entry.Target,
				testOnlySourceClass,
			)
		}
		return productionSourceClass, nil
	}
	class := boundarySourceClass(entry.Class)
	if !class.valid() {
		return "", fmt.Errorf(
			"%s entry %s has invalid class %q; want %q or %q",
			baselineFile,
			entry.Target,
			entry.Class,
			productionSourceClass,
			testOnlySourceClass,
		)
	}
	if class != expected {
		return "", fmt.Errorf(
			"%s entry %s class = %q does not match source file %s (%q)",
			baselineFile,
			entry.Target,
			entry.Class,
			entry.FilePath,
			expected,
		)
	}
	return class, nil
}

type classifiedViolationCounts struct {
	production int
	testOnly   int
}

func (counts *classifiedViolationCounts) add(class boundarySourceClass) {
	switch class {
	case productionSourceClass:
		counts.production++
	case testOnlySourceClass:
		counts.testOnly++
	}
}

func countClassifiedViolations(unrecorded []finding, stale []baselineEntry) classifiedViolationCounts {
	var counts classifiedViolationCounts
	for _, item := range unrecorded {
		counts.add(item.Class)
	}
	for _, item := range stale {
		class, err := baselineSourceClass(item)
		if err == nil {
			counts.add(class)
		}
	}
	return counts
}

func writeClassifiedViolationCounts(writer io.Writer, counts classifiedViolationCounts) {
	fmt.Fprintf(
		writer,
		"[agent-factory:ownership-boundary] violation counts: production=%d test-only=%d\n",
		counts.production,
		counts.testOnly,
	)
}
