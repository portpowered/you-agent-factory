package http

var (
	SettingsRootErrorResponseForTest           = RootErrorResponse
	SettingsRequestContextErrorResponseForTest = settingsRequestContextErrorResponse
	WriteRootOrInternalErrorForTest            = (*Adapter).writeRootOrInternalError
	WriteSettingsRequestContextOutcomeForTest  = (*Adapter).writeSettingsRequestContextOutcome
)
