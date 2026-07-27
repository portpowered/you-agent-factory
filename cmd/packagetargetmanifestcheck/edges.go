package main

import (
	"fmt"
	"slices"
	"strings"
)

const edgesFutureDebtID = "FND-06"

// FutureDebt records deferred migration work that this packet intentionally
// does not perform (for example Edges narrowing under FND-06).
type FutureDebt struct {
	ID          string `json:"id"`
	PackagePath string `json:"packagePath,omitempty"`
	Description string `json:"description"`
}

func exceptionSet() map[string]struct{} {
	exceptions := closedDestinationVocabulary().ArchitectureExceptions
	set := make(map[string]struct{}, len(exceptions))
	for _, name := range exceptions {
		set[name] = struct{}{}
	}
	return set
}

// mapEdgesExceptionPackage maps Process Edges production packages to the sole
// architecture-exception destination. Child packages retain the same
// exception destination; this packet does not narrow Edges imports (FND-06).
func mapEdgesExceptionPackage(packagePath string) (PackageMapping, bool) {
	packagePath = strings.TrimSpace(packagePath)
	if packagePath == "pkg/services/edges" || strings.HasPrefix(packagePath, "pkg/services/edges/") {
		return PackageMapping{
			PackagePath: packagePath,
			Disposition: DispositionRetain,
			Destination: "edges",
		}, true
	}
	return PackageMapping{}, false
}

func buildEdgesExceptionPackages(inventory []string) ([]PackageMapping, error) {
	rows := make([]PackageMapping, 0)
	for _, packagePath := range inventory {
		mapping, ok := mapEdgesExceptionPackage(packagePath)
		if !ok {
			continue
		}
		rows = append(rows, mapping)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("inventory is missing pkg/services/edges; Process Edges exception cannot be recorded")
	}
	slices.SortFunc(rows, func(a, b PackageMapping) int {
		return strings.Compare(a.PackagePath, b.PackagePath)
	})
	return rows, nil
}

func mergeEdgesPackageRows(existing []PackageMapping, edgesRows []PackageMapping) []PackageMapping {
	edgesPaths := make(map[string]struct{}, len(edgesRows))
	for _, row := range edgesRows {
		edgesPaths[row.PackagePath] = struct{}{}
	}
	merged := make([]PackageMapping, 0, len(existing)+len(edgesRows))
	for _, row := range existing {
		if _, isEdgesRow := edgesPaths[row.PackagePath]; isEdgesRow {
			continue
		}
		if _, ok := mapEdgesExceptionPackage(row.PackagePath); ok {
			// Drop stale edges rows that are no longer emitted.
			continue
		}
		merged = append(merged, row)
	}
	merged = append(merged, edgesRows...)
	slices.SortFunc(merged, func(a, b PackageMapping) int {
		return strings.Compare(a.PackagePath, b.PackagePath)
	})
	return merged
}

func validateEdgesExceptionCoverage(manifest Manifest) error {
	byPath := make(map[string]PackageMapping, len(manifest.Packages))
	for _, row := range manifest.Packages {
		byPath[row.PackagePath] = row
	}
	for _, packagePath := range manifest.Inventory {
		want, ok := mapEdgesExceptionPackage(packagePath)
		if !ok {
			continue
		}
		got, present := byPath[packagePath]
		if !present {
			return fmt.Errorf("packages missing Process Edges exception mapping for %q", packagePath)
		}
		if got != want {
			return fmt.Errorf("packages[%q] = %#v, want edges exception %#v", packagePath, got, want)
		}
	}

	for _, row := range manifest.Packages {
		root, _, ok := splitDestination(row.Destination)
		if !ok {
			continue
		}
		if _, isException := exceptionSet()[root]; !isException {
			continue
		}
		if root != "edges" {
			return fmt.Errorf("package %q destination %q marks a peer-wide external-effect exception; only edges is allowed", row.PackagePath, row.Destination)
		}
		if row.Disposition != DispositionRetain {
			return fmt.Errorf("package %q edges exception disposition %q; want retain", row.PackagePath, row.Disposition)
		}
	}
	return nil
}

func validateFutureDebt(debt []FutureDebt) error {
	found := false
	for i, entry := range debt {
		prefix := fmt.Sprintf("futureDebt[%d]", i)
		if strings.TrimSpace(entry.ID) == "" {
			return fmt.Errorf("%s.id is required", prefix)
		}
		if strings.TrimSpace(entry.Description) == "" {
			return fmt.Errorf("%s.description is required", prefix)
		}
		if entry.ID == edgesFutureDebtID && strings.Contains(strings.ToLower(entry.Description), "edges") {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("futureDebt must record %s Edges narrowing as deferred debt (not performed in this packet)", edgesFutureDebtID)
	}
	return nil
}

func edgesFutureDebtEntry() FutureDebt {
	return FutureDebt{
		ID:          edgesFutureDebtID,
		PackagePath: "pkg/services/edges",
		Description: "Narrow Edges implementation imports and the broad external-effect surface; deferred to FND-06. This packet only records the architecture exception.",
	}
}

func ensureEdgesFutureDebt(existing []FutureDebt) []FutureDebt {
	want := edgesFutureDebtEntry()
	out := make([]FutureDebt, 0, len(existing)+1)
	replaced := false
	for _, entry := range existing {
		if entry.ID == edgesFutureDebtID {
			out = append(out, want)
			replaced = true
			continue
		}
		out = append(out, entry)
	}
	if !replaced {
		out = append(out, want)
	}
	return out
}
