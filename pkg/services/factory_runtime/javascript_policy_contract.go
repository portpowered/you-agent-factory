package factory

import orchestratorcontract "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"

const (
	JavaScriptPolicyModeReadOnly        = orchestratorcontract.ModeReadOnly
	JavaScriptPolicyOutputAuditModeAuto = orchestratorcontract.OutputAuditModeAuto

	JavaScriptPolicyCapabilityWorkspaceWrite   = orchestratorcontract.CapabilityWorkspaceWrite
	JavaScriptPolicyCapabilityFilesystemWrite  = orchestratorcontract.CapabilityFilesystemWrite
	JavaScriptPolicyCapabilityShellProcess     = orchestratorcontract.CapabilityShellProcess
	JavaScriptPolicyCapabilityNetwork          = orchestratorcontract.CapabilityNetwork
	JavaScriptPolicyCapabilityConnectors       = orchestratorcontract.CapabilityConnectors
	JavaScriptPolicyCapabilityDangerFullAccess = orchestratorcontract.CapabilityDangerFullAccess

	JavaScriptPolicyCodeInvalidConcurrency        = orchestratorcontract.CodeInvalidConcurrency
	JavaScriptPolicyCodeConcurrencyAboveMaxAgents = orchestratorcontract.CodeConcurrencyAboveMaxAgents
	JavaScriptPolicyCodeExcessiveMaxAgents        = orchestratorcontract.CodeExcessiveMaxAgents
	JavaScriptPolicyCodeInvalidMaxAgents          = orchestratorcontract.CodeInvalidMaxAgents
	JavaScriptPolicyCodeWritableRootsReadOnly     = orchestratorcontract.CodeWritableRootsReadOnly
	JavaScriptPolicyCodeUnsupportedRunner         = orchestratorcontract.CodeUnsupportedRunner
	JavaScriptPolicyCodeUnsupportedModel          = orchestratorcontract.CodeUnsupportedModel
	JavaScriptPolicyCodeUnsupportedReasoning      = orchestratorcontract.CodeUnsupportedReasoning
	JavaScriptPolicyCodeUnsupportedRouteProfile   = orchestratorcontract.CodeUnsupportedRouteProfile
	JavaScriptPolicyCodeUnsupportedCommand        = orchestratorcontract.CodeUnsupportedCommand
	JavaScriptPolicyCodeUnsupportedSandboxMode    = orchestratorcontract.CodeUnsupportedSandboxMode
	JavaScriptPolicyCodeDeniedCapability          = orchestratorcontract.CodeDeniedCapability
	JavaScriptPolicyCodeInvalidPolicyDocument     = orchestratorcontract.CodeInvalidPolicyDocument
	JavaScriptPolicyCodeUnsupportedPolicyMode     = orchestratorcontract.CodeUnsupportedPolicyMode

	DefaultJavaScriptPolicyMaxAgents      = orchestratorcontract.DefaultMaxAgents
	DefaultJavaScriptPolicyDeploymentCap  = orchestratorcontract.DefaultDeploymentCap
	DefaultJavaScriptPolicyMaxDepth       = orchestratorcontract.DefaultMaxDepth
	DefaultJavaScriptPolicyMaxRetries     = orchestratorcontract.DefaultMaxRetries
	DefaultJavaScriptPolicyConcurrencyCap = orchestratorcontract.DefaultConcurrencyCap
)

type (
	JavaScriptPolicyIssue            = orchestratorcontract.Issue
	JavaScriptPolicyDiagnostic       = orchestratorcontract.Diagnostic
	JavaScriptPolicyCapability       = orchestratorcontract.Capability
	JavaScriptPolicy                 = orchestratorcontract.EffectivePolicy
	JavaScriptPolicyRequest          = orchestratorcontract.Request
	JavaScriptPolicyResolution       = orchestratorcontract.Resolution
	JavaScriptPolicyPreviewInput     = orchestratorcontract.PreviewInput
	JavaScriptPolicyRunnerDecision   = orchestratorcontract.RunnerDecision
	JavaScriptPolicyModelDecision    = orchestratorcontract.ModelDecision
	JavaScriptPolicyProfileDecision  = orchestratorcontract.ProfileDecision
	JavaScriptPolicyTimeoutDecisions = orchestratorcontract.TimeoutDecisions
	JavaScriptPolicyBudgetDecisions  = orchestratorcontract.BudgetDecisions
	JavaScriptPolicyPreview          = orchestratorcontract.Preview
	JavaScriptPolicyChildRequest     = orchestratorcontract.ChildRequest
)

var (
	ValidateJavaScriptPolicyCapability    = orchestratorcontract.ValidateCapability
	DeniedJavaScriptPolicyCapabilities    = orchestratorcontract.DeniedCapabilitiesForReadOnly
	ValidateJavaScriptPolicyChildRequest  = orchestratorcontract.ValidateChildRequest
	DefaultJavaScriptPolicy               = orchestratorcontract.DefaultEffectivePolicy
	HashJavaScriptPolicy                  = orchestratorcontract.Hash
	HashJavaScriptPolicyDocument          = orchestratorcontract.HashDocument
	BuildJavaScriptPolicyPreview          = orchestratorcontract.BuildPreview
	ResolveJavaScriptPolicy               = orchestratorcontract.Resolve
	ResolveJavaScriptFactoryDefaultPolicy = orchestratorcontract.ResolveFromFactoryDefault
	ValidateJavaScriptPolicy              = orchestratorcontract.Validate
)
