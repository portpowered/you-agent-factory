package interfaces

import (
	"testing"
)

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
