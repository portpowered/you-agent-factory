package structural_test

import (
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/structural"
	"github.com/portpowered/infinite-you/pkg/services/models"
)

func validPetriFactoryConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		Name: "structural-validation",
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "done", Type: factorydefinitions.StateTypeTerminal},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed},
			},
		}},
		Workers: []workerconfig.Config{{Name: "worker-a"}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name:           "process",
			WorkerTypeName: "worker-a",
			Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
			OnFailure:      []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
		}},
	}
}

func TestValidate_ValidPetriFactoryHasNoBlockingStructuralTargets(t *testing.T) {
	t.Parallel()

	result := structural.Validate(validPetriFactoryConfig())
	if result.HasBlockingTargets() {
		t.Fatalf("structural targets = %#v, want none", result.Targets)
	}
}

func TestValidate_DuplicateWorkerReturnsTypedStructuralTarget(t *testing.T) {
	t.Parallel()

	cfg := validPetriFactoryConfig()
	cfg.Workers = append(cfg.Workers, workerconfig.Config{Name: "worker-a"})

	result := structural.Validate(cfg)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking structural targets")
	}
	found := false
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeDuplicateIdentifier &&
			target.Severity == factorydefinitions.ValidationSeverityError &&
			target.Subject.Type == factorydefinitions.ValidationSubjectTypeWorker {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want duplicate worker structural target", result.Targets)
	}
}

func TestManagedRuntimeValidationMatchesModelsBackendMembership(t *testing.T) {
	t.Parallel()

	backends := []string{
		"LLAMACPP",
		"  llamaCpp \t",
		"",
		" \t\n",
		"GGUF",
		"localai-llamacpp",
		"localai-whisper",
		"localai-vibevoice",
	}
	for _, backend := range backends {
		backend := backend
		t.Run(backend, func(t *testing.T) {
			t.Parallel()
			cfg := &factorydefinitions.FactoryConfig{
				Resources: []factorydefinitions.ResourceConfig{{
					Name:    "voice-assets",
					Type:    factorydefinitions.ResourceTypeModel,
					Model:   "OMNIVOICE_Q4_K_M",
					Backend: backend,
				}},
			}

			targets := factoryvalidation.ManagedRuntimeDependencyTargets(cfg)
			managed := models.IsManagedRuntimeBackend(backend)
			if managed && len(targets) != 0 {
				t.Fatalf("managed backend %q produced targets %#v", backend, targets)
			}
			if !managed && len(targets) != 1 {
				t.Fatalf("unsupported backend %q produced %d targets, want one", backend, len(targets))
			}
			if !managed && targets[0].Code != factoryvalidation.CodeManagedRuntimeInvalidBackend {
				t.Fatalf("unsupported backend %q target code = %q, want %q", backend, targets[0].Code, factoryvalidation.CodeManagedRuntimeInvalidBackend)
			}
		})
	}
}

func TestManagedRuntimeDependencyTargets_CharacterizesCurrentMembership(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		model       string
		backend     string
		wantCode    string
		wantMessage string
	}{
		{
			name:    "normalized OmniVoice and LlamaCpp pairing",
			model:   "  omnivoice_q4_k_m ",
			backend: "  llamaCpp \t",
		},
		{
			name:        "known identity with different backend",
			model:       "OMNIVOICE_Q4_K_M",
			backend:     "GGUF",
			wantCode:    factoryvalidation.CodeManagedRuntimeInvalidBackend,
			wantMessage: `requires backend "LLAMACPP"`,
		},
		{
			name:        "unknown identity with managed backend",
			model:       "UNKNOWN_MODEL",
			backend:     "LLAMACPP",
			wantCode:    factoryvalidation.CodeManagedRuntimeUnsupportedIdentity,
			wantMessage: "is not supported in this environment",
		},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			cfg := &factorydefinitions.FactoryConfig{
				Resources: []factorydefinitions.ResourceConfig{{
					Name:       "voice-assets",
					Type:       factorydefinitions.ResourceTypeModel,
					Capacity:   1,
					Model:      testCase.model,
					Backend:    testCase.backend,
					LoadPolicy: "ON_DEMAND",
				}},
			}

			targets := factoryvalidation.ManagedRuntimeDependencyTargets(cfg)
			if testCase.wantCode == "" {
				if len(targets) != 0 {
					t.Fatalf("targets = %#v, want no findings", targets)
				}
				return
			}
			if len(targets) != 1 {
				t.Fatalf("targets = %#v, want one finding", targets)
			}
			if targets[0].Code != testCase.wantCode {
				t.Fatalf("target code = %q, want %q", targets[0].Code, testCase.wantCode)
			}
			if !strings.Contains(targets[0].Message, testCase.wantMessage) {
				t.Fatalf("target message = %q, want substring %q", targets[0].Message, testCase.wantMessage)
			}
		})
	}
}
