package factorysessions

import (
	"context"
	"io"
)

// OwnedExecutionService couples a Factory Session execution role to the
// resources that keep it alive.
type OwnedExecutionService interface {
	ExecutionService
	Close() error
}

// ExecutionServiceBuilder creates one owned execution role from customer
// operation input.
type ExecutionServiceBuilder func(
	context.Context,
	string,
	string,
	string,
	string,
) (OwnedExecutionService, error)

// ExecutionRuntimeOpeningRequest carries invocation-edge roots required to
// open a runtime-backed durable execution service without ambient discovery.
type ExecutionRuntimeOpeningRequest struct {
	ProjectRoot      string
	SystemConfigHome string
}

// StdioOpeningRequest carries only invocation-edge values into the Factory
// Sessions-owned stdio opening policy.
type StdioOpeningRequest struct {
	FixtureCatalogPath string
	RuntimeBacked      bool
	ProjectRoot        string
	SystemConfigHome   string
	Input              io.Reader
	Output             io.Writer
}

// StdioApplication is the lifecycle-ready application returned to the
// process lifecycle owner after Factory Sessions has selected and opened the
// appropriate execution service.
type StdioApplication interface {
	Run(context.Context) error
}

type FixtureStdioApplicationBuilder func(
	context.Context,
	ExecutionService,
	io.Reader,
	io.Writer,
) (StdioApplication, error)

// RuntimeStdioApplicationBuilder assumes ownership of opened runtime resources
// once invoked and closes them when construction fails. This keeps cleanup
// singular across the runtime/lifecycle adapter boundary.
type RuntimeStdioApplicationBuilder func(
	context.Context,
	OpenedExecutionRuntime,
	io.Reader,
	io.Writer,
) (StdioApplication, error)

// StdioExecutionOpening is the exact execution-opening capability used by the
// owner operation. It exists separately from transport and lifecycle roles.
type StdioExecutionOpening interface {
	ResolveProjectRoot(string) (string, error)
	OpenExecutionRuntime(context.Context, ExecutionRuntimeOpeningRequest) (OpenedExecutionRuntime, error)
	Build(context.Context, string, string, string, string) (OwnedExecutionService, error)
}

// StdioOpeningOperation owns runtime-backed versus fixture execution
// selection, service opening, and failure cleanup.
type StdioOpeningOperation interface {
	OpenStdio(context.Context, StdioOpeningRequest) (StdioApplication, error)
}

// DirectJavaScriptRunRequest carries customer-edge values for one raw
// JavaScript workflow invocation. Source resolution and execution policy stay
// behind DirectJavaScriptRunOperation.
type DirectJavaScriptRunRequest struct {
	SourcePath         string
	MockWorkersEnabled bool
	JSONOutput         bool
	Output             io.Writer
}

// DirectJavaScriptSyncRunner is the injected presentation edge used after the
// Factory Sessions operation has normalized and opened direct execution.
type DirectJavaScriptSyncRunner func(
	context.Context,
	ExecutionService,
	StartRequest,
	bool,
	io.Writer,
) error

// DirectJavaScriptRunOperation owns raw JavaScript source recognition,
// execution opening, request identity, provider selection and cleanup.
type DirectJavaScriptRunOperation interface {
	Supports(string) bool
	Run(context.Context, DirectJavaScriptRunRequest) error
}
