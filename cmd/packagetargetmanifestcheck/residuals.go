package main

import (
	"fmt"
	"slices"
	"strings"
)

func nonServiceFamilySet() map[string]struct{} {
	families := closedDestinationVocabulary().NonServiceFamilies
	set := make(map[string]struct{}, len(families))
	for _, name := range families {
		set[name] = struct{}{}
	}
	return set
}

// mapResidualPackage maps approved non-service family packages and any remaining
// production residuals that are not committed owners or the Edges exception.
// Unknown residuals are queued for deletion with a named successor so discovering
// them does not reopen or extend the top-level 13-owner tree.
func mapResidualPackage(packagePath string) (PackageMapping, bool) {
	packagePath = strings.TrimSpace(packagePath)
	if packagePath == "" {
		return PackageMapping{}, false
	}
	if _, ok := mapCommittedOwnerPackage(packagePath); ok {
		return PackageMapping{}, false
	}
	if _, ok := mapEdgesExceptionPackage(packagePath); ok {
		return PackageMapping{}, false
	}

	if family, ok := matchNonServiceFamily(packagePath); ok {
		return PackageMapping{
			PackagePath: packagePath,
			Disposition: DispositionRetain,
			Destination: family,
		}, true
	}

	return PackageMapping{
		PackagePath:       packagePath,
		Disposition:       DispositionDelete,
		Destination:       "system_initialization",
		DeletionSuccessor: "pkg/services/system_initialization",
		DeletionCondition: "delete after IMP-system_initialization-* cutover proof or when no importers remain; do not invent a new top-level owner",
	}, true
}

func matchNonServiceFamily(packagePath string) (family string, ok bool) {
	for _, name := range closedDestinationVocabulary().NonServiceFamilies {
		prefix := "pkg/" + name
		if packagePath == prefix || strings.HasPrefix(packagePath, prefix+"/") {
			return name, true
		}
	}
	return "", false
}

func buildResidualPackages(inventory []string) ([]PackageMapping, error) {
	rows := make([]PackageMapping, 0)
	seenFamilies := make(map[string]struct{}, len(closedDestinationVocabulary().NonServiceFamilies))
	for _, packagePath := range inventory {
		mapping, ok := mapResidualPackage(packagePath)
		if !ok {
			continue
		}
		rows = append(rows, mapping)
		if mapping.Disposition == DispositionRetain {
			if family, matched := matchNonServiceFamily(packagePath); matched {
				seenFamilies[family] = struct{}{}
			}
		}
	}
	var missing []string
	for _, family := range closedDestinationVocabulary().NonServiceFamilies {
		if _, ok := seenFamilies[family]; !ok {
			missing = append(missing, family)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("residual mappings missing approved non-service families: %s", strings.Join(missing, ", "))
	}
	slices.SortFunc(rows, func(a, b PackageMapping) int {
		return strings.Compare(a.PackagePath, b.PackagePath)
	})
	return rows, nil
}

func mergeResidualPackageRows(existing []PackageMapping, residualRows []PackageMapping) []PackageMapping {
	residualPaths := make(map[string]struct{}, len(residualRows))
	for _, row := range residualRows {
		residualPaths[row.PackagePath] = struct{}{}
	}
	merged := make([]PackageMapping, 0, len(existing)+len(residualRows))
	for _, row := range existing {
		if _, isResidualRow := residualPaths[row.PackagePath]; isResidualRow {
			continue
		}
		if _, ok := mapResidualPackage(row.PackagePath); ok {
			// Drop stale residual rows that are no longer emitted.
			continue
		}
		merged = append(merged, row)
	}
	merged = append(merged, residualRows...)
	slices.SortFunc(merged, func(a, b PackageMapping) int {
		return strings.Compare(a.PackagePath, b.PackagePath)
	})
	return merged
}

func validateResidualCoverage(manifest Manifest) error {
	byPath := make(map[string]PackageMapping, len(manifest.Packages))
	for _, row := range manifest.Packages {
		byPath[row.PackagePath] = row
	}
	for _, packagePath := range manifest.Inventory {
		want, ok := mapResidualPackage(packagePath)
		if !ok {
			continue
		}
		got, present := byPath[packagePath]
		if !present {
			return fmt.Errorf("packages missing residual mapping for %q", packagePath)
		}
		if got != want {
			return fmt.Errorf("packages[%q] = %#v, want residual %#v", packagePath, got, want)
		}
	}
	return nil
}
