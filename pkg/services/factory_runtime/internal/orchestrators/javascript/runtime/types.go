package workflowruntime

import (
	"encoding/json"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/orchestratorcontract"
	workflowresult "github.com/portpowered/infinite-you/pkg/services/factory_runtime/runtimecontract"
)

const (
	CodeCanceled            = "WORKFLOW_RUNTIME_CANCELED"
	CodeTimeout             = "WORKFLOW_RUNTIME_TIMEOUT"
	CodeScriptError         = "WORKFLOW_RUNTIME_SCRIPT_ERROR"
	CodeUnresolvedFinal     = "WORKFLOW_RUNTIME_UNRESOLVED_FINAL"
	CodeDeniedCapability    = "WORKFLOW_RUNTIME_DENIED_CAPABILITY"
	CodeUnsupportedFinal    = "WORKFLOW_RUNTIME_UNSUPPORTED_FINAL"
	CodePreExecutionInvalid = "WORKFLOW_RUNTIME_PRE_EXECUTION_INVALID"
	CodeInvalidResult       = "WORKFLOW_RUNTIME_INVALID_RESULT"
)

// Request carries explicit runtime inputs for one simple workflow execution.
type Request struct {
	Source         string
	SourceRef      string
	SessionID      string
	Args           json.RawMessage
	ArgsSchema     json.RawMessage
	Metadata       map[string]string
	Policy         workflowpolicy.EffectivePolicy
	Resume         *ResumeContext
	Agents         map[string]interfaces.FactoryOrchestratorJavaScriptAgent
	WorkerSettings WorkerSettingsConfig
}

// WorkerSettingsConfig supplies validated operator-owned child worker settings.
type WorkerSettingsConfig struct {
	Presets              map[string]WorkerPreset
	DefaultModelProvider string
	DefaultModel         string
}

// WorkerPreset is one validated reusable operator preset.
type WorkerPreset struct {
	ModelProvider   string
	Model           string
	ReasoningEffort string
}

// ChildRecordSink reserves child dispatch identity and appends typed runtime records.
type ChildRecordSink interface {
	Append(record RuntimeRecord)
	AppendChildDispatch(base ChildDispatchRecord, status string)
	NextChildDispatchIdentity() (dispatchID string, childIndex int)
	NextChildArtifactID() string
}

// ChildExecutorFactory builds the child executor for one workflow run.
type ChildExecutorFactory func(sessionID string, records ChildRecordSink, policy workflowpolicy.EffectivePolicy) ChildExecutor

// Hooks supplies optional terminal result, artifact, and child-execution callbacks.
type Hooks struct {
	OnResult   func(workflowresult.TypedValue) error
	OnArtifact func(kind string, content json.RawMessage) error
	// OnRecord publishes each canonical runtime record as it is appended.
	// Consumers use it for live projections while Outcome.Records remains the
	// durable terminal record set.
	OnRecord         func(RuntimeRecord)
	NewChildExecutor ChildExecutorFactory
}

// Failure is one deterministic runtime failure diagnostic.
type Failure struct {
	Code    string
	Message string
}

// Outcome distinguishes successful finals from runtime failures.
type Outcome struct {
	OK      bool
	Value   workflowresult.TypedValue
	Failure Failure
	Records []RuntimeRecord
}
