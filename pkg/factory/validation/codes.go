package validation

const (
	CodeDuplicateIdentifier              = "factory.duplicateIdentifier"
	CodeDanglingWorkerReference          = "factory.worker.danglingReference"
	CodeDanglingPlaceReference           = "factory.route.danglingPlaceReference"
	CodeDanglingResourceReference        = "factory.resource.danglingReference"
	CodeWorkstationMissingFailureRoute   = "factory.workstation.missingFailureRoute"
	CodeWorkstationMissingRejectionRoute = "factory.workstation.missingRejectionRoute"
	CodeWorkstationConflictingOutputs    = "factory.workstation.conflictingWorkStateOutputs"
	CodeWorkTypeMissingCompletionState   = "factory.workType.missingCompletionState"
	CodeWorkTypeMissingFailureState      = "factory.workType.missingFailureState"
	CodeWorkStateMissingTerminalPath     = "factory.workState.missingTerminalCompletionPath"
)
