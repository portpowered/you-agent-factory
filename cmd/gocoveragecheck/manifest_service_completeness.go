package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// coverageServiceRootPrefix is the import-path prefix under which every product
// service lives. Completeness is checked once per service beneath it.
const coverageServiceRootPrefix = modulePath + "/pkg/services/"

// validateCoverageManifestDefaultFloor rejects a manifest whose optional
// defaultFloorPercent is present but is not a two-decimal percentage.
func validateCoverageManifestDefaultFloor(manifest coverageManifest) error {
	raw := manifest.DefaultFloorPercent
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if _, err := parseCoverageFloor(raw); err != nil {
		return fmt.Errorf("validate go coverage manifest: defaultFloorPercent: %w", err)
	}
	return nil
}

// coverageManifestServiceRoot reports the pkg/services/<name> root import path
// that governs importPath, and whether importPath belongs to a service at all.
// The service root governs itself.
func coverageManifestServiceRoot(importPath string) (string, bool) {
	rest, ok := strings.CutPrefix(importPath, coverageServiceRootPrefix)
	if !ok {
		return "", false
	}
	name := rest
	if head, _, found := strings.Cut(rest, "/"); found {
		name = head
	}
	if name == "" {
		return "", false
	}
	return coverageServiceRootPrefix + name, true
}

// validateCoverageManifestServiceRoots enforces per-service completeness: every
// service with a measured package must declare a floor at its own root. A
// package inside an already-declared service needs no entry of its own, and an
// undeclared service is reported once rather than once per package under it.
func validateCoverageManifestServiceRoots(
	lane string,
	measuredPackages []string,
	declared map[string]struct{},
	measuredTotals map[string]packageCoverageTotals,
) error {
	missing := make(map[string]struct{})
	for _, importPath := range measuredPackages {
		root, ok := coverageManifestServiceRoot(importPath)
		if !ok {
			continue
		}
		if _, declaredRoot := declared[root]; declaredRoot {
			continue
		}
		missing[root] = struct{}{}
	}
	if len(missing) == 0 {
		return nil
	}
	return formatMissingCoverageManifestServiceRoots(lane, slices.Sorted(maps.Keys(missing)), measuredTotals)
}

func formatMissingCoverageManifestServiceRoots(lane string, missing []string, measuredTotals map[string]packageCoverageTotals) error {
	lines := make([]string, 0, len(missing))
	for _, root := range missing {
		line := fmt.Sprintf(
			"- measured %s service %q has no root manifest entry; record one entry for the service root",
			lane, root,
		)
		if measuredTotals != nil {
			totals := measuredTotals[root]
			line += fmt.Sprintf("; the root package measures %d/%d statements", totals.coveredStatements, totals.totalStatements)
		}
		lines = append(lines, line)
	}
	return fmt.Errorf(
		"validate go coverage manifest: measured %s services have no root manifest entry:\n%s",
		lane, strings.Join(lines, "\n"),
	)
}

// isCoverageManifestServiceRoot reports whether importPath is a service root.
func isCoverageManifestServiceRoot(importPath string) bool {
	root, ok := coverageManifestServiceRoot(importPath)
	return ok && root == importPath
}

// coverageManifestServiceRootEntry builds the root entry a service declares
// when its root package carries no measurable statements of its own. The lane
// default is recorded explicitly so the row states the floor it applies.
func coverageManifestServiceRootEntry(lane string, importPath string) coverageManifestEntry {
	return coverageManifestEntry{
		Package: importPath,
		Minimum: laneDefaultCoverageFloorRaw(lane),
	}
}
