package validation

const (
	CodeDuplicateIdentifier              = "factory.duplicateIdentifier"
	CodeDanglingWorkerReference          = "factory.worker.danglingReference"
	CodeDanglingPlaceReference           = "factory.route.danglingPlaceReference"
	CodeDanglingResourceReference        = "factory.resource.danglingReference"
	CodeWorkstationMissingOutputRoutes   = "factory.workstation.missingOutputRoutes"
	CodeWorkstationMissingFailureRoute   = "factory.workstation.missingFailureRoute"
	CodeWorkstationMissingRejectionRoute = "factory.workstation.missingRejectionRoute"
	CodeWorkstationConflictingOutputs    = "factory.workstation.conflictingWorkStateOutputs"
	CodeWorkTypeMissingCompletionState   = "factory.workType.missingCompletionState"
	CodeWorkTypeMissingFailureState      = "factory.workType.missingFailureState"
	CodeWorkStateMissingTerminalPath     = "factory.workState.missingTerminalCompletionPath"
	CodeWorkTypeHandlingBehaviorValue    = "work-type-handling-behavior-value"
	CodeWorkTypeHandlingBehaviorDuplicate = "work-type-handling-behavior-duplicate"
	CodeWorkTypeHandlingBehaviorUniqueDefault = "work-type-handling-behavior-unique-default"
	CodeWorkTypeHandlingBehaviorRequiredDefault = "work-type-handling-behavior-required-default"
)
