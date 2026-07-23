package factorysessions

import "io"

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

// DirectJavaScriptRunRequest carries customer-edge values for one raw
// JavaScript workflow invocation. Source resolution and execution policy stay
// behind DirectJavaScriptRunOperation.
type DirectJavaScriptRunRequest struct {
	SourcePath         string
	MockWorkersEnabled bool
	JSONOutput         bool
	Output             io.Writer
}
