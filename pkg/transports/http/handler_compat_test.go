package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
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

func TestListPackagedFactoriesReturnsPublishedCatalog(t *testing.T) {
	srv := NewServer(nil, nil, nil, zap.NewNop())
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/packaged-factories", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response factoryapi.PackagedFactoryCatalogResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Factories) == 0 {
		t.Fatal("catalog response contained no factories")
	}
	for _, factory := range response.Factories {
		if factory.Name == "" || factory.Project == "" || factory.Slug == "" || len(factory.Json) == 0 || factory.Yaml == "" {
			t.Fatalf("catalog entry is incomplete: %#v", factory)
		}
	}
}
