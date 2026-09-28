package initializer

import (
	"context"
	"errors"
	"sync"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

// LocalRuntimeRunner is an already-constructed runtime selected by an
// initializer entrypoint.
type LocalRuntimeRunner interface {
	Run(context.Context) error
}

// CompletionOperation is the typed one-shot operation run after the
// initializer has established any required host readiness.
type CompletionOperation func(context.Context) error

// PreparationGate serializes repeated calls to one invocation's startup
// preparation. The initializer owns this lifecycle coordination so command
// transports can retain their selection policy without embedding concurrency
// primitives in a transport package.
//
// The zero value is ready for use. The gate does not cache an operation error;
// its caller owns any retry or error-retention policy for the preparation.
type PreparationGate struct {
	mu sync.Mutex
}

func (gate *PreparationGate) Run(operation func() error) error {
	if gate == nil {
		return errors.New("initializer preparation gate is required")
	}
	if operation == nil {
		return errors.New("initializer preparation operation is required")
	}
	gate.mu.Lock()
	defer gate.mu.Unlock()
	return operation()
}

// CompletionRuntimeRunner lets the initializer-owned lifecycle runner keep a
// hosted transport alive while one-shot completion waits for runtime-host
// readiness. Product opening code supplies the completion operation as a
// value; it does not install a callback gate in the opening request.
type CompletionRuntimeRunner interface {
	LocalRuntimeRunner
	RunWithCompletion(context.Context, CompletionOperation) error
}

// RuntimeHostBinding is the detached endpoint selected by an application
// host. It is an initializer value so readiness ordering can be owned by the
// process lifecycle without passing transport callbacks into product services.
type RuntimeHostBinding struct {
	Host string
	Port int
}

// ErrRuntimeHostReadinessUnavailable indicates that an application has no
// externally bound host to await, such as an in-process batch run.
var ErrRuntimeHostReadinessUnavailable = errors.New("runtime host readiness is unavailable")

// ErrRuntimeHostExitedBeforeReadiness identifies a hosted lifecycle that
// ended before it published its externally reachable endpoint.
var ErrRuntimeHostExitedBeforeReadiness = errors.New("runtime host exited before readiness")

// RuntimeHostStartupError preserves the lifecycle cause when a hosted runtime
// ends before readiness. Its Error method is intentionally safe and stable;
// transport boundaries can inspect Unwrap without exposing arbitrary runtime
// error text to operators.
type RuntimeHostStartupError struct {
	Cause error
}

func (err *RuntimeHostStartupError) Error() string {
	if err == nil {
		return ""
	}
	return ErrRuntimeHostExitedBeforeReadiness.Error()
}

func (err *RuntimeHostStartupError) Unwrap() error {
	if err == nil {
		return nil
	}
	if err.Cause == nil {
		return ErrRuntimeHostExitedBeforeReadiness
	}
	return err.Cause
}

func (err *RuntimeHostStartupError) Is(target error) bool {
	return err != nil && target == ErrRuntimeHostExitedBeforeReadiness
}

// LifecycleRunnerBuilder activates an already-composed lifecycle plan.
type LifecycleRunnerBuilder func(
	context.Context,
	lifecycle.Plan,
	runtimeartifact.Diagnostics,
	<-chan RuntimeHostBinding,
) (LocalRuntimeRunner, error)

type RunApplication interface {
	Run(context.Context) error
}
