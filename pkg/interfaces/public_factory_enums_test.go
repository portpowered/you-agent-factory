package interfaces

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestPublicFactoryEnumNormalizers(t *testing.T) {
	tests := []struct {
		name       string
		alias      string
		unknown    string
		want       string
		permissive func(string) string
		strict     func(string) string
	}{
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
			name:       "workstation type",
			alias:      "LOGICAL_MOVE",
			unknown:    "CUSTOM_WORKSTATION",
			want:       WorkstationTypeLogical,
			permissive: PermissivePublicFactoryWorkstationType,
			strict:     StrictPublicFactoryWorkstationType,
		},
	}

	for _, tt := range tests {
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

func TestGeneratedPublicFactoryEnumsPreserveUnknownValues(t *testing.T) {
	if got := GeneratedPublicFactoryWorkerType("  CUSTOM_WORKER  "); got != factoryapi.WorkerType("CUSTOM_WORKER") {
		t.Fatalf("worker type = %q, want trimmed unknown to round-trip", got)
	}
	if got := GeneratedPublicFactoryWorkerModelProvider("  openai  "); got != factoryapi.WorkerModelProvider("CODEX") {
		t.Fatalf("worker model provider = %q, want CODEX from internal openai alias", got)
	}
	if got := GeneratedPublicFactoryWorkerProvider("  local-claude  "); got != factoryapi.WorkerProvider("SCRIPT_WRAP") {
		t.Fatalf("worker provider = %q, want SCRIPT_WRAP from internal local-claude alias", got)
	}
	if got := GeneratedPublicFactoryWorkerModelProvider("  mystery-provider  "); got != factoryapi.WorkerModelProvider("mystery-provider") {
		t.Fatalf("worker model provider = %q, want trimmed unknown to round-trip", got)
	}
	if got := GeneratedPublicFactoryWorkerProvider("  custom-executor  "); got != factoryapi.WorkerProvider("custom-executor") {
		t.Fatalf("worker provider = %q, want trimmed unknown to round-trip", got)
	}
	if got := GeneratedPublicFactoryWorkstationType("  CUSTOM_WORKSTATION  "); got != factoryapi.WorkstationType("CUSTOM_WORKSTATION") {
		t.Fatalf("workstation type = %q, want trimmed unknown to round-trip", got)
	}
}

func TestGeneratedPublicFactoryEnumPtrs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		ptr   func(string) *string
	}{
		{
			name:  "worker type",
			input: "  MODEL_WORKER  ",
			want:  "MODEL_WORKER",
			ptr: func(value string) *string {
				converted := GeneratedPublicFactoryWorkerTypePtr(value)
				if converted == nil {
					return nil
				}
				out := string(*converted)
				return &out
			},
		},
		{
			name:  "worker model provider",
			input: "  openai  ",
			want:  "CODEX",
			ptr: func(value string) *string {
				converted := GeneratedPublicFactoryWorkerModelProviderPtr(value)
				if converted == nil {
					return nil
				}
				out := string(*converted)
				return &out
			},
		},
		{
			name:  "worker provider",
			input: "  local-claude  ",
			want:  "SCRIPT_WRAP",
			ptr: func(value string) *string {
				converted := GeneratedPublicFactoryWorkerProviderPtr(value)
				if converted == nil {
					return nil
				}
				out := string(*converted)
				return &out
			},
		},
		{
			name:  "workstation type",
			input: "  CUSTOM_WORKSTATION  ",
			want:  "CUSTOM_WORKSTATION",
			ptr: func(value string) *string {
				converted := GeneratedPublicFactoryWorkstationTypePtr(value)
				if converted == nil {
					return nil
				}
				out := string(*converted)
				return &out
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ptr("   "); got != nil {
				t.Fatalf("expected nil pointer for whitespace-only input")
			}
			got := tt.ptr(tt.input)
			if got == nil {
				t.Fatalf("pointer helper(%q) returned nil", tt.input)
			}
			if *got != tt.want {
				t.Fatalf("pointer helper(%q) = %q, want %q", tt.input, *got, tt.want)
			}
		})
	}
}
