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
	// PackagedStructuredReviewStateName is the goal state for structured envelope review.
	PackagedStructuredReviewStateName = "structured-review"
	// PackagedAdvanceStructuredReviewWorkstationName advances checked goal work into
	// the structured envelope review lane.
	PackagedAdvanceStructuredReviewWorkstationName = "advance-goal-structured-review"
	// PackagedStructuredReviewWorkstationName routes richer reviewer envelopes through
	// parsed goal decision labels.
	PackagedStructuredReviewWorkstationName = "structured-review-goal"
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
