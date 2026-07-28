package http

var (
	AutomationsRootErrorResponseForTest            = RootErrorResponse
	AutomationsRequestContextErrorResponseForTest  = automationsRequestContextErrorResponse
	WriteRootOrInternalErrorForTest                = (*Adapter).writeRootOrInternalError
	WriteAutomationsRequestContextOutcomeForTest   = (*Adapter).writeAutomationsRequestContextOutcome
)
