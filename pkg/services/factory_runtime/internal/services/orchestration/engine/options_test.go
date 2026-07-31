package engine

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token_transformer"
	"github.com/portpowered/infinite-you/pkg/services/work"
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

// WithAutomaticTicksPaused registers a predicate that suppresses automatic
// subsystem ticks (dispatch, transition, cascade, scheduling) while returning
// true. Operator control ingress such as MoveWork is unaffected.
func WithAutomaticTicksPaused(paused func() bool) Option {
	return func(e *FactoryEngine) {
		e.automaticTicksPaused = paused
	}
}
