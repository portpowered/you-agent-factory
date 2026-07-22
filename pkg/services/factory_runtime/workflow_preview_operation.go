package factory

import "context"

// WorkflowPreviewInput is the transport-independent edge input for previewing
// one workflow source. Factory Runtime owns source-context construction and the
// service-ready WorkflowPreviewRequest derived from these external fields.
type WorkflowPreviewInput struct {
	ProjectRoot          string
	Source               WorkflowSourceRequest
	Metadata             map[string]string
	ArgsSchema           []byte
	FactoryDefaultPolicy []byte
	RequestedPolicy      map[string]any
	RequestedRunner      string
	RequestedModel       string
	RequestedProfile     string
	TimeoutMillis        *int64
}

// WorkflowPreviewOperation is the single Factory Runtime root operation used
// by customer transports for workflow preview and source validation.
type WorkflowPreviewOperation interface {
	PreviewWorkflow(context.Context, WorkflowPreviewInput) (WorkflowPreview, error)
}
