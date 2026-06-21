package goal

const (
	// PackagedFactoryName is the canonical named factory identifier for @you/goal.
	PackagedFactoryName = "@you/goal"
	// PackagedFactoryProject identifies the built-in @you/goal factory project id.
	PackagedFactoryProject = "builtin-goal"
	// PackagedGoalWorkTypeName is the canonical default goal work type name.
	PackagedGoalWorkTypeName = "goal"
	// PackagedPlanWorkstationName is the workstation that plans submitted goal work.
	PackagedPlanWorkstationName = "plan-goal"
	// PackagedExecuteWorkstationName is the workstation that executes planned goal work.
	PackagedExecuteWorkstationName = "execute-goal"
	// PackagedCheckWorkstationName is the workstation that runs goal verification checks.
	PackagedCheckWorkstationName = "check-goal"
	// PackagedReviewWorkstationName is the workstation that classifies review outcomes.
	PackagedReviewWorkstationName = "review-goal"
	// PackagedLoopBreakerWorkstationName is the guarded loop breaker for review retries.
	PackagedLoopBreakerWorkstationName = "goal-loop-breaker"
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
	{Role: "planner", WorkstationName: PackagedPlanWorkstationName, PromptFile: "prompts/planner.md", SourceKind: PackagedGoalRolePromptSourceKindWorkstationPromptFile},
	{Role: "executor", WorkstationName: PackagedExecuteWorkstationName, PromptFile: "prompts/executor.md", SourceKind: PackagedGoalRolePromptSourceKindWorkstationPromptFile},
	{Role: "checker", WorkstationName: PackagedCheckWorkstationName, PromptFile: "prompts/checker.md", SourceKind: PackagedGoalRolePromptSourceKindWorkstationPromptFile},
	{Role: "reviewer", WorkerName: "goal-reviewer", SourceKind: PackagedGoalRolePromptSourceKindWorkerBody},
	{Role: "summarizer", WorkstationName: PackagedReviewWorkstationName, PromptFile: "prompts/summarizer.md", SourceKind: PackagedGoalRolePromptSourceKindWorkstationPromptFile},
}
