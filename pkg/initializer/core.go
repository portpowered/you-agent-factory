package initializer

import (
	"context"
	"errors"
	"io"
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

// OpenedApplication contains only lifecycle-ready process roles. Product
// services retain their request and result contracts and adapt them to this
// value at the composition boundary.
type OpenedApplication struct {
	Plan        lifecycle.Plan
	Diagnostics runtimeartifact.Diagnostics
	Ready       <-chan RuntimeHostBinding
}

// ApplicationOpeningOperation opens one already-composed product application
// and returns only its lifecycle-ready roles.
type ApplicationOpeningOperation func(context.Context) (OpenedApplication, error)

// RuntimeRunnerBuilder turns one neutral application opening into an inert
// managed runner without selecting product components or lifecycle policy.
type RuntimeRunnerBuilder func(
	context.Context,
	ApplicationOpeningOperation,
) (LocalRuntimeRunner, error)

// StdioSessionOpener opens one already-bound stdio protocol session. Product
// execution and preview roles remain captured by the owner/Wire adapter.
type StdioSessionOpener func(
	context.Context,
	io.Reader,
	io.Writer,
) (OpenedApplication, error)

// OpenedStdioApplication carries the neutral opening that supplies a complete
// stdio lifecycle plan, including any owned resources.
type OpenedStdioApplication struct {
	OpenSession StdioSessionOpener
}

type RunApplication interface {
	Run(context.Context) error
}
