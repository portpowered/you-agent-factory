package interfaces

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
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

var retiredSimpleDashboardAggregateSeamNames = []string{
	"FactoryWorldView",
	"FactoryWorldTopologyView",
	"FactoryWorldRuntimeView",
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
