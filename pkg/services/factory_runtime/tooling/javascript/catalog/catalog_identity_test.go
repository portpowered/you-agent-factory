package catalog_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/tooling/javascript/callbehavior"
	jscatalog "github.com/portpowered/infinite-you/pkg/services/factory_runtime/tooling/javascript/catalog"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/tooling/javascript/symbolidentity"
)

func TestCatalogSymbolPathsFromDocument_RejectsInvalidDocument(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		document any
		wantErr  string
	}{
		{
			name:     "non-object document",
			document: []any{},
			wantErr:  "catalog document is not an object",
		},
		{
			name: "missing symbols",
			document: map[string]any{
				"sharedSchemas": map[string]any{},
			},
			wantErr: "catalog document missing symbols",
		},
		{
			name: "symbols not object",
			document: map[string]any{
				"symbols": []any{},
			},
			wantErr: "catalog symbols is not an object",
		},
		{
			name: "symbol not object",
			document: map[string]any{
				"symbols": map[string]any{
					"javascript.log": "not-an-object",
				},
			},
			wantErr: `catalog symbol "javascript.log" is not an object`,
		},
		{
			name: "empty path",
			document: map[string]any{
				"symbols": map[string]any{
					"javascript.log": map[string]any{
						"path": "",
					},
				},
			},
			wantErr: `catalog symbol "javascript.log" has empty path`,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := jscatalog.CatalogSymbolPathsFromDocument(tc.document)
			if err == nil {
				t.Fatal("CatalogSymbolPathsFromDocument() error = nil, want error")
			}
			if err.Error() != tc.wantErr {
				t.Fatalf("CatalogSymbolPathsFromDocument() error = %q, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestVerifyCatalogPathCompleteness_ReturnsFormattedError(t *testing.T) {
	catalog := installedCatalogSymbolPaths(t)
	catalog = removeCatalogPath(catalog, "phase")

	err := jscatalog.VerifyCatalogPathCompleteness(
		catalog,
		symbolidentity.ProjectInstalledBindings(),
		callbehavior.ProjectInstalledCallBehavior(),
	)
	if err == nil {
		t.Fatal("VerifyCatalogPathCompleteness() error = nil, want formatted completeness failure")
	}
	if !strings.Contains(err.Error(), "catalog path completeness failed:") {
		t.Fatalf("VerifyCatalogPathCompleteness() error = %q, want completeness prefix", err)
	}
}

func TestCatalogSymbolPathsFromDocument_ExtractsSortedPaths(t *testing.T) {
	document := map[string]any{
		"symbols": map[string]any{
			"javascript.phase": map[string]any{
				"path": "phase",
			},
			"javascript.log": map[string]any{
				"path": "log",
			},
		},
	}

	got, err := jscatalog.CatalogSymbolPathsFromDocument(document)
	if err != nil {
		t.Fatalf("CatalogSymbolPathsFromDocument() error = %v", err)
	}
	want := []jscatalog.CatalogSymbolPath{
		{SymbolKey: "javascript.log", Path: "log"},
		{SymbolKey: "javascript.phase", Path: "phase"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("CatalogSymbolPathsFromDocument() = %+v, want %+v", got, want)
	}
}

func TestVerifyCatalogPathCompleteness_PassesForInstalledSurface(t *testing.T) {
	catalog := installedCatalogSymbolPaths(t)
	identity := symbolidentity.ProjectInstalledBindings()
	callInventory := callbehavior.ProjectInstalledCallBehavior()

	if err := jscatalog.VerifyCatalogPathCompleteness(catalog, identity, callInventory); err != nil {
		t.Fatalf("VerifyCatalogPathCompleteness() error = %v", err)
	}
}

func TestVerifyCatalogPathCompleteness_FailsWhenInstalledPathMissing(t *testing.T) {
	catalog := installedCatalogSymbolPaths(t)
	catalog = removeCatalogPath(catalog, "phase")

	issues := jscatalog.CatalogPathCompletenessIssues(
		catalog,
		symbolidentity.ProjectInstalledBindings(),
		callbehavior.ProjectInstalledCallBehavior(),
	)
	if len(issues) != 1 {
		t.Fatalf("CatalogPathCompletenessIssues() = %+v, want one missing-path issue", issues)
	}
	issue := issues[0]
	if issue.Code != "javascript.path.missing" || issue.Path != "phase" {
		t.Fatalf("issue = %+v, want missing path %q", issue, "phase")
	}
}

func TestVerifyCatalogPathCompleteness_FailsWhenCatalogContainsExtraPath(t *testing.T) {
	catalog := installedCatalogSymbolPaths(t)
	catalog = append(catalog, jscatalog.CatalogSymbolPath{
		SymbolKey: "javascript.workflow.extra",
		Path:      "workflow.extra",
	})

	issues := jscatalog.CatalogPathCompletenessIssues(
		catalog,
		symbolidentity.ProjectInstalledBindings(),
		callbehavior.ProjectInstalledCallBehavior(),
	)
	if len(issues) != 1 {
		t.Fatalf("CatalogPathCompletenessIssues() = %+v, want one extra-path issue", issues)
	}
	issue := issues[0]
	if issue.Code != "javascript.path.extra" || issue.Path != "workflow.extra" {
		t.Fatalf("issue = %+v, want extra path %q", issue, "workflow.extra")
	}
}

func TestVerifyCatalogPathCompleteness_FailsWhenCatalogPathDuplicated(t *testing.T) {
	catalog := installedCatalogSymbolPaths(t)
	catalog = append(catalog, jscatalog.CatalogSymbolPath{
		SymbolKey: "javascript.duplicate-phase",
		Path:      "phase",
	})

	issues := jscatalog.CatalogPathCompletenessIssues(
		catalog,
		symbolidentity.ProjectInstalledBindings(),
		callbehavior.ProjectInstalledCallBehavior(),
	)
	if len(issues) != 2 {
		t.Fatalf("CatalogPathCompletenessIssues() = %+v, want duplicate-path issues for both symbols", issues)
	}
	for _, issue := range issues {
		if issue.Code != "javascript.path.duplicate" || issue.Path != "phase" {
			t.Fatalf("issue = %+v, want duplicate path %q", issue, "phase")
		}
	}
}

func installedCatalogSymbolPaths(t *testing.T) []jscatalog.CatalogSymbolPath {
	t.Helper()

	paths := symbolidentity.ExpectedInstalledPaths()
	catalog := make([]jscatalog.CatalogSymbolPath, 0, len(paths))
	for _, path := range paths {
		catalog = append(catalog, jscatalog.CatalogSymbolPath{
			SymbolKey: catalogSymbolKeyForPath(path),
			Path:      path,
		})
	}
	return catalog
}

func catalogSymbolKeyForPath(path string) string {
	if path == "workflow.resumeState" {
		return "javascript.workflow.resume-state"
	}
	return "javascript." + path
}

func removeCatalogPath(catalog []jscatalog.CatalogSymbolPath, path string) []jscatalog.CatalogSymbolPath {
	filtered := make([]jscatalog.CatalogSymbolPath, 0, len(catalog))
	for _, entry := range catalog {
		if entry.Path == path {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}
