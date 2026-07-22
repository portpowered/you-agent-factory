package contracts_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

func javascriptManifestFixtureRegistry(fixture string) contractvalidator.Registry {
	const (
		documentationID = "https://schemas.portpowered.com/you/contracts/common/documentation.schema.json"
		deprecationsID  = "https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json"
	)
	return contractvalidator.NewRegistry(contractvalidator.Entry{
		Family:        "javascript",
		FormatVersion: "1.0.0",
		Schemas: []contractvalidator.Schema{
			{ID: documentationID, Path: "contracts/common/documentation.schema.json"},
			{ID: deprecationsID, Path: "contracts/common/deprecations.schema.json"},
			{ID: runtimeManifestSchemaID, Path: "contracts/javascript/runtime-manifest.schema.json"},
		},
		Documents: []contractvalidator.Document{{
			Path:     fixture,
			SchemaID: runtimeManifestSchemaID,
		}},
	})
}

func TestJavaScriptRuntimeAPICatalogAgentRunParallelAndPipeline(t *testing.T) {
	t.Parallel()

	catalog := loadAuthoredJavaScriptRuntimeCatalog(t)
	symbolsByPath := catalogSymbolsByPath(t, catalog)

	identity := loadJavaScriptSymbolInventory(t)
	identityByPath := symbolRecordsByPath(identity)
	wantAgent, ok := identityByPath["agent"]
	if !ok {
		t.Fatal("identity baseline missing agent namespace")
	}

	agentSymbol, ok := symbolsByPath["agent"]
	if !ok {
		t.Fatal("catalog missing agent namespace")
	}
	if got := countCatalogPaths(symbolsByPath, "agent"); got != 1 {
		t.Fatalf("catalog path agent appears %d times, want exactly once", got)
	}
	assertCatalogNamespaceMatchesIdentityBaseline(t, "agent", agentSymbol, wantAgent)

	callInventory := loadJavaScriptCallBehaviorInventory(t)
	callByPath := callRecordsByPath(callInventory)
	wantAgentCallBehavior, ok := callByPath["agent"]
	if !ok {
		t.Fatal("call-behavior baseline missing agent namespace")
	}
	assertCatalogNamespaceMatchesCallBehaviorBaseline(t, "agent", agentSymbol, wantAgentCallBehavior)

	agentRunSymbol, ok := symbolsByPath["agent.run"]
	if !ok {
		t.Fatal("catalog missing agent.run")
	}
	if got := countCatalogPaths(symbolsByPath, "agent.run"); got != 1 {
		t.Fatalf("catalog path agent.run appears %d times, want exactly once", got)
	}

	wantAgentRunIdentity, ok := identityByPath["agent.run"]
	if !ok {
		t.Fatal("identity baseline missing agent.run")
	}
	assertCatalogMethodMatchesIdentityBaseline(t, "agent.run", agentRunSymbol, wantAgentRunIdentity)

	wantAgentRunCallBehavior, ok := callByPath["agent.run"]
	if !ok {
		t.Fatal("call-behavior baseline missing agent.run")
	}
	assertCatalogMethodMatchesCallBehaviorBaseline(t, "agent.run", agentRunSymbol, wantAgentRunCallBehavior)

	for _, path := range []string{"parallel", "pipeline"} {
		symbol, ok := symbolsByPath[path]
		if !ok {
			t.Fatalf("catalog missing async root helper at path %q", path)
		}
		if got := countCatalogPaths(symbolsByPath, path); got != 1 {
			t.Fatalf("catalog path %q appears %d times, want exactly once", path, got)
		}

		wantIdentity, ok := identityByPath[path]
		if !ok {
			t.Fatalf("identity baseline missing path %q", path)
		}
		assertCatalogCallableMatchesIdentityBaseline(t, path, symbol, wantIdentity)

		wantCallBehavior, ok := callByPath[path]
		if !ok {
			t.Fatalf("call-behavior baseline missing path %q", path)
		}
		assertCatalogAsyncCallableMatchesCallBehaviorBaseline(t, path, symbol, wantCallBehavior)
	}
}

func TestJavaScriptRuntimeAPICatalogWorkflowNamespaceAndMembers(t *testing.T) {
	t.Parallel()

	catalog := loadAuthoredJavaScriptRuntimeCatalog(t)
	symbolsByPath := catalogSymbolsByPath(t, catalog)

	identity := loadJavaScriptSymbolInventory(t)
	identityByPath := symbolRecordsByPath(identity)
	wantWorkflow, ok := identityByPath["workflow"]
	if !ok {
		t.Fatal("identity baseline missing workflow namespace")
	}

	workflowSymbol, ok := symbolsByPath["workflow"]
	if !ok {
		t.Fatal("catalog missing workflow namespace")
	}
	if got := countCatalogPaths(symbolsByPath, "workflow"); got != 1 {
		t.Fatalf("catalog path workflow appears %d times, want exactly once", got)
	}
	assertCatalogNamespaceMatchesIdentityBaseline(t, "workflow", workflowSymbol, wantWorkflow)

	callInventory := loadJavaScriptCallBehaviorInventory(t)
	callByPath := callRecordsByPath(callInventory)
	wantWorkflowCallBehavior, ok := callByPath["workflow"]
	if !ok {
		t.Fatal("call-behavior baseline missing workflow namespace")
	}
	assertCatalogNamespaceMatchesCallBehaviorBaseline(t, "workflow", workflowSymbol, wantWorkflowCallBehavior)

	workflowMembers := []string{
		"workflow.artifact",
		"workflow.budget",
		"workflow.checkpoint",
		"workflow.final",
		"workflow.log",
		"workflow.resumeState",
	}
	for _, path := range workflowMembers {
		symbol, ok := symbolsByPath[path]
		if !ok {
			t.Fatalf("catalog missing workflow member at path %q", path)
		}
		if got := countCatalogPaths(symbolsByPath, path); got != 1 {
			t.Fatalf("catalog path %q appears %d times, want exactly once", path, got)
		}

		wantIdentity, ok := identityByPath[path]
		if !ok {
			t.Fatalf("identity baseline missing path %q", path)
		}
		assertCatalogMethodMatchesIdentityBaseline(t, path, symbol, wantIdentity)

		wantCallBehavior, ok := callByPath[path]
		if !ok {
			t.Fatalf("call-behavior baseline missing path %q", path)
		}
		assertCatalogMethodMatchesCallBehaviorBaseline(t, path, symbol, wantCallBehavior)
	}
}

func TestJavaScriptRuntimeAPICatalogRepresentativeCallBehaviorParity(t *testing.T) {
	t.Parallel()

	catalog := loadAuthoredJavaScriptRuntimeCatalog(t)
	symbolsByPath := catalogSymbolsByPath(t, catalog)
	for _, want := range loadJavaScriptCallBehaviorInventory(t).Records {
		symbol, ok := symbolsByPath[want.Path]
		if !ok {
			t.Fatalf("catalog missing call-behavior path %q", want.Path)
		}
		switch want.Kind {
		case "namespace":
			assertCatalogNamespaceMatchesCallBehaviorBaseline(t, want.Path, symbol, want)
		case "value":
			assertCatalogValueMatchesCallBehaviorBaseline(t, want.Path, symbol, want)
		default:
			assertCatalogCallableMatchesCallBehaviorBaseline(t, want.Path, symbol, want)
		}
	}
}

func TestJavaScriptRuntimeAPICatalogRepresentativeCallBehaviorParityDrift(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "javascript", "invalid-representative-call-behavior-drift.json"))
	if err != nil {
		t.Fatalf("read parity drift fixture: %v", err)
	}
	var catalog map[string]any
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("unmarshal parity drift fixture: %v", err)
	}

	symbol := catalogSymbolsByPath(t, catalog)["workflow.final"]
	want, ok := callRecordsByPath(loadJavaScriptCallBehaviorInventory(t))["workflow.final"]
	if !ok {
		t.Fatal("call-behavior inventory missing workflow.final")
	}
	if got, _ := symbol["determinism"].(string); got == want.Determinism {
		t.Fatalf("workflow.final determinism = %q, want drift from committed value %q", got, want.Determinism)
	}
}

func TestJavaScriptRuntimeAPICatalogPathCompleteness(t *testing.T) {
	t.Parallel()

	catalog := loadAuthoredJavaScriptRuntimeCatalog(t)
	symbolsByPath := catalogSymbolsByPath(t, catalog)
	catalogPaths := make([]string, 0, len(symbolsByPath))
	for path := range symbolsByPath {
		catalogPaths = append(catalogPaths, path)
	}
	slices.Sort(catalogPaths)
	identityPaths := make([]string, 0, len(loadJavaScriptSymbolInventory(t).Symbols))
	for _, record := range loadJavaScriptSymbolInventory(t).Symbols {
		identityPaths = append(identityPaths, record.Path)
	}
	callPaths := make([]string, 0, len(loadJavaScriptCallBehaviorInventory(t).Records))
	for _, record := range loadJavaScriptCallBehaviorInventory(t).Records {
		callPaths = append(callPaths, record.Path)
	}
	if !slices.Equal(catalogPaths, identityPaths) {
		t.Fatalf("catalog paths = %q, want symbol inventory paths %q", catalogPaths, identityPaths)
	}
	if !slices.Equal(catalogPaths, callPaths) {
		t.Fatalf("catalog paths = %q, want call-behavior inventory paths %q", catalogPaths, callPaths)
	}
}

func TestJavaScriptRuntimeAPICatalogBrokenWorkflowParentMember(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	const fixture = "contracts/testdata/javascript/invalid-broken-workflow-parent-member.json"
	diagnostics := contractvalidator.Validate(root, javascriptManifestFixtureRegistry(fixture), "javascript", "1.0.0")
	if len(diagnostics) == 0 {
		t.Fatal("expected broken workflow parent/member diagnostics, got none")
	}

	wantChecks := []struct {
		code string
		path string
	}{
		{
			code: "javascript.parent.unresolved",
			path: "/symbols/javascript.workflow.checkpoint/parent",
		},
		{
			code: "javascript.member.unresolved",
			path: "/symbols/javascript.workflow/members/0",
		},
	}
	for _, want := range wantChecks {
		found := slices.ContainsFunc(diagnostics, func(diagnostic contractvalidator.Diagnostic) bool {
			return diagnostic.Code == want.code &&
				diagnostic.Path == want.path &&
				diagnostic.Document == fixture
		})
		if !found {
			t.Fatalf("diagnostics = %+v, want code=%q path=%q document=%q", diagnostics, want.code, want.path, fixture)
		}
	}
}

func TestJavaScriptRuntimeAPICatalogSyncRootHelpersLogAndPhase(t *testing.T) {
	t.Parallel()

	catalog := loadAuthoredJavaScriptRuntimeCatalog(t)
	symbolsByPath := catalogSymbolsByPath(t, catalog)

	for _, path := range []string{"log", "phase"} {
		symbol, ok := symbolsByPath[path]
		if !ok {
			t.Fatalf("catalog missing sync root helper at path %q", path)
		}
		if got := countCatalogPaths(symbolsByPath, path); got != 1 {
			t.Fatalf("catalog path %q appears %d times, want exactly once", path, got)
		}

		identity := loadJavaScriptSymbolInventory(t)
		identityByPath := symbolRecordsByPath(identity)
		wantIdentity, ok := identityByPath[path]
		if !ok {
			t.Fatalf("identity baseline missing path %q", path)
		}
		assertCatalogCallableMatchesIdentityBaseline(t, path, symbol, wantIdentity)

		callInventory := loadJavaScriptCallBehaviorInventory(t)
		callByPath := callRecordsByPath(callInventory)
		wantCallBehavior, ok := callByPath[path]
		if !ok {
			t.Fatalf("call-behavior baseline missing path %q", path)
		}
		assertCatalogCallableMatchesCallBehaviorBaseline(t, path, symbol, wantCallBehavior)
	}
}

func TestJavaScriptRuntimeAPICatalogRootValuesArgsAndMeta(t *testing.T) {
	t.Parallel()

	catalog := loadAuthoredJavaScriptRuntimeCatalog(t)
	symbolsByPath := catalogSymbolsByPath(t, catalog)

	for _, path := range []string{"args", "meta"} {
		symbol, ok := symbolsByPath[path]
		if !ok {
			t.Fatalf("catalog missing root value symbol at path %q", path)
		}
		if got := countCatalogPaths(symbolsByPath, path); got != 1 {
			t.Fatalf("catalog path %q appears %d times, want exactly once", path, got)
		}

		identity := loadJavaScriptSymbolInventory(t)
		identityByPath := symbolRecordsByPath(identity)
		wantIdentity, ok := identityByPath[path]
		if !ok {
			t.Fatalf("identity baseline missing path %q", path)
		}
		assertCatalogValueMatchesIdentityBaseline(t, path, symbol, wantIdentity)

		callInventory := loadJavaScriptCallBehaviorInventory(t)
		callByPath := callRecordsByPath(callInventory)
		wantCallBehavior, ok := callByPath[path]
		if !ok {
			t.Fatalf("call-behavior baseline missing path %q", path)
		}
		assertCatalogValueMatchesCallBehaviorBaseline(t, path, symbol, wantCallBehavior)
	}
}

func TestJavaScriptRuntimeAPIAuthoredCatalogBoundary(t *testing.T) {
	t.Parallel()

	catalogPath := filepath.Join("javascript", "runtime-api.json")
	if _, err := os.Stat(catalogPath); err != nil {
		t.Fatalf("contracts/javascript/runtime-api.json must exist as the authored catalog: %v", err)
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	diagnostics := contractvalidator.Validate(root, contractvalidator.JavaScriptRegistry(), "javascript", "1.0.0")
	if len(diagnostics) != 0 {
		t.Fatalf("authored catalog validation diagnostics = %+v", diagnostics)
	}
}

func TestJavaScriptRuntimeAPICatalogContainsNoUnsupportedSurfaces(t *testing.T) {
	t.Parallel()

	catalog := loadAuthoredJavaScriptRuntimeCatalog(t)
	byPath := catalogSymbolsByPath(t, catalog)

	forbiddenRoots := []string{"context", "orchestrator"}
	comparisonHelpers := []string{"workflow.sleep", "agent.verify", "agent.parallel"}
	for path := range byPath {
		for _, forbidden := range forbiddenRoots {
			if path == forbidden || strings.HasPrefix(path, forbidden+".") {
				t.Fatalf("authored catalog documents forbidden global path %q", path)
			}
		}
		if slices.Contains(comparisonHelpers, path) {
			t.Fatalf("authored catalog documents comparison-project-only helper path %q", path)
		}
		if path == "agent" {
			if symbol, ok := byPath[path]; ok {
				if kind, _ := symbol["kind"].(string); kind == "function" {
					t.Fatalf("authored catalog documents comparison-project-only callable agent global at path %q", path)
				}
			}
		}
	}
}

func TestJavaScriptRuntimeAPICatalogRejectsUnsupportedSurfaceFixtures(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}

	tests := []struct {
		name     string
		fixture  string
		wantCode string
		wantPath string
	}{
		{
			name:     "context global",
			fixture:  "contracts/testdata/javascript/invalid-unsupported-context-global.json",
			wantCode: "javascript.surface.forbidden_global",
			wantPath: "/symbols/example.context/path",
		},
		{
			name:     "orchestrator global",
			fixture:  "contracts/testdata/javascript/invalid-unsupported-orchestrator-global.json",
			wantCode: "javascript.surface.forbidden_global",
			wantPath: "/symbols/example.orchestrator/path",
		},
		{
			name:     "comparison-project helper",
			fixture:  "contracts/testdata/javascript/invalid-unsupported-comparison-helper.json",
			wantCode: "javascript.surface.unsupported_helper",
			wantPath: "/symbols/example.workflow.sleep/path",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnostics := contractvalidator.Validate(root, javascriptManifestFixtureRegistry(test.fixture), "javascript", "1.0.0")
			if len(diagnostics) == 0 {
				t.Fatal("expected unsupported-surface diagnostics, got none")
			}
			found := false
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == test.wantCode && diagnostic.Path == test.wantPath && diagnostic.Document == test.fixture {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("diagnostics = %+v, want code=%q path=%q document=%q", diagnostics, test.wantCode, test.wantPath, test.fixture)
			}
		})
	}
}

func TestJavaScriptStagedRuntimeAPIMatchesAuthoredCatalog(t *testing.T) {
	t.Parallel()

	authored, err := os.ReadFile(filepath.Join("javascript", "runtime-api.json"))
	if err != nil {
		t.Fatalf("read authored catalog: %v", err)
	}
	staged, err := os.ReadFile(filepath.Join("..", "packages", "api", "generated", "javascript", "runtime-api.json"))
	if err != nil {
		t.Fatalf("read staged runtime-api projection: %v", err)
	}
	if !bytes.Equal(authored, staged) {
		t.Fatal("staged packages/api/generated/javascript/runtime-api.json must be a byte-identical copy of contracts/javascript/runtime-api.json")
	}
}

func TestJavaScriptRuntimeAPIBrokenSharedSchemaReference(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	const (
		fixture  = "contracts/testdata/javascript/invalid-broken-shared-schema-ref.json"
		wantPath = "/symbols/javascript.workflow.checkpoint/parameters/0/serializableValue/$ref"
	)
	diagnostics := contractvalidator.Validate(root, javascriptManifestFixtureRegistry(fixture), "javascript", "1.0.0")
	if len(diagnostics) == 0 {
		t.Fatal("expected broken shared-schema reference diagnostics, got none")
	}
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "reference.fragment" && diagnostic.Path == wantPath && diagnostic.Document == fixture {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v, want code=reference.fragment path=%q document=%q", diagnostics, wantPath, fixture)
	}
}

func TestJavaScriptRuntimeAPIOpenSharedSchema(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	const (
		fixture  = "contracts/testdata/javascript/invalid-open-shared-schema.json"
		wantPath = "/sharedSchemas/javascript.schema.open_object/schema"
	)
	diagnostics := contractvalidator.Validate(root, javascriptManifestFixtureRegistry(fixture), "javascript", "1.0.0")
	if len(diagnostics) == 0 {
		t.Fatal("expected open shared-schema diagnostics, got none")
	}
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "javascript.serializable_value.open" && diagnostic.Path == wantPath && diagnostic.Document == fixture {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v, want code=javascript.serializable_value.open path=%q document=%q", diagnostics, wantPath, fixture)
	}
}

func loadAuthoredJavaScriptRuntimeCatalog(t *testing.T) map[string]any {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("javascript", "runtime-api.json"))
	if err != nil {
		t.Fatalf("read authored catalog: %v", err)
	}
	var catalog map[string]any
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("unmarshal authored catalog: %v", err)
	}
	return catalog
}

func catalogSymbolsByPath(t *testing.T, catalog map[string]any) map[string]map[string]any {
	t.Helper()

	symbolsValue, ok := catalog["symbols"].(map[string]any)
	if !ok {
		t.Fatalf("catalog symbols = %#v, want object", catalog["symbols"])
	}
	byPath := make(map[string]map[string]any, len(symbolsValue))
	for _, symbolValue := range symbolsValue {
		symbol, ok := symbolValue.(map[string]any)
		if !ok {
			t.Fatalf("catalog symbol = %#v, want object", symbolValue)
		}
		path, _ := symbol["path"].(string)
		if path == "" {
			t.Fatal("catalog symbol missing path")
		}
		byPath[path] = symbol
	}
	return byPath
}

func countCatalogPaths(byPath map[string]map[string]any, path string) int {
	count := 0
	for candidate := range byPath {
		if candidate == path {
			count++
		}
	}
	return count
}

func symbolRecordsByPath(inventory javascriptSymbolInventory) map[string]javascriptSymbolRecord {
	byPath := make(map[string]javascriptSymbolRecord, len(inventory.Symbols))
	for _, record := range inventory.Symbols {
		byPath[record.Path] = record
	}
	return byPath
}

func callRecordsByPath(inventory javascriptCallBehaviorInventory) map[string]javascriptCallBehaviorRecord {
	byPath := make(map[string]javascriptCallBehaviorRecord, len(inventory.Records))
	for _, record := range inventory.Records {
		byPath[record.Path] = record
	}
	return byPath
}

func assertCatalogValueMatchesIdentityBaseline(
	t *testing.T,
	path string,
	symbol map[string]any,
	want javascriptSymbolRecord,
) {
	t.Helper()

	if got, _ := symbol["id"].(string); got != "javascript."+path {
		t.Fatalf("catalog %q id = %q, want %q", path, got, "javascript."+path)
	}
	if got, _ := symbol["name"].(string); got != want.Name {
		t.Fatalf("catalog %q name = %q, want %q", path, got, want.Name)
	}
	if got, _ := symbol["path"].(string); got != want.Path {
		t.Fatalf("catalog %q path = %q, want %q", path, got, want.Path)
	}
	if got, _ := symbol["kind"].(string); got != want.Kind {
		t.Fatalf("catalog %q kind = %q, want %q", path, got, want.Kind)
	}
	if parent, ok := symbol["parent"].(string); ok && parent != "" {
		t.Fatalf("catalog %q parent = %q, want no parent", path, parent)
	}
	if members, ok := symbol["members"].([]any); ok && len(members) > 0 {
		t.Fatalf("catalog %q members = %#v, want no members", path, members)
	}
	assertCatalogValueHasDocumentationExample(t, path, symbol)
}

func assertCatalogValueMatchesCallBehaviorBaseline(
	t *testing.T,
	path string,
	symbol map[string]any,
	want javascriptCallBehaviorRecord,
) {
	t.Helper()

	if got, _ := symbol["mutability"].(string); got != want.Mutability {
		t.Fatalf("catalog %q mutability = %q, want %q", path, got, want.Mutability)
	}
	if got, _ := symbol["nullability"].(string); got != want.Nullability {
		t.Fatalf("catalog %q nullability = %q, want %q", path, got, want.Nullability)
	}
	if got, _ := symbol["bindingLifecycle"].(string); got != want.Lifecycle {
		t.Fatalf("catalog %q bindingLifecycle = %q, want %q", path, got, want.Lifecycle)
	}
}

func assertCatalogCallableMatchesIdentityBaseline(
	t *testing.T,
	path string,
	symbol map[string]any,
	want javascriptSymbolRecord,
) {
	t.Helper()

	if got, _ := symbol["id"].(string); got != "javascript."+path {
		t.Fatalf("catalog %q id = %q, want %q", path, got, "javascript."+path)
	}
	if got, _ := symbol["name"].(string); got != want.Name {
		t.Fatalf("catalog %q name = %q, want %q", path, got, want.Name)
	}
	if got, _ := symbol["path"].(string); got != want.Path {
		t.Fatalf("catalog %q path = %q, want %q", path, got, want.Path)
	}
	if got, _ := symbol["kind"].(string); got != want.Kind {
		t.Fatalf("catalog %q kind = %q, want %q", path, got, want.Kind)
	}
	if parent, ok := symbol["parent"].(string); ok && parent != "" {
		t.Fatalf("catalog %q parent = %q, want no parent", path, parent)
	}
	if members, ok := symbol["members"].([]any); ok && len(members) > 0 {
		t.Fatalf("catalog %q members = %#v, want no members", path, members)
	}
	assertCatalogValueHasDocumentationExample(t, path, symbol)
}

func assertCatalogCallableMatchesCallBehaviorBaseline(
	t *testing.T,
	path string,
	symbol map[string]any,
	want javascriptCallBehaviorRecord,
) {
	t.Helper()

	if got, ok := symbol["async"].(bool); !ok || got != want.Async {
		t.Fatalf("catalog %q async = %#v, want %v", path, symbol["async"], want.Async)
	}
	assertCatalogCallableParametersMatchBaseline(t, path, symbol, want.Parameters)
	assertCatalogCallableReturnMatchesBaseline(t, path, symbol, want.Return)
	assertCatalogCallableEmittedRecordsMatchBaseline(t, path, symbol, want.EmittedRecords)
	assertCatalogCallableErrorsMatchBaseline(t, path, symbol, want.Errors)
}

func assertCatalogAsyncCallableMatchesCallBehaviorBaseline(
	t *testing.T,
	path string,
	symbol map[string]any,
	want javascriptCallBehaviorRecord,
) {
	t.Helper()

	assertCatalogCallableMatchesCallBehaviorBaseline(t, path, symbol, want)
	assertCatalogCallbackMatchesBaseline(t, path, symbol, want.Callback)
	assertCatalogPolicyChecksMatchBaseline(t, path, symbol, want.PolicyChecks)
	assertCatalogDeterminismMatchesBaseline(t, path, symbol, want.Determinism)
	assertCatalogResumeNotesMatchBaseline(t, path, symbol, want.ResumeNotes)
}

func assertCatalogCallableParametersMatchBaseline(
	t *testing.T,
	path string,
	symbol map[string]any,
	want []javascriptParameter,
) {
	t.Helper()

	paramsValue, ok := symbol["parameters"].([]any)
	if !ok {
		t.Fatalf("catalog %q parameters = %#v, want array", path, symbol["parameters"])
	}
	if len(paramsValue) != len(want) {
		t.Fatalf("catalog %q parameter count = %d, want %d", path, len(paramsValue), len(want))
	}
	for i, wantParam := range want {
		param, ok := paramsValue[i].(map[string]any)
		if !ok {
			t.Fatalf("catalog %q parameters[%d] = %#v, want object", path, i, paramsValue[i])
		}
		if got, _ := param["position"].(float64); int(got) != i {
			t.Fatalf("catalog %q parameters[%d].position = %v, want %d", path, i, param["position"], i)
		}
		if got, _ := param["name"].(string); got != wantParam.Name {
			t.Fatalf("catalog %q parameters[%d].name = %q, want %q", path, i, got, wantParam.Name)
		}
		if got, _ := param["required"].(bool); got != wantParam.Required {
			t.Fatalf("catalog %q parameters[%d].required = %v, want %v", path, i, got, wantParam.Required)
		}
		if got, _ := param["type"].(string); got != wantParam.Type {
			t.Fatalf("catalog %q parameters[%d].type = %q, want %q", path, i, got, wantParam.Type)
		}
		if wantParam.Rest {
			if got, _ := param["rest"].(bool); !got {
				t.Fatalf("catalog %q parameters[%d].rest = %v, want true", path, i, param["rest"])
			}
		}
		if wantParam.Default != "" {
			if got, _ := param["default"].(string); got != wantParam.Default {
				t.Fatalf("catalog %q parameters[%d].default = %q, want %q", path, i, got, wantParam.Default)
			}
		}
	}
}

func assertCatalogCallableReturnMatchesBaseline(
	t *testing.T,
	path string,
	symbol map[string]any,
	want *javascriptReturnBehavior,
) {
	t.Helper()

	if want == nil {
		t.Fatalf("call-behavior baseline %q missing return metadata", path)
	}
	returnValue, ok := symbol["return"].(map[string]any)
	if !ok {
		t.Fatalf("catalog %q return = %#v, want object", path, symbol["return"])
	}
	if want.Async {
		if got, _ := returnValue["kind"].(string); got != "promise" {
			t.Fatalf("catalog %q return.kind = %q, want promise", path, got)
		}
		if got, _ := returnValue["type"].(string); got != want.PromiseType {
			t.Fatalf("catalog %q return.type = %q, want %q", path, got, want.PromiseType)
		}
		return
	}
	if got, _ := returnValue["kind"].(string); got != "sync" {
		t.Fatalf("catalog %q return.kind = %q, want sync", path, got)
	}
	if got, _ := returnValue["type"].(string); got != want.SyncType {
		t.Fatalf("catalog %q return.type = %q, want %q", path, got, want.SyncType)
	}
}

func assertCatalogCallableEmittedRecordsMatchBaseline(
	t *testing.T,
	path string,
	symbol map[string]any,
	want []string,
) {
	t.Helper()

	if len(want) == 0 {
		return
	}
	gotValue, ok := symbol["emittedRecords"].([]any)
	if !ok {
		t.Fatalf("catalog %q emittedRecords = %#v, want array", path, symbol["emittedRecords"])
	}
	got := make([]string, 0, len(gotValue))
	for _, item := range gotValue {
		record, ok := item.(string)
		if !ok {
			t.Fatalf("catalog %q emittedRecords item = %#v, want string", path, item)
		}
		got = append(got, record)
	}
	if len(got) != len(want) {
		t.Fatalf("catalog %q emittedRecords = %v, want %v", path, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("catalog %q emittedRecords[%d] = %q, want %q", path, i, got[i], want[i])
		}
	}
}

func assertCatalogCallableErrorsMatchBaseline(
	t *testing.T,
	path string,
	symbol map[string]any,
	want []javascriptErrorCase,
) {
	t.Helper()

	if len(want) == 0 {
		return
	}
	gotValue, ok := symbol["errors"].([]any)
	if !ok {
		t.Fatalf("catalog %q errors = %#v, want array", path, symbol["errors"])
	}
	if len(gotValue) != len(want) {
		t.Fatalf("catalog %q error count = %d, want %d", path, len(gotValue), len(want))
	}
	for i, wantErr := range want {
		errValue, ok := gotValue[i].(map[string]any)
		if !ok {
			t.Fatalf("catalog %q errors[%d] = %#v, want object", path, i, gotValue[i])
		}
		if got, _ := errValue["condition"].(string); got != wantErr.Condition {
			t.Fatalf("catalog %q errors[%d].condition = %q, want %q", path, i, got, wantErr.Condition)
		}
		if got, _ := errValue["type"].(string); got != wantErr.Type {
			t.Fatalf("catalog %q errors[%d].type = %q, want %q", path, i, got, wantErr.Type)
		}
		if got, _ := errValue["message"].(string); got != wantErr.Message {
			t.Fatalf("catalog %q errors[%d].message = %q, want %q", path, i, got, wantErr.Message)
		}
	}
}

func assertCatalogCallbackMatchesBaseline(
	t *testing.T,
	path string,
	symbol map[string]any,
	want *javascriptCallbackShape,
) {
	t.Helper()

	if want == nil {
		if callbackValue, ok := symbol["callback"]; ok && callbackValue != nil {
			t.Fatalf("catalog %q callback = %#v, want no callback metadata", path, callbackValue)
		}
		return
	}
	callbackValue, ok := symbol["callback"].(map[string]any)
	if !ok {
		t.Fatalf("catalog %q callback = %#v, want object", path, symbol["callback"])
	}
	if got, _ := callbackValue["role"].(string); got != want.Role {
		t.Fatalf("catalog %q callback.role = %q, want %q", path, got, want.Role)
	}
	if want.Notes != "" {
		if got, _ := callbackValue["notes"].(string); got != want.Notes {
			t.Fatalf("catalog %q callback.notes = %q, want %q", path, got, want.Notes)
		}
	}
	assertCatalogCallableParametersMatchBaseline(t, path+".callback", callbackValue, want.Parameters)
}

func assertCatalogNamespaceMatchesIdentityBaseline(
	t *testing.T,
	path string,
	symbol map[string]any,
	want javascriptSymbolRecord,
) {
	t.Helper()

	if got, _ := symbol["id"].(string); got != "javascript."+path {
		t.Fatalf("catalog %q id = %q, want %q", path, got, "javascript."+path)
	}
	if got, _ := symbol["name"].(string); got != want.Name {
		t.Fatalf("catalog %q name = %q, want %q", path, got, want.Name)
	}
	if got, _ := symbol["path"].(string); got != want.Path {
		t.Fatalf("catalog %q path = %q, want %q", path, got, want.Path)
	}
	if got, _ := symbol["kind"].(string); got != want.Kind {
		t.Fatalf("catalog %q kind = %q, want %q", path, got, want.Kind)
	}
	if parent, ok := symbol["parent"].(string); ok && parent != "" {
		t.Fatalf("catalog %q parent = %q, want no parent", path, parent)
	}
	membersValue, ok := symbol["members"].([]any)
	if !ok {
		t.Fatalf("catalog %q members = %#v, want array", path, symbol["members"])
	}
	gotMembers := make([]string, 0, len(membersValue))
	for _, memberValue := range membersValue {
		member, ok := memberValue.(string)
		if !ok {
			t.Fatalf("catalog %q members item = %#v, want string", path, memberValue)
		}
		gotMembers = append(gotMembers, member)
	}
	if len(gotMembers) != len(want.Members) {
		t.Fatalf("catalog %q member count = %d, want %d", path, len(gotMembers), len(want.Members))
	}
	for i := range want.Members {
		if gotMembers[i] != want.Members[i] {
			t.Fatalf("catalog %q members[%d] = %q, want %q", path, i, gotMembers[i], want.Members[i])
		}
	}
	assertCatalogValueHasDocumentationExample(t, path, symbol)
}

func assertCatalogNamespaceMatchesCallBehaviorBaseline(
	t *testing.T,
	path string,
	symbol map[string]any,
	want javascriptCallBehaviorRecord,
) {
	t.Helper()

	if got, _ := symbol["mutability"].(string); got != want.Mutability {
		t.Fatalf("catalog %q mutability = %q, want %q", path, got, want.Mutability)
	}
	if got, _ := symbol["nullability"].(string); got != want.Nullability {
		t.Fatalf("catalog %q nullability = %q, want %q", path, got, want.Nullability)
	}
	if got, _ := symbol["bindingLifecycle"].(string); got != want.Lifecycle {
		t.Fatalf("catalog %q bindingLifecycle = %q, want %q", path, got, want.Lifecycle)
	}
}

func catalogExpectedSymbolID(path string) string {
	if path == "workflow.resumeState" {
		return "javascript.workflow.resume-state"
	}
	return "javascript." + path
}

func assertCatalogMethodMatchesIdentityBaseline(
	t *testing.T,
	path string,
	symbol map[string]any,
	want javascriptSymbolRecord,
) {
	t.Helper()

	wantID := catalogExpectedSymbolID(path)
	if got, _ := symbol["id"].(string); got != wantID {
		t.Fatalf("catalog %q id = %q, want %q", path, got, wantID)
	}
	if got, _ := symbol["name"].(string); got != want.Name {
		t.Fatalf("catalog %q name = %q, want %q", path, got, want.Name)
	}
	if got, _ := symbol["path"].(string); got != want.Path {
		t.Fatalf("catalog %q path = %q, want %q", path, got, want.Path)
	}
	if got, _ := symbol["kind"].(string); got != "method" {
		t.Fatalf("catalog %q kind = %q, want method", path, got)
	}
	wantParent := "javascript." + want.Parent
	if got, _ := symbol["parent"].(string); got != wantParent {
		t.Fatalf("catalog %q parent = %q, want %q", path, got, wantParent)
	}
	if members, ok := symbol["members"].([]any); ok && len(members) > 0 {
		t.Fatalf("catalog %q members = %#v, want no members", path, members)
	}
	assertCatalogValueHasDocumentationExample(t, path, symbol)
}

func assertCatalogMethodMatchesCallBehaviorBaseline(
	t *testing.T,
	path string,
	symbol map[string]any,
	want javascriptCallBehaviorRecord,
) {
	t.Helper()

	if got, ok := symbol["async"].(bool); !ok || got != want.Async {
		t.Fatalf("catalog %q async = %#v, want %v", path, symbol["async"], want.Async)
	}
	assertCatalogCallableParametersMatchBaseline(t, path, symbol, want.Parameters)
	assertCatalogCallableReturnMatchesBaseline(t, path, symbol, want.Return)
	assertCatalogCallableEmittedRecordsMatchBaseline(t, path, symbol, want.EmittedRecords)
	assertCatalogCallableErrorsMatchBaseline(t, path, symbol, want.Errors)
	assertCatalogPolicyChecksMatchBaseline(t, path, symbol, want.PolicyChecks)
	assertCatalogDeterminismMatchesBaseline(t, path, symbol, want.Determinism)
	assertCatalogResumeNotesMatchBaseline(t, path, symbol, want.ResumeNotes)
}

func assertCatalogPolicyChecksMatchBaseline(
	t *testing.T,
	path string,
	symbol map[string]any,
	want []javascriptPolicyCheck,
) {
	t.Helper()

	if len(want) == 0 {
		return
	}
	gotValue, ok := symbol["policyChecks"].([]any)
	if !ok {
		t.Fatalf("catalog %q policyChecks = %#v, want array", path, symbol["policyChecks"])
	}
	if len(gotValue) != len(want) {
		t.Fatalf("catalog %q policyChecks count = %d, want %d", path, len(gotValue), len(want))
	}
	for i, wantCheck := range want {
		checkValue, ok := gotValue[i].(map[string]any)
		if !ok {
			t.Fatalf("catalog %q policyChecks[%d] = %#v, want object", path, i, gotValue[i])
		}
		if got, _ := checkValue["kind"].(string); got != wantCheck.Kind {
			t.Fatalf("catalog %q policyChecks[%d].kind = %q, want %q", path, i, got, wantCheck.Kind)
		}
		if wantCheck.Field != "" {
			if got, _ := checkValue["field"].(string); got != wantCheck.Field {
				t.Fatalf("catalog %q policyChecks[%d].field = %q, want %q", path, i, got, wantCheck.Field)
			}
		}
		if wantCheck.Message != "" {
			if got, _ := checkValue["message"].(string); got != wantCheck.Message {
				t.Fatalf("catalog %q policyChecks[%d].message = %q, want %q", path, i, got, wantCheck.Message)
			}
		}
	}
}

func assertCatalogDeterminismMatchesBaseline(t *testing.T, path string, symbol map[string]any, want string) {
	t.Helper()

	if want == "" {
		return
	}
	if got, _ := symbol["determinism"].(string); got != want {
		t.Fatalf("catalog %q determinism = %q, want %q", path, got, want)
	}
}

func assertCatalogResumeNotesMatchBaseline(t *testing.T, path string, symbol map[string]any, want string) {
	t.Helper()

	if want == "" {
		return
	}
	if got, _ := symbol["resumeNotes"].(string); got != want {
		t.Fatalf("catalog %q resumeNotes = %q, want %q", path, got, want)
	}
}

func assertCatalogValueHasDocumentationExample(t *testing.T, path string, symbol map[string]any) {
	t.Helper()

	documentation, ok := symbol["documentation"].(map[string]any)
	if !ok {
		t.Fatalf("catalog %q documentation = %#v, want object", path, symbol["documentation"])
	}
	examples, ok := documentation["examples"].([]any)
	if !ok || len(examples) == 0 {
		t.Fatalf("catalog %q documentation.examples = %#v, want at least one example", path, documentation["examples"])
	}
	first, ok := examples[0].(string)
	if !ok || first == "" {
		t.Fatalf("catalog %q first documentation example = %#v, want non-empty string", path, examples[0])
	}
}
