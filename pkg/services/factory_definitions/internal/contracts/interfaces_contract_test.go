// backendsizecheck:ignore-file consolidated interface contract and runtime-topology tests remain together until dedicated interface test seams split.
// pkgmaintcheck:ignore-file-lines consolidated interface contract and runtime-topology tests remain together until dedicated interface test seams split.
package factorycontracts

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	workerrunner "github.com/portpowered/infinite-you/pkg/services/workers/runner"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestFactoryEventClone_DetachesPayloadAndContextSlices(t *testing.T) {
	sessionID := "session-1"
	workIDs := []string{"work-1"}
	event := FactoryEvent{
		Payload: json.RawMessage(`{"status":"RUNNING"}`),
		Context: FactoryEventContext{
			SessionID: &sessionID,
			WorkIDs:   &workIDs,
		},
	}

	clone := event.Clone()
	clone.Payload[2] = 'X'
	*clone.Context.SessionID = "changed"
	(*clone.Context.WorkIDs)[0] = "changed"

	if string(event.Payload) != `{"status":"RUNNING"}` {
		t.Fatalf("payload mutated through clone: %s", event.Payload)
	}
	if *event.Context.SessionID != "session-1" {
		t.Fatalf("session ID mutated through clone: %q", *event.Context.SessionID)
	}
	if (*event.Context.WorkIDs)[0] != "work-1" {
		t.Fatalf("work IDs mutated through clone: %v", *event.Context.WorkIDs)
	}
}

func TestFactoryWorkstationConfigUnmarshalJSON_DecodesCanonicalRuntimeAndCronFields(t *testing.T) {
	var workstation FactoryWorkstationConfig
	if err := json.Unmarshal([]byte(`{
		"name":"nightly-review",
		"type":"MODEL_WORKSTATION",
		"worker":"planner",
		"cron":{
			"schedule":"*/5 * * * *",
			"triggerAtStart":true,
			"jitter":"2s",
			"expiryWindow":"30s"
		}
	}`), &workstation); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if workstation.Type != "MODEL_WORKSTATION" {
		t.Fatalf("expected canonical type to populate workstation runtime, got %+v", workstation)
	}
	if workstation.Cron == nil {
		t.Fatalf("expected canonical cron config to decode, got %+v", workstation)
	}
	if workstation.Cron.Schedule != "*/5 * * * *" || !workstation.Cron.TriggerAtStart || workstation.Cron.Jitter != "2s" || workstation.Cron.ExpiryWindow != "30s" {
		t.Fatalf("expected canonical cron config to decode intact, got %+v", workstation.Cron)
	}
}

func TestFactoryWorkstationConfigUnmarshalJSON_DecodesClassificationRoutes(t *testing.T) {
	var workstation FactoryWorkstationConfig
	if err := json.Unmarshal([]byte(`{
		"name":"classify-review",
		"type":"CLASSIFIER_WORKSTATION",
		"worker":"planner",
		"inputs":[{"workType":"task","state":"init"}],
		"classification_routes":[
			{"label":"approved","outputs":[{"workType":"task","state":"done"}]},
			{"label":"needs_review","outputs":[{"workType":"task","state":"failed"}]}
		]
	}`), &workstation); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if workstation.Type != WorkstationTypeClassify {
		t.Fatalf("expected classifier type to decode, got %+v", workstation)
	}
	if len(workstation.ClassificationRoutes) != 2 {
		t.Fatalf("expected two classification routes, got %+v", workstation.ClassificationRoutes)
	}
	if workstation.ClassificationRoutes[0].Label != "approved" || workstation.ClassificationRoutes[0].Outputs[0].StateName != "done" {
		t.Fatalf("expected first classification route to decode intact, got %+v", workstation.ClassificationRoutes[0])
	}
}

func TestCronConfigUnmarshalJSON_DecodesCanonicalFields(t *testing.T) {
	var cron CronConfig
	if err := json.Unmarshal([]byte(`{
		"schedule":"*/5 * * * *",
		"triggerAtStart":true,
		"jitter":"1s",
		"expiryWindow":"20s"
	}`), &cron); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if cron.Schedule != "*/5 * * * *" || !cron.TriggerAtStart || cron.Jitter != "1s" || cron.ExpiryWindow != "20s" {
		t.Fatalf("expected canonical cron fields to decode intact, got %+v", cron)
	}
}

func TestCronConfigUnmarshalJSON_IgnoresRetiredAliases(t *testing.T) {
	var cron CronConfig
	if err := json.Unmarshal([]byte(`{
		"schedule":"*/5 * * * *",
		"trigger_at_start":true,
		"expiry_window":"20s"
	}`), &cron); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if cron.TriggerAtStart {
		t.Fatalf("expected retired trigger_at_start alias to be ignored, got %+v", cron)
	}
	if cron.ExpiryWindow != "" {
		t.Fatalf("expected retired expiry_window alias to be ignored, got %+v", cron)
	}
}

func TestCanonicalPublicFactoryEnumsPreserveUnknownValues(t *testing.T) {
	for _, tt := range generatedPublicFactoryEnumPreservationCases() {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn(tt.input); got != tt.want {
				t.Fatalf("%s = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

type generatedPublicFactoryEnumPreservationCase struct {
	name  string
	input string
	want  string
	fn    func(string) string
}

func generatedPublicFactoryEnumPreservationCases() []generatedPublicFactoryEnumPreservationCase {
	return []generatedPublicFactoryEnumPreservationCase{
		{name: "worker type", input: "  CUSTOM_WORKER  ", want: "CUSTOM_WORKER", fn: generatedWorkerTypeString},
		{name: "worker model provider alias", input: "  openai  ", want: "CODEX", fn: generatedWorkerModelProviderString},
		{name: "worker provider alias", input: "  local-claude  ", want: "SCRIPT_WRAP", fn: generatedWorkerProviderString},
		{name: "hosted worker provider", input: "  LINEAR  ", want: "LINEAR", fn: generatedHostedWorkerProviderString},
		{name: "worker model locality", input: "  LOCAL  ", want: "LOCAL", fn: generatedWorkerModelLocalityString},
		{name: "worker operation content type", input: "  AUDIO  ", want: "AUDIO", fn: generatedWorkerOperationContentTypeString},
		{name: "resource type", input: "  MODEL  ", want: ResourceTypeModel, fn: permissiveResourceTypeString},
		{name: "worker model provider unknown", input: "  mystery-provider  ", want: "mystery-provider", fn: generatedWorkerModelProviderString},
		{name: "worker provider unknown", input: "  custom-executor  ", want: "custom-executor", fn: generatedWorkerProviderString},
		{name: "hosted worker provider unknown", input: "  custom-hosted  ", want: "custom-hosted", fn: generatedHostedWorkerProviderString},
		{name: "worker model locality unknown", input: "  edge  ", want: "edge", fn: generatedWorkerModelLocalityString},
		{name: "worker operation content type unknown", input: "  sound  ", want: "sound", fn: generatedWorkerOperationContentTypeString},
		{name: "resource type unknown", input: "  custom-resource  ", want: "custom-resource", fn: permissiveResourceTypeString},
		{name: "workstation type", input: "  CUSTOM_WORKSTATION  ", want: "CUSTOM_WORKSTATION", fn: generatedWorkstationTypeString},
		{name: "runner id", input: "  GEMINI  ", want: "gemini", fn: generatedRunnerIDString},
		{name: "runner id pi", input: "  PI  ", want: "pi", fn: generatedRunnerIDString},
		{name: "runner id unknown", input: "  custom-runner  ", want: "custom-runner", fn: generatedRunnerIDString},
		{name: "runner selection source", input: "  default  ", want: "default", fn: generatedRunnerSelectionSourceString},
		{name: "runner selection source unknown", input: "  custom-source  ", want: "custom-source", fn: generatedRunnerSelectionSourceString},
	}
}

func TestCloneProviderMetadata_PreserveNilValuesAndDetachCopies(t *testing.T) {
	if CloneProviderSessionMetadata(nil) != nil {
		t.Fatal("CloneProviderSessionMetadata(nil) = non-nil, want nil")
	}
	if CloneWorkFailureMetadata(nil) != nil {
		t.Fatal("CloneWorkFailureMetadata(nil) = non-nil, want nil")
	}

	session := &ProviderSessionMetadata{Provider: "openai", Kind: "session_id", ID: "sess-1"}
	clonedSession := CloneProviderSessionMetadata(session)
	clonedSession.ID = "sess-2"
	if session.ID != "sess-1" {
		t.Fatalf("original provider session = %#v, want sess-1 unchanged", session)
	}

	failure := &WorkFailureMetadata{Family: WorkFailureFamilyRetryable, Type: WorkFailureTypeTimeout}
	clonedFailure := CloneWorkFailureMetadata(failure)
	clonedFailure.Family = WorkFailureFamilyTerminal
	clonedFailure.Type = WorkFailureTypeInternalServerError
	if failure.Family != WorkFailureFamilyRetryable || failure.Type != WorkFailureTypeTimeout {
		t.Fatalf("original provider failure = %#v, want retryable timeout unchanged", failure)
	}
}

func TestCanonicalProviderSessionProvider(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty", input: "", expected: ""},
		{name: "cursor already canonical", input: "cursor", expected: "cursor"},
		{name: "legacy cursor command", input: string(ModelProviderCursor), expected: "cursor"},
		{name: "cursor alias", input: "cursor-agent", expected: "cursor"},
		{name: "other provider unchanged", input: "codex", expected: "codex"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanonicalProviderSessionProvider(tc.input); got != tc.expected {
				t.Fatalf("CanonicalProviderSessionProvider(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestCloneFactoryWorldProviderSessionRecord_ClonesCanonicalSafeContracts(t *testing.T) {
	original := FactoryWorldProviderSessionRecord{
		DispatchID:      "dispatch-1",
		ProviderSession: ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "sess-1"},
		Diagnostics: &SafeWorkDiagnostics{
			Provider: &SafeProviderDiagnostic{RequestMetadata: map[string]string{"session_id": "sess-1"}},
		},
		ConsumedInputs: []WorkstationInput{{
			TokenID: "token-1",
			WorkItem: &FactoryWorkItem{
				ID:                       "work-1",
				WorkTypeID:               "task",
				PreviousChainingTraceIDs: []string{"chain-a"},
				Tags:                     map[string]string{"priority": "high"},
			},
		}},
		PreviousChainingTraceIDs: []string{"chain-a", "chain-b"},
		TraceIDs:                 []string{"trace-1"},
	}

	cloned := CloneFactoryWorldProviderSessionRecord(original)
	cloned.ProviderSession.ID = "sess-2"
	cloned.Diagnostics.Provider.RequestMetadata["session_id"] = "sess-2"
	cloned.PreviousChainingTraceIDs[0] = "chain-z"
	cloned.ConsumedInputs[0].WorkItem.PreviousChainingTraceIDs[0] = "chain-z"
	cloned.ConsumedInputs[0].WorkItem.Tags["priority"] = "low"
	cloned.TraceIDs[0] = "trace-2"

	if original.ProviderSession.ID != "sess-1" {
		t.Fatalf("original provider session = %#v, want sess-1 unchanged", original.ProviderSession)
	}
	if original.Diagnostics.Provider.RequestMetadata["session_id"] != "sess-1" {
		t.Fatalf("original diagnostics = %#v, want session_id unchanged", original.Diagnostics)
	}
	if original.PreviousChainingTraceIDs[0] != "chain-a" {
		t.Fatalf("original previous chaining trace IDs = %#v, want chain-a unchanged", original.PreviousChainingTraceIDs)
	}
	if original.ConsumedInputs[0].WorkItem.PreviousChainingTraceIDs[0] != "chain-a" {
		t.Fatalf("original consumed input previous chaining trace IDs = %#v, want chain-a unchanged", original.ConsumedInputs[0].WorkItem.PreviousChainingTraceIDs)
	}
	if original.ConsumedInputs[0].WorkItem.Tags["priority"] != "high" {
		t.Fatalf("original consumed input tags = %#v, want high unchanged", original.ConsumedInputs[0].WorkItem.Tags)
	}
	if original.TraceIDs[0] != "trace-1" {
		t.Fatalf("original trace IDs = %#v, want trace-1 unchanged", original.TraceIDs)
	}
}

func generatedWorkerTypeString(value string) string {
	return PublicWorkerTypeFromInternalRuntime(value)
}
func generatedWorkerModelProviderString(value string) string {
	return PublicWorkerModelProviderFromInternalRuntime(value)
}
func generatedWorkerProviderString(value string) string {
	return PublicWorkerProviderFromInternalRuntime(value)
}
func generatedHostedWorkerProviderString(value string) string {
	return PermissivePublicFactoryHostedWorkerProvider(value)
}
func generatedWorkerModelLocalityString(value string) string {
	return PermissivePublicFactoryWorkerModelLocality(value)
}
func generatedWorkerOperationContentTypeString(value string) string {
	return PermissivePublicFactoryWorkerModelOperationContentType(value)
}
func permissiveResourceTypeString(value string) string {
	return PermissivePublicFactoryResourceType(value)
}
func generatedWorkstationTypeString(value string) string {
	return PublicWorkstationTypeFromInternalRuntime(value, "", "")
}
func generatedRunnerIDString(value string) string {
	return PermissivePublicFactoryRunnerID(workerrunner.NormalizeRunnerID(value))
}
func generatedRunnerSelectionSourceString(value string) string {
	return PermissivePublicFactoryRunnerSelectionSource(value)
}

// Generated enum helpers below are test-local compatibility fixtures. The
// production conversion is owned by config and transport boundary adapters.
func GeneratedPublicFactoryWorkerType(value string) factoryapi.WorkerType {
	return factoryapi.WorkerType(PublicWorkerTypeFromInternalRuntime(value))
}

func GeneratedPublicFactoryWorkerModelProvider(value string) factoryapi.WorkerModelProvider {
	return factoryapi.WorkerModelProvider(PublicWorkerModelProviderFromInternalRuntime(value))
}

func GeneratedPublicFactoryWorkerProvider(value string) factoryapi.WorkerProvider {
	return factoryapi.WorkerProvider(PublicWorkerProviderFromInternalRuntime(value))
}

func GeneratedPublicFactoryHostedWorkerProvider(value string) factoryapi.HostedWorkerProvider {
	return factoryapi.HostedWorkerProvider(PermissivePublicFactoryHostedWorkerProvider(value))
}

func GeneratedPublicFactoryWorkerModelLocality(value string) factoryapi.WorkerModelLocality {
	return factoryapi.WorkerModelLocality(PermissivePublicFactoryWorkerModelLocality(value))
}

func GeneratedPublicFactoryWorkerModelOperationContentType(value string) factoryapi.ModelOperationContentType {
	return factoryapi.ModelOperationContentType(PermissivePublicFactoryWorkerModelOperationContentType(value))
}

func GeneratedPublicFactoryWorkstationType(value string) factoryapi.WorkstationType {
	return factoryapi.WorkstationType(PublicWorkstationTypeFromInternalRuntime(value, "", ""))
}

func GeneratedPublicFactoryRunnerID(value string) factoryapi.RunnerID {
	return factoryapi.RunnerID(PermissivePublicFactoryRunnerID(workerrunner.NormalizeRunnerID(value)))
}

func GeneratedPublicFactoryRunnerSelectionSource(value string) factoryapi.RunnerSelectionSource {
	return factoryapi.RunnerSelectionSource(PermissivePublicFactoryRunnerSelectionSource(value))
}

func GeneratedPublicFactoryWorkerTypePtr(value string) *factoryapi.WorkerType {
	return generatedEnumPtrForTest(value, GeneratedPublicFactoryWorkerType)
}

func GeneratedPublicFactoryWorkerModelProviderPtr(value string) *factoryapi.WorkerModelProvider {
	return generatedEnumPtrForTest(value, GeneratedPublicFactoryWorkerModelProvider)
}

func GeneratedPublicFactoryWorkerProviderPtr(value string) *factoryapi.WorkerProvider {
	return generatedEnumPtrForTest(value, GeneratedPublicFactoryWorkerProvider)
}

func GeneratedPublicFactoryHostedWorkerProviderPtr(value string) *factoryapi.HostedWorkerProvider {
	return generatedEnumPtrForTest(value, GeneratedPublicFactoryHostedWorkerProvider)
}

func GeneratedPublicFactoryWorkerModelLocalityPtr(value string) *factoryapi.WorkerModelLocality {
	return generatedEnumPtrForTest(value, GeneratedPublicFactoryWorkerModelLocality)
}

func GeneratedPublicFactoryWorkerModelOperationContentTypePtr(value string) *factoryapi.ModelOperationContentType {
	return generatedEnumPtrForTest(value, GeneratedPublicFactoryWorkerModelOperationContentType)
}

func GeneratedPublicFactoryWorkstationTypePtr(value string) *factoryapi.WorkstationType {
	return generatedEnumPtrForTest(value, GeneratedPublicFactoryWorkstationType)
}

func GeneratedPublicFactoryRunnerIDPtr(value string) *factoryapi.RunnerID {
	return generatedEnumPtrForTest(value, GeneratedPublicFactoryRunnerID)
}

func GeneratedPublicFactoryRunnerSelectionSourcePtr(value string) *factoryapi.RunnerSelectionSource {
	return generatedEnumPtrForTest(value, GeneratedPublicFactoryRunnerSelectionSource)
}

func GeneratedPublicWorkstationKind(kind WorkstationKind) factoryapi.WorkstationKind {
	return factoryapi.WorkstationKind(CanonicalPublicWorkstationKind(kind))
}

func GeneratedPublicWorkstationKindPtr(kind WorkstationKind) *factoryapi.WorkstationKind {
	return generatedEnumPtrForTest(string(kind), func(value string) factoryapi.WorkstationKind {
		return GeneratedPublicWorkstationKind(WorkstationKind(value))
	})
}

func generatedEnumPtrForTest[T ~string](value string, convert func(string) T) *T {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	converted := convert(value)
	return &converted
}

func TestGeneratedPublicFactoryEnumPtrs(t *testing.T) {
	for _, tt := range generatedPublicFactoryEnumPtrCases() {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ptr("   "); got != nil {
				t.Fatalf("expected nil pointer for whitespace-only input")
			}
			got := tt.ptr(tt.supported)
			if got == nil {
				t.Fatalf("pointer helper(%q) returned nil", tt.supported)
			}
			if *got != tt.wantSupported {
				t.Fatalf("pointer helper(%q) = %q, want %q", tt.supported, *got, tt.wantSupported)
			}
			got = tt.ptr(tt.unknown)
			if got == nil {
				t.Fatalf("pointer helper(%q) returned nil", tt.unknown)
			}
			if *got != tt.wantUnknown {
				t.Fatalf("pointer helper(%q) = %q, want %q", tt.unknown, *got, tt.wantUnknown)
			}
		})
	}
}

type generatedPublicFactoryEnumPtrCase struct {
	name          string
	supported     string
	wantSupported string
	unknown       string
	wantUnknown   string
	ptr           func(string) *string
}

func generatedPublicFactoryEnumPtrCases() []generatedPublicFactoryEnumPtrCase {
	return []generatedPublicFactoryEnumPtrCase{
		{
			name:          "worker type",
			supported:     "  MODEL_WORKER  ",
			wantSupported: "INFERENCE_WORKER",
			unknown:       "  CUSTOM_WORKER  ",
			wantUnknown:   "CUSTOM_WORKER",
			ptr:           generatedPublicFactoryWorkerTypeStringPtr,
		},
		{
			name:          "worker model provider",
			supported:     "  openai  ",
			wantSupported: "CODEX",
			unknown:       "  mystery-provider  ",
			wantUnknown:   "mystery-provider",
			ptr:           generatedPublicFactoryWorkerModelProviderStringPtr,
		},
		{
			name:          "worker provider",
			supported:     "  local-claude  ",
			wantSupported: "SCRIPT_WRAP",
			unknown:       "  custom-executor  ",
			wantUnknown:   "custom-executor",
			ptr:           generatedPublicFactoryWorkerProviderStringPtr,
		},
		{
			name:          "hosted worker provider",
			supported:     "  LINEAR  ",
			wantSupported: "LINEAR",
			unknown:       "  custom-hosted  ",
			wantUnknown:   "custom-hosted",
			ptr:           generatedPublicFactoryHostedWorkerProviderStringPtr,
		},
		{
			name:          "worker model locality",
			supported:     "  LOCAL  ",
			wantSupported: "LOCAL",
			unknown:       "  edge  ",
			wantUnknown:   "edge",
			ptr:           generatedPublicFactoryWorkerModelLocalityStringPtr,
		},
		{
			name:          "worker operation content type",
			supported:     "  AUDIO  ",
			wantSupported: "AUDIO",
			unknown:       "  sound  ",
			wantUnknown:   "sound",
			ptr:           generatedPublicFactoryWorkerModelOperationContentTypeStringPtr,
		},
		{
			name:          "workstation type inference",
			supported:     "  INFERENCE_RUN  ",
			wantSupported: "INFERENCE_RUN",
			unknown:       "  CUSTOM_WORKSTATION  ",
			wantUnknown:   "CUSTOM_WORKSTATION",
			ptr:           generatedPublicFactoryWorkstationTypeStringPtr,
		},
		{
			name:          "workstation type legacy invoke alias",
			supported:     "  MODEL_INVOKE  ",
			wantSupported: "INFERENCE_RUN",
			unknown:       "  CUSTOM_WORKSTATION  ",
			wantUnknown:   "CUSTOM_WORKSTATION",
			ptr:           generatedPublicFactoryWorkstationTypeStringPtr,
		},
		{
			name:          "runner id",
			supported:     "  GEMINI  ",
			wantSupported: "gemini",
			unknown:       "  custom-runner  ",
			wantUnknown:   "custom-runner",
			ptr:           generatedPublicFactoryRunnerIDStringPtr,
		},
		{
			name:          "runner selection source",
			supported:     "  workstation  ",
			wantSupported: "workstation",
			unknown:       "  custom-source  ",
			wantUnknown:   "custom-source",
			ptr:           generatedPublicFactoryRunnerSelectionSourceStringPtr,
		},
	}
}

func generatedPublicFactoryWorkerTypeStringPtr(value string) *string {
	return generatedPublicFactoryStringPtr(GeneratedPublicFactoryWorkerTypePtr(value))
}

func generatedPublicFactoryWorkerModelProviderStringPtr(value string) *string {
	return generatedPublicFactoryStringPtr(GeneratedPublicFactoryWorkerModelProviderPtr(value))
}

func generatedPublicFactoryWorkerProviderStringPtr(value string) *string {
	return generatedPublicFactoryStringPtr(GeneratedPublicFactoryWorkerProviderPtr(value))
}

func generatedPublicFactoryHostedWorkerProviderStringPtr(value string) *string {
	return generatedPublicFactoryStringPtr(GeneratedPublicFactoryHostedWorkerProviderPtr(value))
}

func generatedPublicFactoryWorkerModelLocalityStringPtr(value string) *string {
	return generatedPublicFactoryStringPtr(GeneratedPublicFactoryWorkerModelLocalityPtr(value))
}

func generatedPublicFactoryWorkerModelOperationContentTypeStringPtr(value string) *string {
	return generatedPublicFactoryStringPtr(GeneratedPublicFactoryWorkerModelOperationContentTypePtr(value))
}

func generatedPublicFactoryWorkstationTypeStringPtr(value string) *string {
	return generatedPublicFactoryStringPtr(GeneratedPublicFactoryWorkstationTypePtr(value))
}

func generatedPublicFactoryRunnerIDStringPtr(value string) *string {
	return generatedPublicFactoryStringPtr(GeneratedPublicFactoryRunnerIDPtr(value))
}

func generatedPublicFactoryRunnerSelectionSourceStringPtr(value string) *string {
	return generatedPublicFactoryStringPtr(GeneratedPublicFactoryRunnerSelectionSourcePtr(value))
}

func generatedPublicFactoryStringPtr[T ~string](value *T) *string {
	if value == nil {
		return nil
	}
	out := string(*value)
	return &out
}

func TestGeneratedPublicWorkstationKind(t *testing.T) {
	tests := []struct {
		name  string
		input WorkstationKind
		want  factoryapi.WorkstationKind
	}{
		{
			name:  "standard runtime kind",
			input: WorkstationKindStandard,
			want:  factoryapi.WorkstationKindStandard,
		},
		{
			name:  "repeater public kind",
			input: WorkstationKind(publicFactoryWorkstationKindRepeater),
			want:  factoryapi.WorkstationKindRepeater,
		},
		{
			name:  "cron trimmed public kind",
			input: WorkstationKind("  CRON  "),
			want:  factoryapi.WorkstationKindCron,
		},
		{
			name:  "poller runtime kind",
			input: WorkstationKindPoller,
			want:  factoryapi.WorkstationKind("POLLER"),
		},
		{
			name:  "trimmed unknown kind",
			input: WorkstationKind("  custom-kind  "),
			want:  factoryapi.WorkstationKind("custom-kind"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GeneratedPublicWorkstationKind(tt.input); got != tt.want {
				t.Fatalf("GeneratedPublicWorkstationKind(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStrictPublicWorkstationKind(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "canonical public standard kind",
			input: publicFactoryWorkstationKindStandard,
			want:  publicFactoryWorkstationKindStandard,
		},
		{
			name:  "trimmed public repeater",
			input: "  REPEATER  ",
			want:  publicFactoryWorkstationKindRepeater,
		},
		{
			name:  "internal lowercase kind rejected",
			input: string(WorkstationKindCron),
			want:  "",
		},
		{
			name:  "trimmed public poller",
			input: "  POLLER  ",
			want:  publicFactoryWorkstationKindPoller,
		},
		{
			name:  "unknown kind rejected",
			input: "custom-kind",
			want:  "",
		},
		{
			name:  "whitespace only rejected",
			input: "   ",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StrictPublicWorkstationKind(tt.input); got != tt.want {
				t.Fatalf("StrictPublicWorkstationKind(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGeneratedPublicWorkstationKindPtr(t *testing.T) {
	if got := GeneratedPublicWorkstationKindPtr(WorkstationKind("   ")); got != nil {
		t.Fatalf("GeneratedPublicWorkstationKindPtr returned %#v, want nil for whitespace-only input", got)
	}

	tests := []struct {
		name  string
		input WorkstationKind
		want  factoryapi.WorkstationKind
	}{
		{
			name:  "supported kind",
			input: WorkstationKind("  REPEATER  "),
			want:  factoryapi.WorkstationKindRepeater,
		},
		{
			name:  "poller kind",
			input: WorkstationKind("  POLLER  "),
			want:  factoryapi.WorkstationKind("POLLER"),
		},
		{
			name:  "unknown trimmed kind",
			input: WorkstationKind("  custom-kind  "),
			want:  factoryapi.WorkstationKind("custom-kind"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GeneratedPublicWorkstationKindPtr(tt.input)
			if got == nil {
				t.Fatal("GeneratedPublicWorkstationKindPtr returned nil for non-empty input")
			}
			if *got != tt.want {
				t.Fatalf("GeneratedPublicWorkstationKindPtr(%q) returned %q, want %q", tt.input, *got, tt.want)
			}
		})
	}
}

func TestV1RunnerBaselineCapabilities_AreExplicitAndLimited(t *testing.T) {
	t.Parallel()

	got := V1RunnerBaselineCapabilities()
	want := []RunnerBaselineCapability{
		RunnerBaselineCapabilityPromptSubmission,
		RunnerBaselineCapabilityToolExecution,
	}

	if len(got) != len(want) {
		t.Fatalf("baseline capability count = %d, want %d", len(got), len(want))
	}
	for index, capability := range want {
		if got[index] != capability {
			t.Fatalf("baseline[%d] = %q, want %q", index, got[index], capability)
		}
	}

	got[0] = RunnerBaselineCapabilityToolExecution
	if fresh := V1RunnerBaselineCapabilities(); fresh[0] != RunnerBaselineCapabilityPromptSubmission {
		t.Fatalf("baseline capabilities were not detached: %#v", fresh)
	}
}

func TestNewRunnerCapabilities_ClonesOptionalSupport(t *testing.T) {
	t.Parallel()

	original := []RunnerOptionalCapabilitySupport{
		{
			Capability: RunnerOptionalCapabilityStructuredOutput,
			Status:     RunnerOptionalCapabilityStatusUnsupported,
			Detail:     "schema-guided output is not available",
		},
	}

	capabilities := NewRunnerCapabilities(original...)
	if len(capabilities.Baseline) != 2 {
		t.Fatalf("baseline = %#v, want two explicit baseline capabilities", capabilities.Baseline)
	}
	if len(capabilities.Optional) != 1 {
		t.Fatalf("optional = %#v, want one entry", capabilities.Optional)
	}

	original[0].Status = RunnerOptionalCapabilityStatusSupported
	if capabilities.Optional[0].Status != RunnerOptionalCapabilityStatusUnsupported {
		t.Fatalf("optional capability status mutated through source slice: %#v", capabilities.Optional)
	}
}

func TestResolveRunnerSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		workstationRunner string
		factoryRunner     string
		modelProvider     string
		wantRunner        string
		wantSource        RunnerSelectionSource
	}{
		{
			name:              "WorkstationWins",
			workstationRunner: "  GEMINI ",
			factoryRunner:     RunnerIDCodex,
			modelProvider:     RunnerIDCodex,
			wantRunner:        RunnerIDGemini,
			wantSource:        RunnerSelectionSourceWorkstation,
		},
		{
			name:          "FactoryWinsWhenWorkstationUnset",
			factoryRunner: "cursor-cli",
			wantRunner:    RunnerIDCursorCLI,
			wantSource:    RunnerSelectionSourceFactory,
		},
		{
			name:          "LegacyModelProviderCompatibility",
			modelProvider: "codex",
			wantRunner:    RunnerIDCodex,
			wantSource:    RunnerSelectionSourceLegacyProvider,
		},
		{
			name:          "DefaultFallsBackToCodex",
			modelProvider: "claude",
			wantRunner:    RunnerIDCodex,
			wantSource:    RunnerSelectionSourceDefault,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ResolveRunnerSelection(tt.workstationRunner, tt.factoryRunner, tt.modelProvider)
			if got.RunnerID != tt.wantRunner || got.Source != tt.wantSource {
				t.Fatalf("ResolveRunnerSelection(...) = %#v, want runner=%q source=%q", got, tt.wantRunner, tt.wantSource)
			}
		})
	}
}

func TestBuiltInRunnerMetadata(t *testing.T) {
	t.Parallel()

	metadata, ok := BuiltInRunnerMetadata("  CURSOR-CLI ")
	if !ok {
		t.Fatal("expected cursor-cli metadata")
	}
	if metadata.ID != RunnerIDCursorCLI {
		t.Fatalf("metadata.ID = %q, want %q", metadata.ID, RunnerIDCursorCLI)
	}
}

func TestBuiltInRunnerMetadata_CodexWorktreeDetailMatchesRuntimeBehavior(t *testing.T) {
	t.Parallel()

	metadata, ok := BuiltInRunnerMetadata(RunnerIDCodex)
	if !ok {
		t.Fatal("expected codex metadata")
	}

	for _, capability := range metadata.Capabilities.Optional {
		if capability.Capability != RunnerOptionalCapabilityWorktree {
			continue
		}
		if capability.Status != RunnerOptionalCapabilityStatusSupported {
			t.Fatalf("worktree status = %q, want %q", capability.Status, RunnerOptionalCapabilityStatusSupported)
		}
		if capability.Detail != "factory-managed git worktree preparation under the factory root" {
			t.Fatalf("worktree detail = %q", capability.Detail)
		}
		return
	}

	t.Fatal("expected codex worktree capability metadata")
}

func TestSupportedModelProviders_IncludesAllCanonicalCommands(t *testing.T) {
	got := SupportedModelProviders()
	want := []ModelProvider{
		ModelProviderClaude,
		ModelProviderCodex,
		ModelProviderGemini,
		ModelProviderKiro,
		ModelProviderCursor,
		ModelProviderOpenCode,
		ModelProviderPi,
		ModelProviderAgy,
	}
	if len(got) != len(want) {
		t.Fatalf("supported provider count = %d, want %d", len(got), len(want))
	}
	for _, provider := range want {
		if !slices.Contains(got, provider) {
			t.Fatalf("supported providers missing %q", provider)
		}
	}
}

func TestModelProviderPublicInternalMapping_RoundTripsAllSupportedProviders(t *testing.T) {
	cases := []struct {
		public string
		want   ModelProvider
	}{
		{string(factoryapi.WorkerModelProviderClaude), ModelProviderClaude},
		{string(factoryapi.WorkerModelProviderCodex), ModelProviderCodex},
		{string(factoryapi.WorkerModelProviderCursor), ModelProviderCursor},
		{string(factoryapi.WorkerModelProviderGemini), ModelProviderGemini},
		{string(factoryapi.WorkerModelProviderKiro), ModelProviderKiro},
		{string(factoryapi.WorkerModelProviderOpenCode), ModelProviderOpenCode},
		{string(factoryapi.WorkerModelProviderPi), ModelProviderPi},
		{string(factoryapi.WorkerModelProviderAgy), ModelProviderAgy},
	}

	for _, tt := range cases {
		t.Run(string(tt.public), func(t *testing.T) {
			internal, ok := InternalModelProviderFromPublicWorkerModelProvider(tt.public)
			if !ok || internal != tt.want {
				t.Fatalf("InternalModelProviderFromPublicWorkerModelProvider(%q) = (%q, %v), want (%q, true)", tt.public, internal, ok, tt.want)
			}
			public, ok := PublicWorkerModelProviderFromInternal(internal)
			if !ok || public != tt.public {
				t.Fatalf("PublicWorkerModelProviderFromInternal(%q) = (%q, %v), want (%q, true)", internal, public, ok, tt.public)
			}
		})
	}
}

func TestPublicWorkerModelProviderFromInternalRuntime_CanonicalizesProviderAliases(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"gemini", string(factoryapi.WorkerModelProviderGemini)},
		{"kiro-cli", string(factoryapi.WorkerModelProviderKiro)},
		{"opencode", string(factoryapi.WorkerModelProviderOpenCode)},
		{"GEMINI", string(factoryapi.WorkerModelProviderGemini)},
		{"KIRO", string(factoryapi.WorkerModelProviderKiro)},
		{"OPENCODE", string(factoryapi.WorkerModelProviderOpenCode)},
		{"agy", string(factoryapi.WorkerModelProviderAgy)},
		{"AGY", string(factoryapi.WorkerModelProviderAgy)},
		{"antigravity", string(factoryapi.WorkerModelProviderAgy)},
	}

	for _, tt := range cases {
		t.Run(tt.input, func(t *testing.T) {
			if got := PublicWorkerModelProviderFromInternalRuntime(tt.input); got != tt.want {
				t.Fatalf("PublicWorkerModelProviderFromInternalRuntime(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStrictPublicFactoryWorkerModelProvider_AcceptsAllCanonicalPublicValues(t *testing.T) {
	for _, provider := range []string{
		"CLAUDE", "CODEX", "CURSOR", "GEMINI", "KIRO", "OPENCODE", "PI", "AGY",
	} {
		if got := StrictPublicFactoryWorkerModelProvider(provider); got != provider {
			t.Fatalf("StrictPublicFactoryWorkerModelProvider(%q) = %q, want %q", provider, got, provider)
		}
	}
}

func TestBuildPendingFactoryGraphTopology_DerivesCanonicalNodesAndEdges(t *testing.T) {
	t.Parallel()

	cfg, topology := newPendingFactoryGraphTopologyFixture()
	assertPendingFactoryGraphTopologyNodes(t, topology)
	assertPendingFactoryGraphTopologyEdges(t, topology)
	if cfg.Workstations[0].ID != "workstation-review" {
		t.Fatalf("fixture workstation id = %q, want workstation-review", cfg.Workstations[0].ID)
	}
}

func newPendingFactoryGraphTopologyFixture() (*FactoryConfig, PendingFactoryGraphTopology) {
	cfg := &FactoryConfig{
		ResourceManifest: &PortableResourceManifestConfig{
			BundledFiles: []BundledFileConfig{
				{ID: "doc-guide", Type: BundledFileTypeDoc, TargetPath: "docs/guide.md"},
				{Type: BundledFileTypeScript, TargetPath: "scripts/build.sh"},
				{Type: BundledFileTypeInput, TargetPath: "inputs/request.json"},
				{Type: BundledFileTypeRootHelper, TargetPath: "Makefile"},
				{Type: "UNSUPPORTED", TargetPath: "ignored.txt"},
			},
		},
		Resources: []ResourceConfig{
			{ID: "resource-api", Name: "api"},
		},
		Workers: []WorkerConfig{
			{
				ID:   "worker-exec",
				Name: "executor",
				Resources: []ResourceConfig{
					{Name: "api"},
				},
			},
		},
		WorkTypes: []WorkTypeConfig{
			{
				ID:   "worktype-story",
				Name: "story",
				States: []StateConfig{
					{ID: "state-ready", Name: "ready"},
					{ID: "state-done", Name: "done"},
				},
			},
		},
		Workstations: []FactoryWorkstationConfig{
			{
				ID:             "workstation-review",
				Name:           "review",
				WorkerTypeName: "executor",
				Resources: []ResourceConfig{
					{ID: "resource-api", Name: "api"},
				},
				Inputs: []IOConfig{
					{WorkTypeName: "story", StateName: "ready"},
					{WorkTypeName: "missing", StateName: "ready"},
				},
				Outputs: []IOConfig{
					{WorkTypeName: "story", StateName: "done"},
				},
				OnContinue: []IOConfig{
					{WorkTypeName: "story", StateName: "done"},
				},
				OnFailure: []IOConfig{
					{WorkTypeName: "story", StateName: "ready"},
				},
				OnRejection: []IOConfig{
					{WorkTypeName: "story", StateName: "done"},
				},
			},
		},
	}

	return cfg, BuildPendingFactoryGraphTopology(cfg)
}

func assertPendingFactoryGraphTopologyNodes(t *testing.T, topology PendingFactoryGraphTopology) {
	t.Helper()

	for _, nodeID := range []string{
		"doc:doc-guide",
		"script:scripts/build.sh",
		"doc:scripts/build.sh",
		"input:inputs/request.json",
		"doc:inputs/request.json",
		"root-helper:Makefile",
		"doc:Makefile",
		"resource:resource-api",
		"worker:worker-exec",
		"work-type:worktype-story",
		"work-state:worktype-story:state-ready",
		"work-state:worktype-story:state-done",
		"workstation:workstation-review",
	} {
		if _, ok := topology.NodeIDs[nodeID]; !ok {
			t.Fatalf("topology.NodeIDs missing %q", nodeID)
		}
	}
	if _, ok := topology.NodeIDs["doc:ignored.txt"]; !ok {
		t.Fatalf("topology.NodeIDs missing legacy compatibility node for unsupported bundled file target")
	}
	if _, ok := topology.NodeIDs["unsupported:ignored.txt"]; ok {
		t.Fatalf("topology.NodeIDs unexpectedly contained typed unsupported bundled file node")
	}
}

func assertPendingFactoryGraphTopologyEdges(t *testing.T, topology PendingFactoryGraphTopology) {
	t.Helper()

	for _, edgeID := range []string{
		"worker-resource:resource:resource-api->worker:worker-exec",
		"work-type-state:work-type:worktype-story->work-state:worktype-story:state-ready",
		"work-type-state:work-type:worktype-story->work-state:worktype-story:state-done",
		"worker-assignment:worker:worker-exec->workstation:workstation-review",
		"workstation-resource:resource:resource-api->workstation:workstation-review",
		"workstation-input:work-state:worktype-story:state-ready->workstation:workstation-review",
		"workstation-output:workstation:workstation-review->work-state:worktype-story:state-done",
		"workstation-on-continue:workstation:workstation-review->work-state:worktype-story:state-done",
		"workstation-on-failure:workstation:workstation-review->work-state:worktype-story:state-ready",
		"workstation-on-rejection:workstation:workstation-review->work-state:worktype-story:state-done",
	} {
		if _, ok := topology.EdgeIDs[edgeID]; !ok {
			t.Fatalf("topology.EdgeIDs missing %q", edgeID)
		}
	}
}

func TestCloneSubprocessExecutionRequest_DetachesMutableFields(t *testing.T) {
	t.Parallel()

	request := SubprocessExecutionRequest{
		Command:                  "runner",
		Args:                     []string{"--flag"},
		Stdin:                    []byte("stdin"),
		Env:                      []string{"KEY=value"},
		PreviousChainingTraceIDs: []string{"chain-a"},
		Execution: ExecutionMetadata{
			WorkIDs: []string{"work-1"},
		},
		InputTokens:   []any{"token"},
		InputBindings: map[string][]string{"slot": {"work-1"}},
	}

	cloned := CloneSubprocessExecutionRequest(request)
	cloned.Args[0] = "--changed"
	cloned.Stdin[0] = 'X'
	cloned.Env[0] = "KEY=changed"
	cloned.PreviousChainingTraceIDs[0] = "chain-z"
	cloned.Execution.WorkIDs[0] = "work-2"
	cloned.InputTokens[0] = "changed"
	cloned.InputBindings["slot"][0] = "work-2"

	if request.Args[0] != "--flag" {
		t.Fatalf("original subprocess args mutated: %#v", request.Args)
	}
	if string(request.Stdin) != "stdin" {
		t.Fatalf("original subprocess stdin mutated: %q", string(request.Stdin))
	}
	if request.Env[0] != "KEY=value" {
		t.Fatalf("original subprocess env mutated: %#v", request.Env)
	}
	if request.PreviousChainingTraceIDs[0] != "chain-a" {
		t.Fatalf("original subprocess trace ids mutated: %#v", request.PreviousChainingTraceIDs)
	}
	if request.Execution.WorkIDs[0] != "work-1" {
		t.Fatalf("original subprocess execution mutated: %#v", request.Execution)
	}
	if request.InputTokens[0] != "token" {
		t.Fatalf("original subprocess input tokens mutated: %#v", request.InputTokens)
	}
	if request.InputBindings["slot"][0] != "work-1" {
		t.Fatalf("original subprocess input bindings mutated: %#v", request.InputBindings)
	}
}

func TestCanonicalBundledFileHelpers(t *testing.T) {
	t.Parallel()

	if got := CanonicalBundledFileID(" explicit-id ", "docs/guide.md"); got != "explicit-id" {
		t.Fatalf("CanonicalBundledFileID explicit = %q, want explicit-id", got)
	}
	if got := CanonicalBundledFileID("", "docs/guide.md"); got != "docs/guide.md" {
		t.Fatalf("CanonicalBundledFileID fallback = %q, want docs/guide.md", got)
	}

	for _, tt := range []struct {
		fileType string
		wantKind string
	}{
		{fileType: BundledFileTypeDoc, wantKind: "doc"},
		{fileType: BundledFileTypeScript, wantKind: "script"},
		{fileType: BundledFileTypeInput, wantKind: "input"},
		{fileType: BundledFileTypeRootHelper, wantKind: "root-helper"},
		{fileType: "unknown", wantKind: ""},
	} {
		if got := CanonicalBundledFileGraphNodeKind(tt.fileType); got != tt.wantKind {
			t.Fatalf("CanonicalBundledFileGraphNodeKind(%q) = %q, want %q", tt.fileType, got, tt.wantKind)
		}
	}

	file := BundledFileConfig{ID: "file-1", Type: BundledFileTypeDoc, TargetPath: "docs/guide.md"}
	if got := CanonicalBundledFileGraphNodeID(file); got != "doc:file-1" {
		t.Fatalf("CanonicalBundledFileGraphNodeID = %q, want doc:file-1", got)
	}
	if got := CanonicalBundledFileGraphNodeID(BundledFileConfig{Type: "unknown", TargetPath: "docs/guide.md"}); got != "" {
		t.Fatalf("CanonicalBundledFileGraphNodeID unsupported = %q, want empty", got)
	}
	if got := CanonicalBundledFileGraphNodeID(BundledFileConfig{Type: BundledFileTypeDoc}); got != "" {
		t.Fatalf("CanonicalBundledFileGraphNodeID empty id = %q, want empty", got)
	}

	if !IsBundledFileGraphNodeID("doc:file-1") || !IsBundledFileGraphNodeID("script:file-1") ||
		!IsBundledFileGraphNodeID("input:file-1") || !IsBundledFileGraphNodeID("root-helper:file-1") {
		t.Fatal("IsBundledFileGraphNodeID did not recognize supported bundled-file node prefixes")
	}
	if IsBundledFileGraphNodeID("resource:file-1") {
		t.Fatal("IsBundledFileGraphNodeID recognized unrelated node prefix")
	}
}

func TestCanonicalGraphEntityHelpers(t *testing.T) {
	t.Parallel()

	resource := ResourceConfig{ID: "resource-api", Name: "api"}
	worker := WorkerConfig{ID: "worker-exec", Name: "executor"}
	workType := WorkTypeConfig{ID: "worktype-story", Name: "story"}
	state := StateConfig{ID: "state-ready", Name: "ready"}

	if got := CanonicalFactoryGraphResourceID(resource); got != "resource-api" {
		t.Fatalf("CanonicalFactoryGraphResourceID = %q, want resource-api", got)
	}
	if got := CanonicalFactoryGraphWorkerID(worker); got != "worker-exec" {
		t.Fatalf("CanonicalFactoryGraphWorkerID = %q, want worker-exec", got)
	}
	if got := CanonicalFactoryGraphWorkTypeID(workType); got != "worktype-story" {
		t.Fatalf("CanonicalFactoryGraphWorkTypeID = %q, want worktype-story", got)
	}
	if got := CanonicalFactoryGraphWorkStateID(workType, state); got != "worktype-story:state-ready" {
		t.Fatalf("CanonicalFactoryGraphWorkStateID = %q, want worktype-story:state-ready", got)
	}
}

func TestCloneFactoryWorldInferenceAttemptsByDispatchID_DeepCopyMutableFields(t *testing.T) {
	t.Parallel()

	exitCode := 7
	attempts := map[string]map[string]FactoryWorldInferenceAttempt{
		"dispatch-1": {
			"request-1": {
				DispatchID: "dispatch-1",
				ExitCode:   &exitCode,
				ProviderSession: &ProviderSessionMetadata{
					Provider: "codex",
					ID:       "sess-1",
				},
				Diagnostics: &SafeWorkDiagnostics{
					Provider: &SafeProviderDiagnostic{
						RequestMetadata: map[string]string{"session_id": "sess-1"},
					},
				},
			},
		},
		"dispatch-empty": {},
	}

	cloned := CloneFactoryWorldInferenceAttemptsByDispatchID(attempts)
	if _, ok := cloned["dispatch-empty"]; ok {
		t.Fatalf("CloneFactoryWorldInferenceAttemptsByDispatchID preserved empty dispatch entry: %#v", cloned)
	}

	attempt := cloned["dispatch-1"]["request-1"]
	attempt.ProviderSession.ID = "sess-2"
	attempt.Diagnostics.Provider.RequestMetadata["session_id"] = "sess-2"
	*attempt.ExitCode = 9

	originalAttempt := attempts["dispatch-1"]["request-1"]
	if originalAttempt.ProviderSession.ID != "sess-1" {
		t.Fatalf("original inference attempt provider session mutated: %#v", originalAttempt.ProviderSession)
	}
	if originalAttempt.Diagnostics.Provider.RequestMetadata["session_id"] != "sess-1" {
		t.Fatalf("original inference attempt diagnostics mutated: %#v", originalAttempt.Diagnostics.Provider.RequestMetadata)
	}
	if *originalAttempt.ExitCode != 7 {
		t.Fatalf("original inference attempt exit code mutated: %#v", originalAttempt.ExitCode)
	}

	if got := CloneFactoryWorldInferenceAttemptsByDispatchID(nil); got != nil {
		t.Fatalf("CloneFactoryWorldInferenceAttemptsByDispatchID(nil) = %#v, want nil", got)
	}
}

func TestWorkerWorkstationCompatibilityBehaviorProjection(t *testing.T) {
	t.Parallel()

	t.Run("effective behavior class", testWorkerBehaviorProjectionEffectiveClass)
	t.Run("legacy and required compatibility", testWorkerBehaviorProjectionLegacyAndRequiredCompatibility)
	t.Run("compatibility labels and messages", testWorkerBehaviorProjectionLabelsAndMessages)
}

func testWorkerBehaviorProjectionEffectiveClass(t *testing.T) {
	t.Helper()

	if got := EffectiveWorkstationBehaviorClass("", WorkstationKindStandard, true); got != WorkstationTypeAgent {
		t.Fatalf("EffectiveWorkstationBehaviorClass blank standard = %q, want %q", got, WorkstationTypeAgent)
	}
	if got := EffectiveWorkstationBehaviorClass("", WorkstationKindPoller, true); got != WorkstationTypePoller {
		t.Fatalf("EffectiveWorkstationBehaviorClass blank poller = %q, want %q", got, WorkstationTypePoller)
	}
	if got := EffectiveWorkstationBehaviorClass("", WorkstationKindStandard, false); got != "" {
		t.Fatalf("EffectiveWorkstationBehaviorClass without worker = %q, want empty", got)
	}
}

func testWorkerBehaviorProjectionLegacyAndRequiredCompatibility(t *testing.T) {
	t.Helper()

	for _, tt := range []struct {
		workerType      string
		workstationType string
		kind            WorkstationKind
	}{
		{workerType: WorkerTypeModel, workstationType: WorkstationTypeModel},
		{workerType: WorkerTypeScript, workstationType: WorkstationTypeModel},
		{workerType: WorkerTypeInference, workstationType: "", kind: WorkstationKindStandard},
		{workerType: WorkerTypeHosted, workstationType: "", kind: WorkstationKindPoller},
	} {
		if !IsLegacyGrandfatheredWorkerWorkstationPair(tt.workerType, tt.workstationType, tt.kind) {
			t.Fatalf("IsLegacyGrandfatheredWorkerWorkstationPair(%q, %q, %q) = false, want true", tt.workerType, tt.workstationType, tt.kind)
		}
	}
	if IsLegacyGrandfatheredWorkerWorkstationPair(WorkerTypeAgent, WorkstationTypeInference, "") {
		t.Fatal("agent worker and inference workstation should not be grandfathered")
	}

	if !RequiresWorkerWorkstationBehaviorCompatibility(WorkstationTypeAgent, "", "worker-name") {
		t.Fatal("agent workstation with bound worker should require compatibility")
	}
	if RequiresWorkerWorkstationBehaviorCompatibility(WorkstationTypeLogical, "", "worker-name") {
		t.Fatal("logical workstation should not require compatibility")
	}
	if RequiresWorkerWorkstationBehaviorCompatibility(WorkstationTypeAgent, "", "") {
		t.Fatal("workstation without bound worker should not require compatibility")
	}
}

func testWorkerBehaviorProjectionLabelsAndMessages(t *testing.T) {
	t.Helper()

	if !CompatibleWorkerWorkstationBehavior(WorkerTypeModel, WorkstationTypeModel, "") {
		t.Fatal("legacy model worker/model workstation should be compatible")
	}
	if !CompatibleWorkerWorkstationBehavior("", WorkstationTypeAgent, "") {
		t.Fatal("empty worker type should be treated as compatible")
	}
	if CompatibleWorkerWorkstationBehavior(WorkerTypeAgent, WorkstationTypeInference, "") {
		t.Fatal("agent worker and inference workstation should not be compatible")
	}

	if got := RuntimeBehaviorClassLabel(WorkerTypeInference); got != "inference" {
		t.Fatalf("RuntimeBehaviorClassLabel inference = %q, want inference", got)
	}
	if got := RuntimeBehaviorClassLabel("  CUSTOM_BEHAVIOR  "); got != "custom_behavior" {
		t.Fatalf("RuntimeBehaviorClassLabel custom = %q, want custom_behavior", got)
	}

	if got := WorkerWorkstationBehaviorMismatchMessage("review", "", WorkstationKindPoller, "planner", ""); got == "" {
		t.Fatal("WorkerWorkstationBehaviorMismatchMessage returned empty string")
	}

	if got := PublicWorkerTypeForFactoryUsage(
		WorkerConfig{Name: "", Type: WorkerTypeModel},
		[]Workstation{{Type: WorkstationTypeAgent, WorkerTypeName: "executor"}},
	); got != WorkerTypeInference {
		t.Fatalf("model worker without name = %q, want %q", got, WorkerTypeInference)
	}
	if got := PublicWorkerTypeForFactoryUsage(WorkerConfig{Name: "executor", Type: WorkerTypeAgent}, nil); got != WorkerTypeAgent {
		t.Fatalf("non-model worker type projection = %q, want %q", got, WorkerTypeAgent)
	}
}

func TestSafeAgentRunDiagnosticFromWorkDiagnostics_ProjectsMetadata(t *testing.T) {
	diagnostics := &WorkDiagnostics{
		Metadata: map[string]string{
			AgentRunMetadataExecutionBehavior: AgentRunExecutionBehavior,
			AgentRunMetadataFailureClass:      "agent_run_timeout",
			AgentRunMetadataRecoveryAction:    "retry later",
			AgentRunMetadataToolPolicy:        AgentWorkerToolPolicyDisabled,
			AgentRunMetadataToolCallCount:     "2",
			AgentRunMetadataToolDiagnostics:   "read_file:denied:policy=disabled,write_file:start",
		},
	}

	got := SafeAgentRunDiagnosticFromWorkDiagnostics(diagnostics)
	if got == nil || got.ExecutionBehavior != AgentRunExecutionBehavior {
		t.Fatalf("SafeAgentRunDiagnosticFromWorkDiagnostics() = %#v, want agent_run behavior", got)
	}
	if got.FailureClass != "agent_run_timeout" || got.RecoveryAction != "retry later" {
		t.Fatalf("failure metadata = %#v, want timeout + retry later", got)
	}
	if got.ToolPolicy != AgentWorkerToolPolicyDisabled || got.ToolCallCount != 2 {
		t.Fatalf("tool metadata = %#v, want disabled policy and count 2", got)
	}
	if len(got.ToolDiagnostics) != 2 || got.ToolDiagnostics[0].ToolName != "read_file" {
		t.Fatalf("tool diagnostics = %#v, want parsed entries", got.ToolDiagnostics)
	}
}

func TestGeneratedSafeWorkDiagnostics_IncludesAgentRun(t *testing.T) {
	safe := &SafeWorkDiagnostics{
		AgentRun: &SafeAgentRunDiagnostic{
			ExecutionBehavior: AgentRunExecutionBehavior,
			ToolPolicy:        AgentWorkerToolPolicyReadOnly,
			Transcript: []AgentRunTranscriptEntry{
				{Role: "assistant", Summary: "final answer"},
			},
		},
	}
	generated := GeneratedSafeWorkDiagnostics(safe)
	if generated == nil || generated.AgentRun == nil {
		t.Fatalf("GeneratedSafeWorkDiagnostics() = %#v, want agentRun populated", generated)
	}
	if generated.AgentRun.ToolPolicy == nil || *generated.AgentRun.ToolPolicy != AgentWorkerToolPolicyReadOnly {
		t.Fatalf("generated tool policy = %#v, want read_only", generated.AgentRun.ToolPolicy)
	}
}
