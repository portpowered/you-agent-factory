package symbolidentity

import (
	"fmt"
	"sort"
	"strings"
)

// ForbiddenRootGlobals are script-visible roots that must not appear in the
// installed binding surface or symbol identity inventory.
var ForbiddenRootGlobals = []string{"context", "orchestrator"}

var comparisonProjectHelperPaths = map[string]struct{}{
	"workflow.sleep": {},
	"agent.verify":   {},
	"agent.parallel": {},
}

// SurfaceClassification describes whether a documented JavaScript symbol is
// part of the installed supported surface.
type SurfaceClassification string

const (
	SurfaceSupported               SurfaceClassification = "supported"
	SurfaceForbiddenHostGlobal     SurfaceClassification = "forbidden_host_global"
	SurfaceComparisonProjectHelper SurfaceClassification = "comparison_project_helper"
	SurfaceCallableAgentGlobal     SurfaceClassification = "callable_agent_global"
)

// ClassifySurface centralizes forbidden and comparison-project-only symbol
// policy for contract validation and installed-catalog parity checks.
func ClassifySurface(path, kind string) SurfaceClassification {
	if isForbiddenSymbolPath(path) {
		return SurfaceForbiddenHostGlobal
	}
	if _, ok := comparisonProjectHelperPaths[path]; ok {
		return SurfaceComparisonProjectHelper
	}
	if path == "agent" && kind == "function" {
		return SurfaceCallableAgentGlobal
	}
	return SurfaceSupported
}

// IsUnsupportedSurfacePath reports path-only unsupported surfaces. Callable
// agent classification additionally requires the catalog symbol kind.
func IsUnsupportedSurfacePath(path string) bool {
	classification := ClassifySurface(path, "")
	return classification != SurfaceSupported
}

// VerifyProjectedInstalledBindings projects the canonical inventory and fails
// when paths are missing, duplicated, unexpected, unsorted, or forbidden.
func VerifyProjectedInstalledBindings() error {
	return VerifyInventory(ProjectInstalledBindings())
}

// VerifyInventory fails when symbol paths are missing, duplicated, unexpected,
// unsorted, or include forbidden globals.
func VerifyInventory(inv Inventory) error {
	if inv.FormatVersion != FormatVersion {
		return fmt.Errorf("formatVersion = %q, want %q", inv.FormatVersion, FormatVersion)
	}
	if err := verifyNoDuplicateSymbolPaths(inv.Symbols); err != nil {
		return err
	}
	if err := verifyForbiddenGlobalsAbsent(inv.Symbols); err != nil {
		return err
	}
	if err := verifyExpectedSymbolPaths(inv.Symbols); err != nil {
		return err
	}
	return verifySymbolsSortedByPath(inv.Symbols)
}

func verifySymbolsSortedByPath(symbols []SymbolRecord) error {
	for i := 1; i < len(symbols); i++ {
		prev := symbols[i-1].Path
		curr := symbols[i].Path
		if prev > curr {
			return fmt.Errorf("symbols not sorted by path at index %d: %q > %q", i, prev, curr)
		}
	}
	return nil
}

func verifyNoDuplicateSymbolPaths(symbols []SymbolRecord) error {
	seen := make(map[string]struct{}, len(symbols))
	for _, record := range symbols {
		if _, exists := seen[record.Path]; exists {
			return fmt.Errorf("duplicate symbol path %q", record.Path)
		}
		seen[record.Path] = struct{}{}
	}
	return nil
}

func verifyExpectedSymbolPaths(symbols []SymbolRecord) error {
	expected := ExpectedInstalledPaths()
	got := symbolPaths(symbols)
	return compareSymbolPathSets(expected, got)
}

func compareSymbolPathSets(expected, got []string) error {
	expectedSet := make(map[string]struct{}, len(expected))
	for _, path := range expected {
		expectedSet[path] = struct{}{}
	}

	for _, path := range got {
		if _, ok := expectedSet[path]; !ok {
			return fmt.Errorf("unexpected symbol path %q", path)
		}
		delete(expectedSet, path)
	}

	missing := make([]string, 0, len(expectedSet))
	for path := range expectedSet {
		missing = append(missing, path)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("missing symbol path %q", missing[0])
	}
	return nil
}

func verifyForbiddenGlobalsAbsent(symbols []SymbolRecord) error {
	for _, record := range symbols {
		if isForbiddenSymbolPath(record.Path) {
			return fmt.Errorf("forbidden symbol path %q", record.Path)
		}
	}
	return nil
}

func isForbiddenSymbolPath(path string) bool {
	for _, forbidden := range ForbiddenRootGlobals {
		if path == forbidden || strings.HasPrefix(path, forbidden+".") {
			return true
		}
	}
	return false
}

// InstalledRootGlobals returns sorted root-level globals from the expected
// installed binding surface.
func InstalledRootGlobals() []string {
	roots := make(map[string]struct{})
	for _, path := range ExpectedInstalledPaths() {
		root, _, ok := strings.Cut(path, ".")
		if !ok {
			root = path
		}
		roots[root] = struct{}{}
	}
	out := make([]string, 0, len(roots))
	for root := range roots {
		out = append(out, root)
	}
	sort.Strings(out)
	return out
}

func symbolPaths(symbols []SymbolRecord) []string {
	paths := make([]string, len(symbols))
	for i, record := range symbols {
		paths[i] = record.Path
	}
	return paths
}
