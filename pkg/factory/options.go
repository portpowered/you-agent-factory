package factory

import (
	"context"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// FactoryOption configures a factoryImpl via the functional options pattern.
type FactoryOption func(*FactoryConfig)

// FactoryConfig holds all configurable settings for factory construction.
// Fields are set via functional options (With* functions). The net field
// is package-private; use GetNet() to read it from outside the package.
type FactoryConfig struct {
	net                           *state.Net
	Scheduler                     scheduler.Scheduler
	WorkerExecutors               map[string]workers.WorkerExecutor
	RuntimeConfig                 interfaces.RuntimeDefinitionLookup
	WorkflowContext               *factory_context.FactoryContext
	RuntimeMode                   interfaces.RuntimeMode
	Logger                        logging.Logger
	Clock                         Clock
	ProviderThrottlePauseDuration time.Duration
	EventHistory                  *FactoryEventHistory
	SubmissionRecorder            SubmissionRecorder
	FactoryEventRecorder          FactoryEventRecorder
	SubmissionHooks               []SubmissionHook
	DispatchRecorder              DispatchRecorder
	CompletionRecorder            CompletionRecorder
	CompletionDeliveryPlanner     CompletionDeliveryPlanner
	// inlineDispatch enables synchronous dispatch mode through registered
	// worker executors. When true, dispatches are executed inline during
	// engine ticks instead of being routed through the async worker pool.
	inlineDispatch bool
}

// GetNet returns the CPN net definition.
func (c *FactoryConfig) GetNet() *state.Net { return c.net }

// IsInlineDispatch returns whether synchronous inline dispatch is enabled.
func (c *FactoryConfig) IsInlineDispatch() bool { return c.inlineDispatch }

// SubmissionRecorder receives authoritative submission observations from the
// engine before submitted work is injected into the marking.
type SubmissionRecorder func(interfaces.FactorySubmissionRecord)

// DispatchRecorder receives authoritative dispatch observations from the
// engine after in-flight tracking is updated and before worker submission.
type DispatchRecorder func(interfaces.FactoryDispatchRecord)

// CompletionRecorder receives completed worker results after dispatch/result
// hooks make them visible to the engine at a logical tick boundary.
type CompletionRecorder func(interfaces.FactoryCompletionRecord)

// FactoryEventRecorder receives canonical generated FactoryEvent messages in
// append order as runtime history records them.
type FactoryEventRecorder func(factoryapi.FactoryEvent)

// SubmissionHook provides generated work batches, results, and events that
// become visible to the engine at deterministic tick boundaries.
type SubmissionHook interface {
	Name() string
	Priority() int
	OnTick(ctx context.Context, input interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.SubmissionHookResult, error)
}

// DispatchResultHook bridges engine-owned dispatch creation with worker
// execution and tick-owned result delivery.
type DispatchResultHook interface {
	SubmitDispatch(ctx context.Context, dispatch interfaces.WorkDispatch) error
	OnTick(ctx context.Context, input interfaces.DispatchResultHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.DispatchResultHookResult, error)
	WaitCh() <-chan struct{}
}

// CompletionDeliveryPlanner maps a runtime dispatch to the logical tick at
// which a completed worker result may become visible.
type CompletionDeliveryPlanner interface {
	DeliveryTickForDispatch(dispatch interfaces.WorkDispatch) (int, bool, error)
}

// WithNet sets the CPN definition for the factory. Required.
func WithNet(n *state.Net) FactoryOption {
	return func(c *FactoryConfig) {
		c.net = n
	}
}

// WithScheduler sets the scheduling strategy. Default: FIFO.
func WithScheduler(s scheduler.Scheduler) FactoryOption {
	return func(c *FactoryConfig) {
		c.Scheduler = s
	}
}

// WithWorkerExecutor registers a worker executor for the given worker type.
func WithWorkerExecutor(workerType string, e workers.WorkerExecutor) FactoryOption {
	return func(c *FactoryConfig) {
		if c.WorkerExecutors == nil {
			c.WorkerExecutors = make(map[string]workers.WorkerExecutor)
		}
		c.WorkerExecutors[workerType] = e
	}
}

// WithRuntimeConfig sets the authoritative runtime-loaded worker/workstation
// config used by subsystems that need AGENTS.md-backed execution metadata.
func WithRuntimeConfig(runtimeCfg interfaces.RuntimeDefinitionLookup) FactoryOption {
	return func(c *FactoryConfig) {
		c.RuntimeConfig = runtimeCfg
	}
}

// WithWorkflowContext sets execution context exposed to prompt and workstation
// field templates.
func WithWorkflowContext(wfCtx *factory_context.FactoryContext) FactoryOption {
	return func(c *FactoryConfig) {
		c.WorkflowContext = wfCtx
	}
}

// WithRuntimeMode sets the runtime lifecycle mode. Batch mode terminates on
// idle completion; service mode stays alive until the run context is canceled.
func WithRuntimeMode(mode interfaces.RuntimeMode) FactoryOption {
	return func(c *FactoryConfig) {
		if mode == "" {
			mode = interfaces.RuntimeModeBatch
		}
		c.RuntimeMode = mode
	}
}

// WithServiceMode keeps the runtime alive while idle so callers can submit new
// work after startup. This is a convenience wrapper around WithRuntimeMode.
func WithServiceMode() FactoryOption {
	return WithRuntimeMode(interfaces.RuntimeModeService)
}

// WithLogger sets the logger for the factory. Default: no-op.
func WithLogger(l logging.Logger) FactoryOption {
	return func(c *FactoryConfig) {
		c.Logger = l
	}
}

// WithClock sets the runtime time source used by engine and subsystem paths.
// Nil clocks are ignored and the runtime defaults to RealClock.
func WithClock(clock Clock) FactoryOption {
	return func(c *FactoryConfig) {
		if clock != nil {
			c.Clock = clock
		}
	}
}

// WithInlineDispatch enables synchronous inline dispatch mode. When enabled,
// dispatches are executed inline during engine ticks through the registered
// worker executors instead of being routed through the async worker pool.
// Used by the test harness for deterministic tick-based testing.
func WithInlineDispatch() FactoryOption {
	return func(c *FactoryConfig) {
		c.inlineDispatch = true
	}
}

// WithProviderThrottlePauseDuration overrides the internal runtime pause window
// used after a normalized throttling failure for a specific provider/model lane.
func WithProviderThrottlePauseDuration(d time.Duration) FactoryOption {
	return func(c *FactoryConfig) {
		c.ProviderThrottlePauseDuration = d
	}
}

// WithFactoryEventHistory injects a preconstructed canonical event history.
// This lets service wiring provide the same append surface to provider
// wrappers before worker executors are constructed.
func WithFactoryEventHistory(history *FactoryEventHistory) FactoryOption {
	return func(c *FactoryConfig) {
		if history != nil {
			c.EventHistory = history
		}
	}
}

// WithSubmissionRecorder records submissions after hook output is observed and
// before the engine injects tokens.
func WithSubmissionRecorder(recorder SubmissionRecorder) FactoryOption {
	return func(c *FactoryConfig) {
		c.SubmissionRecorder = recorder
	}
}

// WithFactoryEventRecorder records canonical generated events as they are
// appended to the runtime event history.
func WithFactoryEventRecorder(recorder FactoryEventRecorder) FactoryOption {
	return func(c *FactoryConfig) {
		c.FactoryEventRecorder = recorder
	}
}

// WithSubmissionHook registers a logical-tick hook that can return generated
// batches, work results, and work events to the engine.
func WithSubmissionHook(hook SubmissionHook) FactoryOption {
	return func(c *FactoryConfig) {
		if hook != nil {
			c.SubmissionHooks = append(c.SubmissionHooks, hook)
		}
	}
}

// WithDispatchRecorder records dispatches after engine tracking is updated
// and before the dispatch/result hook receives the work.
func WithDispatchRecorder(recorder DispatchRecorder) FactoryOption {
	return func(c *FactoryConfig) {
		c.DispatchRecorder = recorder
	}
}

// WithCompletionRecorder records worker completions at the logical tick where
// dispatch/result hooks return them to the engine.
func WithCompletionRecorder(recorder CompletionRecorder) FactoryOption {
	return func(c *FactoryConfig) {
		c.CompletionRecorder = recorder
	}
}

// WithCompletionDeliveryPlanner delays worker-pool completions until their
// planned logical tick. Dispatches without a planned tick use normal delivery.
func WithCompletionDeliveryPlanner(planner CompletionDeliveryPlanner) FactoryOption {
	return func(c *FactoryConfig) {
		c.CompletionDeliveryPlanner = planner
	}
}
