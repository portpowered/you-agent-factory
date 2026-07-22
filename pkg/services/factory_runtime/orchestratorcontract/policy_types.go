package orchestratorcontract

import "encoding/json"

const (
	ModeReadOnly        = "READ_ONLY"
	OutputAuditModeAuto = "AUTO"
)

// Issue is one workflow policy validation diagnostic.
type Issue struct {
	Code    string
	Message string
	Path    string
}

// Diagnostic is a structured policy or capability diagnostic.
type Diagnostic struct {
	Code    string
	Message string
}

// Capability names one host capability checked against effective policy.
type Capability string

const (
	CapabilityWorkspaceWrite   Capability = "workspace-write"
	CapabilityFilesystemWrite  Capability = "filesystem-write"
	CapabilityShellProcess     Capability = "shell-process"
	CapabilityNetwork          Capability = "network"
	CapabilityConnectors       Capability = "connectors"
	CapabilityDangerFullAccess Capability = "danger-full-access"
)

// EffectivePolicy is the resolved bounded orchestrator policy contract.
type EffectivePolicy struct {
	Mode                    string   `json:"mode"`
	MaxAgents               int      `json:"maxAgents"`
	Concurrency             int      `json:"concurrency"`
	MaxDepth                int      `json:"maxDepth"`
	MaxRetries              int      `json:"maxRetries"`
	AllowNetwork            bool     `json:"allowNetwork"`
	AllowConnectors         bool     `json:"allowConnectors"`
	AllowDangerFullAccess   bool     `json:"allowDangerFullAccess"`
	WritableRoots           []string `json:"writableRoots"`
	OutputAuditMode         string   `json:"outputAuditMode"`
	AllowedRunners          []string `json:"allowedRunners,omitempty"`
	AllowedModels           []string `json:"allowedModels,omitempty"`
	AllowedReasoningEfforts []string `json:"allowedReasoningEfforts,omitempty"`
	AllowedRouteProfiles    []string `json:"allowedRouteProfiles,omitempty"`
	AllowedCommands         []string `json:"allowedCommands,omitempty"`
	SandboxMode             string   `json:"sandboxMode,omitempty"`
	MaxRunDurationMs        *int64   `json:"maxRunDurationMs,omitempty"`
	MaxWorkerDurationMs     *int64   `json:"maxWorkerDurationMs,omitempty"`
	MaxOutputBytesPerWorker *int64   `json:"maxOutputBytesPerWorker,omitempty"`
	MaxArtifactBytes        *int64   `json:"maxArtifactBytes,omitempty"`
	MaxTokens               *int64   `json:"maxTokens,omitempty"`
	Secrets                 string   `json:"secrets,omitempty"`
}

// Request resolves effective policy from optional request and factory layers.
type Request struct {
	Requested      map[string]any
	FactoryDefault json.RawMessage
	DeploymentCap  int
}

// Resolution is the resolved policy plus validation diagnostics.
type Resolution struct {
	Policy EffectivePolicy
	Hash   string
	Issues []Issue
}

// PreviewInput supplies preview/session-start projection inputs.
type PreviewInput struct {
	Request
	RequestedRunner  string
	RequestedModel   string
	RequestedProfile string
	TimeoutMillis    *int64
}

// RunnerDecision records the resolved runner choice for preview surfaces.
type RunnerDecision struct {
	Requested  string
	Resolved   string
	Allowed    bool
	Diagnostic *Diagnostic
}

// ModelDecision records the resolved model choice for preview surfaces.
type ModelDecision struct {
	Requested  string
	Resolved   string
	Allowed    bool
	Diagnostic *Diagnostic
}

// ProfileDecision records the resolved route profile for preview surfaces.
type ProfileDecision struct {
	Requested  string
	Resolved   string
	Allowed    bool
	Diagnostic *Diagnostic
}

// TimeoutDecisions records timeout/budget decisions for preview surfaces.
type TimeoutDecisions struct {
	RequestedMillis *int64
	EffectiveMillis *int64
}

// BudgetDecisions records child and concurrency budgets for preview surfaces.
type BudgetDecisions struct {
	MaxChildCount  int
	MaxConcurrency int
}

// Preview is the shared preview/session-start policy projection contract.
type Preview struct {
	EffectivePolicy    EffectivePolicy
	PolicyHash         string
	MaxChildCount      int
	MaxConcurrency     int
	RunnerDecision     *RunnerDecision
	ModelDecision      *ModelDecision
	ProfileDecision    *ProfileDecision
	TimeoutDecisions   TimeoutDecisions
	BudgetDecisions    BudgetDecisions
	DeniedCapabilities []Diagnostic
	ValidationIssues   []Issue
}
