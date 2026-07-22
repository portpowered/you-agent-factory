package orchestratorcontract

const (
	CodeInvalidConcurrency        = "workflow.policy.invalidConcurrency"
	CodeConcurrencyAboveMaxAgents = "workflow.policy.concurrencyAboveMaxAgents"
	CodeExcessiveMaxAgents        = "workflow.policy.excessiveMaxAgents"
	CodeInvalidMaxAgents          = "workflow.policy.invalidMaxAgents"
	CodeWritableRootsReadOnly     = "workflow.policy.writableRootsReadOnly"
	CodeUnsupportedRunner         = "workflow.policy.unsupportedRunner"
	CodeUnsupportedModel          = "workflow.policy.unsupportedModel"
	CodeUnsupportedReasoning      = "workflow.policy.unsupportedReasoningEffort"
	CodeUnsupportedRouteProfile   = "workflow.policy.unsupportedRouteProfile"
	CodeUnsupportedCommand        = "workflow.policy.unsupportedCommand"
	CodeUnsupportedSandboxMode    = "workflow.policy.unsupportedSandboxMode"
	CodeDeniedCapability          = "workflow.policy.deniedCapability"
	CodeInvalidPolicyDocument     = "workflow.policy.invalidDocument"
	CodeUnsupportedPolicyMode     = "workflow.policy.unsupportedMode"
)
