package smoke

// These values are customer-facing identifiers used by CLI scenarios. Keeping
// them at the functional boundary prevents tests from importing the packaged
// Factory implementation merely to spell public command arguments and event
// fields.
var publicGoal = struct {
	PackagedFactoryName                     string
	PackagedGoalWorkTypeName                string
	PackagedPlanWorkstationName             string
	PackagedExecuteWorkstationName          string
	PackagedCheckWorkstationName            string
	PackagedReviewWorkstationName           string
	PackagedStructuredReviewWorkstationName string
	PackagedCheckReviewModeEnvVar           string
	PackagedReviewModeStructuredLabel       string
}{
	PackagedFactoryName:                     "@you/goal",
	PackagedGoalWorkTypeName:                "goal",
	PackagedPlanWorkstationName:             "plan-goal",
	PackagedExecuteWorkstationName:          "execute-goal",
	PackagedCheckWorkstationName:            "check-goal",
	PackagedReviewWorkstationName:           "review-goal",
	PackagedStructuredReviewWorkstationName: "structured-review-goal",
	PackagedCheckReviewModeEnvVar:           "YOU_GOAL_REVIEW_MODE",
	PackagedReviewModeStructuredLabel:       "structured",
}
