package http

var (
	CatalogRootErrorResponseForTest           = CatalogRootErrorResponse
	WriteCatalogOrInternalErrorForTest        = (*Adapter).writeCatalogOrInternalError
	ExecuteRequestContextErrorResponseForTest = executeRequestContextErrorResponse
	ExecuteRootErrorResponseForTest           = ExecuteRootErrorResponse
	WriteExecuteOrInternalErrorForTest        = (*Adapter).writeExecuteOrInternalError
)
