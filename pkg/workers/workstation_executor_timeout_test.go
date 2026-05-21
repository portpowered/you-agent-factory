package workers

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestWorkstationExecutor_AppliesWorkstationExecutionTimeout(t *testing.T) {
	mock := &wsMockExecutor{
		err: context.DeadlineExceeded,
	}
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
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
		Renderer: &DefaultPromptRenderer{},
	}

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "worker-a",
		WorkstationName: "timed",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if result.Error != "execution timeout" {
		t.Fatalf("Error = %q, want %q", result.Error, "execution timeout")
	}
	if result.ProviderFailure == nil || result.ProviderFailure.Type != interfaces.ProviderErrorTypeTimeout {
		t.Fatalf("ProviderFailure = %#v, want timeout metadata", result.ProviderFailure)
	}
}

func TestWorkstationExecutor_InvalidWorkstationExecutionLimitReturnsActionableFailure(t *testing.T) {
	mock := &wsMockExecutor{}
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
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
		Renderer: &DefaultPromptRenderer{},
	}

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "worker-a",
		WorkstationName: "timed",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.called {
		t.Fatal("executor should not be called when workstation execution limit is invalid")
	}
	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if got, want := result.Error, `invalid workstation limits.maxExecutionTime "not-a-duration": time: invalid duration "not-a-duration"`; got != want {
		t.Fatalf("Error = %q, want %q", got, want)
	}
}

func TestWorkstationExecutor_WorkstationExecutionLimitSetsTimeout(t *testing.T) {
	mock := &deadlineCapturingExecutor{}
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
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
		Renderer: &DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "worker-a",
		WorkstationName: "timed",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
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
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
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
		Renderer: &DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "script-worker",
		WorkstationName: "timed",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
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
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"script-worker": {Type: interfaces.WorkerTypeScript, Timeout: "75ms"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
			},
		},
		Executor: mock,
		Renderer: &DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "script-worker",
		WorkstationName: "timed",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
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
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"script-worker": {Type: interfaces.WorkerTypeScript, Timeout: "1h"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work", Limits: interfaces.WorkstationLimits{MaxExecutionTime: "75ms"}},
			},
		},
		Executor: mock,
		Renderer: &DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "script-worker",
		WorkstationName: "timed",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
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
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"script-worker": {Type: interfaces.WorkerTypeScript},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
			},
		},
		Executor: mock,
		Renderer: &DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "script-worker",
		WorkstationName: "timed",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
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
		workerDef         *interfaces.WorkerConfig
		workstationConfig *interfaces.FactoryWorkstationConfig
	}{
		{
			name:              "worker_zero",
			workerDef:         &interfaces.WorkerConfig{Type: interfaces.WorkerTypeScript, Timeout: "0s"},
			workstationConfig: &interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
		},
		{
			name:              "workstation_zero",
			workerDef:         &interfaces.WorkerConfig{Type: interfaces.WorkerTypeScript},
			workstationConfig: &interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work", Limits: interfaces.WorkstationLimits{MaxExecutionTime: "0s"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &deadlineCapturingExecutor{}
			we := &WorkstationExecutor{
				RuntimeConfig: staticRuntimeConfig{
					Workers: map[string]*interfaces.WorkerConfig{
						"script-worker": tt.workerDef,
					},
					Workstations: map[string]*interfaces.FactoryWorkstationConfig{
						"timed": tt.workstationConfig,
					},
				},
				Executor: mock,
				Renderer: &DefaultPromptRenderer{},
			}

			start := time.Now()
			_, err := we.Execute(context.Background(), interfaces.WorkDispatch{
				DispatchID:      "d-timeout",
				TransitionID:    "t-timeout",
				WorkerType:      "script-worker",
				WorkstationName: "timed",
				InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
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
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"model-worker": {Type: interfaces.WorkerTypeModel, Timeout: "75ms"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
			},
		},
		Executor: mock,
		Renderer: &DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-model-timeout",
		TransitionID:    "t-model-timeout",
		WorkerType:      "model-worker",
		WorkstationName: "standard",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
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
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"model-worker": {Type: interfaces.WorkerTypeModel, Timeout: "20ms"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
			},
		},
		Executor: &contextBlockingExecutor{},
		Renderer: &DefaultPromptRenderer{},
	}

	start := time.Now()
	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-model-timeout",
		TransitionID:    "t-model-timeout",
		WorkerType:      "model-worker",
		WorkstationName: "standard",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed > time.Second {
		t.Fatalf("execution elapsed = %v, want cancellation before 1s", elapsed)
	}
	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if result.Error != "execution timeout" {
		t.Fatalf("Error = %q, want execution timeout", result.Error)
	}
	if result.ProviderFailure == nil || result.ProviderFailure.Type != interfaces.ProviderErrorTypeTimeout {
		t.Fatalf("ProviderFailure = %#v, want timeout metadata", result.ProviderFailure)
	}
}
