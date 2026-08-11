package outcometests

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
)

func TestValidate_RequiresCompletionForRecurringControllerShape(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "controller",
				States: []factorydefinitions.StateConfig{
					{Name: "active", Type: factorydefinitions.StateTypeInitial},
					{Name: "failed", Type: factorydefinitions.StateTypeFailed},
				},
			},
			{
				Name: "execution",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
					{Name: "failed", Type: factorydefinitions.StateTypeFailed},
				},
			},
		},
		Workers: []factorydefinitions.FactoryWorkerConfig{
			{Name: "trigger", Type: factorydefinitions.WorkerTypeAgent},
			{Name: "executor", Type: factorydefinitions.WorkerTypeAgent},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name:           "schedule",
				Kind:           factorydefinitions.WorkstationKindCron,
				WorkerTypeName: "trigger",
				Cron:           &factorydefinitions.CronConfig{Every: "1m"},
				Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "controller", StateName: "active"}},
				Outputs: []factorydefinitions.IOConfig{
					{WorkTypeName: "controller", StateName: "active"},
					{WorkTypeName: "execution", StateName: "init"},
				},
				OnFailure: []factorydefinitions.IOConfig{{WorkTypeName: "controller", StateName: "failed"}},
			},
			{
				Name:           "execute",
				WorkerTypeName: "executor",
				Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "execution", StateName: "init"}},
				Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "execution", StateName: "complete"}},
				OnFailure:      []factorydefinitions.IOConfig{{WorkTypeName: "execution", StateName: "failed"}},
			},
		},
	}

	assertMissingCompletionTarget(t, factoryvalidation.Validate(cfg), "controller")
}

func TestValidate_RequiresCompletionForCompositeParentShape(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "request",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "waiting", Type: factorydefinitions.StateTypeProcessing},
					{Name: "failed", Type: factorydefinitions.StateTypeFailed},
				},
			},
			{
				Name: "merge",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
					{Name: "failed", Type: factorydefinitions.StateTypeFailed},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name:   "split",
				Type:   factorydefinitions.WorkstationTypeLogical,
				Inputs: []factorydefinitions.IOConfig{{WorkTypeName: "request", StateName: "init"}},
				Outputs: []factorydefinitions.IOConfig{
					{WorkTypeName: "request", StateName: "waiting"},
					{WorkTypeName: "merge", StateName: "init"},
				},
				OnFailure: []factorydefinitions.IOConfig{{WorkTypeName: "request", StateName: "failed"}},
			},
			{
				Name:    "finish",
				Type:    factorydefinitions.WorkstationTypeLogical,
				Inputs:  []factorydefinitions.IOConfig{{WorkTypeName: "request", StateName: "waiting"}},
				Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "merge", StateName: "complete"}},
			},
		},
	}

	assertMissingCompletionTarget(t, factoryvalidation.Validate(cfg), "request")
}

func TestValidate_RejectsNonTerminalWorkTypeWithoutDelegatedOutcome(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed},
			},
		}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name:    "stuck",
			Type:    factorydefinitions.WorkstationTypeLogical,
			Inputs:  []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			OnFailure: []factorydefinitions.IOConfig{{
				WorkTypeName: "task",
				StateName:    "failed",
			}},
		}},
	}

	assertMissingCompletionTarget(t, factoryvalidation.Validate(cfg), "task")
}

func assertMissingCompletionTarget(t *testing.T, result factorydefinitions.ValidationResult, workType string) {
	t.Helper()
	for _, target := range result.BlockingTargets() {
		if target.Code == factoryvalidation.CodeWorkTypeMissingCompletionState && target.Subject.ID == workType {
			return
		}
	}
	t.Fatalf("validation targets = %#v, want missing completion-state target for %q", result.BlockingTargets(), workType)
}
