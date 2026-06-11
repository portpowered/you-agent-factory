// Deprecated: use github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy instead.
// This package is a Batch 001 compatibility shim; core runtime and API code must import the orchestrator-owned path directly.
package workflowpolicy

import target "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"

type (
	Issue            = target.Issue
	Diagnostic       = target.Diagnostic
	Capability       = target.Capability
	EffectivePolicy  = target.EffectivePolicy
	Request          = target.Request
	Resolution       = target.Resolution
	PreviewInput     = target.PreviewInput
	RunnerDecision   = target.RunnerDecision
	ModelDecision    = target.ModelDecision
	ProfileDecision  = target.ProfileDecision
	TimeoutDecisions = target.TimeoutDecisions
	BudgetDecisions  = target.BudgetDecisions
	Preview          = target.Preview
)

const (
	CodeInvalidConcurrency          = target.CodeInvalidConcurrency
	CodeConcurrencyAboveMaxAgents   = target.CodeConcurrencyAboveMaxAgents
	CodeExcessiveMaxAgents          = target.CodeExcessiveMaxAgents
	CodeInvalidMaxAgents            = target.CodeInvalidMaxAgents
	CodeWritableRootsReadOnly       = target.CodeWritableRootsReadOnly
	CodeUnsupportedRunner           = target.CodeUnsupportedRunner
	CodeUnsupportedModel            = target.CodeUnsupportedModel
	CodeUnsupportedReasoning        = target.CodeUnsupportedReasoning
	CodeUnsupportedRouteProfile     = target.CodeUnsupportedRouteProfile
	CodeUnsupportedCommand          = target.CodeUnsupportedCommand
	CodeUnsupportedSandboxMode      = target.CodeUnsupportedSandboxMode
	CodeDeniedCapability            = target.CodeDeniedCapability
	CodeInvalidPolicyDocument       = target.CodeInvalidPolicyDocument
	CodeUnsupportedPolicyMode       = target.CodeUnsupportedPolicyMode
	DefaultMaxAgents                = target.DefaultMaxAgents
	DefaultDeploymentCap            = target.DefaultDeploymentCap
	DefaultMaxDepth                 = target.DefaultMaxDepth
	DefaultMaxRetries               = target.DefaultMaxRetries
	DefaultConcurrencyCap           = target.DefaultConcurrencyCap
	ModeReadOnly                    = target.ModeReadOnly
	OutputAuditModeAuto             = target.OutputAuditModeAuto
	CapabilityWorkspaceWrite        = target.CapabilityWorkspaceWrite
	CapabilityFilesystemWrite       = target.CapabilityFilesystemWrite
	CapabilityShellProcess          = target.CapabilityShellProcess
	CapabilityNetwork               = target.CapabilityNetwork
	CapabilityConnectors            = target.CapabilityConnectors
	CapabilityDangerFullAccess      = target.CapabilityDangerFullAccess
)
