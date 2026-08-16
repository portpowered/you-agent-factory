package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestGetEventsBySessionId_DurableHistoryUsesHistoricalQueryAndPreservesOrder(t *testing.T) {
	t.Parallel()

	sessionID := "dur-sess-history-http-001"
	artifact := recordings.RecordingArtifactReference("artifact-history-http-001")
	first := historicalHTTPTestEvent(sessionID, "event-history-1", 0)
	second := historicalHTTPTestEvent(sessionID, "event-history-2", 1)
	var queried bool
	adapter := NewAdapter(&rootFake{
		queryRecordingStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
				RecordingID: request.RecordingID, Artifact: artifact, State: recordings.RecordingFinalized,
			}}, nil
		},
		queryHistoricalRecording: func(request recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
			queried = true
			if request.Recording.Artifact != artifact || request.Recording.Scope.FactorySessionID != sessionID {
				t.Fatalf("historical request = %#v, want artifact and session scope", request)
			}
			return recordings.HistoricalRecordingQueryResult{
				Recording: request.Recording, Events: []recordings.CanonicalEvent{first, second},
				Status: recordings.RecordingStatusFacts{RecordingID: request.Recording.RecordingID, Artifact: artifact, State: recordings.RecordingFinalized},
			}, nil
		},
	})
	recorder := httptest.NewRecorder()
	adapter.GetEventsBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/events", nil),
		factoryapi.SessionID(sessionID),
		factoryapi.GetEventsBySessionIdParams{},
	)

	if !queried {
		t.Fatal("durable event history did not invoke QueryHistoricalRecording")
	}
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, `"id":"event-history-1"`) || !strings.Contains(body, `"id":"event-history-2"`) {
		t.Fatalf("response = %d %s, want ordered historical SSE", recorder.Code, body)
	}
	if strings.Index(body, "event-history-1") > strings.Index(body, "event-history-2") {
		t.Fatalf("historical events are out of order: %s", body)
	}
}

func TestGetFactorySessionResults_DurableHistoryUsesHistoricalWorldState(t *testing.T) {
	t.Parallel()

	sessionID := "dur-sess-history-http-result-001"
	artifact := recordings.RecordingArtifactReference("artifact-history-http-result-001")
	adapter := NewAdapter(&rootFake{
		queryRecordingStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
				RecordingID: request.RecordingID, Artifact: artifact, State: recordings.RecordingFinalized,
			}}, nil
		},
		queryHistoricalRecording: func(request recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
			return recordings.HistoricalRecordingQueryResult{
				Recording:  request.Recording,
				Status:     recordings.RecordingStatusFacts{RecordingID: request.Recording.RecordingID, State: recordings.RecordingFinalized},
				WorldState: recordings.WorldStateView{SchemaVersion: recordings.WorldStateViewSchemaV1, Payload: `{}`},
			}, nil
		},
	})
	recorder := httptest.NewRecorder()
	adapter.GetFactorySessionResults(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/results", nil),
		factoryapi.SessionID(sessionID),
		factoryapi.GetFactorySessionResultsParams{},
	)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"sessionId":"`+sessionID+`"`) || !strings.Contains(recorder.Body.String(), `"resultStatus":"FINAL"`) {
		t.Fatalf("response = %d %s, want historical final result", recorder.Code, recorder.Body.String())
	}
}

func TestGetFactorySessionResults_DurableHistoryMapsMissingHistoryToNotFound(t *testing.T) {
	t.Parallel()

	sessionID := "dur-sess-history-http-missing-001"
	adapter := NewAdapter(&rootFake{
		queryRecordingStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
				RecordingID: request.RecordingID, Artifact: "artifact-history-http-missing-001", State: recordings.RecordingFinalized,
			}}, nil
		},
		queryHistoricalRecording: func(request recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
			return recordings.HistoricalRecordingQueryResult{}, &recordings.HistoricalRecordingQueryError{
				Kind: recordings.HistoricalRecordingQueryErrorMissingHistory, RecordingID: request.Recording.RecordingID,
			}
		},
	})
	recorder := httptest.NewRecorder()
	adapter.GetFactorySessionResults(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/results", nil),
		factoryapi.SessionID(sessionID),
		factoryapi.GetFactorySessionResultsParams{},
	)
	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("response = %d %s, want typed not found", recorder.Code, recorder.Body.String())
	}
}

func historicalHTTPTestEvent(sessionID, id string, sequence int64) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID: recordings.CanonicalEventID(id), Sequence: recordings.CanonicalEventSequence(sequence), FactoryTick: int(sequence + 1),
		Scope:      recordings.CanonicalEventScope{FactorySessionID: sessionID},
		Cursor:     recordings.CanonicalEventCursor{StreamGenerationID: "history-http-generation", Sequence: recordings.CanonicalEventSequence(sequence)},
		RecordedAt: time.Unix(1_700_000_000+sequence, 0).UTC(), Kind: "WORK_REQUEST", Payload: `{"workTypeId":"task"}`,
	}
}
