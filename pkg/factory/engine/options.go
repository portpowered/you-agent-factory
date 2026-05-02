package engine

import (
	"github.com/portpowered/infinite-you/pkg/buffers"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/token_transformer"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
)

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
func WithDispatchHandler(fn func(interfaces.WorkDispatch)) Option {
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
func WithResultBuffer(buffer *buffers.TypedBuffer[interfaces.WorkResult]) Option {
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
func WithSubmissionRecorder(fn func(interfaces.FactorySubmissionRecord)) Option {
	return func(e *FactoryEngine) {
		e.recordSubmission = fn
	}
}

// WithWorkRequestRecorder registers a callback invoked once for each request
// batch observed before its work items are injected into the marking.
func WithWorkRequestRecorder(fn func(int, interfaces.WorkRequestRecord)) Option {
	return func(e *FactoryEngine) {
		e.recordWorkRequest = fn
	}
}

// WithWorkInputRecorder registers a callback invoked after a submit request is
// converted to a runtime token and injected into the marking.
func WithWorkInputRecorder(fn func(int, interfaces.SubmitRequest, interfaces.Token)) Option {
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
func WithWorkstationResponseRecorder(fn func(int, interfaces.WorkResult, interfaces.CompletedDispatch)) Option {
	return func(e *FactoryEngine) {
		e.recordResponse = fn
	}
}
