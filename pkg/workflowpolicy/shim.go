// Package workflowpolicy is a transitional compatibility shim for JavaScript
// orchestrator policy defaults and preview projection.
//
// Deprecated: use pkg/orchestrators/javascript/policy. This shim delegates to
// orchestrator ownership and is not the final package boundary.
package workflowpolicy

import jspolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"

type (
	BudgetDecisions  = jspolicy.BudgetDecisions
	Capability       = jspolicy.Capability
	Diagnostic       = jspolicy.Diagnostic
	EffectivePolicy  = jspolicy.EffectivePolicy
	Issue            = jspolicy.Issue
	ModelDecision    = jspolicy.ModelDecision
	Preview          = jspolicy.Preview
	PreviewInput     = jspolicy.PreviewInput
	ProfileDecision  = jspolicy.ProfileDecision
	Request          = jspolicy.Request
	Resolution       = jspolicy.Resolution
	RunnerDecision   = jspolicy.RunnerDecision
	TimeoutDecisions = jspolicy.TimeoutDecisions
)

const (
	CodeConcurrencyAboveMaxAgents = jspolicy.CodeConcurrencyAboveMaxAgents
	CodeDeniedCapability          = jspolicy.CodeDeniedCapability
	CodeExcessiveMaxAgents        = jspolicy.CodeExcessiveMaxAgents
	CodeInvalidConcurrency        = jspolicy.CodeInvalidConcurrency
	CodeUnsupportedPolicyMode     = jspolicy.CodeUnsupportedPolicyMode
	CodeUnsupportedRunner         = jspolicy.CodeUnsupportedRunner
	CodeWritableRootsReadOnly     = jspolicy.CodeWritableRootsReadOnly
	DefaultMaxAgents              = jspolicy.DefaultMaxAgents
	ModeReadOnly                  = jspolicy.ModeReadOnly
	OutputAuditModeAuto           = jspolicy.OutputAuditModeAuto

	CapabilityConnectors       = jspolicy.CapabilityConnectors
	CapabilityDangerFullAccess = jspolicy.CapabilityDangerFullAccess
	CapabilityFilesystemWrite  = jspolicy.CapabilityFilesystemWrite
	CapabilityNetwork          = jspolicy.CapabilityNetwork
	CapabilityShellProcess     = jspolicy.CapabilityShellProcess
	CapabilityWorkspaceWrite   = jspolicy.CapabilityWorkspaceWrite
)

var (
	BuildPreview            = jspolicy.BuildPreview
	DefaultEffectivePolicy  = jspolicy.DefaultEffectivePolicy
	DeniedCapabilitiesForReadOnly = jspolicy.DeniedCapabilitiesForReadOnly
	Hash                    = jspolicy.Hash
	HashDocument            = jspolicy.HashDocument
	Resolve                 = jspolicy.Resolve
	ResolveFromFactoryDefault = jspolicy.ResolveFromFactoryDefault
	Validate                = jspolicy.Validate
	ValidateCapability      = jspolicy.ValidateCapability
)
