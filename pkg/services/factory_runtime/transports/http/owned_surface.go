package http

// OwnedHTTPOperationIDs lists the generated OpenAPI operationIds adapted by
// this package. HTTP-RUN owns Runtime status reads only in the initial binding
// packet; later stories add control, dispatch-plan, and checkpoint slices
// without authoring new shared OpenAPI operations.
var OwnedHTTPOperationIDs = []string{
	"getStatus",
	"getStatusBySessionId",
}
