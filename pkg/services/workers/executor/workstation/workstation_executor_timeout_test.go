package workstation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/executor"
)

type wsMockExecutor struct {
	called bool
	result workerexecution.WorkResult
	err    error
}

type deadlineCapturingExecutor struct {
	deadline    time.Time
	hasDeadline bool
}

type contextBlockingExecutor struct{}

func scriptedTimeoutExecutionPolicy() interfaces.WorkstationExecutionPolicyService {
	return factorydefinitionfixtures.WorkstationExecutionPolicy{
		Resolve: func(workstation *interfaces.FactoryWorkstationConfig) (time.Duration, error) {
			if workstation == nil {
				return 0, nil
			}
			switch workstation.Limits.MaxExecutionTime {
			case "", "0s":
				return 0, nil
			case "20ms":
				return 20 * time.Millisecond, nil
			case "50ms":
				return 50 * time.Millisecond, nil
			case "75ms":
				return 75 * time.Millisecond, nil
			case "not-a-duration":
				return 0, errors.New(`invalid workstation limits.maxExecutionTime "not-a-duration": time: invalid duration "not-a-duration"`)
			default:
				return 0, errors.New("unscripted workstation execution limit")
			}
		},
	}
}

func (m *wsMockExecutor) Execute(_ context.Context, _ workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	m.called = true
	return m.result, m.err
}

func (m *deadlineCapturingExecutor) Execute(ctx context.Context, _ workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	m.deadline, m.hasDeadline = ctx.Deadline()
	return workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted}, nil
}

func (m *contextBlockingExecutor) Execute(ctx context.Context, _ workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	<-ctx.Done()
	return workerexecution.WorkResult{}, ctx.Err()
}

func TestWorkstationExecutor_AppliesWorkstationExecutionTimeout(t *testing.T) {
	mock := &wsMockExecutor{
		err: context.DeadlineExceeded,
	}
	we := &executor.WorkstationExecutor{Now: time.Now, ExecutionPolicy: scriptedTimeoutExecutionPolicy(),
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"worker-a": {Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {
					Type:           interfaces.WorkstationTypeModel,
					PromptTemplate: "do work",
					Limits:         interfaces.WorkstationLimits{MaxExecutionTime: "50ms"},
				},
			},
		},
		Executor: mock,
		Renderer: &executor.DefaultPromptRenderer{},
	}

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "worker-a",
		WorkstationName: "timed",
		InputTokens:     executor.InputTokens(factoryruntime.RuntimeToken{ID: "tok-1", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if result.Error != "execution timeout" {
		t.Fatalf("Error = %q, want %q", result.Error, "execution timeout")
	}
	if result.FailureMetadata == nil || result.FailureMetadata.Type != workerexecution.WorkFailureTypeTimeout {
		t.Fatalf("FailureMetadata = %#v, want timeout metadata", result.FailureMetadata)
	}
}

func TestWorkstationExecutor_InvalidWorkstationExecutionLimitReturnsActionableFailure(t *testing.T) {
	mock := &wsMockExecutor{}
	we := &executor.WorkstationExecutor{Now: time.Now, ExecutionPolicy: scriptedTimeoutExecutionPolicy(),
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system", Timeout: "75ms"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {
					Type:           interfaces.WorkstationTypeModel,
					PromptTemplate: "do work",
					Limits:         interfaces.WorkstationLimits{MaxExecutionTime: "not-a-duration"},
				},
			},
		},
		Executor: mock,
		Renderer: &executor.DefaultPromptRenderer{},
	}

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "worker-a",
		WorkstationName: "timed",
		InputTokens:     executor.InputTokens(factoryruntime.RuntimeToken{ID: "tok-1", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.called {
		t.Fatal("executor should not be called when workstation execution limit is invalid")
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if got, want := result.Error, `invalid workstation limits.maxExecutionTime "not-a-duration": time: invalid duration "not-a-duration"`; got != want {
		t.Fatalf("Error = %q, want %q", got, want)
	}
}

func TestWorkstationExecutor_WorkstationExecutionLimitSetsTimeout(t *testing.T) {
	mock := &deadlineCapturingExecutor{}
	we := &executor.WorkstationExecutor{Now: time.Now, ExecutionPolicy: scriptedTimeoutExecutionPolicy(),
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {
					Type:           interfaces.WorkstationTypeModel,
					PromptTemplate: "do work",
					Limits:         interfaces.WorkstationLimits{MaxExecutionTime: "50ms"},
				},
			},
		},
		Executor: mock,
		Renderer: &executor.DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "worker-a",
		WorkstationName: "timed",
		InputTokens:     executor.InputTokens(factoryruntime.RuntimeToken{ID: "tok-1", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.hasDeadline {
		t.Fatal("expected timeout-derived deadline on executor context")
	}

	remaining := mock.deadline.Sub(start)
	if remaining < 20*time.Millisecond || remaining > 250*time.Millisecond {
		t.Fatalf("deadline offset = %v, want workstation execution-limit range", remaining)
	}
}

func TestWorkstationExecutor_ScriptWorkerTimeoutPrefersWorkstationLimit(t *testing.T) {
	mock := &deadlineCapturingExecutor{}
	we := &executor.WorkstationExecutor{Now: time.Now, ExecutionPolicy: scriptedTimeoutExecutionPolicy(),
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"script-worker": {Type: interfaces.WorkerTypeScript, Timeout: "90m"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {
					Type:           interfaces.WorkstationTypeModel,
					PromptTemplate: "do work",
					Limits:         interfaces.WorkstationLimits{MaxExecutionTime: "50ms"},
				},
			},
		},
		Executor: mock,
		Renderer: &executor.DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "script-worker",
		WorkstationName: "timed",
		InputTokens:     executor.InputTokens(factoryruntime.RuntimeToken{ID: "tok-1", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.hasDeadline {
		t.Fatal("expected timeout-derived deadline on executor context")
	}

	remaining := mock.deadline.Sub(start)
	if remaining < 20*time.Millisecond || remaining > 250*time.Millisecond {
		t.Fatalf("deadline offset = %v, want workstation timeout range", remaining)
	}
}

func TestWorkstationExecutor_ScriptWorkerTimeoutFallsBackToWorkerTimeout(t *testing.T) {
	mock := &deadlineCapturingExecutor{}
	we := &executor.WorkstationExecutor{Now: time.Now, ExecutionPolicy: scriptedTimeoutExecutionPolicy(),
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"script-worker": {Type: interfaces.WorkerTypeScript, Timeout: "75ms"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
			},
		},
		Executor: mock,
		Renderer: &executor.DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "script-worker",
		WorkstationName: "timed",
		InputTokens:     executor.InputTokens(factoryruntime.RuntimeToken{ID: "tok-1", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.hasDeadline {
		t.Fatal("expected timeout-derived deadline on executor context")
	}

	remaining := mock.deadline.Sub(start)
	if remaining < 30*time.Millisecond || remaining > 250*time.Millisecond {
		t.Fatalf("deadline offset = %v, want worker timeout range", remaining)
	}
}

func TestWorkstationExecutor_ExplicitPositiveTimeoutOverridesDefaults(t *testing.T) {
	mock := &deadlineCapturingExecutor{}
	we := &executor.WorkstationExecutor{Now: time.Now, ExecutionPolicy: scriptedTimeoutExecutionPolicy(),
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"script-worker": {Type: interfaces.WorkerTypeScript, Timeout: "1h"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work", Limits: interfaces.WorkstationLimits{MaxExecutionTime: "75ms"}},
			},
		},
		Executor: mock,
		Renderer: &executor.DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "script-worker",
		WorkstationName: "timed",
		InputTokens:     executor.InputTokens(factoryruntime.RuntimeToken{ID: "tok-1", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.hasDeadline {
		t.Fatal("expected timeout-derived deadline on executor context")
	}

	remaining := mock.deadline.Sub(start)
	if remaining < 30*time.Millisecond || remaining > 250*time.Millisecond {
		t.Fatalf("deadline offset = %v, want explicit workstation timeout range", remaining)
	}
}

func TestWorkstationExecutor_ScriptWorkerTimeoutDefaultsToTwoHours(t *testing.T) {
	mock := &deadlineCapturingExecutor{}
	we := &executor.WorkstationExecutor{Now: time.Now, ExecutionPolicy: scriptedTimeoutExecutionPolicy(),
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"script-worker": {Type: interfaces.WorkerTypeScript},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
			},
		},
		Executor: mock,
		Renderer: &executor.DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "script-worker",
		WorkstationName: "timed",
		InputTokens:     executor.InputTokens(factoryruntime.RuntimeToken{ID: "tok-1", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.hasDeadline {
		t.Fatal("expected timeout-derived deadline on executor context")
	}

	remaining := mock.deadline.Sub(start)
	if remaining < 119*time.Minute || remaining > 121*time.Minute {
		t.Fatalf("deadline offset = %v, want approximately 2h", remaining)
	}
}

func TestWorkstationExecutor_ZeroTimeoutDefaultsToTwoHours(t *testing.T) {
	tests := []struct {
		name              string
		workerDef         *interfaces.FactoryWorkerConfig
		workstationConfig *interfaces.FactoryWorkstationConfig
	}{
		{
			name:              "worker_zero",
			workerDef:         &interfaces.FactoryWorkerConfig{Type: interfaces.WorkerTypeScript, Timeout: "0s"},
			workstationConfig: &interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
		},
		{
			name:              "workstation_zero",
			workerDef:         &interfaces.FactoryWorkerConfig{Type: interfaces.WorkerTypeScript},
			workstationConfig: &interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work", Limits: interfaces.WorkstationLimits{MaxExecutionTime: "0s"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &deadlineCapturingExecutor{}
			we := &executor.WorkstationExecutor{Now: time.Now, ExecutionPolicy: scriptedTimeoutExecutionPolicy(),
				RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
					Workers: map[string]*interfaces.FactoryWorkerConfig{
						"script-worker": tt.workerDef,
					},
					Workstations: map[string]*interfaces.FactoryWorkstationConfig{
						"timed": tt.workstationConfig,
					},
				},
				Executor: mock,
				Renderer: &executor.DefaultPromptRenderer{},
			}

			start := time.Now()
			_, err := we.Execute(context.Background(), work.WorkDispatch{
				DispatchID:      "d-timeout",
				TransitionID:    "t-timeout",
				WorkerType:      "script-worker",
				WorkstationName: "timed",
				InputTokens:     executor.InputTokens(factoryruntime.RuntimeToken{ID: "tok-1", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"}}),
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !mock.hasDeadline {
				t.Fatal("expected timeout-derived deadline on executor context")
			}

			remaining := mock.deadline.Sub(start)
			if remaining < 119*time.Minute || remaining > 121*time.Minute {
				t.Fatalf("deadline offset = %v, want approximately 2h", remaining)
			}
		})
	}
}

func TestWorkstationExecutor_ModelWorkerTimeoutFallsBackToWorkerTimeout(t *testing.T) {
	mock := &deadlineCapturingExecutor{}
	we := &executor.WorkstationExecutor{Now: time.Now, ExecutionPolicy: scriptedTimeoutExecutionPolicy(),
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"model-worker": {Type: interfaces.WorkerTypeModel, Timeout: "75ms"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
			},
		},
		Executor: mock,
		Renderer: &executor.DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-model-timeout",
		TransitionID:    "t-model-timeout",
		WorkerType:      "model-worker",
		WorkstationName: "standard",
		InputTokens:     executor.InputTokens(factoryruntime.RuntimeToken{ID: "tok-1", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.hasDeadline {
		t.Fatal("expected timeout-derived deadline on executor context")
	}

	remaining := mock.deadline.Sub(start)
	if remaining < 30*time.Millisecond || remaining > 250*time.Millisecond {
		t.Fatalf("deadline offset = %v, want worker timeout range", remaining)
	}
}

func TestWorkstationExecutor_ModelWorkerTimeoutCancelsLongRunningExecutor(t *testing.T) {
	we := &executor.WorkstationExecutor{Now: time.Now, ExecutionPolicy: scriptedTimeoutExecutionPolicy(),
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"model-worker": {Type: interfaces.WorkerTypeModel, Timeout: "20ms"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
			},
		},
		Executor: &contextBlockingExecutor{},
		Renderer: &executor.DefaultPromptRenderer{},
	}

	start := time.Now()
	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-model-timeout",
		TransitionID:    "t-model-timeout",
		WorkerType:      "model-worker",
		WorkstationName: "standard",
		InputTokens:     executor.InputTokens(factoryruntime.RuntimeToken{ID: "tok-1", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed > time.Second {
		t.Fatalf("execution elapsed = %v, want cancellation before 1s", elapsed)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if result.Error != "execution timeout" {
		t.Fatalf("Error = %q, want execution timeout", result.Error)
	}
	if result.FailureMetadata == nil || result.FailureMetadata.Type != workerexecution.WorkFailureTypeTimeout {
		t.Fatalf("FailureMetadata = %#v, want timeout metadata", result.FailureMetadata)
	}
}
