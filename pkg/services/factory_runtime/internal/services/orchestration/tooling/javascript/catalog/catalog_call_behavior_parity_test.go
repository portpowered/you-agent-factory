package catalog_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/callbehavior"
	jscatalog "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/catalog"
)

func TestVerifyCatalogCallBehaviorParity_ReturnsFormattedError(t *testing.T) {
	document := loadAuthoredJavaScriptRuntimeCatalogDocument(t)
	symbols := document["symbols"].(map[string]any)
	parallel := cloneMap(t, symbols["javascript.parallel"].(map[string]any))
	returnValue := cloneMap(t, parallel["return"].(map[string]any))
	returnValue["type"] = "object"
	parallel["return"] = returnValue
	symbols["javascript.parallel"] = parallel

	err := jscatalog.VerifyCatalogCallBehaviorParity(document, callbehavior.ProjectInstalledCallBehavior())
	if err == nil {
		t.Fatal("VerifyCatalogCallBehaviorParity() error = nil, want formatted parity failure")
	}
	if !strings.Contains(err.Error(), "catalog call-behavior parity failed:") {
		t.Fatalf("VerifyCatalogCallBehaviorParity() error = %q, want parity prefix", err)
	}
	if !strings.Contains(err.Error(), "/symbols/javascript.parallel/return") {
		t.Fatalf("VerifyCatalogCallBehaviorParity() error = %q, want symbol field path", err)
	}
}

func TestCatalogCallBehaviorParityIssues_DetectsParameterAndCallbackDrift(t *testing.T) {
	document := loadAuthoredJavaScriptRuntimeCatalogDocument(t)
	symbols := document["symbols"].(map[string]any)

	checkpoint := cloneMap(t, symbols["javascript.workflow.checkpoint"].(map[string]any))
	params := cloneSlice(t, checkpoint["parameters"].([]any))
	firstParam := cloneMap(t, params[0].(map[string]any))
	firstParam["name"] = "wrong-name"
	params[0] = firstParam
	checkpoint["parameters"] = params
	symbols["javascript.workflow.checkpoint"] = checkpoint

	pipeline := cloneMap(t, symbols["javascript.pipeline"].(map[string]any))
	callback := cloneMap(t, pipeline["callback"].(map[string]any))
	callback["role"] = "wrong-role"
	pipeline["callback"] = callback
	symbols["javascript.pipeline"] = pipeline

	issues, err := jscatalog.CatalogCallBehaviorParityIssues(document, callbehavior.ProjectInstalledCallBehavior())
	if err != nil {
		t.Fatalf("CatalogCallBehaviorParityIssues() error = %v", err)
	}
	if len(issues) < 2 {
		t.Fatalf("CatalogCallBehaviorParityIssues() = %+v, want parameter and callback mismatches", issues)
	}
}

func TestCatalogCallBehaviorParityIssues_RejectsInvalidDocument(t *testing.T) {
	_, err := jscatalog.CatalogCallBehaviorParityIssues(map[string]any{}, callbehavior.ProjectInstalledCallBehavior())
	if err == nil {
		t.Fatal("CatalogCallBehaviorParityIssues() error = nil, want missing symbols error")
	}
	if !strings.Contains(err.Error(), "missing symbols") {
		t.Fatalf("CatalogCallBehaviorParityIssues() error = %q, want missing symbols", err)
	}
}

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

func TestCatalogCallBehaviorParityIssues_ReportsWorkflowFinalDeterminismDrift(t *testing.T) {
	raw, err := os.ReadFile(testutil.MustRepoPath(
		t,
		"contracts/testdata/javascript/invalid-representative-call-behavior-drift.json",
	))
	if err != nil {
		t.Fatalf("read parity drift fixture: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("unmarshal parity drift fixture: %v", err)
	}

	issues, err := jscatalog.CatalogCallBehaviorParityIssues(
		document,
		callbehavior.ProjectInstalledCallBehavior(),
	)
	if err != nil {
		t.Fatalf("CatalogCallBehaviorParityIssues() error = %v", err)
	}
	for _, issue := range issues {
		if issue.Code == "javascript.call_behavior.mismatch" &&
			issue.Path == "workflow.final" &&
			issue.Field == "determinism" {
			return
		}
	}
	t.Fatalf("issues = %+v, want workflow.final determinism mismatch", issues)
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

	raw, err := os.ReadFile(testutil.MustRepoPath(t, "contracts/javascript/runtime-api.json"))
	if err != nil {
		t.Fatalf("read authored catalog: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("unmarshal authored catalog: %v", err)
	}
	return document
}

func cloneSlice(t *testing.T, value []any) []any {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal slice: %v", err)
	}
	var cloned []any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		t.Fatalf("unmarshal cloned slice: %v", err)
	}
	return cloned
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
