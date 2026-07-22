package engine

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/runtime/buffers"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/token"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/token_transformer"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

var testWorkRequestIdentity atomic.Uint64

func TestNewFactoryEngine_RequiresClock(t *testing.T) {
	t.Parallel()

	engine, err := NewFactoryEngine(
		nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	if engine != nil || err == nil || !strings.Contains(err.Error(), "clock is required") {
		t.Fatalf("NewFactoryEngine() = (%v, %v), want nil engine and clock dependency error", engine, err)
	}
}

func newTestFactoryEngine(
	net *state.Net,
	marking *petri.Marking,
	runtimeSubsystems []subsystems.Subsystem,
	opts ...Option,
) *FactoryEngine {
	engine, err := NewFactoryEngine(
		net, marking, runtimeSubsystems,
		nil, platformclock.Real{}, func() string { return fmt.Sprintf("test-id-%d", testWorkRequestIdentity.Add(1)) }, nil, nil,
		token_transformer.New(net.Places, net.WorkTypes, petri.NewWorkIDGenerator()),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	if err != nil {
		panic(err)
	}
	for _, option := range opts {
		option(engine)
	}
	engine.submissionHooks = sortedSubmissionHooks(engine.submissionHooks)
	return engine
}

// Option configures a FactoryEngine.
type Option func(*FactoryEngine)

// WithLogger sets the logger for the engine. Default: no-op logger.
func WithLogger(l logging.Logger) Option {
	return func(e *FactoryEngine) {
		e.logger = logging.EnsureLogger(l)
	}
}

// WithClock sets the engine time source used for submit and dispatch stamps.
func WithClock(clock factory.Clock) Option {
	return func(e *FactoryEngine) {
		if clock != nil {
			e.clock = clock
		}
	}
}

// WithDispatchHandler registers a callback invoked for each WorkDispatch produced
// during a tick. The runtime uses this to forward dispatches to the WorkerPool.
func WithDispatchHandler(fn func(work.WorkDispatch)) Option {
	return func(e *FactoryEngine) {
		e.dispatchHandler = fn
	}
}

// WithDispatchResultHook registers a tick-aware bridge that accepts generated
// dispatches and returns completed worker results at logical tick boundaries.
func WithDispatchResultHook(hook factory.DispatchResultHook) Option {
	return func(e *FactoryEngine) {
		e.dispatchHook = hook
	}
}

// WithTokenTransformer injects the token conversion component used for submit-time token creation.
func WithTokenTransformer(transformer *token_transformer.Transformer) Option {
	return func(e *FactoryEngine) {
		if transformer != nil {
			e.transformer = transformer
		}
	}
}

// WithResultBuffer sets the runtime-owned work result buffer used to collect
// worker completions before transition processing.
func WithResultBuffer(buffer *buffers.TypedBuffer[workerexecution.WorkResult]) Option {
	return func(e *FactoryEngine) {
		if buffer != nil {
			e.runtimeState.ResultBuffer = buffer
		}
	}
}

// WithSubmissionHook registers an engine-owned source of generated batches,
// results, and events that should be observed at logical tick boundaries.
func WithSubmissionHook(hook factory.SubmissionHook) Option {
	return func(e *FactoryEngine) {
		if hook != nil {
			e.submissionHooks = append(e.submissionHooks, hook)
		}
	}
}

// WithSubmissionRecorder registers a callback invoked after a submission hook
// returns work and before the engine injects that work into the marking.
func WithSubmissionRecorder(fn func(work.FactorySubmissionRecord)) Option {
	return func(e *FactoryEngine) {
		e.recordSubmission = fn
	}
}

// WithWorkRequestRecorder registers a callback invoked once for each request
// batch observed before its work items are injected into the marking.
func WithWorkRequestRecorder(fn func(int, work.WorkRequestRecord)) Option {
	return func(e *FactoryEngine) {
		e.recordWorkRequest = fn
	}
}

// WithWorkInputRecorder registers a callback invoked after a submit request is
// converted to a runtime token and injected into the marking.
func WithWorkInputRecorder(fn func(int, work.SubmitRequest, factorytoken.Token)) Option {
	return func(e *FactoryEngine) {
		e.recordWorkInput = fn
	}
}

// WithDispatchRecorder registers a callback invoked after dispatch tracking is
// updated and before the dispatch is submitted to the dispatch/result hook.
func WithDispatchRecorder(fn func(interfaces.FactoryDispatchRecord)) Option {
	return func(e *FactoryEngine) {
		e.recordDispatch = fn
	}
}

// WithCompletionRecorder registers a callback invoked when dispatch/result
// hook completions become visible to the engine at a logical tick.
func WithCompletionRecorder(fn func(interfaces.FactoryCompletionRecord)) Option {
	return func(e *FactoryEngine) {
		e.recordCompletion = fn
	}
}

// WithWorkstationResponseRecorder registers a callback invoked after a worker
// result has been routed and a completed dispatch summary is available.
func WithWorkstationResponseRecorder(fn func(int, workerexecution.WorkResult, interfaces.CompletedDispatch)) Option {
	return func(e *FactoryEngine) {
		e.recordResponse = fn
	}
}

// WithPetriMutationRecorder registers the persistence boundary invoked after
// transition routing and before the resulting marking mutations are applied.
func WithPetriMutationRecorder(fn func([]interfaces.TokenMutationRecord) error) Option {
	return func(e *FactoryEngine) {
		e.recordPetriMutations = fn
	}
}

// WithAutomaticTicksPaused registers a predicate that suppresses automatic
// subsystem ticks (dispatch, transition, cascade, scheduling) while returning
// true. Operator control ingress such as MoveWork is unaffected.
func WithAutomaticTicksPaused(paused func() bool) Option {
	return func(e *FactoryEngine) {
		e.automaticTicksPaused = paused
	}
}

// WithResultBufferDrainObserver registers a callback invoked after buffered
// worker results are drained into runtime state during a tick.
func WithResultBufferDrainObserver(observer func(drainedCount int)) Option {
	return func(e *FactoryEngine) {
		e.onResultBufferDrained = observer
	}
}
