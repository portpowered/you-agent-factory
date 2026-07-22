// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
// pkgmaintcheck:ignore-file-lines service-ownership migration preserves this consolidated file; split responsibilities and remove this exemption.
package validation

import (
	"context"
	"fmt"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func validateFactoryDefinition(
	ctx context.Context,
	cfg *interfaces.FactoryConfig,
) error {
	validator := New(nil)
	if result := validator.ValidateTopology(ctx, cfg, nil); result.HasErrors() {
		return fmt.Errorf("%s", result.Error())
	}
	validationResult := validator.Validate(ctx, cfg, nil)
	if validationResult.HasBlockingTargets() {
		var details strings.Builder
		for index, target := range validationResult.BlockingTargets() {
			if index > 0 {
				details.WriteString("; ")
			}
			details.WriteString(target.Code)
			details.WriteString(": ")
			details.WriteString(target.Message)
		}
		return fmt.Errorf("%s", details.String())
	}
	return nil
}

func TestFactoryDefinitionValidation_PerInputGuard_RejectsMissingParentInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "collector",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type: interfaces.GuardTypeAllChildrenComplete,
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for per-input guard missing parent_input")
	}
}

func TestFactoryDefinitionValidation_PerInputGuard_RejectsSelfReference(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "bad-guard",
				Inputs: []interfaces.IOConfig{
					{
						StateName:    "init",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeAllChildrenComplete,
							ParentInput: "page", // Self-reference.
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "page"},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for per-input guard referencing its own input")
	}
}

func TestFactoryDefinitionValidation_PerInputGuard_RejectsInvalidParentInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "collector",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeAllChildrenComplete,
							ParentInput: "nonexistent",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for per-input guard referencing non-existent parent input")
	}
}

func TestFactoryDefinitionValidation_PerInputGuard_RejectsInvalidSpawnedBy(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "collector",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeAllChildrenComplete,
							ParentInput: "task",
							SpawnedBy:   "nonexistent-workstation",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for per-input guard referencing non-existent spawned_by workstation")
	}
}

func TestFactoryDefinitionValidation_PerInputGuard_RejectsUnsupportedType(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "collector",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeVisitCount,
							ParentInput: "task",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for unsupported per-input guard type")
	}
}

func TestFactoryDefinitionValidation_PerInputGuard_RejectsSameNameMissingMatchInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{StateName: "ready", WorkTypeName: "plan"},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type: interfaces.GuardTypeSameName,
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for same-name guard missing match_input")
	}
}

func TestFactoryDefinitionValidation_PerInputGuard_RejectsSameNameSelfReference(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{
						StateName:    "ready",
						WorkTypeName: "plan",
					},
					{
						StateName:    "ready",
						WorkTypeName: "task",
					},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type:       interfaces.GuardTypeSameName,
							MatchInput: "task",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for same-name guard referencing its own input")
	}
}

func TestFactoryDefinitionValidation_PerInputGuard_RejectsSameNameUnknownMatchInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{StateName: "ready", WorkTypeName: "plan"},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type:       interfaces.GuardTypeSameName,
							MatchInput: "other",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for same-name guard referencing non-existent input")
	}
}

func TestFactoryDefinitionValidation_PerInputGuard_RejectsSameTraceIDMissingMatchInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{StateName: "ready", WorkTypeName: "plan"},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type: interfaces.GuardTypeSameTraceID,
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for same-trace guard missing match_input")
	}
	if !strings.Contains(err.Error(), "[per-input-guard-same-trace-match-input]") {
		t.Fatalf("expected same-trace-specific validation rule in error, got %v", err)
	}
	if !strings.Contains(err.Error(), `workstations[0](match-items).inputs[1].guard`) {
		t.Fatalf("expected input guard path in error, got %v", err)
	}
}

func TestFactoryDefinitionValidation_PerInputGuard_RejectsSameTraceIDSelfReference(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{
						StateName:    "ready",
						WorkTypeName: "plan",
					},
					{
						StateName:    "ready",
						WorkTypeName: "task",
					},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type:       interfaces.GuardTypeSameTraceID,
							MatchInput: "task",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for same-trace guard referencing its own input")
	}
	if !strings.Contains(err.Error(), "[per-input-guard-same-trace-self-ref]") {
		t.Fatalf("expected same-trace self-reference validation rule in error, got %v", err)
	}
	if !strings.Contains(err.Error(), `workstations[0](match-items).inputs[2].guard`) {
		t.Fatalf("expected input guard path in error, got %v", err)
	}
}

func TestFactoryDefinitionValidation_PerInputGuard_RejectsSameTraceIDUnknownMatchInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{StateName: "ready", WorkTypeName: "plan"},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type:       interfaces.GuardTypeSameTraceID,
							MatchInput: "other",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for same-trace guard referencing non-existent input")
	}
	if !strings.Contains(err.Error(), "[per-input-guard-same-trace-match-input]") {
		t.Fatalf("expected same-trace-specific validation rule in error, got %v", err)
	}
	if !strings.Contains(err.Error(), `workstations[0](match-items).inputs[1].guard`) {
		t.Fatalf("expected input guard path in error, got %v", err)
	}
}

func TestFactoryDefinitionValidation_RejectsNonexistentResource(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Resources: []interfaces.ResourceConfig{
					{Name: "nonexistent-gpu", Capacity: 1},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for resource_usage referencing non-existent resource")
	}
}

func TestFactoryDefinitionValidation_RejectsInvalidResourceCount(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Resources: []interfaces.ResourceConfig{
			{Name: "gpu", Capacity: 2},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Resources: []interfaces.ResourceConfig{
					{Name: "gpu", Capacity: 0},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for resource_usage with zero count")
	}
}

func TestFactoryDefinitionValidation_RejectsUnknownWorkstationKind(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Kind: "unknown_kind",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for unknown workstation kind")
	}
}

func TestFactoryDefinitionValidation_RejectsNonexistentWorker(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workers: []interfaces.FactoryWorkerConfig{
			{Name: "real-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "processor",
				WorkerTypeName: "ghost-worker",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for workstation referencing non-existent worker")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, `references non-existent worker "ghost-worker"`) {
		t.Errorf("unexpected error message:\ngot: %s\nwant it to mention: references non-existent worker \"ghost-worker\"", errMsg)
	}
}

func TestFactoryDefinitionValidation_RejectsInvalidOnRejection(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				OnRejection: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "nonexistent"}},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for on_rejection pointing to non-existent state")
	}
}

func TestFactoryDefinitionValidation_RejectsInvalidOnFailure(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "nonexistent-type", StateName: "failed"}},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for on_failure referencing non-existent work type")
	}
}

func TestFactoryDefinitionValidation_RejectsWorkstationLevelChildFanInGuards(t *testing.T) {
	tests := []struct {
		name      string
		guardType interfaces.GuardType
	}{
		{name: "all children complete", guardType: interfaces.GuardTypeAllChildrenComplete},
		{name: "any child failed", guardType: interfaces.GuardTypeAnyChildFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &interfaces.FactoryConfig{
				WorkTypes: []interfaces.WorkTypeConfig{
					{
						Name: "task",
						States: []interfaces.StateConfig{
							{Name: "init", Type: interfaces.StateTypeInitial},
							{Name: "complete", Type: interfaces.StateTypeTerminal},
							{Name: "failed", Type: interfaces.StateTypeFailed},
						},
					},
				},
				Workstations: []interfaces.FactoryWorkstationConfig{
					{
						Name: "collector",
						Inputs: []interfaces.IOConfig{
							{StateName: "init", WorkTypeName: "task"},
						},
						Outputs: []interfaces.IOConfig{
							{StateName: "complete", WorkTypeName: "task"},
						},
						Guards: []interfaces.GuardConfig{
							{Type: tt.guardType},
						},
					},
				},
			}

			err := validateFactoryDefinition(context.Background(), input)
			if err == nil {
				t.Fatalf("expected validation error for workstation-level %s guard", tt.guardType)
			}
			if !strings.Contains(err.Error(), "use per-input guards for child fan-in") {
				t.Fatalf("expected per-input guard guidance, got %v", err)
			}
		})
	}
}

func TestFactoryDefinitionValidation_RejectsUnknownGuardType(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Guards: []interfaces.GuardConfig{
					{Type: "nonexistent_guard"},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for unknown guard type")
	}
}

func TestFactoryDefinitionValidation_RejectsFactoryInferenceThrottleGuardMissingModelProvider(t *testing.T) {
	input := &interfaces.FactoryConfig{
		Guards: []interfaces.FactoryGuardConfig{{
			Type:          interfaces.GuardTypeInferenceThrottle,
			RefreshWindow: "15m",
		}},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "processor",
			Inputs: []interfaces.IOConfig{
				{StateName: "init", WorkTypeName: "task"},
			},
			Outputs: []interfaces.IOConfig{
				{StateName: "complete", WorkTypeName: "task"},
			},
		}},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for factory inference throttle guard missing modelProvider")
	}
	if !strings.Contains(err.Error(), "guards[0](inference_throttle_guard).modelProvider") {
		t.Fatalf("expected modelProvider field path, got %v", err)
	}
}

func TestFactoryDefinitionValidation_RejectsFactoryInferenceThrottleGuardInvalidRefreshWindow(t *testing.T) {
	input := &interfaces.FactoryConfig{
		Guards: []interfaces.FactoryGuardConfig{{
			Type:          interfaces.GuardTypeInferenceThrottle,
			ModelProvider: "claude",
			RefreshWindow: "later",
		}},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "processor",
			Inputs: []interfaces.IOConfig{
				{StateName: "init", WorkTypeName: "task"},
			},
			Outputs: []interfaces.IOConfig{
				{StateName: "complete", WorkTypeName: "task"},
			},
		}},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for factory inference throttle guard invalid refreshWindow")
	}
	if !strings.Contains(err.Error(), "guards[0](inference_throttle_guard).refreshWindow") {
		t.Fatalf("expected refreshWindow field path, got %v", err)
	}
}

func TestFactoryDefinitionValidation_RejectsMatchesFieldsMissingInputKey(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{Name: "matcher"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "processor",
			WorkerTypeName: "matcher",
			Inputs:         []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
			Outputs:        []interfaces.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
			Guards:         []interfaces.GuardConfig{{Type: interfaces.GuardTypeMatchesFields}},
		}},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for matches_fields guard missing matchConfig.inputKey")
	}
}

func TestFactoryDefinitionValidation_RejectsMatchesFieldsEmptyInputKey(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{Name: "matcher"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "processor",
			WorkerTypeName: "matcher",
			Inputs:         []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
			Outputs:        []interfaces.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
			Guards: []interfaces.GuardConfig{{
				Type:        interfaces.GuardTypeMatchesFields,
				MatchConfig: &interfaces.GuardMatchConfig{InputKey: " "},
			}},
		}},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for matches_fields guard empty matchConfig.inputKey")
	}
}

func TestFactoryDefinitionValidation_RejectsVisitCountGuardMissingParams(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Guards: []interfaces.GuardConfig{
					{Type: interfaces.GuardTypeVisitCount},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for visit_count guard missing workstation")
	}
}

func TestFactoryDefinitionValidation_RejectsGuardReferencingNonexistentWorkstation(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Guards: []interfaces.GuardConfig{
					{
						Type:        interfaces.GuardTypeVisitCount,
						Workstation: "nonexistent",
						MaxVisits:   3,
					},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for guard referencing non-existent workstation")
	}
}

func TestFactoryDefinitionValidation_RejectsNonClassifierWithoutOutputs(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "process-task",
			Type:           interfaces.WorkstationTypeModel,
			WorkerTypeName: "executor",
			Inputs:         []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
			OnFailure:      []interfaces.IOConfig{{StateName: "failed", WorkTypeName: "task"}},
		}},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation to reject non-classifier workstation without outputs")
	}
	if !strings.Contains(err.Error(), "workstation-outputs") {
		t.Fatalf("expected workstation-outputs validation failure, got %v", err)
	}
}

func TestFactoryDefinitionValidation_RejectsNonClassifierClassificationRoutes(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "process-task",
			Type:           interfaces.WorkstationTypeModel,
			WorkerTypeName: "executor",
			Inputs:         []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
			Outputs:        []interfaces.IOConfig{{StateName: "done", WorkTypeName: "task"}},
			ClassificationRoutes: []interfaces.ClassificationRouteConfig{
				{Label: "approved", Outputs: []interfaces.IOConfig{{StateName: "done", WorkTypeName: "task"}}},
			},
			OnFailure: []interfaces.IOConfig{{StateName: "failed", WorkTypeName: "task"}},
		}},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation to reject non-classifier classificationRoutes")
	}
	if !strings.Contains(err.Error(), "workstation-classification-routes") {
		t.Fatalf("expected workstation-classification-routes validation failure, got %v", err)
	}
}

func TestFactoryDefinitionValidation_RejectsSingleInputWithTwoSameTypeOutputs(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "review", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "splitter",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "review", WorkTypeName: "task"},
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected type-alignment validation error")
	}
	if !strings.Contains(err.Error(), "factory.workstation.conflictingWorkStateOutputs") {
		t.Fatalf("expected conflicting-work-state output finding in error, got %v", err)
	}
}

func TestFactoryDefinitionValidation_RejectsMismatchedCountsAcrossMultiInputSameTypeRoutes(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready-a", Type: interfaces.StateTypeInitial},
					{Name: "ready-b", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "combiner",
				Inputs: []interfaces.IOConfig{
					{StateName: "ready-a", WorkTypeName: "task"},
					{StateName: "ready-b", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected type-alignment validation error")
	}
	if !strings.Contains(err.Error(), "factory.workstation.conflictingWorkStateOutputs") {
		t.Fatalf("expected conflicting-work-state output finding in error, got %v", err)
	}
}
