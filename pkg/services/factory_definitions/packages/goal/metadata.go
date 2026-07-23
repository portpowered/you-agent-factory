package goal

const (
	// PackagedFactoryName is the canonical named factory identifier for @you/goal.
	PackagedFactoryName = "@you/goal"
	// PackagedFactoryProject identifies the built-in @you/goal factory project id.
	PackagedFactoryProject = "builtin-goal"
	// PackagedGoalWorkTypeName is the canonical default goal work type name.
	PackagedGoalWorkTypeName = "goal"
	// PackagedExecuteWorkstationName is the single repeater workstation for goal work.
	PackagedExecuteWorkstationName = "execute-goal"
	// Deprecated multi-stage workstation names retained for transitional test and mock helpers.
	PackagedPlanWorkstationName                  = "plan-goal"
	PackagedCheckWorkstationName                 = "check-goal"
	PackagedReviewWorkstationName                = "review-goal"
	PackagedStructuredReviewStateName            = "structured-review"
	PackagedStructuredReviewWorkstationName      = "structured-review-goal"
	PackagedLoopBreakerWorkstationName           = "goal-loop-breaker"
	PackagedStructuredLoopBreakerWorkstationName = "goal-structured-loop-breaker"
	PackagedCheckReviewModeEnvVar                = "YOU_GOAL_REVIEW_MODE"
	PackagedReviewModePlainLabel                 = "plain"
	PackagedReviewModeStructuredLabel            = "structured"
	// PackagedInvokeWorkstationName aliases the execute workstation for simplified
	// invocation-primary-result scaffolds and legacy references.
	PackagedInvokeWorkstationName = PackagedExecuteWorkstationName
	// PackagedInvocationReturnWorkTypeName is the work type selected by the goal
	// factory invocationReturn policy.
	PackagedInvocationReturnWorkTypeName = PackagedGoalWorkTypeName
	// PackagedInvocationReturnTerminalState is the terminal state selected by the
	// goal factory invocationReturn policy.
	PackagedInvocationReturnTerminalState = "complete"
)

// PackagedGoalRolePromptSource identifies the authored split prompt file for a goal role.
type PackagedGoalRolePromptSource struct {
	Role            string
	WorkerName      string
	WorkstationName string
	PromptFile      string
	SourceKind      string
}

const (
	PackagedGoalRolePromptSourceKindWorkstationPromptFile = "workstation_prompt_file"
	PackagedGoalRolePromptSourceKindWorkerBody            = "worker_body"
)

// PackagedGoalRolePromptSources lists each role-specific authored prompt source in
// the packaged goal factory's normal worker/workstation load paths.
var PackagedGoalRolePromptSources = []PackagedGoalRolePromptSource{
	{Role: "executor", WorkstationName: PackagedExecuteWorkstationName, PromptFile: "prompts/executor.md", SourceKind: PackagedGoalRolePromptSourceKindWorkstationPromptFile},
}
