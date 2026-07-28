// Package goal is a transitional shim over the Distribution-owned goal package
// asset/metadata implementation. Decision-envelope interpretation remains here
// temporarily until invocation_policy cutover.
package goal

import (
	distributiongoal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/goal"
)

const (
	PackagedFactoryName                            = distributiongoal.PackagedFactoryName
	PackagedFactoryProject                         = distributiongoal.PackagedFactoryProject
	PackagedGoalWorkTypeName                       = distributiongoal.PackagedGoalWorkTypeName
	PackagedExecuteWorkstationName                 = distributiongoal.PackagedExecuteWorkstationName
	PackagedPlanWorkstationName                    = distributiongoal.PackagedPlanWorkstationName
	PackagedCheckWorkstationName                   = distributiongoal.PackagedCheckWorkstationName
	PackagedReviewWorkstationName                  = distributiongoal.PackagedReviewWorkstationName
	PackagedStructuredReviewStateName              = distributiongoal.PackagedStructuredReviewStateName
	PackagedStructuredReviewWorkstationName        = distributiongoal.PackagedStructuredReviewWorkstationName
	PackagedLoopBreakerWorkstationName             = distributiongoal.PackagedLoopBreakerWorkstationName
	PackagedStructuredLoopBreakerWorkstationName   = distributiongoal.PackagedStructuredLoopBreakerWorkstationName
	PackagedCheckReviewModeEnvVar                  = distributiongoal.PackagedCheckReviewModeEnvVar
	PackagedReviewModePlainLabel                   = distributiongoal.PackagedReviewModePlainLabel
	PackagedReviewModeStructuredLabel              = distributiongoal.PackagedReviewModeStructuredLabel
	PackagedInvokeWorkstationName                  = distributiongoal.PackagedInvokeWorkstationName
	PackagedInvocationReturnWorkTypeName           = distributiongoal.PackagedInvocationReturnWorkTypeName
	PackagedInvocationReturnTerminalState          = distributiongoal.PackagedInvocationReturnTerminalState
	PackagedGoalRolePromptSourceKindWorkstationPromptFile = distributiongoal.PackagedGoalRolePromptSourceKindWorkstationPromptFile
	PackagedGoalRolePromptSourceKindWorkerBody     = distributiongoal.PackagedGoalRolePromptSourceKindWorkerBody
)

// PackagedGoalRolePromptSource identifies the authored split prompt file for a goal role.
type PackagedGoalRolePromptSource = distributiongoal.PackagedGoalRolePromptSource

// PackagedGoalRolePromptSources lists each role-specific authored prompt source in
// the packaged goal factory's normal worker/workstation load paths.
var PackagedGoalRolePromptSources = distributiongoal.PackagedGoalRolePromptSources
