package http

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	sessionEventStreamBackendScopeHeader      = factorysessionshttp.SessionEventStreamBackendScopeHeader
	sessionEventStreamLogicalSessionKeyHeader = factorysessionshttp.SessionEventStreamLogicalSessionKeyHeader
	sessionEventStreamFactorySessionHeader    = factorysessionshttp.SessionEventStreamFactorySessionHeader
	sessionEventStreamGenerationHeader        = factorysessionshttp.SessionEventStreamGenerationHeader
	submitWorkItemTypeMetadataKey             = factorysessionshttp.SubmitWorkItemTypeMetadataKey
	submitWorkFileNameMetadataKey             = factorysessionshttp.SubmitWorkFileNameMetadataKey
)

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func reconnectCursorFromParams(
	afterEventID *factoryapi.AfterEventId,
	afterSequence *factoryapi.AfterSequence,
) *interfaces.FactoryEventReconnectCursor {
	return factorysessionshttp.ReconnectCursorFromParams(afterEventID, afterSequence)
}

func workReadModelToGenerated(item work.ReadModel) factoryapi.Work {
	return factorysessionshttp.WorkReadModelToGenerated(item)
}

func decodeStrictJSON[T any](body io.Reader) (T, error) {
	return factorysessionshttp.DecodeStrictJSON[T](body)
}

func requestFieldValidationMessage(err error) (string, bool) {
	return factorysessionshttp.RequestFieldValidationMessage(err)
}

func submitWorkResponseFromResult(result work.WorkRequestSubmitResult, sessionID string) factoryapi.SubmitWorkResponse {
	return factorysessionshttp.SubmitWorkResponseFromResult(result, sessionID)
}

func (s *Server) getEvents(
	w http.ResponseWriter,
	r *http.Request,
	includeSessionHandshake bool,
	subscribe func(context.Context) (*interfaces.FactoryEventStream, error),
) {
	s.Adapter.StreamFactoryEvents(w, r, includeSessionHandshake, subscribe)
}

func TestServerConstructionBoundary_RetiredAggregateSurfaceCannotReturn(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server source: %v", err)
	}
	text := string(source)
	for _, retired := range []string{
		"NewServerFromSurface",
		"type Binding struct",
		"type StableDependencies struct",
		"func NewHandler(",
		"func NewStrictRoleServer(",
		"optionalDurableExecutionSessionLister",
		"legacyDurableExecutionSessionLister",
	} {
		if strings.Contains(text, retired) {
			t.Fatalf("server source contains retired aggregate construction surface %q", retired)
		}
	}
}
