package main

import (
	"fmt"
	"slices"
	"strings"
)

// validatePackageCoverage proves the surviving migration rows are a clean set:
// at most one row per package path, in a deterministic stable sort.
//
// It no longer requires one row per live package. The ledger tracks only
// unfinished migration intent, so the absence of a row is the normal state for
// the overwhelming majority of packages.
func validatePackageCoverage(moves UnfinishedMoves) error {
	seen := make(map[string]struct{}, len(moves.Moves))
	for _, row := range moves.Moves {
		packagePath := row.PackagePath
		if _, dup := seen[packagePath]; dup {
			return fmt.Errorf("moves contains duplicate package path %q", packagePath)
		}
		seen[packagePath] = struct{}{}
	}

	if !slices.IsSortedFunc(moves.Moves, func(a, b PackageMapping) int {
		return strings.Compare(a.PackagePath, b.PackagePath)
	}) {
		return fmt.Errorf("moves must be stable-sorted by packagePath")
	}
	return nil
}
