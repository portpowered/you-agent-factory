package contracttests

import (
	"reflect"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
	"github.com/portpowered/infinite-you/pkg/services/work"
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

	workDispatchType := reflect.TypeOf(work.WorkDispatch{})
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

	workDispatchType := reflect.TypeOf(work.WorkDispatch{})
	for _, fieldName := range retiredWorkDispatchWorkerFields {
		if _, ok := workDispatchType.FieldByName(fieldName); ok {
			t.Fatalf("WorkDispatch must not reintroduce worker-owned field %s", fieldName)
		}
	}
}

func TestFactoryWorldContractGuard_RuntimeShellUsesCanonicalSelectedTickTypes(t *testing.T) {
	t.Parallel()

	topologyType := reflect.TypeOf(interfaces.FactoryWorldTopologyView{})
	assertWorldViewSliceType(t, topologyType, "SubmitWorkTypes", reflect.TypeOf(interfaces.FactoryWorldSubmitWorkType{}))

	runtimeType := reflect.TypeOf(interfaces.FactoryWorldRuntimeView{})
	assertWorldViewFieldType(t, runtimeType, "InferenceAttemptsByDispatchID", reflect.TypeOf(map[string]map[string]interfaces.FactoryWorldInferenceAttempt{}))
	if _, ok := runtimeType.FieldByName("WorkstationRequestsByDispatchID"); ok {
		t.Fatal("FactoryWorldRuntimeView must not retain the API-owned workstation_requests_by_dispatch_id projection")
	}

	sessionField, ok := runtimeType.FieldByName("Session")
	if !ok {
		t.Fatalf("FactoryWorldRuntimeView missing Session field")
	}
	if sessionField.Type != reflect.TypeOf(interfaces.FactoryWorldSessionRuntime{}) {
		t.Fatalf("FactoryWorldRuntimeView.Session = %v, want %v", sessionField.Type, reflect.TypeOf(interfaces.FactoryWorldSessionRuntime{}))
	}

	activeExecutionType := reflect.TypeOf(interfaces.FactoryWorldActiveExecution{})
	assertWorldViewSliceType(t, activeExecutionType, "ConsumedInputs", reflect.TypeOf(interfaces.WorkstationInput{}))
	assertWorldViewFieldAbsent(t, activeExecutionType, "ConsumedTokens")
	assertWorldViewFieldAbsent(t, activeExecutionType, "OutputMutations")

	sessionType := reflect.TypeOf(interfaces.FactoryWorldSessionRuntime{})
	assertWorldViewSliceType(t, sessionType, "DispatchHistory", reflect.TypeOf(interfaces.FactoryWorldDispatchCompletion{}))
	assertWorldViewSliceType(t, sessionType, "ProviderSessions", reflect.TypeOf(interfaces.FactoryWorldProviderSessionRecord{}))
	assertWorldViewFieldAbsent(t, sessionType, "CompletedWorkLabels")
	assertWorldViewFieldAbsent(t, sessionType, "FailedWorkLabels")
	assertWorldViewFieldAbsent(t, sessionType, "FailedWorkDetailsByWorkID")
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
	factory *interfaces.FactoryConfig
}

func (s *runtimeFactoryConfigLookupStub) FactoryConfig() *interfaces.FactoryConfig {
	return s.factory
}

type runtimeLookupDefinitionStub struct {
	workers      map[string]*workerconfig.Config
	workstations map[string]*interfaces.FactoryWorkstationConfig
}

func (s *runtimeLookupDefinitionStub) Worker(name string) (*workerconfig.Config, bool) {
	worker, ok := s.workers[name]
	return worker, ok
}

func (s *runtimeLookupDefinitionStub) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	workstation, ok := s.workstations[name]
	return workstation, ok
}

type runtimeLookupWorkstationStub struct {
	workstations map[string]*interfaces.FactoryWorkstationConfig
}

func (s *runtimeLookupWorkstationStub) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	workstation, ok := s.workstations[name]
	return workstation, ok
}

func TestFirstRuntimeDefinitionLookup_ReturnsFirstNonNilCandidate(t *testing.T) {
	t.Parallel()

	first := &runtimeLookupDefinitionStub{
		workers: map[string]*workerconfig.Config{
			"planner": {Type: "planner"},
		},
	}
	second := &runtimeLookupDefinitionStub{
		workers: map[string]*workerconfig.Config{
			"reviewer": {Type: "reviewer"},
		},
	}

	got := interfaces.FirstRuntimeDefinitionLookup(nil, first, second)
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

	if got := interfaces.FirstRuntimeDefinitionLookup(nil, nil); got != nil {
		t.Fatalf("FirstRuntimeDefinitionLookup() = %p, want nil", got)
	}
}

func TestFirstRuntimeWorkstationLookup_ReturnsFirstNonNilCandidate(t *testing.T) {
	t.Parallel()

	first := &runtimeLookupWorkstationStub{
		workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"review": {Name: "review"},
		},
	}
	second := &runtimeLookupWorkstationStub{
		workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"publish": {Name: "publish"},
		},
	}

	got := interfaces.FirstRuntimeWorkstationLookup(nil, first, second)
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

	if got := interfaces.FirstRuntimeWorkstationLookup(nil, nil); got != nil {
		t.Fatalf("FirstRuntimeWorkstationLookup() = %p, want nil", got)
	}
}

func TestFirstRuntimeFactoryConfigLookup_ReturnsFirstNonNilCandidate(t *testing.T) {
	t.Parallel()

	first := &runtimeFactoryConfigLookupStub{
		factory: &interfaces.FactoryConfig{Name: "alpha"},
	}
	second := &runtimeFactoryConfigLookupStub{
		factory: &interfaces.FactoryConfig{Name: "beta"},
	}

	got := interfaces.FirstRuntimeFactoryConfigLookup(nil, first, second)
	if got != first {
		t.Fatalf("FirstRuntimeFactoryConfigLookup() returned %p, want first non-nil candidate %p", got, first)
	}

	if factory := got.FactoryConfig(); factory == nil || factory.Name != "alpha" {
		t.Fatalf("FirstRuntimeFactoryConfigLookup() did not preserve the selected lookup behavior, got factory=%#v", factory)
	}
}

func TestFirstRuntimeFactoryConfigLookup_ReturnsNilWhenEveryCandidateIsNil(t *testing.T) {
	t.Parallel()

	if got := interfaces.FirstRuntimeFactoryConfigLookup(nil, nil); got != nil {
		t.Fatalf("FirstRuntimeFactoryConfigLookup() = %p, want nil", got)
	}
}
