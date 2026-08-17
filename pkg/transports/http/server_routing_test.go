package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingshttp "github.com/portpowered/infinite-you/pkg/services/recordings/transports/http"
	"go.uber.org/zap"
)

func TestUnknownRouteReturnsStructuredNotFound(t *testing.T) {
	srv := newFactoryDefinitionTestServer(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/functional-routing-unknown-route", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "route not found")
}

func TestWrongMethodReturnsDocumentedMethodError(t *testing.T) {
	srv := newFactoryDefinitionTestServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/status", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}

func TestServerWithRecordings_RoutesDurableResultToRecordingsAdapter(t *testing.T) {
	t.Parallel()

	sessionID := "dur-sess-server-recordings-001"
	root := &serverRecordingsRoot{}
	srv := NewServerWithRecordings(
		recordingshttp.NewAdapter(root),
		nil, nil, nil, nil, nil, zap.NewNop(),
	)
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/results", nil),
	)

	if recorder.Code != http.StatusOK || !root.queried ||
		!containsJSONField(recorder.Body.String(), "sessionId", sessionID) {
		t.Fatalf("response = %d %s, queried=%v, want Recordings-owned result", recorder.Code, recorder.Body.String(), root.queried)
	}
}

func containsJSONField(body, field, value string) bool {
	return strings.Contains(body, `"`+field+`":"`+value+`"`)
}

type serverRecordingsRoot struct {
	recordings.Service
	queried bool
}

func (root *serverRecordingsRoot) QueryRecordingStatus(
	request recordings.RecordingStatusRequest,
) (recordings.RecordingStatusResult, error) {
	return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
		RecordingID: request.RecordingID,
		Artifact:    "artifact-server-recordings-001",
		State:       recordings.RecordingFinalized,
	}}, nil
}

func (root *serverRecordingsRoot) QueryHistoricalRecording(
	request recordings.HistoricalRecordingQueryRequest,
) (recordings.HistoricalRecordingQueryResult, error) {
	root.queried = true
	return recordings.HistoricalRecordingQueryResult{
		Recording: request.Recording,
		Status: recordings.RecordingStatusFacts{
			RecordingID: request.Recording.RecordingID,
			State:       recordings.RecordingFinalized,
		},
		WorldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Payload:       `{}`,
		},
	}, nil
}
