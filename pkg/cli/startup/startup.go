// Package startup defines the narrow handoff from command parsing to the
// process root. It deliberately carries behavior rather than domain services so
// the CLI does not depend on the process owner or application graph packages.
package startup

import "context"

// Kind identifies the command behavior that requested process startup.
type Kind string

const (
	KindRun      Kind = "run"
	KindMCPServe Kind = "mcp-serve"
)

// RunIntent contains only the process-policy facts resolved by the run command.
type RunIntent struct {
	DefaultInvocation     bool
	Continuous            bool
	APIEnabled            bool
	DashboardEnabled      bool
	WorkerSidecarsEnabled bool
}

// Lifecycle is an already-selected startup behavior constructed behind the
// application graph boundary and executed by the initializer boundary.
type Lifecycle interface {
	Run(context.Context) error
}

// LifecycleFunc adapts a function to Lifecycle.
type LifecycleFunc func(context.Context) error

func (run LifecycleFunc) Run(ctx context.Context) error { return run(ctx) }

// Construct builds the lifecycle collaborator for one startup request.
type Construct func(context.Context) (Lifecycle, error)

// Request is emitted after command parsing and before application construction.
type Request struct {
	Kind      Kind
	Run       RunIntent
	Construct Construct
}

// Handler delegates one startup request to the process owner.
type Handler func(context.Context, Request) error
