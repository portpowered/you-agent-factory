package workflowruntime

import (
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
)

// Request carries explicit runtime inputs for one simple workflow execution.
type Request struct {
	Source         string
	SourceRef      string
	SessionID      string
	Args           json.RawMessage
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
	OnResult         func(workflowresult.TypedValue) error
	OnArtifact       func(kind string, content json.RawMessage) error
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
