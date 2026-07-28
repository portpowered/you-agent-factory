// Package javascriptcontractsmoke compares the authored JavaScript runtime
// catalog, staged package projection, installed binding descriptor, and
// call-behavior baseline. It is build/verification tooling and must not be
// imported by runtime packages.
package javascriptcontractsmoke

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const (
	AuthoredCatalogRelativePath  = "contracts/javascript/runtime-api.json"
	StagedProjectionRelativePath = "packages/api/generated/javascript/runtime-api.json"
)

// Diagnostic records one deterministic repository-relative parity failure.
type Diagnostic struct {
	Code    string
	Path    string
	Message string
}

// Check compares the authored catalog, staged projection, installed binding
// descriptor, and call-behavior baseline using the existing B02/B03 helpers.
// It never mutates repository files.
func Check(root string) ([]Diagnostic, error) {
	repositoryRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}

	authoredPath := filepath.Join(repositoryRoot, filepath.FromSlash(AuthoredCatalogRelativePath))
	stagedPath := filepath.Join(repositoryRoot, filepath.FromSlash(StagedProjectionRelativePath))

	authoredBytes, err := os.ReadFile(authoredPath)
	if err != nil {
		return nil, fmt.Errorf("read authored catalog %s: %w", AuthoredCatalogRelativePath, err)
	}
	stagedBytes, err := os.ReadFile(stagedPath)
	if err != nil {
		return nil, fmt.Errorf("read staged projection %s: %w", StagedProjectionRelativePath, err)
	}

	var diagnostics []Diagnostic
	if !bytes.Equal(authoredBytes, stagedBytes) {
		message := "staged JavaScript runtime projection diverges from the authored catalog"
		if paths := staleProjectionSymbolPaths(authoredBytes, stagedBytes); len(paths) > 0 {
			message += "; divergent symbol paths: " + strings.Join(paths, ", ")
		}
		diagnostics = append(diagnostics, Diagnostic{
			Code:    "javascript.projection.stale",
			Path:    StagedProjectionRelativePath,
			Message: message + "; run `make contracts-generate` and `make contracts-check` to restore staging parity",
		})
	}

	var catalog map[string]any
	if err := json.Unmarshal(authoredBytes, &catalog); err != nil {
		return nil, fmt.Errorf("decode authored catalog %s: %w", AuthoredCatalogRelativePath, err)
	}

	identity := factoryruntime.JavaScriptProjectInstalledBindings()
	callInventory := factoryruntime.JavaScriptProjectInstalledCallBehavior()

	if err := factoryruntime.JavaScriptVerifyProjectedInstalledBindings(); err != nil {
		diagnostics = append(diagnostics, bindingDescriptorDiagnostic(err))
	}

	catalogPaths, err := factoryruntime.JavaScriptCatalogSymbolPathsFromDocument(catalog)
	if err != nil {
		return nil, fmt.Errorf("extract catalog symbol paths: %w", err)
	}
	symbols, _ := catalog["symbols"].(map[string]any)
	for _, issue := range factoryruntime.JavaScriptCatalogForbiddenSymbolIssues(catalogPaths, symbols) {
		diagnostics = append(diagnostics, forbiddenSymbolDiagnostic(issue))
	}
	for _, issue := range factoryruntime.JavaScriptCatalogPathCompletenessIssues(catalogPaths, identity, callInventory) {
		diagnostics = append(diagnostics, pathCompletenessDiagnostic(issue))
	}

	callIssues, err := factoryruntime.JavaScriptCatalogCallBehaviorParityIssues(catalog, callInventory)
	if err != nil {
		return nil, fmt.Errorf("compare catalog call-behavior parity: %w", err)
	}
	for _, issue := range callIssues {
		diagnostics = append(diagnostics, callBehaviorDiagnostic(issue))
	}

	sortDiagnostics(diagnostics)
	return diagnostics, nil
}

func staleProjectionSymbolPaths(authoredBytes, stagedBytes []byte) []string {
	var authored, staged struct {
		Symbols map[string]map[string]any `json:"symbols"`
	}
	if json.Unmarshal(authoredBytes, &authored) != nil || json.Unmarshal(stagedBytes, &staged) != nil {
		return nil
	}

	keys := make(map[string]struct{}, len(authored.Symbols)+len(staged.Symbols))
	for key := range authored.Symbols {
		keys[key] = struct{}{}
	}
	for key := range staged.Symbols {
		keys[key] = struct{}{}
	}

	paths := make(map[string]struct{})
	for key := range keys {
		if reflect.DeepEqual(authored.Symbols[key], staged.Symbols[key]) {
			continue
		}
		for _, record := range []map[string]any{authored.Symbols[key], staged.Symbols[key]} {
			if path, ok := record["path"].(string); ok && path != "" {
				paths[path] = struct{}{}
			}
		}
	}

	out := make([]string, 0, len(paths))
	for path := range paths {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func bindingDescriptorDiagnostic(err error) Diagnostic {
	message := strings.TrimSpace(err.Error())
	path := AuthoredCatalogRelativePath
	if quoted := firstQuotedString(message); quoted != "" {
		path = quoted
	}
	return Diagnostic{
		Code:    "javascript.binding.descriptor",
		Path:    path,
		Message: message + "; restore parity between the installed binding descriptor and authored catalog",
	}
}

func forbiddenSymbolDiagnostic(issue factoryruntime.JavaScriptCatalogPathCompletenessIssue) Diagnostic {
	path := issue.Path
	if path == "" || path == "/symbols" {
		path = AuthoredCatalogRelativePath
	}
	return Diagnostic{
		Code:    issue.Code,
		Path:    path,
		Message: issue.Message + "; remove the path from the contracted supported surface",
	}
}

func pathCompletenessDiagnostic(issue factoryruntime.JavaScriptCatalogPathCompletenessIssue) Diagnostic {
	path := issue.Path
	if path == "" || path == "/symbols" {
		path = AuthoredCatalogRelativePath
	}
	return Diagnostic{
		Code:    issue.Code,
		Path:    path,
		Message: issue.Message + "; restore authored catalog and staging parity with the installed binding descriptor",
	}
}

func callBehaviorDiagnostic(issue factoryruntime.JavaScriptCatalogCallBehaviorParityIssue) Diagnostic {
	path := issue.Path
	if path == "" {
		path = AuthoredCatalogRelativePath
	}
	message := issue.Message
	if issue.SymbolKey != "" && issue.Field != "" {
		message = fmt.Sprintf("%s at /symbols/%s/%s", issue.Message, issue.SymbolKey, issue.Field)
	}
	return Diagnostic{
		Code:    issue.Code,
		Path:    path,
		Message: message + "; restore catalog call metadata parity with the installed call-behavior baseline",
	}
}

func sortDiagnostics(diagnostics []Diagnostic) {
	sort.Slice(diagnostics, func(i, j int) bool {
		left, right := diagnostics[i], diagnostics[j]
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		return left.Message < right.Message
	})
}

func firstQuotedString(message string) string {
	start := strings.Index(message, `"`)
	if start < 0 {
		return ""
	}
	rest := message[start+1:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}
