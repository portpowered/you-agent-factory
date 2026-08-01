package http

import (
	"context"
	"io"
	"net/http"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workhttp "github.com/portpowered/infinite-you/pkg/services/work/transports/http"
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
	return workhttp.WorkReadModelToAPI(item)
}

func decodeStrictJSON[T any](body io.Reader) (T, error) {
	return factorysessionshttp.DecodeStrictJSON[T](body)
}

func requestFieldValidationMessage(err error) (string, bool) {
	return factorysessionshttp.RequestFieldValidationMessage(err)
}

func submitWorkResponseFromResult(result work.WorkRequestSubmitResult, sessionID string) factoryapi.SubmitWorkResponse {
	return workhttp.SubmitWorkResponseToAPI(result, sessionID)
}

func (s *Server) getEvents(
	w http.ResponseWriter,
	r *http.Request,
	includeSessionHandshake bool,
	subscribe func(context.Context) (*interfaces.FactoryEventStream, error),
) {
	s.Adapter.StreamFactoryEvents(w, r, includeSessionHandshake, subscribe)
}
