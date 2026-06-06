package interfaces

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractguard"
)

var approvedWorkDispatchFields = map[string]string{
	"DispatchID":               "dispatch_id",
	"TransitionID":             "transition_id",
	"WorkerType":               "worker_type",
	"WorkstationName":          "workstation_name",
	"ProjectID":                "project_id",
	"CurrentChainingTraceID":   "current_chaining_trace_id",
	"PreviousChainingTraceIDs": "previous_chaining_trace_ids",
	"Execution":                "execution",
	"InputTokens":              "input_tokens",
	"InputBindings":            "input_bindings",
}

var retiredWorkDispatchWorkerFields = []string{
	"WorkstationType",
	"SystemPrompt",
	"UserMessage",
	"OutputSchema",
	"EnvVars",
	"Worktree",
	"WorkingDirectory",
	"Model",
	"ModelProvider",
	"Provider",
	"SessionID",
	"Command",
	"Args",
	"Stdin",
	"Env",
	"WorkDir",
}

func TestWorkDispatchContractGuard_FieldInventoryStaysDispatchOwned(t *testing.T) {
	t.Parallel()

	workDispatchType := reflect.TypeOf(WorkDispatch{})
	seen := make(map[string]struct{}, workDispatchType.NumField())
	for i := 0; i < workDispatchType.NumField(); i++ {
		field := workDispatchType.Field(i)
		wantJSONTag, ok := approvedWorkDispatchFields[field.Name]
		if !ok {
			t.Fatalf("WorkDispatch field %s is not in the approved dispatch-owned inventory; update the split-contract artifact and this guard before expanding the canonical dispatch payload", field.Name)
		}
		gotJSONTag := strings.Split(field.Tag.Get("json"), ",")[0]
		if gotJSONTag != wantJSONTag {
			t.Fatalf("WorkDispatch field %s json tag = %q, want %q", field.Name, gotJSONTag, wantJSONTag)
		}
		seen[field.Name] = struct{}{}
	}

	for fieldName := range approvedWorkDispatchFields {
		if _, ok := seen[fieldName]; !ok {
			t.Fatalf("WorkDispatch is missing approved field %s", fieldName)
		}
	}
}

func TestWorkDispatchContractGuard_WorkerOwnedFieldsStayDeleted(t *testing.T) {
	t.Parallel()

	workDispatchType := reflect.TypeOf(WorkDispatch{})
	for _, fieldName := range retiredWorkDispatchWorkerFields {
		if _, ok := workDispatchType.FieldByName(fieldName); ok {
			t.Fatalf("WorkDispatch must not reintroduce worker-owned field %s", fieldName)
		}
	}
}

var retiredFactoryBoundaryMirrorNames = []string{
	"FactoryWorldWorkstationRequestView",
	"FactoryWorldWorkstationRequestCountView",
	"FactoryWorldWorkstationRequestRequestView",
	"FactoryWorldWorkstationRequestResponseView",
	"FactoryWorldTokenView",
	"FactoryWorldMutationView",
}

var retiredFactoryCanonicalMirrorNames = []string{
	"FactoryProviderFailure",
	"FactoryProviderSession",
	"FactoryWorkDiagnostics",
	"FactoryRenderedPromptDiagnostic",
	"FactoryProviderDiagnostic",
	"FactoryEnabledTransitionView",
	"FactoryFiringDecisionView",
	"FactoryWorldDispatchView",
	"FactoryWorldProviderSessionView",
	"FactoryWorldInferenceAttemptView",
}

var approvedBoundaryViews = map[string]struct{}{
	"FactoryWorldView":         {},
	"FactoryWorldTopologyView": {},
	"FactoryWorldRuntimeView":  {},
}

var retiredSimpleDashboardAggregateSeamNames = []string{
	"FactoryWorldView",
	"FactoryWorldTopologyView",
	"FactoryWorldRuntimeView",
}

func TestFactoryWorldContractGuard_RetiredMirrorTypesStayDeleted(t *testing.T) {
	t.Parallel()

	forbidden := toStringSet(allRetiredFactoryMirrorNames())

	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob interface package files: %v", err)
	}

	fset := token.NewFileSet()
	for _, path := range paths {
		if filepath.Ext(path) != ".go" || filepath.Base(path) == "world_view_contract_guard_test.go" {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				typeName := typeSpec.Name.Name
				if _, blocked := forbidden[typeName]; blocked {
					t.Fatalf("%s reintroduces retired mirror type %s", path, typeName)
				}
				if !strings.HasPrefix(typeName, "FactoryWorld") || !strings.HasSuffix(typeName, "View") {
					continue
				}
				if _, approved := approvedBoundaryViews[typeName]; !approved {
					t.Fatalf("%s introduces unapproved FactoryWorld*View mirror %s; update docs/development/world-view-contract-cleanup-data-model.md and this allowlist before adding new boundary-only views", path, typeName)
				}
			}
		}
	}
}

func TestFactoryWorldContractGuard_RetiredBoundaryMirrorNamesStayOutOfInterfacesGoFiles(t *testing.T) {
	t.Parallel()

	names := append([]string(nil), retiredFactoryBoundaryMirrorNames...)
	sort.Strings(names)
	patterns := make([]string, 0, len(names))
	for _, name := range names {
		patterns = append(patterns, regexp.QuoteMeta(name))
	}
	matcher := regexp.MustCompile(`\b(?:` + strings.Join(patterns, "|") + `)\b`)
	allowed := map[string]struct{}{
		filepath.Clean("interfaces/world_view_contract_guard_test.go"): {},
	}

	err := walkWorldViewMirrorFiles(".", "interfaces", false, allowed, func(path, rel string) error {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if match := matcher.FindString(string(data)); match != "" {
			t.Fatalf("%s still contains retired boundary mirror name %q; keep API-owned workstation-request, token, and mutation DTOs out of pkg/interfaces", rel, match)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan interface package go files: %v", err)
	}
}

func TestFactoryWorldContractGuard_RetiredCanonicalMirrorNamesStayOutOfPkgGoFiles(t *testing.T) {
	t.Parallel()

	names := append([]string(nil), retiredFactoryCanonicalMirrorNames...)
	sort.Strings(names)
	patterns := make([]string, 0, len(names))
	for _, name := range names {
		patterns = append(patterns, regexp.QuoteMeta(name))
	}
	matcher := regexp.MustCompile(`\b(?:` + strings.Join(patterns, "|") + `)\b`)
	allowed := map[string]struct{}{
		filepath.Clean("interfaces/world_view_contract_guard_test.go"): {},
	}

	err := walkWorldViewMirrorFiles("..", "", true, allowed, func(path, rel string) error {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if match := matcher.FindString(string(data)); match != "" {
			t.Fatalf("%s still contains retired mirror name %q; equivalent rg guard is `rg -n %q pkg -g \"*.go\"` from the repository root and should only hit approved guard notes", rel, match, strings.Join(names, "|"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan pkg go files: %v", err)
	}
}

func TestFactoryWorldContractGuard_SkipsHiddenAndGeneratedDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorldViewGuardFixture(t, root, ".hidden/hidden.go", `package hidden

type Hidden struct{}

var _ = "FactoryProviderFailure"
`)
	writeWorldViewGuardFixture(t, root, "api/generated/generated.go", `package generated

var _ = "FactoryProviderFailure"
`)
	writeWorldViewGuardFixture(t, root, "feature/kept.go", `package feature

var _ = "FactoryProviderFailure"
`)

	allowed := map[string]struct{}{}
	var visited []string
	if err := walkWorldViewMirrorFiles(root, "", true, allowed, func(path, rel string) error {
		visited = append(visited, rel)
		return nil
	}); err != nil {
		t.Fatalf("walk world view mirror files: %v", err)
	}
	if len(visited) != 1 || visited[0] != filepath.Clean("feature/kept.go") {
		t.Fatalf("visited = %v, want only handwritten source file %s", visited, filepath.Clean("feature/kept.go"))
	}
}

func TestFactoryWorldContractGuard_RuntimeShellUsesCanonicalSelectedTickTypes(t *testing.T) {
	t.Parallel()

	topologyType := reflect.TypeOf(FactoryWorldTopologyView{})
	assertWorldViewSliceType(t, topologyType, "SubmitWorkTypes", reflect.TypeOf(FactoryWorldSubmitWorkType{}))

	runtimeType := reflect.TypeOf(FactoryWorldRuntimeView{})
	assertWorldViewFieldType(t, runtimeType, "InferenceAttemptsByDispatchID", reflect.TypeOf(map[string]map[string]FactoryWorldInferenceAttempt{}))
	if _, ok := runtimeType.FieldByName("WorkstationRequestsByDispatchID"); ok {
		t.Fatal("FactoryWorldRuntimeView must not retain the API-owned workstation_requests_by_dispatch_id projection")
	}

	sessionField, ok := runtimeType.FieldByName("Session")
	if !ok {
		t.Fatalf("FactoryWorldRuntimeView missing Session field")
	}
	if sessionField.Type != reflect.TypeOf(FactoryWorldSessionRuntime{}) {
		t.Fatalf("FactoryWorldRuntimeView.Session = %v, want %v", sessionField.Type, reflect.TypeOf(FactoryWorldSessionRuntime{}))
	}

	activeExecutionType := reflect.TypeOf(FactoryWorldActiveExecution{})
	assertWorldViewSliceType(t, activeExecutionType, "ConsumedInputs", reflect.TypeOf(WorkstationInput{}))
	assertWorldViewFieldAbsent(t, activeExecutionType, "ConsumedTokens")
	assertWorldViewFieldAbsent(t, activeExecutionType, "OutputMutations")

	sessionType := reflect.TypeOf(FactoryWorldSessionRuntime{})
	assertWorldViewSliceType(t, sessionType, "DispatchHistory", reflect.TypeOf(FactoryWorldDispatchCompletion{}))
	assertWorldViewSliceType(t, sessionType, "ProviderSessions", reflect.TypeOf(FactoryWorldProviderSessionRecord{}))
	assertWorldViewFieldAbsent(t, sessionType, "CompletedWorkLabels")
	assertWorldViewFieldAbsent(t, sessionType, "FailedWorkLabels")
	assertWorldViewFieldAbsent(t, sessionType, "FailedWorkDetailsByWorkID")
}

func TestFactoryWorldContractGuard_SimpleDashboardSeamStaysOffBroadAggregateShell(t *testing.T) {
	t.Parallel()

	names := append([]string(nil), retiredSimpleDashboardAggregateSeamNames...)
	sort.Strings(names)
	patterns := make([]string, 0, len(names))
	for _, name := range names {
		patterns = append(patterns, regexp.QuoteMeta(name))
	}
	matcher := regexp.MustCompile(`\b(?:` + strings.Join(patterns, "|") + `)\b`)

	guardedFiles := []string{
		filepath.Clean("../service/factory.go"),
		filepath.Clean("../cli/dashboard/dashboard.go"),
	}
	for _, path := range guardedFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if match := matcher.FindString(string(data)); match != "" {
			t.Fatalf("%s references %q; the simple-dashboard aggregate-retirement decision keeps this seam on projections.BuildSimpleDashboardWorldView(...) and forbids reintroducing pkg/interfaces aggregate shell ownership here", filepath.Clean(path), match)
		}
	}
}

func assertWorldViewFieldType(t *testing.T, structType reflect.Type, fieldName string, want reflect.Type) {
	t.Helper()

	field, ok := structType.FieldByName(fieldName)
	if !ok {
		t.Fatalf("%s missing %s field", structType.Name(), fieldName)
	}
	if field.Type != want {
		t.Fatalf("%s.%s = %v, want %v", structType.Name(), fieldName, field.Type, want)
	}
}

func assertWorldViewSliceType(t *testing.T, structType reflect.Type, fieldName string, want reflect.Type) {
	t.Helper()

	field, ok := structType.FieldByName(fieldName)
	if !ok {
		t.Fatalf("%s missing %s field", structType.Name(), fieldName)
	}
	if field.Type.Kind() != reflect.Slice {
		t.Fatalf("%s.%s kind = %s, want slice", structType.Name(), fieldName, field.Type.Kind())
	}
	if field.Type.Elem() != want {
		t.Fatalf("%s.%s element = %v, want %v", structType.Name(), fieldName, field.Type.Elem(), want)
	}
}

func assertWorldViewFieldAbsent(t *testing.T, structType reflect.Type, fieldName string) {
	t.Helper()

	if _, ok := structType.FieldByName(fieldName); ok {
		t.Fatalf("%s must not expose display-only %s field", structType.Name(), fieldName)
	}
}

func toStringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func allRetiredFactoryMirrorNames() []string {
	names := make([]string, 0, len(retiredFactoryBoundaryMirrorNames)+len(retiredFactoryCanonicalMirrorNames))
	names = append(names, retiredFactoryBoundaryMirrorNames...)
	names = append(names, retiredFactoryCanonicalMirrorNames...)
	return names
}

func walkWorldViewMirrorFiles(root, relPrefix string, includeGenerated bool, allowed map[string]struct{}, visit func(path, rel string) error) error {
	root = filepath.Clean(root)

	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			explicitSkips := []string{}
			if includeGenerated {
				explicitSkips = append(explicitSkips, "api/generated")
			}
			if contractguard.ShouldSkipDir(root, path, explicitSkips...) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if relPrefix != "" {
			rel = filepath.Join(relPrefix, rel)
		}
		rel = filepath.Clean(rel)
		if _, ok := allowed[rel]; ok {
			return nil
		}
		return visit(path, rel)
	})
}

func writeWorldViewGuardFixture(t *testing.T, root, relativePath, contents string) {
	t.Helper()

	path := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

type runtimeFactoryConfigLookupStub struct {
	factory *FactoryConfig
}

func (s *runtimeFactoryConfigLookupStub) FactoryConfig() *FactoryConfig {
	return s.factory
}

func TestFirstRuntimeDefinitionLookup_ReturnsFirstNonNilCandidate(t *testing.T) {
	t.Parallel()

	first := &runtimeLookupDefinitionStub{
		workers: map[string]*WorkerConfig{
			"planner": {Type: "planner"},
		},
	}
	second := &runtimeLookupDefinitionStub{
		workers: map[string]*WorkerConfig{
			"reviewer": {Type: "reviewer"},
		},
	}

	got := FirstRuntimeDefinitionLookup(nil, first, second)
	if got != first {
		t.Fatalf("FirstRuntimeDefinitionLookup() returned %p, want first non-nil candidate %p", got, first)
	}

	worker, ok := got.Worker("planner")
	if !ok || worker == nil || worker.Type != "planner" {
		t.Fatalf("FirstRuntimeDefinitionLookup() did not preserve the selected lookup behavior, got worker=%#v ok=%v", worker, ok)
	}
}

func TestFirstRuntimeDefinitionLookup_ReturnsNilWhenEveryCandidateIsNil(t *testing.T) {
	t.Parallel()

	if got := FirstRuntimeDefinitionLookup(nil, nil); got != nil {
		t.Fatalf("FirstRuntimeDefinitionLookup() = %p, want nil", got)
	}
}

func TestFirstRuntimeWorkstationLookup_ReturnsFirstNonNilCandidate(t *testing.T) {
	t.Parallel()

	first := &runtimeLookupWorkstationStub{
		workstations: map[string]*FactoryWorkstationConfig{
			"review": {Name: "review"},
		},
	}
	second := &runtimeLookupWorkstationStub{
		workstations: map[string]*FactoryWorkstationConfig{
			"publish": {Name: "publish"},
		},
	}

	got := FirstRuntimeWorkstationLookup(nil, first, second)
	if got != first {
		t.Fatalf("FirstRuntimeWorkstationLookup() returned %p, want first non-nil candidate %p", got, first)
	}

	workstation, ok := got.Workstation("review")
	if !ok || workstation == nil || workstation.Name != "review" {
		t.Fatalf("FirstRuntimeWorkstationLookup() did not preserve the selected lookup behavior, got workstation=%#v ok=%v", workstation, ok)
	}
}

func TestFirstRuntimeWorkstationLookup_ReturnsNilWhenEveryCandidateIsNil(t *testing.T) {
	t.Parallel()

	if got := FirstRuntimeWorkstationLookup(nil, nil); got != nil {
		t.Fatalf("FirstRuntimeWorkstationLookup() = %p, want nil", got)
	}
}

func TestFirstRuntimeFactoryConfigLookup_ReturnsFirstNonNilCandidate(t *testing.T) {
	t.Parallel()

	first := &runtimeFactoryConfigLookupStub{
		factory: &FactoryConfig{Name: "alpha"},
	}
	second := &runtimeFactoryConfigLookupStub{
		factory: &FactoryConfig{Name: "beta"},
	}

	got := FirstRuntimeFactoryConfigLookup(nil, first, second)
	if got != first {
		t.Fatalf("FirstRuntimeFactoryConfigLookup() returned %p, want first non-nil candidate %p", got, first)
	}

	if factory := got.FactoryConfig(); factory == nil || factory.Name != "alpha" {
		t.Fatalf("FirstRuntimeFactoryConfigLookup() did not preserve the selected lookup behavior, got factory=%#v", factory)
	}
}

func TestFirstRuntimeFactoryConfigLookup_ReturnsNilWhenEveryCandidateIsNil(t *testing.T) {
	t.Parallel()

	if got := FirstRuntimeFactoryConfigLookup(nil, nil); got != nil {
		t.Fatalf("FirstRuntimeFactoryConfigLookup() = %p, want nil", got)
	}
}
