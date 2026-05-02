package factory

import (
	"context"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestWithInlineDispatch_EnablesInlineDispatch(t *testing.T) {
	cfg := &FactoryConfig{}

	WithInlineDispatch()(cfg)

	if !cfg.IsInlineDispatch() {
		t.Fatal("expected InlineDispatch to be enabled")
	}
}

func TestWithRuntimeMode_DefaultsEmptyValueToBatch(t *testing.T) {
	cfg := &FactoryConfig{}

	WithRuntimeMode("")(cfg)

	if cfg.RuntimeMode != interfaces.RuntimeModeBatch {
		t.Fatalf("RuntimeMode = %q, want %q", cfg.RuntimeMode, interfaces.RuntimeModeBatch)
	}
}

func TestWithServiceMode_SetsServiceRuntimeMode(t *testing.T) {
	cfg := &FactoryConfig{}

	WithServiceMode()(cfg)

	if cfg.RuntimeMode != interfaces.RuntimeModeService {
		t.Fatalf("RuntimeMode = %q, want %q", cfg.RuntimeMode, interfaces.RuntimeModeService)
	}
}

func TestFactoryOptions_PreserveSupportedCoreConstructionSurface(t *testing.T) {
	cfg := &FactoryConfig{}
	net := &state.Net{ID: "test-net"}
	sched := scheduler.NewWorkInQueueScheduler(1)
	executor := &workers.NoopExecutor{}
	runtimeCfg := stubRuntimeConfig(&runtimefixtures.RuntimeDefinitionLookupFixture{})
	workflowContext := &factory_context.FactoryContext{ProjectID: "test-project"}
	clock := fixedClock{now: time.Unix(1, 0)}
	logger := logging.NoopLogger{}
	planner := stubCompletionDeliveryPlanner{}

	options := []FactoryOption{
		WithNet(net),
		WithScheduler(sched),
		WithLogger(logger),
		WithClock(clock),
		WithRuntimeMode(interfaces.RuntimeModeService),
		WithWorkerExecutor("mock", executor),
		WithRuntimeConfig(runtimeCfg),
		WithWorkflowContext(workflowContext),
		WithInlineDispatch(),
		WithProviderThrottlePauseDuration(time.Second),
		WithCompletionDeliveryPlanner(planner),
	}

	for _, opt := range options {
		opt(cfg)
	}

	if cfg.GetNet() != net {
		t.Fatal("expected WithNet to preserve the provided net")
	}
	if cfg.Scheduler != sched {
		t.Fatal("expected WithScheduler to preserve the provided scheduler")
	}
	if cfg.Logger != logger {
		t.Fatal("expected WithLogger to preserve the provided logger")
	}
	if cfg.Clock != clock {
		t.Fatal("expected WithClock to preserve the provided clock")
	}
	if cfg.RuntimeMode != interfaces.RuntimeModeService {
		t.Fatalf("RuntimeMode = %q, want %q", cfg.RuntimeMode, interfaces.RuntimeModeService)
	}
	if cfg.WorkerExecutors["mock"] != executor {
		t.Fatal("expected WithWorkerExecutor to register the provided executor")
	}
	if cfg.RuntimeConfig != runtimeCfg {
		t.Fatal("expected WithRuntimeConfig to preserve runtime config")
	}
	if cfg.WorkflowContext != workflowContext {
		t.Fatal("expected WithWorkflowContext to preserve the provided context")
	}
	if !cfg.IsInlineDispatch() {
		t.Fatal("expected WithInlineDispatch to enable inline dispatch")
	}
	if cfg.ProviderThrottlePauseDuration != time.Second {
		t.Fatalf("ProviderThrottlePauseDuration = %s, want %s", cfg.ProviderThrottlePauseDuration, time.Second)
	}
	if cfg.CompletionDeliveryPlanner != planner {
		t.Fatal("expected WithCompletionDeliveryPlanner to preserve the provided planner")
	}
}

func TestFactoryOptions_PreserveSupportedHookAndRecorderSurface(t *testing.T) {
	cfg := &FactoryConfig{}
	hook := stubSubmissionHook{}
	submissionRecorderCalled := false
	dispatchRecorderCalled := false
	completionRecorderCalled := false
	factoryEventRecorderCalled := false

	options := []FactoryOption{
		WithSubmissionHook(hook),
		WithSubmissionRecorder(func(interfaces.FactorySubmissionRecord) {
			submissionRecorderCalled = true
		}),
		WithDispatchRecorder(func(interfaces.FactoryDispatchRecord) {
			dispatchRecorderCalled = true
		}),
		WithCompletionRecorder(func(interfaces.FactoryCompletionRecord) {
			completionRecorderCalled = true
		}),
		WithFactoryEventRecorder(func(factoryapi.FactoryEvent) {
			factoryEventRecorderCalled = true
		}),
	}

	for _, opt := range options {
		opt(cfg)
	}

	if len(cfg.SubmissionHooks) != 1 {
		t.Fatalf("SubmissionHooks length = %d, want 1", len(cfg.SubmissionHooks))
	}
	cfg.SubmissionRecorder(interfaces.FactorySubmissionRecord{})
	cfg.DispatchRecorder(interfaces.FactoryDispatchRecord{})
	cfg.CompletionRecorder(interfaces.FactoryCompletionRecord{})
	cfg.FactoryEventRecorder(factoryapi.FactoryEvent{})

	if !submissionRecorderCalled || !dispatchRecorderCalled || !completionRecorderCalled || !factoryEventRecorderCalled {
		t.Fatal("expected supported recorder options to preserve callbacks")
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type stubSubmissionHook struct{}

func (stubSubmissionHook) Name() string {
	return "stub"
}

func (stubSubmissionHook) Priority() int {
	return 0
}

func (stubSubmissionHook) OnTick(context.Context, interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.SubmissionHookResult, error) {
	return interfaces.SubmissionHookResult{}, nil
}

type stubCompletionDeliveryPlanner struct{}

func (stubCompletionDeliveryPlanner) DeliveryTickForDispatch(interfaces.WorkDispatch) (int, bool, error) {
	return 0, false, nil
}

type stubRuntimeConfig = *runtimefixtures.RuntimeDefinitionLookupFixture
