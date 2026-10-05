package main

import (
	"slices"
	"strings"
)

// coverageRequirementApplies reports whether a package contributes to the
// active lane's aggregate and package-local coverage requirements. Wire
// packages are composition boundaries with no unit tests or unit coverage
// requirements, including private implementation descendants during migration.
func coverageRequirementApplies(lane string, importPath string) bool {
	return lane == functionalCoverageSuite || !slices.Contains(strings.Split(importPath, "/"), "wire")
}

func filterCoverageRequirementPackages(lane string, packages []string) []string {
	filtered := make([]string, 0, len(packages))
	for _, importPath := range packages {
		if coverageRequirementApplies(lane, importPath) {
			filtered = append(filtered, importPath)
		}
	}
	slices.Sort(filtered)
	return filtered
}
