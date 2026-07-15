package catalog_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	jscatalog "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime/catalog"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime/callbehavior"
)

func TestVerifyCatalogCallBehaviorParity_PassesForAuthoredCatalog(t *testing.T) {
	document := loadAuthoredJavaScriptRuntimeCatalogDocument(t)
	callInventory := callbehavior.ProjectInstalledCallBehavior()

	if err := jscatalog.VerifyCatalogCallBehaviorParity(document, callInventory); err != nil {
		t.Fatalf("VerifyCatalogCallBehaviorParity() error = %v", err)
	}
}

func TestCatalogCallBehaviorParityIssues_FailsWhenRepresentativeReturnDrifts(t *testing.T) {
	document := loadAuthoredJavaScriptRuntimeCatalogDocument(t)
	symbols := document["symbols"].(map[string]any)
	parallel := cloneMap(t, symbols["javascript.parallel"].(map[string]any))
	returnValue := cloneMap(t, parallel["return"].(map[string]any))
	returnValue["type"] = "object"
	parallel["return"] = returnValue
	symbols["javascript.parallel"] = parallel

	issues, err := jscatalog.CatalogCallBehaviorParityIssues(document, callbehavior.ProjectInstalledCallBehavior())
	if err != nil {
		t.Fatalf("CatalogCallBehaviorParityIssues() error = %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("CatalogCallBehaviorParityIssues() = %+v, want one return mismatch", issues)
	}
	issue := issues[0]
	if issue.Code != "javascript.call_behavior.mismatch" ||
		issue.Path != "parallel" ||
		issue.Field != "return" {
		t.Fatalf("issue = %+v, want parallel return mismatch", issue)
	}
}

func TestCatalogCallBehaviorParityIssues_FailsWhenRepresentativeSymbolMissing(t *testing.T) {
	document := loadAuthoredJavaScriptRuntimeCatalogDocument(t)
	symbols := document["symbols"].(map[string]any)
	delete(symbols, "javascript.workflow.final")

	issues, err := jscatalog.CatalogCallBehaviorParityIssues(document, callbehavior.ProjectInstalledCallBehavior())
	if err != nil {
		t.Fatalf("CatalogCallBehaviorParityIssues() error = %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("CatalogCallBehaviorParityIssues() = %+v, want one missing representative path", issues)
	}
	issue := issues[0]
	if issue.Code != "javascript.call_behavior.missing" || issue.Path != "workflow.final" {
		t.Fatalf("issue = %+v, want missing workflow.final", issue)
	}
}

func loadAuthoredJavaScriptRuntimeCatalogDocument(t *testing.T) map[string]any {
	t.Helper()

	repoRoot := filepath.Join("..", "..", "..", "..", "..")
	raw, err := os.ReadFile(filepath.Join(repoRoot, "contracts", "javascript", "runtime-api.json"))
	if err != nil {
		t.Fatalf("read authored catalog: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("unmarshal authored catalog: %v", err)
	}
	return document
}

func cloneMap(t *testing.T, value map[string]any) map[string]any {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal map: %v", err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		t.Fatalf("unmarshal cloned map: %v", err)
	}
	return cloned
}
