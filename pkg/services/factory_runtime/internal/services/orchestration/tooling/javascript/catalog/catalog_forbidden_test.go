package catalog_test

import (
	"testing"

	jscatalog "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/catalog"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/symbolidentity"
)

func TestCatalogForbiddenSymbolIssues_FailsForForbiddenRootGlobal(t *testing.T) {
	catalog := []jscatalog.CatalogSymbolPath{
		{SymbolKey: "javascript.context", Path: "context"},
	}
	symbols := map[string]any{
		"javascript.context": map[string]any{
			"path": "context",
			"kind": "value",
		},
	}

	issues := jscatalog.CatalogForbiddenSymbolIssues(catalog, symbols)
	if len(issues) != 1 {
		t.Fatalf("CatalogForbiddenSymbolIssues() = %+v, want one forbidden issue", issues)
	}
	issue := issues[0]
	if issue.Code != "javascript.path.forbidden" || issue.Path != "context" {
		t.Fatalf("issue = %+v, want forbidden path %q", issue, "context")
	}
}

func TestCatalogForbiddenSymbolIssues_FailsForOrchestratorGlobal(t *testing.T) {
	catalog := []jscatalog.CatalogSymbolPath{
		{SymbolKey: "javascript.orchestrator", Path: "orchestrator"},
	}
	symbols := map[string]any{
		"javascript.orchestrator": map[string]any{
			"path": "orchestrator",
			"kind": "namespace",
		},
	}

	issues := jscatalog.CatalogForbiddenSymbolIssues(catalog, symbols)
	if len(issues) != 1 {
		t.Fatalf("CatalogForbiddenSymbolIssues() = %+v, want one forbidden issue", issues)
	}
	issue := issues[0]
	if issue.Code != "javascript.path.forbidden" || issue.Path != "orchestrator" {
		t.Fatalf("issue = %+v, want forbidden path %q", issue, "orchestrator")
	}
}

func TestCatalogForbiddenSymbolIssues_FailsForComparisonProjectHelper(t *testing.T) {
	catalog := []jscatalog.CatalogSymbolPath{
		{SymbolKey: "javascript.workflow.sleep", Path: "workflow.sleep"},
	}
	symbols := map[string]any{
		"javascript.workflow.sleep": map[string]any{
			"path": "workflow.sleep",
			"kind": "method",
		},
	}

	issues := jscatalog.CatalogForbiddenSymbolIssues(catalog, symbols)
	if len(issues) != 1 {
		t.Fatalf("CatalogForbiddenSymbolIssues() = %+v, want one unsupported-helper issue", issues)
	}
	issue := issues[0]
	if issue.Code != "javascript.path.unsupported_helper" || issue.Path != "workflow.sleep" {
		t.Fatalf("issue = %+v, want unsupported helper path %q", issue, "workflow.sleep")
	}
}

func TestCatalogForbiddenSymbolIssues_FailsForCallableAgentGlobal(t *testing.T) {
	catalog := []jscatalog.CatalogSymbolPath{
		{SymbolKey: "javascript.agent", Path: "agent"},
	}
	symbols := map[string]any{
		"javascript.agent": map[string]any{
			"path": "agent",
			"kind": "function",
		},
	}

	issues := jscatalog.CatalogForbiddenSymbolIssues(catalog, symbols)
	if len(issues) != 1 {
		t.Fatalf("CatalogForbiddenSymbolIssues() = %+v, want one unsupported-helper issue", issues)
	}
	issue := issues[0]
	if issue.Code != "javascript.path.unsupported_helper" || issue.Path != "agent" {
		t.Fatalf("issue = %+v, want unsupported helper path %q", issue, "agent")
	}
}

func TestCatalogForbiddenSymbolIssues_PassesForInstalledSurface(t *testing.T) {
	catalog := installedCatalogSymbolPaths(t)
	symbols := installedCatalogSymbols(t)

	issues := jscatalog.CatalogForbiddenSymbolIssues(catalog, symbols)
	if len(issues) != 0 {
		t.Fatalf("CatalogForbiddenSymbolIssues() = %+v, want none on installed surface", issues)
	}
}

func installedCatalogSymbols(t *testing.T) map[string]any {
	t.Helper()

	symbols := make(map[string]any, len(symbolidentity.ExpectedInstalledPaths()))
	for _, path := range symbolidentity.ExpectedInstalledPaths() {
		key := catalogSymbolKeyForPath(path)
		kind := "method"
		switch path {
		case "agent":
			kind = "namespace"
		case "workflow":
			kind = "namespace"
		case "phase", "log":
			kind = "function"
		}
		symbols[key] = map[string]any{
			"path": path,
			"kind": kind,
		}
	}
	return symbols
}
