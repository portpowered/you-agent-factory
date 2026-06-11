package workflowruntime

import (
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
)

// Request carries explicit runtime inputs for one simple workflow execution.
type Request struct {
	Source    string
	SourceRef string
	SessionID string
	Args      json.RawMessage
	Metadata  map[string]string
	Policy    workflowpolicy.EffectivePolicy
}

// Hooks supplies optional terminal result and artifact callbacks.
type Hooks struct {
	OnResult   func(workflowresult.TypedValue) error
	OnArtifact func(kind string, content json.RawMessage) error
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
}
