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
	// PackagedSummarizeWorkstationName hosts the summarizer split prompt without adding a summarize lifecycle stage.
	PackagedSummarizeWorkstationName = "summarize-goal"
)

// PackagedGoalRolePromptSource identifies the authored split prompt file for a goal role.
type PackagedGoalRolePromptSource struct {
	Role            string
	WorkstationName string
	PromptFile      string
}

// PackagedGoalRolePromptSources lists each role-specific split prompt wired through
// the normal workstation promptFile resolution path.
var PackagedGoalRolePromptSources = []PackagedGoalRolePromptSource{
	{Role: "planner", WorkstationName: PackagedPlanWorkstationName, PromptFile: "prompts/planner.md"},
	{Role: "executor", WorkstationName: PackagedExecuteWorkstationName, PromptFile: "prompts/executor.md"},
	{Role: "checker", WorkstationName: PackagedCheckWorkstationName, PromptFile: "prompts/checker.md"},
	{Role: "reviewer", WorkstationName: PackagedReviewWorkstationName, PromptFile: "prompts/reviewer.md"},
	{Role: "summarizer", WorkstationName: PackagedSummarizeWorkstationName, PromptFile: "prompts/summarizer.md"},
}
