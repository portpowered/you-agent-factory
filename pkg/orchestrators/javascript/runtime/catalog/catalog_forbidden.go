package catalog

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime/symbolidentity"
)

var comparisonProjectHelperPaths = map[string]struct{}{
	"workflow.sleep": {},
	"agent.verify":   {},
	"agent.parallel": {},
}

// CatalogForbiddenSymbolIssues reports catalog symbol paths that document
// forbidden host-only globals or comparison-project-only helpers.
func CatalogForbiddenSymbolIssues(catalog []CatalogSymbolPath, symbols map[string]any) []PathCompletenessIssue {
	var issues []PathCompletenessIssue
	for _, entry := range catalog {
		path := entry.Path
		if isForbiddenInstalledRootGlobalPath(path) {
			issues = append(issues, PathCompletenessIssue{
				Code:      "javascript.path.forbidden",
				SymbolKey: entry.SymbolKey,
				Path:      path,
				Message: fmt.Sprintf(
					"symbol path %s documents a forbidden host-only global",
					strconv.Quote(path),
				),
			})
			continue
		}
		if isComparisonProjectHelperPath(path) {
			issues = append(issues, PathCompletenessIssue{
				Code:      "javascript.path.unsupported_helper",
				SymbolKey: entry.SymbolKey,
				Path:      path,
				Message: fmt.Sprintf(
					"symbol path %s documents a comparison-project-only helper that is not part of the installed supported surface",
					strconv.Quote(path),
				),
			})
			continue
		}
		if path == "agent" && catalogSymbolKind(symbols, entry.SymbolKey) == "function" {
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

func isForbiddenInstalledRootGlobalPath(path string) bool {
	for _, forbidden := range symbolidentity.ForbiddenRootGlobals {
		if path == forbidden || strings.HasPrefix(path, forbidden+".") {
			return true
		}
	}
	return false
}

func isComparisonProjectHelperPath(path string) bool {
	_, ok := comparisonProjectHelperPaths[path]
	return ok
}

func isUnsupportedCatalogSurfacePath(path string) bool {
	return isForbiddenInstalledRootGlobalPath(path) || isComparisonProjectHelperPath(path)
}

func catalogSymbolKind(symbols map[string]any, key string) string {
	record, ok := symbols[key].(map[string]any)
	if !ok {
		return ""
	}
	kind, _ := record["kind"].(string)
	return kind
}
