// Package contractjoiner builds portable contract documents from explicit
// repository-authored roots and components. It is build tooling and must not be
// imported by runtime packages.
package contractjoiner

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

const joinedOutputDirectory = "packages/api/generated/joined"

// Input identifies the repository boundary and the complete authored set for
// one join operation. Roots become joined documents; Components are permitted
// reference targets but do not produce independent outputs.
type Input struct {
	RepositoryRoot string
	Roots          []string
	Components     []string
}

// Document is one in-memory, self-contained joined contract document.
type Document struct {
	Path  string
	Value any
}

// Join resolves every root against only the explicit authored input set. The
// operation performs no writes and returns documents in repository-path order.
func Join(input Input) ([]Document, []contractvalidator.Diagnostic) {
	authored := normalizedUniquePaths(append(append([]string(nil), input.Roots...), input.Components...))
	roots := normalizedUniquePaths(input.Roots)
	if diagnostics := contractvalidator.ValidateAuthoredPaths(input.RepositoryRoot, authored, joinedOutputDirectory); len(diagnostics) != 0 {
		return nil, diagnostics
	}
	documents := make([]Document, 0, len(roots))
	var diagnostics []contractvalidator.Diagnostic
	for _, root := range roots {
		value, issues := contractvalidator.LoadAndResolve(input.RepositoryRoot, root, authored)
		if len(issues) != 0 {
			diagnostics = append(diagnostics, issues...)
			continue
		}
		documents = append(documents, Document{Path: root, Value: value})
	}
	if len(diagnostics) != 0 {
		contractvalidator.SortDiagnostics(diagnostics)
		return nil, diagnostics
	}
	return documents, nil
}

func normalizedUniquePaths(paths []string) []string {
	unique := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		normalized := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.ReplaceAll(path, `\`, "/"))))
		unique[normalized] = struct{}{}
	}
	result := make([]string, 0, len(unique))
	for path := range unique {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}
