package climanifestparity

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

const (
	SessionShowHandlerID   = "you.session.show.handler"
	SessionShowOperationID = "getFactorySession"
	SessionShowHTTPMethod  = "GET"
	SessionShowHTTPPath    = "/factory-sessions/{session_id}"

	WorkListHandlerID    = "you.work.list.handler"
	WorkListOperationID  = "listWorkBySessionId"
	WorkListHTTPMethod   = "GET"
	WorkListHTTPPath     = "/factory-sessions/{session_id}/work"

	WorkShowHandlerID   = "you.work.show.handler"
	WorkShowOperationID = "getWorkBySessionId"
	WorkShowHTTPMethod  = "GET"
	WorkShowHTTPPath    = "/factory-sessions/{session_id}/work/{id}"

	WorkMoveHandlerID   = "you.work.move.handler"
	WorkMoveOperationID = "moveWorkBySessionId"
	WorkMoveHTTPMethod  = "POST"
	WorkMoveHTTPPath    = "/factory-sessions/{session_id}/work/{id}/move"
)

// CompareDeclaredHandler asserts contracted handler identity and optional OpenAPI operationId.
func CompareDeclaredHandler(record climanifest.Command, wantHandlerID, wantOperationID string) []Mismatch {
	var mismatches []Mismatch
	if record.Handler == nil {
		return []Mismatch{{
			CommandID: record.ID,
			Field:     "handler",
			Want:      fmt.Sprintf("declared handler id %q", wantHandlerID),
			Got:       "missing",
		}}
	}
	if record.Handler.ID != wantHandlerID {
		mismatches = append(mismatches, Mismatch{
			CommandID: record.ID,
			Field:     "handler.id",
			Want:      wantHandlerID,
			Got:       record.Handler.ID,
		})
	}
	if wantOperationID != "" && record.Handler.OperationID != wantOperationID {
		mismatches = append(mismatches, Mismatch{
			CommandID: record.ID,
			Field:     "handler.operationId",
			Want:      wantOperationID,
			Got:       record.Handler.OperationID,
		})
	}
	return mismatches
}

// OpenAPIOperationBinding resolves one operationId to its HTTP method and path.
func OpenAPIOperationBinding(doc *openapi3.T, operationID string) (method string, path string, ok bool) {
	if doc == nil || doc.Paths == nil {
		return "", "", false
	}
	for itemPath, pathItem := range doc.Paths.Map() {
		if pathItem == nil {
			continue
		}
		for _, candidate := range []struct {
			method string
			op     *openapi3.Operation
		}{
			{method: "GET", op: pathItem.Get},
			{method: "POST", op: pathItem.Post},
			{method: "PUT", op: pathItem.Put},
			{method: "PATCH", op: pathItem.Patch},
			{method: "DELETE", op: pathItem.Delete},
			{method: "HEAD", op: pathItem.Head},
			{method: "OPTIONS", op: pathItem.Options},
			{method: "TRACE", op: pathItem.Trace},
		} {
			if candidate.op != nil && candidate.op.OperationID == operationID {
				return candidate.method, itemPath, true
			}
		}
	}
	return "", "", false
}

// CompareHandlerOpenAPIBinding asserts a contracted operationId maps to the expected HTTP route.
func CompareHandlerOpenAPIBinding(record climanifest.Command, doc *openapi3.T, wantMethod, wantPath string) []Mismatch {
	if record.Handler == nil || strings.TrimSpace(record.Handler.OperationID) == "" {
		return []Mismatch{{
			CommandID: record.ID,
			Field:     "handler.operationId",
			Want:      "declared OpenAPI operationId",
			Got:       "missing",
		}}
	}

	method, path, ok := OpenAPIOperationBinding(doc, record.Handler.OperationID)
	if !ok {
		return []Mismatch{{
			CommandID: record.ID,
			Field:     "handler.operationId.openapi",
			Want:      fmt.Sprintf("OpenAPI operation %q", record.Handler.OperationID),
			Got:       "operation not found in bundled contract",
		}}
	}
	if method != wantMethod || path != wantPath {
		return []Mismatch{{
			CommandID: record.ID,
			Field:     "handler.operationId.route",
			Want:      fmt.Sprintf("%s %s", wantMethod, wantPath),
			Got:       fmt.Sprintf("%s %s", method, path),
		}}
	}
	return nil
}
