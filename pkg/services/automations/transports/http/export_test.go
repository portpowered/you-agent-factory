package http

var (
	AutomationsRootErrorResponseForTest = RootErrorResponse
	WriteRootOrInternalErrorForTest     = (*Adapter).writeRootOrInternalError
)
