// Package startup defines the narrow handoff from command parsing to the
// process root. It deliberately carries behavior rather than domain services so
// the CLI does not depend on the process owner or application graph packages.
package startup

import (
	"context"
	"io"

	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
)

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

// MCPIntent contains the parsed MCP transport inputs needed by the graph
// constructor. Process streams remain explicit so startup never consults
// mutable process globals.
type MCPIntent struct {
	FixtureCatalogPath string
	RuntimeBacked      bool
	ProjectRoot        string
	Stdin              io.Reader
	Stdout             io.Writer
}

// Request is emitted after command parsing and before application construction.
type Request struct {
	Kind      Kind
	Run       RunIntent
	RunConfig *runcli.RunConfig
	MCP       MCPIntent
}

// Handler delegates one startup request to the process owner.
type Handler func(context.Context, Request) error

// Handle invokes the startup boundary while preserving the supplied context
// and immutable request value.
func (handler Handler) Handle(ctx context.Context, request Request) error {
	return handler(ctx, request)
}
