package engine

import (
	"context"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"strings"
	"testing"
	"time"

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

func TestFactoryEngineSelectedLoggerRunFailureParity(t *testing.T) {
	t.Parallel()
	for _, later := range []bool{false, true} {
		for _, mode := range []string{"capture", "noop"} {
			t.Run(fmt.Sprintf("later=%v/%s", later, mode), func(t *testing.T) {
				t.Parallel()
				core, logs := observer.New(zapcore.DebugLevel)
				logger := logging.NewZapLogger(zap.New(core), false)
				if mode == "noop" {
					logger = logging.NoopLogger{}
				}
				message, cause := runSelectedLoggerFailure(t, logger, later)
				if mode == "noop" {
					if logs.Len() != 0 {
						t.Fatal("quiet failure emitted logs")
					}
					return
				}
				entries := logs.FilterMessage(message).All()
				if len(entries) != 1 || entries[0].Level != zapcore.ErrorLevel || !strings.Contains(fmt.Sprint(entries[0].ContextMap()["error"]), cause.Error()) {
					t.Fatalf("failure diagnostic = %+v", entries)
				}
				if logs.FilterMessage("engine terminated").Len() != 0 || logs.FilterMessage("engine terminated during initial tick pass").Len() != 0 {
					t.Fatal("false success diagnostic")
				}
			})
		}
	}
}

func TestFactoryEngineSelectedLoggerTerminalParity(t *testing.T) {
	t.Parallel()
	for _, classification := range []interfaces.TerminationClassification{interfaces.TerminationClassificationComplete, interfaces.TerminationClassificationIncomplete} {
		for _, mode := range []string{"capture", "noop"} {
			t.Run(string(classification)+"/"+mode, func(t *testing.T) {
				t.Parallel()
				core, logs := observer.New(zapcore.DebugLevel)
				logger := logging.NewZapLogger(zap.New(core), false)
				if mode == "noop" {
					logger = logging.NoopLogger{}
				}
				sub := &mockSubsystem{group: subsystems.TerminationCheck, execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
					return &interfaces.TickResult{ShouldTerminate: true, Termination: &interfaces.TerminationResult{Classification: classification, NonTerminalWorkCount: 3}}, nil
				}}
				engine := newTestFactoryEngineWithLogger(buildTestNet(), petri.NewMarking("terminal"), []subsystems.Subsystem{sub}, logger)
				err := engine.Run(context.Background())
				if classification == interfaces.TerminationClassificationIncomplete {
					var drain *factory.IncompleteDrainError
					if !errors.As(err, &drain) || !errors.Is(err, factory.ErrIncompleteDrain) || drain.NonTerminalWorkCount != 3 {
						t.Fatalf("incomplete outcome = %v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				if sub.callCount != 1 {
					t.Fatalf("terminal calls = %d", sub.callCount)
				}
				if mode == "noop" {
					if logs.Len() != 0 {
						t.Fatal("quiet terminal diagnostics")
					}
					return
				}
				entries := logs.FilterMessage("engine terminated during initial tick pass").All()
				if len(entries) != 1 || entries[0].Level != zapcore.InfoLevel {
					t.Fatalf("terminal diagnostics = %+v", entries)
				}
			})
		}
	}
}

func runSelectedLoggerFailure(t *testing.T, logger logging.Logger, later bool) (string, error) {
	t.Helper()
	cause := errors.New("controlled subsystem failure")
	marking := petri.NewMarking("failure")
	marking.AddToken(&factorytoken.Token{ID: "tok", PlaceID: "task:init", Color: factorytoken.Color{WorkID: "work"}, History: newTestTokenHistory()})
	calls := 0
	hook := newTestDispatchResultHook()
	mover := &mockSubsystem{group: subsystems.CircuitBreaker, execFn: func(_ context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
		if snap.Marking.Tokens["tok"].PlaceID == "task:init" {
			return &interfaces.TickResult{Mutations: []interfaces.MarkingMutation{{Type: interfaces.MutationMove, TokenID: "tok", FromPlace: "task:init", ToPlace: "task:complete"}}}, nil
		}
		return nil, nil
	}}
	failing := &mockSubsystem{group: subsystems.Scheduler, execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
		calls++
		// The first tick mutates; the second proves quiescence before Run waits.
		if later && calls <= 2 {
			if calls == 2 {
				hook.SignalBufferedResults()
			}
			return nil, nil
		}
		return nil, cause
	}}
	after := &mockSubsystem{group: subsystems.Tracer}
	engine := newTestFactoryEngineWithLogger(buildTestNet(), marking, []subsystems.Subsystem{after, failing, mover}, logger, WithDispatchResultHook(hook))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := engine.Run(ctx)
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "subsystem") {
		t.Fatalf("Run error = %v", err)
	}
	wantAfter := 0
	message := "engine initial tick error"
	if later {
		wantAfter = 2
		message = "engine tick error"
	}
	if after.callCount != wantAfter || engine.GetMarking().Tokens["tok"].PlaceID != "task:complete" {
		t.Fatalf("partial state/later work changed: calls=%d marking=%+v", after.callCount, engine.GetMarking())
	}

	return message, cause
}
