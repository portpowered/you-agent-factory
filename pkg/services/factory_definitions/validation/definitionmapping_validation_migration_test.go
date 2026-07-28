// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
// pkgmaintcheck:ignore-file-lines service-ownership migration preserves this consolidated file; split responsibilities and remove this exemption.
package validation

import (
	"context"
	"fmt"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func validateFactoryDefinition(
	ctx context.Context,
	cfg *factorydefinitions.FactoryConfig,
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []factorydefinitions.StateConfig{
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "collector",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &factorydefinitions.InputGuardConfig{
							Type: factorydefinitions.GuardTypeAllChildrenComplete,
						},
					},
				},
				Outputs: []factorydefinitions.IOConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "page",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "bad-guard",
				Inputs: []factorydefinitions.IOConfig{
					{
						StateName:    "init",
						WorkTypeName: "page",
						Guard: &factorydefinitions.InputGuardConfig{
							Type:        factorydefinitions.GuardTypeAllChildrenComplete,
							ParentInput: "page", // Self-reference.
						},
					},
				},
				Outputs: []factorydefinitions.IOConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []factorydefinitions.StateConfig{
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "collector",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &factorydefinitions.InputGuardConfig{
							Type:        factorydefinitions.GuardTypeAllChildrenComplete,
							ParentInput: "nonexistent",
						},
					},
				},
				Outputs: []factorydefinitions.IOConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []factorydefinitions.StateConfig{
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "collector",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &factorydefinitions.InputGuardConfig{
							Type:        factorydefinitions.GuardTypeAllChildrenComplete,
							ParentInput: "task",
							SpawnedBy:   "nonexistent-workstation",
						},
					},
				},
				Outputs: []factorydefinitions.IOConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []factorydefinitions.StateConfig{
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "collector",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &factorydefinitions.InputGuardConfig{
							Type:        factorydefinitions.GuardTypeVisitCount,
							ParentInput: "task",
						},
					},
				},
				Outputs: []factorydefinitions.IOConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "plan",
				States: []factorydefinitions.StateConfig{
					{Name: "ready", Type: factorydefinitions.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "ready", Type: factorydefinitions.StateTypeProcessing},
					{Name: "matched", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "ready", WorkTypeName: "plan"},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &factorydefinitions.InputGuardConfig{
							Type: factorydefinitions.GuardTypeSameName,
						},
					},
				},
				Outputs: []factorydefinitions.IOConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "plan",
				States: []factorydefinitions.StateConfig{
					{Name: "ready", Type: factorydefinitions.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "ready", Type: factorydefinitions.StateTypeProcessing},
					{Name: "matched", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []factorydefinitions.IOConfig{
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
						Guard: &factorydefinitions.InputGuardConfig{
							Type:       factorydefinitions.GuardTypeSameName,
							MatchInput: "task",
						},
					},
				},
				Outputs: []factorydefinitions.IOConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "plan",
				States: []factorydefinitions.StateConfig{
					{Name: "ready", Type: factorydefinitions.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "ready", Type: factorydefinitions.StateTypeProcessing},
					{Name: "matched", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "ready", WorkTypeName: "plan"},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &factorydefinitions.InputGuardConfig{
							Type:       factorydefinitions.GuardTypeSameName,
							MatchInput: "other",
						},
					},
				},
				Outputs: []factorydefinitions.IOConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "plan",
				States: []factorydefinitions.StateConfig{
					{Name: "ready", Type: factorydefinitions.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "ready", Type: factorydefinitions.StateTypeProcessing},
					{Name: "matched", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "ready", WorkTypeName: "plan"},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &factorydefinitions.InputGuardConfig{
							Type: factorydefinitions.GuardTypeSameTraceID,
						},
					},
				},
				Outputs: []factorydefinitions.IOConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "plan",
				States: []factorydefinitions.StateConfig{
					{Name: "ready", Type: factorydefinitions.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "ready", Type: factorydefinitions.StateTypeProcessing},
					{Name: "matched", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []factorydefinitions.IOConfig{
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
						Guard: &factorydefinitions.InputGuardConfig{
							Type:       factorydefinitions.GuardTypeSameTraceID,
							MatchInput: "task",
						},
					},
				},
				Outputs: []factorydefinitions.IOConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "plan",
				States: []factorydefinitions.StateConfig{
					{Name: "ready", Type: factorydefinitions.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "ready", Type: factorydefinitions.StateTypeProcessing},
					{Name: "matched", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "ready", WorkTypeName: "plan"},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &factorydefinitions.InputGuardConfig{
							Type:       factorydefinitions.GuardTypeSameTraceID,
							MatchInput: "other",
						},
					},
				},
				Outputs: []factorydefinitions.IOConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []factorydefinitions.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Resources: []factorydefinitions.ResourceConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Resources: []factorydefinitions.ResourceConfig{
			{Name: "gpu", Capacity: 2},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []factorydefinitions.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Resources: []factorydefinitions.ResourceConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "processor",
				Kind: "unknown_kind",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []factorydefinitions.IOConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workers: []factorydefinitions.FactoryWorkerConfig{
			{Name: "real-worker"},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name:           "processor",
				WorkerTypeName: "ghost-worker",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []factorydefinitions.IOConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []factorydefinitions.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				OnRejection: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "nonexistent"}},
			},
		},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for on_rejection pointing to non-existent state")
	}
}

func TestFactoryDefinitionValidation_RejectsInvalidOnFailure(t *testing.T) {
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
					{Name: "failed", Type: factorydefinitions.StateTypeFailed},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []factorydefinitions.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				OnFailure: []factorydefinitions.IOConfig{{WorkTypeName: "nonexistent-type", StateName: "failed"}},
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
		guardType factorydefinitions.GuardType
	}{
		{name: "all children complete", guardType: factorydefinitions.GuardTypeAllChildrenComplete},
		{name: "any child failed", guardType: factorydefinitions.GuardTypeAnyChildFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &factorydefinitions.FactoryConfig{
				WorkTypes: []factorydefinitions.WorkTypeConfig{
					{
						Name: "task",
						States: []factorydefinitions.StateConfig{
							{Name: "init", Type: factorydefinitions.StateTypeInitial},
							{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
							{Name: "failed", Type: factorydefinitions.StateTypeFailed},
						},
					},
				},
				Workstations: []factorydefinitions.FactoryWorkstationConfig{
					{
						Name: "collector",
						Inputs: []factorydefinitions.IOConfig{
							{StateName: "init", WorkTypeName: "task"},
						},
						Outputs: []factorydefinitions.IOConfig{
							{StateName: "complete", WorkTypeName: "task"},
						},
						Guards: []factorydefinitions.GuardConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []factorydefinitions.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Guards: []factorydefinitions.GuardConfig{
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
	input := &factorydefinitions.FactoryConfig{
		Guards: []factorydefinitions.FactoryGuardConfig{{
			Type:          factorydefinitions.GuardTypeInferenceThrottle,
			RefreshWindow: "15m",
		}},
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
			},
		}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name: "processor",
			Inputs: []factorydefinitions.IOConfig{
				{StateName: "init", WorkTypeName: "task"},
			},
			Outputs: []factorydefinitions.IOConfig{
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
	input := &factorydefinitions.FactoryConfig{
		Guards: []factorydefinitions.FactoryGuardConfig{{
			Type:          factorydefinitions.GuardTypeInferenceThrottle,
			ModelProvider: "claude",
			RefreshWindow: "later",
		}},
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
			},
		}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name: "processor",
			Inputs: []factorydefinitions.IOConfig{
				{StateName: "init", WorkTypeName: "task"},
			},
			Outputs: []factorydefinitions.IOConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
			},
		}},
		Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "matcher"}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name:           "processor",
			WorkerTypeName: "matcher",
			Inputs:         []factorydefinitions.IOConfig{{StateName: "init", WorkTypeName: "task"}},
			Outputs:        []factorydefinitions.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
			Guards:         []factorydefinitions.GuardConfig{{Type: factorydefinitions.GuardTypeMatchesFields}},
		}},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for matches_fields guard missing matchConfig.inputKey")
	}
}

func TestFactoryDefinitionValidation_RejectsMatchesFieldsEmptyInputKey(t *testing.T) {
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
			},
		}},
		Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "matcher"}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name:           "processor",
			WorkerTypeName: "matcher",
			Inputs:         []factorydefinitions.IOConfig{{StateName: "init", WorkTypeName: "task"}},
			Outputs:        []factorydefinitions.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
			Guards: []factorydefinitions.GuardConfig{{
				Type:        factorydefinitions.GuardTypeMatchesFields,
				MatchConfig: &factorydefinitions.GuardMatchConfig{InputKey: " "},
			}},
		}},
	}

	err := validateFactoryDefinition(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for matches_fields guard empty matchConfig.inputKey")
	}
}

func TestFactoryDefinitionValidation_RejectsVisitCountGuardMissingParams(t *testing.T) {
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []factorydefinitions.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Guards: []factorydefinitions.GuardConfig{
					{Type: factorydefinitions.GuardTypeVisitCount},
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []factorydefinitions.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Guards: []factorydefinitions.GuardConfig{
					{
						Type:        factorydefinitions.GuardTypeVisitCount,
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "done", Type: factorydefinitions.StateTypeTerminal},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed},
			},
		}},
		Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "executor"}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name:           "process-task",
			Type:           factorydefinitions.WorkstationTypeModel,
			WorkerTypeName: "executor",
			Inputs:         []factorydefinitions.IOConfig{{StateName: "init", WorkTypeName: "task"}},
			OnFailure:      []factorydefinitions.IOConfig{{StateName: "failed", WorkTypeName: "task"}},
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "done", Type: factorydefinitions.StateTypeTerminal},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed},
			},
		}},
		Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "executor"}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name:           "process-task",
			Type:           factorydefinitions.WorkstationTypeModel,
			WorkerTypeName: "executor",
			Inputs:         []factorydefinitions.IOConfig{{StateName: "init", WorkTypeName: "task"}},
			Outputs:        []factorydefinitions.IOConfig{{StateName: "done", WorkTypeName: "task"}},
			ClassificationRoutes: []factorydefinitions.ClassificationRouteConfig{
				{Label: "approved", Outputs: []factorydefinitions.IOConfig{{StateName: "done", WorkTypeName: "task"}}},
			},
			OnFailure: []factorydefinitions.IOConfig{{StateName: "failed", WorkTypeName: "task"}},
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "review", Type: factorydefinitions.StateTypeProcessing},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
					{Name: "failed", Type: factorydefinitions.StateTypeFailed},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "splitter",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []factorydefinitions.IOConfig{
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
	input := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "ready-a", Type: factorydefinitions.StateTypeInitial},
					{Name: "ready-b", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
					{Name: "failed", Type: factorydefinitions.StateTypeFailed},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "combiner",
				Inputs: []factorydefinitions.IOConfig{
					{StateName: "ready-a", WorkTypeName: "task"},
					{StateName: "ready-b", WorkTypeName: "task"},
				},
				Outputs: []factorydefinitions.IOConfig{
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
