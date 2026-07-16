package workstation_test

import (
	"context"
	"testing"
	"time"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers/executor"
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
	we := &executor.WorkstationExecutor{
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*workerconfig.Config{
				"worker-a": {Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {
					Type:           workertaxonomy.WorkstationTypeModel,
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
		InputTokens:     executor.InputTokens(factorytoken.Token{ID: "tok-1", Color: factorytoken.Color{WorkID: "work-1"}}),
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
	we := &executor.WorkstationExecutor{
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*workerconfig.Config{
				"worker-a": {Type: workertaxonomy.WorkerTypeModel, Body: "system", Timeout: "75ms"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {
					Type:           workertaxonomy.WorkstationTypeModel,
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
		InputTokens:     executor.InputTokens(factorytoken.Token{ID: "tok-1", Color: factorytoken.Color{WorkID: "work-1"}}),
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
	we := &executor.WorkstationExecutor{
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*workerconfig.Config{
				"worker-a": {Type: workertaxonomy.WorkerTypeModel, Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {
					Type:           workertaxonomy.WorkstationTypeModel,
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
		InputTokens:     executor.InputTokens(factorytoken.Token{ID: "tok-1", Color: factorytoken.Color{WorkID: "work-1"}}),
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
	we := &executor.WorkstationExecutor{
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*workerconfig.Config{
				"script-worker": {Type: workertaxonomy.WorkerTypeScript, Timeout: "90m"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {
					Type:           workertaxonomy.WorkstationTypeModel,
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
		InputTokens:     executor.InputTokens(factorytoken.Token{ID: "tok-1", Color: factorytoken.Color{WorkID: "work-1"}}),
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
	we := &executor.WorkstationExecutor{
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*workerconfig.Config{
				"script-worker": {Type: workertaxonomy.WorkerTypeScript, Timeout: "75ms"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {Type: workertaxonomy.WorkstationTypeModel, PromptTemplate: "do work"},
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
		InputTokens:     executor.InputTokens(factorytoken.Token{ID: "tok-1", Color: factorytoken.Color{WorkID: "work-1"}}),
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
	we := &executor.WorkstationExecutor{
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*workerconfig.Config{
				"script-worker": {Type: workertaxonomy.WorkerTypeScript, Timeout: "1h"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {Type: workertaxonomy.WorkstationTypeModel, PromptTemplate: "do work", Limits: interfaces.WorkstationLimits{MaxExecutionTime: "75ms"}},
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
		InputTokens:     executor.InputTokens(factorytoken.Token{ID: "tok-1", Color: factorytoken.Color{WorkID: "work-1"}}),
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
	we := &executor.WorkstationExecutor{
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*workerconfig.Config{
				"script-worker": {Type: workertaxonomy.WorkerTypeScript},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {Type: workertaxonomy.WorkstationTypeModel, PromptTemplate: "do work"},
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
		InputTokens:     executor.InputTokens(factorytoken.Token{ID: "tok-1", Color: factorytoken.Color{WorkID: "work-1"}}),
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
		workerDef         *workerconfig.Config
		workstationConfig *interfaces.FactoryWorkstationConfig
	}{
		{
			name:              "worker_zero",
			workerDef:         &workerconfig.Config{Type: workertaxonomy.WorkerTypeScript, Timeout: "0s"},
			workstationConfig: &interfaces.FactoryWorkstationConfig{Type: workertaxonomy.WorkstationTypeModel, PromptTemplate: "do work"},
		},
		{
			name:              "workstation_zero",
			workerDef:         &workerconfig.Config{Type: workertaxonomy.WorkerTypeScript},
			workstationConfig: &interfaces.FactoryWorkstationConfig{Type: workertaxonomy.WorkstationTypeModel, PromptTemplate: "do work", Limits: interfaces.WorkstationLimits{MaxExecutionTime: "0s"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &deadlineCapturingExecutor{}
			we := &executor.WorkstationExecutor{
				RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
					Workers: map[string]*workerconfig.Config{
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
				InputTokens:     executor.InputTokens(factorytoken.Token{ID: "tok-1", Color: factorytoken.Color{WorkID: "work-1"}}),
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
	we := &executor.WorkstationExecutor{
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*workerconfig.Config{
				"model-worker": {Type: workertaxonomy.WorkerTypeModel, Timeout: "75ms"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {Type: workertaxonomy.WorkstationTypeModel, PromptTemplate: "do work"},
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
		InputTokens:     executor.InputTokens(factorytoken.Token{ID: "tok-1", Color: factorytoken.Color{WorkID: "work-1"}}),
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
	we := &executor.WorkstationExecutor{
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workers: map[string]*workerconfig.Config{
				"model-worker": {Type: workertaxonomy.WorkerTypeModel, Timeout: "20ms"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {Type: workertaxonomy.WorkstationTypeModel, PromptTemplate: "do work"},
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
		InputTokens:     executor.InputTokens(factorytoken.Token{ID: "tok-1", Color: factorytoken.Color{WorkID: "work-1"}}),
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
