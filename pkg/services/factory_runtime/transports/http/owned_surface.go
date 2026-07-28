package http

// OwnedHTTPOperationIDs lists the generated OpenAPI operationIds adapted by
// this package. HTTP-RUN adds Runtime control, move-work, dispatch-plan, and
// checkpoint slices without authoring new shared OpenAPI operations.
var OwnedHTTPOperationIDs = []string{
	"getStatus",
	"getStatusBySessionId",
	"moveWorkBySessionId",
}
