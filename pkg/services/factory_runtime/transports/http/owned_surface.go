package http

// OwnedHTTPOperationIDs lists the generated OpenAPI operationIds adapted by
// this package. HTTP-RUN adds Runtime control, move-work, and dispatch-plan
// slices without authoring new shared OpenAPI operations; later stories add
// checkpoint slices.
var OwnedHTTPOperationIDs = []string{
	"getStatus",
	"getStatusBySessionId",
	"moveWorkBySessionId",
}
