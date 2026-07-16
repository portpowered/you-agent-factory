package workflowsource

import (
	"encoding/json"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// Request is the normalized workflow source request shared across API, CLI, MCP,
// and website adapters.
type Request struct {
	Kind               Kind
	Value              string
	InlineSource       string
	ArtifactRoot       string
	AllowFactoryLookup bool
}

// Context supplies filesystem roots used by ordered workflow lookup.
type Context struct {
	ProjectRoot         string
	PackageRoot         string
	ProjectWorkflowRoot string
	GlobalWorkflowRoot  string
	ProjectFactoryRoot  string
	GlobalFactoryRoot   string
}

// Diagnostic is a structured lookup or policy diagnostic.
type Diagnostic struct {
	Code    string
	Message string
}

// ArtifactRootDecision records artifact-root validation for one request.
type ArtifactRootDecision struct {
	Requested  string
	Effective  string
	Allowed    bool
	Diagnostic *Diagnostic
}

// Resolution is the shared workflow source resolution contract.
type Resolution struct {
	RequestKind      Kind
	RequestValue     string
	ResolvedKind     Kind
	LookupStage      LookupStage
	SourceRef        string
	SourceHash       string
	OrchestratorKind string
	Dialect          string
	Content          string
	Agents           map[string]interfaces.FactoryOrchestratorJavaScriptAgent
	ArgsSchema       json.RawMessage
	DefaultPolicy    json.RawMessage
	Diagnostics      []Diagnostic
	ArtifactRoot     ArtifactRootDecision
	Found            bool
}
