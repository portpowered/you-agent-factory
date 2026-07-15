package catalog_test

import (
	"slices"
	"testing"

	jscatalog "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime/catalog"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime/callbehavior"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime/symbolidentity"
)

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
