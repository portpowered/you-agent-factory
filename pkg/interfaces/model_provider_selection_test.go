package interfaces

import (
	"reflect"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestResolveModelProviderSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		workstationModelProvider string
		factoryModelProvider     string
		workerModelProvider      string
		wantProvider             ModelProvider
		wantSource               ModelProviderSelectionSource
	}{
		{
			name:                     "WorkstationWins",
			workstationModelProvider: "GEMINI",
			factoryModelProvider:     string(ModelProviderCodex),
			workerModelProvider:      string(ModelProviderCodex),
			wantProvider:             ModelProviderGemini,
			wantSource:               ModelProviderSelectionSourceWorkstation,
		},
		{
			name:                 "FactoryWinsWhenWorkstationUnset",
			factoryModelProvider: "cursor-cli",
			wantProvider:         ModelProviderCursor,
			wantSource:           ModelProviderSelectionSourceFactory,
		},
		{
			name:                "WorkerProviderCompatibility",
			workerModelProvider: string(ModelProviderCodex),
			wantProvider:        ModelProviderCodex,
			wantSource:          ModelProviderSelectionSourceWorker,
		},
		{
			name:                "WorkerClaudeProvider",
			workerModelProvider: string(ModelProviderClaude),
			wantProvider:        ModelProviderClaude,
			wantSource:          ModelProviderSelectionSourceWorker,
		},
		{
			name:                "OperatorDefaultFallsBackToCodex",
			workerModelProvider: "unknown-provider",
			wantProvider:        OperatorDefaultModelProvider,
			wantSource:          ModelProviderSelectionSourceOperatorDefault,
		},
		{
			name:                     "DefaultDefersFromWorkstationToFactory",
			workstationModelProvider: FactoryModelProviderDefault,
			factoryModelProvider:     "CODEX",
			workerModelProvider:      string(ModelProviderClaude),
			wantProvider:             ModelProviderCodex,
			wantSource:               ModelProviderSelectionSourceFactory,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ResolveModelProviderSelection(tt.workstationModelProvider, tt.factoryModelProvider, tt.workerModelProvider)
			if got.Provider != tt.wantProvider || got.Source != tt.wantSource {
				t.Fatalf("ResolveModelProviderSelection(...) = %#v, want provider=%q source=%q", got, tt.wantProvider, tt.wantSource)
			}
		})
	}
}

func TestResolveRunnerSelection_AdapterMapsProviderSelection(t *testing.T) {
	t.Parallel()

	selection := ResolveRunnerSelection("GEMINI", "CODEX", string(ModelProviderClaude))
	if selection.RunnerID != RunnerIDGemini {
		t.Fatalf("runner = %q, want %q", selection.RunnerID, RunnerIDGemini)
	}
	if selection.Source != RunnerSelectionSourceWorkstation {
		t.Fatalf("source = %q, want %q", selection.Source, RunnerSelectionSourceWorkstation)
	}
}

func TestBuiltInModelProviderMetadata_MatchesRunnerCapabilities(t *testing.T) {
	t.Parallel()

	metadata, ok := BuiltInModelProviderMetadata(ModelProviderCodex)
	if !ok {
		t.Fatal("expected codex provider metadata")
	}
	if metadata.Provider != ModelProviderCodex {
		t.Fatalf("provider = %q, want %q", metadata.Provider, ModelProviderCodex)
	}
	if metadata.DisplayName != "Codex" {
		t.Fatalf("display name = %q, want Codex", metadata.DisplayName)
	}
}

func TestValidateOpenCodeAgentForModelProviderSelection(t *testing.T) {
	t.Parallel()

	err := ValidateOpenCodeAgentForModelProviderSelection(
		"reviewer",
		"",
		ResolvedModelProviderSelection{Provider: ModelProviderGemini, Source: ModelProviderSelectionSourceFactory},
	)
	if err == nil {
		t.Fatal("expected openCodeAgent validation error for non-opencode provider")
	}

	err = ValidateOpenCodeAgentForModelProviderSelection(
		"reviewer",
		"",
		ResolvedModelProviderSelection{Provider: ModelProviderOpenCode, Source: ModelProviderSelectionSourceWorkstation},
	)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestRunnerMetadataFromDispatchRequestMetadata_MapsModelProviderFields(t *testing.T) {
	modelProvider := factoryapi.WorkerModelProviderGemini
	source := factoryapi.ModelProviderSelectionSourceFactory
	metadata := &factoryapi.DispatchRequestEventMetadata{
		ModelProvider:                &modelProvider,
		ModelProviderSelectionSource: &source,
	}

	runnerID, selectionSource := RunnerMetadataFromDispatchRequestMetadata(metadata)
	if runnerID != RunnerIDGemini {
		t.Fatalf("runnerID = %q, want %q", runnerID, RunnerIDGemini)
	}
	if selectionSource != RunnerSelectionSourceFactory {
		t.Fatalf("selection source = %q, want %q", selectionSource, RunnerSelectionSourceFactory)
	}
}

func TestPublicModelProviderFromLegacyRunnerID_MapsBuiltInRunnerIDs(t *testing.T) {
	public, err := PublicModelProviderFromLegacyRunnerID("cursor-cli")
	if err != nil {
		t.Fatalf("PublicModelProviderFromLegacyRunnerID: %v", err)
	}
	if public != factoryapi.WorkerModelProviderCursor {
		t.Fatalf("modelProvider = %q, want %q", public, factoryapi.WorkerModelProviderCursor)
	}
}

func TestPublicModelProviderFromLegacyRunnerID_RejectsUnknownValues(t *testing.T) {
	_, err := PublicModelProviderFromLegacyRunnerID("mystery-runner")
	if err == nil {
		t.Fatal("expected error for unknown legacy runnerId")
	}
	if !strings.Contains(err.Error(), `unknown legacy runnerId "mystery-runner"`) {
		t.Fatalf("error = %q, want legacy runnerId naming", err)
	}
}

func TestPublicModelProviderSelectionSourceFromLegacyRunnerSelectionSource_MapsLegacyAliases(t *testing.T) {
	got := PublicModelProviderSelectionSourceFromLegacyRunnerSelectionSource("legacy_provider")
	if got != factoryapi.ModelProviderSelectionSourceWorker {
		t.Fatalf("selection source = %q, want %q", got, factoryapi.ModelProviderSelectionSourceWorker)
	}
}

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
