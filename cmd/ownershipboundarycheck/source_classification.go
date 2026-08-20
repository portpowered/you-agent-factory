package main

import (
	"fmt"
	"io"
	"strings"
)

// boundarySourceClass identifies the kind of Go source that produced a
// boundary observation. Keeping it on the finding prevents production and
// test-only edges from collapsing into one report or baseline key.
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

// sourceClassFromBaseline keeps the existing production baseline format
// readable while allowing new entries to carry an explicit class. The source
// filename remains authoritative so a malformed class cannot relabel an edge.
func sourceClassFromBaseline(value, filePath string) (boundarySourceClass, error) {
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

type classifiedFindingCounts struct {
	production int
	testOnly   int
}

func (counts *classifiedFindingCounts) add(class boundarySourceClass) {
	switch class {
	case productionSourceClass:
		counts.production++
	case testOnlySourceClass:
		counts.testOnly++
	}
}

func countFindingClasses(findings ...[]finding) classifiedFindingCounts {
	var counts classifiedFindingCounts
	for _, group := range findings {
		for _, item := range group {
			counts.add(effectiveBoundarySourceClass(item.class, item.FilePath))
		}
	}
	return counts
}

func writeClassifiedFindingCounts(writer io.Writer, counts classifiedFindingCounts) {
	fmt.Fprintf(
		writer,
		"[agent-factory:ownership-boundary] dependency violation counts: production=%d test-only=%d\n",
		counts.production,
		counts.testOnly,
	)
}

func filterFindingsByClass(findings []finding, want boundarySourceClass) []finding {
	filtered := make([]finding, 0, len(findings))
	for _, item := range findings {
		if effectiveBoundarySourceClass(item.class, item.FilePath) == want {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func productionBaseline(recorded baseline) (baseline, error) {
	result := baseline{Version: recorded.Version}
	for _, entry := range recorded.Entries {
		class, err := sourceClassFromBaseline(entry.Class, entry.FilePath)
		if err != nil {
			return baseline{}, err
		}
		if class == productionSourceClass {
			result.Entries = append(result.Entries, entry)
		}
	}
	return result, nil
}
