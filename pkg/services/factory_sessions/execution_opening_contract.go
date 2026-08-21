package factorysessions

import "context"

// ProviderIdentityResolver resolves one authored provider selection through
// the immutable process registry without exposing a second service interface.
type ProviderIdentityResolver func(string) (string, error)

// ExecutionRuntimeOpeningRequest carries invocation-edge roots required to
// open a runtime-backed durable execution service without ambient discovery.
type ExecutionRuntimeOpeningRequest struct {
	ProjectRoot       string
	SystemConfigHome  string
	FactorySessionID  string
	ReplayPath        string
	PersistencePolicy PersistencePolicy
}

// OpenedExecutionRuntime is the narrow durable owner and cleanup result
// returned by the process-composed runtime opening.
type OpenedExecutionRuntime struct {
	Execution DurableExecutionService
	Close     func() error
}

// ExecutionRuntimeOpeningFunc carries the canonical runtime-opening operation
// without publishing another named service interface from Factory Sessions.
type ExecutionRuntimeOpeningFunc func(context.Context, ExecutionRuntimeOpeningRequest) (OpenedExecutionRuntime, error)

// StdioOpeningRequest carries only invocation-edge values into the Factory
// Sessions-owned stdio opening policy. Transport streams stay on the explicit
// presentation boundary below so durable selection cannot retain protocol
// handles.
type StdioOpeningRequest struct {
	FixtureCatalogPath string
	RuntimeBacked      bool
	ProjectRoot        string
	SystemConfigHome   string
	ScopeID            OpeningScopeID
}

// DirectJavaScriptRunRequest carries customer-edge values for one raw
// JavaScript workflow invocation. Source resolution and execution policy stay
// behind DirectJavaScriptRunOperation. Protocol output and host observation are
// owner-private state selected by ScopeID.
type DirectJavaScriptRunRequest struct {
	SourcePath         string
	MockWorkersEnabled bool
	JSONOutput         bool
	Host               *RuntimeHostRequest
	ScopeID            OpeningScopeID
}
