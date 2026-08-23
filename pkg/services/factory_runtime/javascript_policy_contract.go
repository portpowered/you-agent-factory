package factory

import orchestratorcontract "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"

const (
	JavaScriptPolicyOutputAuditModeAuto       = orchestratorcontract.OutputAuditModeAuto
	JavaScriptPolicyPermissionDefault         = orchestratorcontract.PermissionModeDefault
	JavaScriptPolicyPermissionSkipPermissions = orchestratorcontract.PermissionModeSkipPermissions

	JavaScriptPolicyCodeInvalidConcurrency        = orchestratorcontract.CodeInvalidConcurrency
	JavaScriptPolicyCodeConcurrencyAboveMaxAgents = orchestratorcontract.CodeConcurrencyAboveMaxAgents
	JavaScriptPolicyCodeExcessiveMaxAgents        = orchestratorcontract.CodeExcessiveMaxAgents
	JavaScriptPolicyCodeInvalidMaxAgents          = orchestratorcontract.CodeInvalidMaxAgents
	JavaScriptPolicyCodeUnsupportedRunner         = orchestratorcontract.CodeUnsupportedRunner
	JavaScriptPolicyCodeUnsupportedModel          = orchestratorcontract.CodeUnsupportedModel
	JavaScriptPolicyCodeUnsupportedReasoning      = orchestratorcontract.CodeUnsupportedReasoning
	JavaScriptPolicyCodeUnsupportedRouteProfile   = orchestratorcontract.CodeUnsupportedRouteProfile
	JavaScriptPolicyCodeUnsupportedCommand        = orchestratorcontract.CodeUnsupportedCommand
	JavaScriptPolicyCodeUnsupportedSandboxMode    = orchestratorcontract.CodeUnsupportedSandboxMode
	JavaScriptPolicyCodeUnsupportedPermission     = orchestratorcontract.CodeUnsupportedPermission
	JavaScriptPolicyCodeUnsupportedPolicyField    = orchestratorcontract.CodeUnsupportedPolicyField
	JavaScriptPolicyCodeInvalidPolicyDocument     = orchestratorcontract.CodeInvalidPolicyDocument

	DefaultJavaScriptPolicyMaxAgents      = orchestratorcontract.DefaultMaxAgents
	DefaultJavaScriptPolicyDeploymentCap  = orchestratorcontract.DefaultDeploymentCap
	DefaultJavaScriptPolicyMaxDepth       = orchestratorcontract.DefaultMaxDepth
	DefaultJavaScriptPolicyMaxRetries     = orchestratorcontract.DefaultMaxRetries
	DefaultJavaScriptPolicyConcurrencyCap = orchestratorcontract.DefaultConcurrencyCap
)

type (
	JavaScriptPolicyIssue            = orchestratorcontract.Issue
	JavaScriptPolicyDiagnostic       = orchestratorcontract.Diagnostic
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
	ValidateJavaScriptPolicyChildRequest  = orchestratorcontract.ValidateChildRequest
	DefaultJavaScriptPolicy               = orchestratorcontract.DefaultEffectivePolicy
	HashJavaScriptPolicy                  = orchestratorcontract.Hash
	HashJavaScriptPolicyDocument          = orchestratorcontract.HashDocument
	BuildJavaScriptPolicyPreview          = orchestratorcontract.BuildPreview
	ResolveJavaScriptPolicy               = orchestratorcontract.Resolve
	ResolveJavaScriptFactoryDefaultPolicy = orchestratorcontract.ResolveFromFactoryDefault
	ValidateJavaScriptPolicy              = orchestratorcontract.Validate
)
