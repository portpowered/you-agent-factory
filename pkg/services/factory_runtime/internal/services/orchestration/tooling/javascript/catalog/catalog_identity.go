package catalog

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/callbehavior"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/symbolidentity"
)

// CatalogSymbolPath records one catalog symbol map key and its installed path.
type CatalogSymbolPath struct {
	SymbolKey string
	Path      string
}

// PathCompletenessIssue records one catalog path completeness failure by path.
type PathCompletenessIssue struct {
	Code      string
	SymbolKey string
	Path      string
	Message   string
}

// CatalogSymbolPathsFromDocument extracts symbol paths from one resolved runtime
// manifest catalog document value.
func CatalogSymbolPathsFromDocument(value any) ([]CatalogSymbolPath, error) {
	root, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("catalog document is not an object")
	}
	symbolsValue, ok := root["symbols"]
	if !ok {
		return nil, fmt.Errorf("catalog document missing symbols")
	}
	symbols, ok := symbolsValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("catalog symbols is not an object")
	}

	keys := make([]string, 0, len(symbols))
	for key := range symbols {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	paths := make([]CatalogSymbolPath, 0, len(keys))
	for _, key := range keys {
		record, ok := symbols[key].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("catalog symbol %q is not an object", key)
		}
		path, _ := record["path"].(string)
		if path == "" {
			return nil, fmt.Errorf("catalog symbol %q has empty path", key)
		}
		paths = append(paths, CatalogSymbolPath{SymbolKey: key, Path: path})
	}
	return paths, nil
}

// CatalogPathCompletenessIssues compares catalog symbol paths to the reviewed
// identity baseline and installed call-behavior descriptor.
func CatalogPathCompletenessIssues(
	catalog []CatalogSymbolPath,
	identity symbolidentity.Inventory,
	callInventory callbehavior.Inventory,
) []PathCompletenessIssue {
	identityPaths := pathsFromIdentityInventory(identity)
	callPaths := pathsFromCallInventory(callInventory)
	if !slices.Equal(identityPaths, callPaths) {
		return []PathCompletenessIssue{{
			Code:    "javascript.path.baseline_drift",
			Path:    "/symbols",
			Message: "identity baseline and installed call-behavior descriptor paths differ",
		}}
	}

	pathToKeys := make(map[string][]string, len(catalog))
	for _, entry := range catalog {
		pathToKeys[entry.Path] = append(pathToKeys[entry.Path], entry.SymbolKey)
	}

	var issues []PathCompletenessIssue
	issues = append(issues, catalogDuplicatePathIssues(pathToKeys)...)

	catalogPaths := make(map[string]struct{}, len(catalog))
	for _, entry := range catalog {
		catalogPaths[entry.Path] = struct{}{}
	}

	for _, expected := range identityPaths {
		if _, ok := catalogPaths[expected]; !ok {
			issues = append(issues, PathCompletenessIssue{
				Code:    "javascript.path.missing",
				Path:    expected,
				Message: fmt.Sprintf("installed symbol path %s missing from catalog", strconv.Quote(expected)),
			})
		}
	}

	pathToKey := make(map[string]string, len(catalog))
	for _, entry := range catalog {
		pathToKey[entry.Path] = entry.SymbolKey
	}
	issues = append(issues, catalogExtraPathIssues(catalogPaths, identityPaths, pathToKey)...)

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

func catalogDuplicatePathIssues(pathToKeys map[string][]string) []PathCompletenessIssue {
	var issues []PathCompletenessIssue
	for path, keys := range pathToKeys {
		if len(keys) <= 1 {
			continue
		}
		message := fmt.Sprintf("symbol path %s appears more than once", strconv.Quote(path))
		for _, key := range keys {
			issues = append(issues, PathCompletenessIssue{
				Code:      "javascript.path.duplicate",
				SymbolKey: key,
				Path:      path,
				Message:   message,
			})
		}
	}
	return issues
}

func catalogExtraPathIssues(
	catalogPaths map[string]struct{},
	identityPaths []string,
	pathToKey map[string]string,
) []PathCompletenessIssue {
	expectedSet := make(map[string]struct{}, len(identityPaths))
	for _, path := range identityPaths {
		expectedSet[path] = struct{}{}
	}
	extraPaths := make([]string, 0)
	for path := range catalogPaths {
		if _, ok := expectedSet[path]; !ok {
			if isUnsupportedCatalogSurfacePath(path) {
				continue
			}
			extraPaths = append(extraPaths, path)
		}
	}
	sort.Strings(extraPaths)

	issues := make([]PathCompletenessIssue, 0, len(extraPaths))
	for _, path := range extraPaths {
		issues = append(issues, PathCompletenessIssue{
			Code:      "javascript.path.extra",
			SymbolKey: pathToKey[path],
			Path:      path,
			Message:   fmt.Sprintf("catalog path %s is not part of the installed supported surface", strconv.Quote(path)),
		})
	}
	return issues
}

// VerifyCatalogPathCompleteness ensures every installed binding path occurs
// exactly once in the catalog and no extra supported-surface paths are present.
func VerifyCatalogPathCompleteness(
	catalog []CatalogSymbolPath,
	identity symbolidentity.Inventory,
	callInventory callbehavior.Inventory,
) error {
	issues := CatalogPathCompletenessIssues(catalog, identity, callInventory)
	if len(issues) == 0 {
		return nil
	}
	messages := make([]string, 0, len(issues))
	for _, issue := range issues {
		if issue.SymbolKey != "" {
			messages = append(messages, fmt.Sprintf("%s at /symbols/%s/path", issue.Message, issue.SymbolKey))
			continue
		}
		messages = append(messages, issue.Message)
	}
	return fmt.Errorf("catalog path completeness failed: %s", strings.Join(messages, "; "))
}

func pathsFromIdentityInventory(inventory symbolidentity.Inventory) []string {
	paths := make([]string, len(inventory.Symbols))
	for i, record := range inventory.Symbols {
		paths[i] = record.Path
	}
	sort.Strings(paths)
	return paths
}

func pathsFromCallInventory(inventory callbehavior.Inventory) []string {
	paths := make([]string, len(inventory.Records))
	for i, record := range inventory.Records {
		paths[i] = record.Path
	}
	sort.Strings(paths)
	return paths
}
