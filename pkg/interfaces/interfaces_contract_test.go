package interfaces

import (
	"encoding/json"
	"slices"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

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

func TestPublicFactoryEnumNormalizers(t *testing.T) {
	for _, tt := range publicFactoryEnumNormalizerCases() {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.permissive("  " + tt.alias + "  "); got != tt.want {
				t.Fatalf("permissive(%q) = %q, want %q", tt.alias, got, tt.want)
			}
			if got := tt.strict("  " + tt.alias + "  "); got != tt.want {
				t.Fatalf("strict(%q) = %q, want %q", tt.alias, got, tt.want)
			}
			if got := tt.permissive("  " + tt.unknown + "  "); got != tt.unknown {
				t.Fatalf("permissive(%q) = %q, want trimmed unknown %q", tt.unknown, got, tt.unknown)
			}
			if got := tt.strict("  " + tt.unknown + "  "); got != "" {
				t.Fatalf("strict(%q) = %q, want rejection", tt.unknown, got)
			}
		})
	}
}

type publicFactoryEnumNormalizerCase struct {
	name       string
	alias      string
	unknown    string
	want       string
	permissive func(string) string
	strict     func(string) string
}

func publicFactoryEnumNormalizerCases() []publicFactoryEnumNormalizerCase {
	return []publicFactoryEnumNormalizerCase{
		{
			name:       "worker type",
			alias:      "MODEL_WORKER",
			unknown:    "CUSTOM_WORKER",
			want:       WorkerTypeModel,
			permissive: PermissivePublicFactoryWorkerType,
			strict:     StrictPublicFactoryWorkerType,
		},
		{
			name:       "worker model provider",
			alias:      "CODEX",
			unknown:    "mystery-provider",
			want:       "CODEX",
			permissive: PermissivePublicFactoryWorkerModelProvider,
			strict:     StrictPublicFactoryWorkerModelProvider,
		},
		{
			name:       "worker provider",
			alias:      "SCRIPT_WRAP",
			unknown:    "custom-executor",
			want:       "SCRIPT_WRAP",
			permissive: PermissivePublicFactoryWorkerProvider,
			strict:     StrictPublicFactoryWorkerProvider,
		},
		{
			name:       "hosted worker provider",
			alias:      "LINEAR",
			unknown:    "custom-hosted",
			want:       HostedWorkerProviderLinear,
			permissive: PermissivePublicFactoryHostedWorkerProvider,
			strict:     StrictPublicFactoryHostedWorkerProvider,
		},
		{
			name:       "worker model locality",
			alias:      "LOCAL",
			unknown:    "edge",
			want:       ModelLocalityLocal,
			permissive: PermissivePublicFactoryWorkerModelLocality,
			strict:     StrictPublicFactoryWorkerModelLocality,
		},
		{
			name:       "worker operation content type",
			alias:      "AUDIO",
			unknown:    "sound",
			want:       ModelOperationContentTypeAudio,
			permissive: PermissivePublicFactoryWorkerModelOperationContentType,
			strict:     StrictPublicFactoryWorkerModelOperationContentType,
		},
		{
			name:       "resource type",
			alias:      "MODEL",
			unknown:    "custom-resource",
			want:       ResourceTypeModel,
			permissive: PermissivePublicFactoryResourceType,
			strict:     StrictPublicFactoryResourceType,
		},
		{
			name:       "workstation type",
			alias:      "MODEL_INVOKE",
			unknown:    "CUSTOM_WORKSTATION",
			want:       WorkstationTypeInvoke,
			permissive: PermissivePublicFactoryWorkstationType,
			strict:     StrictPublicFactoryWorkstationType,
		},
		{
			name:       "runner id",
			alias:      "cursor-cli",
			unknown:    "custom-runner",
			want:       RunnerIDCursorCLI,
			permissive: PermissivePublicFactoryRunnerID,
			strict:     StrictPublicFactoryRunnerID,
		},
		{
			name:       "runner selection source",
			alias:      "factory",
			unknown:    "custom-source",
			want:       string(RunnerSelectionSourceFactory),
			permissive: PermissivePublicFactoryRunnerSelectionSource,
			strict:     StrictPublicFactoryRunnerSelectionSource,
		},
		{
			name:       "work type handling behavior",
			alias:      WorkTypeHandlingBehaviorDefault,
			unknown:    "PROMPT",
			want:       WorkTypeHandlingBehaviorDefault,
			permissive: PermissivePublicWorkTypeHandlingBehavior,
			strict:     StrictPublicWorkTypeHandlingBehavior,
		},
	}
}

func TestGeneratedPublicFactoryEnumsPreserveUnknownValues(t *testing.T) {
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
		{name: "runner id unknown", input: "  custom-runner  ", want: "custom-runner", fn: generatedRunnerIDString},
		{name: "runner selection source", input: "  default  ", want: "default", fn: generatedRunnerSelectionSourceString},
		{name: "runner selection source unknown", input: "  custom-source  ", want: "custom-source", fn: generatedRunnerSelectionSourceString},
	}
}

func generatedWorkerTypeString(value string) string {
	return string(GeneratedPublicFactoryWorkerType(value))
}
func generatedWorkerModelProviderString(value string) string {
	return string(GeneratedPublicFactoryWorkerModelProvider(value))
}
func generatedWorkerProviderString(value string) string {
	return string(GeneratedPublicFactoryWorkerProvider(value))
}
func generatedHostedWorkerProviderString(value string) string {
	return string(GeneratedPublicFactoryHostedWorkerProvider(value))
}
func generatedWorkerModelLocalityString(value string) string {
	return string(GeneratedPublicFactoryWorkerModelLocality(value))
}
func generatedWorkerOperationContentTypeString(value string) string {
	return string(GeneratedPublicFactoryWorkerModelOperationContentType(value))
}
func permissiveResourceTypeString(value string) string {
	return PermissivePublicFactoryResourceType(value)
}
func generatedWorkstationTypeString(value string) string {
	return string(GeneratedPublicFactoryWorkstationType(value))
}
func generatedRunnerIDString(value string) string {
	return string(GeneratedPublicFactoryRunnerID(value))
}
func generatedRunnerSelectionSourceString(value string) string {
	return string(GeneratedPublicFactoryRunnerSelectionSource(value))
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
			wantSupported: "MODEL_WORKER",
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
			name:          "workstation type",
			supported:     "  MODEL_INVOKE  ",
			wantSupported: "MODEL_INVOKE",
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

type runtimeLookupDefinitionStub struct {
	workers      map[string]*WorkerConfig
	workstations map[string]*FactoryWorkstationConfig
}

func (s *runtimeLookupDefinitionStub) Worker(name string) (*WorkerConfig, bool) {
	worker, ok := s.workers[name]
	return worker, ok
}

func (s *runtimeLookupDefinitionStub) Workstation(name string) (*FactoryWorkstationConfig, bool) {
	workstation, ok := s.workstations[name]
	return workstation, ok
}

type runtimeLookupWorkstationStub struct {
	workstations map[string]*FactoryWorkstationConfig
}

func (s *runtimeLookupWorkstationStub) Workstation(name string) (*FactoryWorkstationConfig, bool) {
	workstation, ok := s.workstations[name]
	return workstation, ok
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

func TestResolveOpenCodeAgent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		workstationAgent  string
		workerAgent       string
		wantOpenCodeAgent string
	}{
		{
			name:              "WorkstationOverridesWorker",
			workstationAgent:  " implementer ",
			workerAgent:       "reviewer",
			wantOpenCodeAgent: "implementer",
		},
		{
			name:              "WorkerDefaultWhenWorkstationUnset",
			workerAgent:       "reviewer",
			wantOpenCodeAgent: "reviewer",
		},
		{
			name: "EmptyWhenUnset",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ResolveOpenCodeAgent(tt.workstationAgent, tt.workerAgent)
			if got != tt.wantOpenCodeAgent {
				t.Fatalf("ResolveOpenCodeAgent(...) = %q, want %q", got, tt.wantOpenCodeAgent)
			}
		})
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
		if capability.Status != RunnerOptionalCapabilityStatusUnsupported {
			t.Fatalf("worktree status = %q, want %q", capability.Status, RunnerOptionalCapabilityStatusUnsupported)
		}
		if capability.Detail != "codex rejects workstation worktree selection in v1" {
			t.Fatalf("worktree detail = %q", capability.Detail)
		}
		return
	}

	t.Fatal("expected codex worktree capability metadata")
}

func TestCanonicalChainingTraceIDs_SortsAndDedupesNonEmptyValues(t *testing.T) {
	got := CanonicalChainingTraceIDs([]string{"trace-b", "", "trace-a", "trace-b", "trace-c"})
	want := []string{"trace-a", "trace-b", "trace-c"}
	assertCanonicalTraceIDs(t, got, want)
}

func TestPreviousChainingTraceIDsFromTokens_SingleInputFanOutPreservesOnePredecessor(t *testing.T) {
	got := PreviousChainingTraceIDsFromTokens([]Token{
		{Color: TokenColor{DataType: DataTypeWork, TraceID: "trace-parent"}},
		{Color: TokenColor{DataType: DataTypeResource, TraceID: "trace-resource-ignored"}},
	})
	want := []string{"trace-parent"}
	assertCanonicalTraceIDs(t, got, want)
}

func TestPreviousChainingTraceIDsFromTokens_MultiInputFanInReturnsSortedUniquePredecessors(t *testing.T) {
	got := PreviousChainingTraceIDsFromTokens([]Token{
		{Color: TokenColor{DataType: DataTypeWork, TraceID: "trace-z"}},
		{Color: TokenColor{DataType: DataTypeWork, TraceID: "trace-a"}},
		{Color: TokenColor{DataType: DataTypeWork, TraceID: "trace-z"}},
		{Color: TokenColor{DataType: DataTypeResource, TraceID: "trace-resource-ignored"}},
	})
	want := []string{"trace-a", "trace-z"}
	assertCanonicalTraceIDs(t, got, want)
}

func TestPreviousChainingTraceIDsFromTokens_PrefersCanonicalCurrentTraceOverLegacyTrace(t *testing.T) {
	got := PreviousChainingTraceIDsFromTokens([]Token{
		{Color: TokenColor{DataType: DataTypeWork, CurrentChainingTraceID: "chain-z", TraceID: "trace-a"}},
		{Color: TokenColor{DataType: DataTypeWork, CurrentChainingTraceID: "chain-a", TraceID: "trace-z"}},
	})
	want := []string{"chain-a", "chain-z"}
	assertCanonicalTraceIDs(t, got, want)
}

func TestPreviousChainingTraceIDsFromWorkItems_MultiInputFanInReturnsSortedUniquePredecessors(t *testing.T) {
	got := PreviousChainingTraceIDsFromWorkItems([]FactoryWorkItem{
		{ID: "work-2", TraceID: "trace-b"},
		{ID: "work-1", TraceID: "trace-a"},
		{ID: "work-3", TraceID: "trace-b"},
		{ID: "work-4"},
	})
	want := []string{"trace-a", "trace-b"}
	assertCanonicalTraceIDs(t, got, want)
}

func TestPreviousChainingTraceIDsFromTokenColors_MultiInputFanInReturnsSortedUniquePredecessors(t *testing.T) {
	got := PreviousChainingTraceIDsFromTokenColors([]TokenColor{
		{DataType: DataTypeWork, TraceID: "trace-z"},
		{DataType: DataTypeWork, TraceID: "trace-a"},
		{DataType: DataTypeWork, TraceID: "trace-z"},
		{DataType: DataTypeResource, TraceID: "trace-resource-ignored"},
	})
	want := []string{"trace-a", "trace-z"}
	assertCanonicalTraceIDs(t, got, want)
}

func TestCurrentChainingTraceIDFromTokens_PrefersCanonicalCurrentTraceOverLegacyTrace(t *testing.T) {
	got := CurrentChainingTraceIDFromTokens([]Token{
		{Color: TokenColor{DataType: DataTypeWork, WorkTypeID: "task", CurrentChainingTraceID: "chain-customer", TraceID: "trace-customer"}},
	})
	if got != "chain-customer" {
		t.Fatalf("current chaining trace ID = %q, want chain-customer", got)
	}
}

func TestCurrentChainingTraceIDFromTokens_PrefersCustomerWorkOverSystemTime(t *testing.T) {
	got := CurrentChainingTraceIDFromTokens([]Token{
		{Color: TokenColor{DataType: DataTypeWork, WorkTypeID: SystemTimeWorkTypeID, TraceID: "trace-system"}},
		{Color: TokenColor{DataType: DataTypeWork, WorkTypeID: "task", TraceID: "trace-customer"}},
	})
	if got != "trace-customer" {
		t.Fatalf("current chaining trace ID = %q, want trace-customer", got)
	}
}

func TestCurrentChainingTraceIDFromWorkItems_PrefersCustomerWorkOverSystemTime(t *testing.T) {
	got := CurrentChainingTraceIDFromWorkItems([]FactoryWorkItem{
		{WorkTypeID: SystemTimeWorkTypeID, CurrentChainingTraceID: "chain-system", TraceID: "trace-system"},
		{WorkTypeID: "task", CurrentChainingTraceID: "chain-customer", TraceID: "trace-customer"},
	})
	if got != "chain-customer" {
		t.Fatalf("current chaining trace ID from work items = %q, want chain-customer", got)
	}
}

func TestChainingTraceDepthFromTokenColors_UsesDeepestNonResourceAncestor(t *testing.T) {
	got := ChainingTraceDepthFromTokenColors([]TokenColor{
		{DataType: DataTypeResource, WorkTypeID: "slot", ChainingTraceDepth: 99},
		{DataType: DataTypeWork, WorkTypeID: "task", ChainingTraceDepth: 2, TraceID: "trace-a"},
		{DataType: DataTypeWork, WorkTypeID: "task", ChainingTraceDepth: 4, TraceID: "trace-b"},
	})
	if got != 5 {
		t.Fatalf("chaining trace depth = %d, want 5", got)
	}
}

func TestChainingTraceDepthForTokenColor_AndWorkItem_FallbackToInitialDepth(t *testing.T) {
	tokenDepth := ChainingTraceDepthForTokenColor(TokenColor{CurrentChainingTraceID: "chain-1"})
	if tokenDepth != 1 {
		t.Fatalf("token chaining trace depth = %d, want 1", tokenDepth)
	}

	workDepth := ChainingTraceDepthForWorkItem(FactoryWorkItem{TraceID: "trace-1"})
	if workDepth != 1 {
		t.Fatalf("work item chaining trace depth = %d, want 1", workDepth)
	}
}

func assertCanonicalTraceIDs(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("trace ID count = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("trace IDs = %#v, want %#v", got, want)
		}
	}
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
		public factoryapi.WorkerModelProvider
		want   ModelProvider
	}{
		{factoryapi.WorkerModelProviderClaude, ModelProviderClaude},
		{factoryapi.WorkerModelProviderCodex, ModelProviderCodex},
		{factoryapi.WorkerModelProviderCursor, ModelProviderCursor},
		{factoryapi.WorkerModelProviderGemini, ModelProviderGemini},
		{factoryapi.WorkerModelProviderKiro, ModelProviderKiro},
		{factoryapi.WorkerModelProviderOpenCode, ModelProviderOpenCode},
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

func TestGeneratedPublicFactoryWorkerModelProvider_CanonicalizesProviderAliases(t *testing.T) {
	cases := []struct {
		input string
		want  factoryapi.WorkerModelProvider
	}{
		{"gemini", factoryapi.WorkerModelProviderGemini},
		{"kiro-cli", factoryapi.WorkerModelProviderKiro},
		{"opencode", factoryapi.WorkerModelProviderOpenCode},
		{"GEMINI", factoryapi.WorkerModelProviderGemini},
		{"KIRO", factoryapi.WorkerModelProviderKiro},
		{"OPENCODE", factoryapi.WorkerModelProviderOpenCode},
	}

	for _, tt := range cases {
		t.Run(tt.input, func(t *testing.T) {
			if got := GeneratedPublicFactoryWorkerModelProvider(tt.input); got != tt.want {
				t.Fatalf("GeneratedPublicFactoryWorkerModelProvider(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStrictPublicFactoryWorkerModelProvider_AcceptsAllCanonicalPublicValues(t *testing.T) {
	for _, provider := range []string{
		"CLAUDE", "CODEX", "CURSOR", "GEMINI", "KIRO", "OPENCODE",
	} {
		if got := StrictPublicFactoryWorkerModelProvider(provider); got != provider {
			t.Fatalf("StrictPublicFactoryWorkerModelProvider(%q) = %q, want %q", provider, got, provider)
		}
	}
}
