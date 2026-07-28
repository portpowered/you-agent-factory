package catalog

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/symbolidentity"
)

// CatalogForbiddenSymbolIssues reports catalog symbol paths that document
// forbidden host-only globals or comparison-project-only helpers.
func CatalogForbiddenSymbolIssues(catalog []CatalogSymbolPath, symbols map[string]any) []PathCompletenessIssue {
	var issues []PathCompletenessIssue
	for _, entry := range catalog {
		path := entry.Path
		switch symbolidentity.ClassifySurface(path, catalogSymbolKind(symbols, entry.SymbolKey)) {
		case symbolidentity.SurfaceForbiddenHostGlobal:
			issues = append(issues, PathCompletenessIssue{
				Code:      "javascript.path.forbidden",
				SymbolKey: entry.SymbolKey,
				Path:      path,
				Message: fmt.Sprintf(
					"symbol path %s documents a forbidden host-only global",
					strconv.Quote(path),
				),
			})
		case symbolidentity.SurfaceComparisonProjectHelper:
			issues = append(issues, PathCompletenessIssue{
				Code:      "javascript.path.unsupported_helper",
				SymbolKey: entry.SymbolKey,
				Path:      path,
				Message: fmt.Sprintf(
					"symbol path %s documents a comparison-project-only helper that is not part of the installed supported surface",
					strconv.Quote(path),
				),
			})
		case symbolidentity.SurfaceCallableAgentGlobal:
			issues = append(issues, PathCompletenessIssue{
				Code:      "javascript.path.unsupported_helper",
				SymbolKey: entry.SymbolKey,
				Path:      path,
				Message: fmt.Sprintf(
					"symbol path %s documents a comparison-project-only callable agent global; installed runtime exposes agent as a namespace with agent.run",
					strconv.Quote(path),
				),
			})
		}
	}

	sort.Slice(issues, func(i, j int) bool {
		left, right := issues[i], issues[j]
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		return left.SymbolKey < right.SymbolKey
	})
	return issues
}

func isUnsupportedCatalogSurfacePath(path string) bool {
	return symbolidentity.IsUnsupportedSurfacePath(path)
}

func catalogSymbolKind(symbols map[string]any, key string) string {
	record, ok := symbols[key].(map[string]any)
	if !ok {
		return ""
	}
	kind, _ := record["kind"].(string)
	return kind
}
