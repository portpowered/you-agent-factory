package initializer

import (
	"context"
	"io"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

// LocalRuntimeRunner is an already-constructed runtime selected by an
// initializer entrypoint.
type LocalRuntimeRunner interface {
	Run(context.Context) error
}

// OpenedApplication contains only lifecycle-ready process roles. Product
// services retain their request and result contracts and adapt them to this
// value at the composition boundary.
type OpenedApplication struct {
	Plan        lifecycle.Plan
	Diagnostics runtimeartifact.Diagnostics
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
