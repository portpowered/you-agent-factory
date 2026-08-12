package initializer

import (
	"context"
	"errors"
	"io"

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
