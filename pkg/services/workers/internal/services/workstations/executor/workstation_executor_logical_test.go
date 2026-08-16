package executor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestWorkstationExecutorExecutesOnlyLogicalWorkstations(t *testing.T) {
	clock := func() time.Time { return time.Unix(10, 0) }
	runner := &WorkstationExecutor{
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"logical": {Name: "logical", Type: interfaces.WorkstationTypeLogical},
				"model":   {Name: "model", Type: interfaces.WorkstationTypeModel},
			},
		},
		Now: clock,
	}

	logical, err := runner.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "dispatch-logical",
		TransitionID:    "transition-logical",
		WorkstationName: "logical",
	})
	if err != nil {
		t.Fatalf("logical Execute() error = %v", err)
	}
	if logical.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("logical outcome = %q, want %q", logical.Outcome, workerexecution.OutcomeAccepted)
	}

	nonLogical, err := runner.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "dispatch-model",
		TransitionID:    "transition-model",
		WorkstationName: "model",
	})
	if err != nil {
		t.Fatalf("non-logical Execute() error = %v", err)
	}
	if nonLogical.Outcome != workerexecution.OutcomeFailed ||
		nonLogical.Error != "non-logical workstation execution is owned by Workers.Execute" {
		t.Fatalf("non-logical result = %#v, want Workers-owned failure", nonLogical)
	}

	missing, err := runner.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "dispatch-missing",
		TransitionID:    "transition-missing",
		WorkstationName: "missing",
	})
	if err != nil {
		t.Fatalf("missing Execute() error = %v", err)
	}
	if missing.Outcome != workerexecution.OutcomeFailed ||
		!strings.Contains(missing.Error, "workstation not found: missing") {
		t.Fatalf("missing result = %#v, want lookup failure", missing)
	}
}

func TestWorkstationExecutorRejectsInvalidExecutionContext(t *testing.T) {
	runner := &WorkstationExecutor{Now: func() time.Time { return time.Unix(10, 0) }}
	if _, err := runner.Execute(nil, work.WorkDispatch{}); err == nil {
		t.Fatal("Execute(nil context) error = nil, want an error")
	}

	if _, err := (&WorkstationExecutor{}).Execute(context.Background(), work.WorkDispatch{}); err == nil {
		t.Fatal("Execute() with no clock error = nil, want an error")
	}

	var nilRunner *WorkstationExecutor
	if _, err := nilRunner.Execute(context.Background(), work.WorkDispatch{}); err == nil {
		t.Fatal("nil executor error = nil, want an error")
	}
}
