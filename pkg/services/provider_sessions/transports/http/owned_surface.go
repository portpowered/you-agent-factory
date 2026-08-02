package http

// OwnedHTTPOperationIDs lists the generated OpenAPI operationIds adapted by
// this package. HTTP-PSES owns getProviderSessionDetails only; root Inspect and
// Project slices remain peer APIs without adapter-owned HTTP mapping in this
// packet. The top-level HTTP transport forwards the generated route to this
// handler without authoring new shared OpenAPI operations.
var OwnedHTTPOperationIDs = []string{
	"getProviderSessionDetails",
}
