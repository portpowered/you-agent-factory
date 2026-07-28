package http

// OwnedHTTPOperationIDs lists the generated OpenAPI operationIds adapted by
// this package. HTTP-RUN adds Runtime control and move-work slices without
// authoring new shared OpenAPI operations; later stories add dispatch-plan and
// checkpoint slices.
var OwnedHTTPOperationIDs = []string{
	"getStatus",
	"getStatusBySessionId",
	"moveWorkBySessionId",
}
