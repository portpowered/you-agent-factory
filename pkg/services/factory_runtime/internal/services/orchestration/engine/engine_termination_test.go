package engine

import (
	"context"
	"errors"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems"
)

func TestRunReturnsIncompleteDrainErrorAfterTerminationClassification(t *testing.T) {
	n := buildTestNet()
	terminator := &mockSubsystem{
		group: subsystems.TerminationCheck,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				ShouldTerminate: true,
				Termination: &interfaces.TerminationResult{
					Classification:       interfaces.TerminationClassificationIncomplete,
					NonTerminalWorkCount: 3,
				},
			}, nil
		},
	}
	engine := newTestFactoryEngine(n, petri.NewMarking("test-wf"), []subsystems.Subsystem{terminator})

	err := engine.Run(context.Background())
	if err == nil {
		t.Fatal("Run() returned nil for an incomplete drain")
	}
	var drainErr *factory.IncompleteDrainError
	if !errors.As(err, &drainErr) {
		t.Fatalf("Run() error = %T %v, want IncompleteDrainError", err, err)
	}
	if drainErr.NonTerminalWorkCount != 3 {
		t.Fatalf("non-terminal Work count = %d, want 3", drainErr.NonTerminalWorkCount)
	}
	if !errors.Is(err, factory.ErrIncompleteDrain) {
		t.Fatalf("Run() error = %v, want errors.Is(..., ErrIncompleteDrain)", err)
	}
}

func TestRunReevaluatesTerminationAfterDispatchHookWake(t *testing.T) {
	n := buildTestNet()
	hook := newTestDispatchResultHook()
	checks := 0
	terminator := &mockSubsystem{
		group: subsystems.TerminationCheck,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			checks++
			if checks == 1 {
				hook.SignalBufferedResults()
			}
			classification := interfaces.TerminationClassificationIncomplete
			if checks > 1 {
				classification = interfaces.TerminationClassificationComplete
			}
			return &interfaces.TickResult{
				ShouldTerminate: true,
				Termination: &interfaces.TerminationResult{
					Classification:       classification,
					NonTerminalWorkCount: 1,
				},
			}, nil
		},
	}
	engine := newTestFactoryEngine(n, petri.NewMarking("test-wf"), []subsystems.Subsystem{terminator},
		WithDispatchResultHook(hook),
	)

	if err := engine.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v, want a successful re-evaluation", err)
	}
	if checks != 2 {
		t.Fatalf("termination checks = %d, want 2 after dispatch hook wake", checks)
	}
}
