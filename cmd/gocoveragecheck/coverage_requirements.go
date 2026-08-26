package main

import (
	"path"
	"slices"
)

// coverageRequirementApplies reports whether a package contributes to the
// active lane's aggregate and package-local coverage requirements. Wire
// packages are construction-only boundaries: unit tests may still execute
// them, but the unit lane does not require statement coverage from them.
func coverageRequirementApplies(lane string, importPath string) bool {
	return lane == functionalCoverageSuite || path.Base(importPath) != "wire"
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
