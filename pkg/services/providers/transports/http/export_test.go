package http

var (
	CatalogRootErrorResponseForTest    = CatalogRootErrorResponse
	WriteCatalogOrInternalErrorForTest = (*Adapter).writeCatalogOrInternalError
	ExecuteRootErrorResponseForTest    = ExecuteRootErrorResponse
	WriteExecuteOrInternalErrorForTest = (*Adapter).writeExecuteOrInternalError
)
