package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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

func TestHistoricalAdapter_LiveResultFallsBackToFactorySessions(t *testing.T) {
	t.Parallel()

	sessionID := "dur-sess-live-fallback-result-001"
	legacyCalls := 0
	historicalCalls := 0
	adapter := NewAdapterWithLegacyFallback(
		&rootFake{
			queryRecordingStatus: func(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
				return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
					RecordingID: recordings.RecordingID(sessionID),
				}}, nil
			},
			queryHistoricalRecording: func(recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
				historicalCalls++
				return recordings.HistoricalRecordingQueryResult{}, nil
			},
		},
		&legacyHistoryFake{
			result: func(context.Context, string, factorysessions.ResultRequest) (factoryapi.FactorySessionResult, error) {
				legacyCalls++
				return factoryapi.FactorySessionResult{
					SessionId: sessionID, ResultStatus: factoryapi.FactorySessionResultStatusNotReady,
				}, nil
			},
		}, nil, nil,
	)
	recorder := httptest.NewRecorder()
	adapter.GetFactorySessionResults(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/results", nil),
		factoryapi.SessionID(sessionID), factoryapi.GetFactorySessionResultsParams{},
	)

	if recorder.Code != http.StatusOK ||
		!strings.Contains(recorder.Body.String(), `"resultStatus":"NOT_READY"`) ||
		legacyCalls != 1 || historicalCalls != 0 {
		t.Fatalf("response = %d %s, legacyCalls=%d historicalCalls=%d, want live legacy result only", recorder.Code, recorder.Body.String(), legacyCalls, historicalCalls)
	}
}

func TestHistoricalAdapter_LiveDispatchAndArtifactReadsFallbackToFactorySessions(t *testing.T) {
	t.Parallel()

	sessionID := "dur-sess-live-fallback-inspection-001"
	legacy := &legacyHistoryFake{
		dispatches: func(context.Context, string, factoryapi.ListFactorySessionDispatchesParams) (factoryapi.ListFactorySessionDispatchesResponse, error) {
			return factoryapi.ListFactorySessionDispatchesResponse{
				SessionId: sessionID,
				Dispatches: []factoryapi.FactorySessionDispatchSummary{{
					Id: "dispatch-live-001", Status: factoryapi.FactoryDispatchStatusCOMPLETED,
				}},
			}, nil
		},
		artifacts: func(context.Context, string) (factoryapi.ListFactorySessionArtifactsResponse, error) {
			return factoryapi.ListFactorySessionArtifactsResponse{
				SessionId: sessionID,
				Artifacts: []factoryapi.FactorySessionArtifactSummary{{Id: "artifact-live-001"}},
			}, nil
		},
	}
	root := &rootFake{
		queryRecordingStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
				RecordingID: request.RecordingID,
			}}, nil
		},
	}
	adapter := NewAdapterWithLegacyFallback(root, legacy, nil, nil)

	list := httptest.NewRecorder()
	adapter.ListFactorySessionDispatches(
		list,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/dispatches", nil),
		factoryapi.SessionID(sessionID), factoryapi.ListFactorySessionDispatchesParams{},
	)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"id":"dispatch-live-001"`) {
		t.Fatalf("dispatch response = %d %s, want legacy live dispatch", list.Code, list.Body.String())
	}

	artifacts := httptest.NewRecorder()
	adapter.ListFactorySessionArtifacts(
		artifacts,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/artifacts", nil),
		factoryapi.SessionID(sessionID),
	)
	if artifacts.Code != http.StatusOK || !strings.Contains(artifacts.Body.String(), `"id":"artifact-live-001"`) {
		t.Fatalf("artifact response = %d %s, want legacy live artifacts", artifacts.Code, artifacts.Body.String())
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

func TestListFactorySessionDispatches_DurableHistoryUsesHistoricalProjection(t *testing.T) {
	t.Parallel()

	sessionID := "dur-sess-history-http-dispatch-001"
	artifact := recordings.RecordingArtifactReference("artifact-history-http-dispatch-001")
	adapter := NewAdapter(&rootFake{
		queryRecordingStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
				RecordingID: request.RecordingID, Artifact: artifact, State: recordings.RecordingFinalized,
			}}, nil
		},
		queryHistoricalRecording: func(request recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
			return recordings.HistoricalRecordingQueryResult{
				Recording: request.Recording,
				Dispatches: []recordings.HistoricalDispatch{{
					ID: "dispatch-history-1", Status: recordings.FactoryDispatchStatusCompleted,
					DispatchKind: recordings.FactoryDispatchKindJavaScriptScript,
				}},
			}, nil
		},
	})
	recorder := httptest.NewRecorder()
	adapter.ListFactorySessionDispatches(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/dispatches", nil),
		factoryapi.SessionID(sessionID), factoryapi.ListFactorySessionDispatchesParams{},
	)

	if recorder.Code != http.StatusOK ||
		!strings.Contains(recorder.Body.String(), `"id":"dispatch-history-1"`) ||
		!strings.Contains(recorder.Body.String(), `"dispatchKind":"JAVASCRIPT_SCRIPT"`) {
		t.Fatalf("response = %d %s, want historical dispatch projection", recorder.Code, recorder.Body.String())
	}
}

func TestGetFactorySessionDispatch_DurableHistoryMapsMissingDispatchToNotFound(t *testing.T) {
	t.Parallel()

	sessionID := "dur-sess-history-http-dispatch-missing-001"
	adapter := NewAdapter(&rootFake{
		queryRecordingStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
				RecordingID: request.RecordingID, Artifact: "artifact-history-http-dispatch-missing-001", State: recordings.RecordingFinalized,
			}}, nil
		},
		queryHistoricalRecording: func(request recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
			return recordings.HistoricalRecordingQueryResult{Recording: request.Recording}, nil
		},
	})
	recorder := httptest.NewRecorder()
	adapter.GetFactorySessionDispatch(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/dispatches/missing", nil),
		factoryapi.SessionID(sessionID), factoryapi.DispatchID("missing"),
	)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("response = %d %s, want typed missing dispatch", recorder.Code, recorder.Body.String())
	}
}

func TestGetEventsBySessionId_DurableProbeDistinguishesStaleCursorAndMissingHistory(t *testing.T) {
	t.Parallel()

	sessionID := "dur-sess-history-http-probe-001"
	artifact := recordings.RecordingArtifactReference("artifact-history-http-probe-001")
	adapter := NewAdapter(&rootFake{
		queryRecordingStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			if request.RecordingID == recordings.RecordingID("dur-sess-history-http-missing-001") {
				return recordings.RecordingStatusResult{}, recordings.ErrMissingRecordingTarget
			}
			return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
				RecordingID: request.RecordingID, Artifact: artifact, State: recordings.RecordingFinalized,
			}}, nil
		},
		queryHistoricalRecording: func(request recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
			return recordings.HistoricalRecordingQueryResult{Recording: request.Recording, Events: []recordings.CanonicalEvent{
				historicalHTTPTestEvent(sessionID, "event-history-probe-1", 0),
			}}, nil
		},
	})

	staleRequest := httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/events", nil)
	staleRequest.Header.Set("Accept", "application/json")
	staleEventID := factoryapi.AfterEventId("missing-event")
	stale := httptest.NewRecorder()
	adapter.GetEventsBySessionId(stale, staleRequest, factoryapi.SessionID(sessionID), factoryapi.GetEventsBySessionIdParams{AfterEventId: &staleEventID})
	if stale.Code != http.StatusOK || !strings.Contains(stale.Body.String(), `"outcome":"CURSOR_STALE"`) {
		t.Fatalf("stale probe = %d %s, want cursor stale", stale.Code, stale.Body.String())
	}

	missingRequest := httptest.NewRequest(http.MethodGet, "/factory-sessions/dur-sess-history-http-missing-001/events", nil)
	missingRequest.Header.Set("Accept", "application/json")
	missing := httptest.NewRecorder()
	adapter.GetEventsBySessionId(missing, missingRequest, "dur-sess-history-http-missing-001", factoryapi.GetEventsBySessionIdParams{})
	if missing.Code != http.StatusOK || !strings.Contains(missing.Body.String(), `"outcome":"UNKNOWN_SESSION"`) {
		t.Fatalf("missing-history probe = %d %s, want unknown session", missing.Code, missing.Body.String())
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

type legacyHistoryFake struct {
	LegacyHistory
	result     func(context.Context, string, factorysessions.ResultRequest) (factoryapi.FactorySessionResult, error)
	dispatches func(context.Context, string, factoryapi.ListFactorySessionDispatchesParams) (factoryapi.ListFactorySessionDispatchesResponse, error)
	artifacts  func(context.Context, string) (factoryapi.ListFactorySessionArtifactsResponse, error)
}

func (fake *legacyHistoryFake) GetDurableFactorySessionResult(ctx context.Context, sessionID string, request factorysessions.ResultRequest) (factoryapi.FactorySessionResult, error) {
	return fake.result(ctx, sessionID, request)
}

func (fake *legacyHistoryFake) ListDurableFactorySessionDispatches(ctx context.Context, sessionID string, params factoryapi.ListFactorySessionDispatchesParams) (factoryapi.ListFactorySessionDispatchesResponse, error) {
	return fake.dispatches(ctx, sessionID, params)
}

func (fake *legacyHistoryFake) ListDurableFactorySessionArtifacts(ctx context.Context, sessionID string) (factoryapi.ListFactorySessionArtifactsResponse, error) {
	return fake.artifacts(ctx, sessionID)
}

func (fake *legacyHistoryFake) ReadDurableFactorySessionEvents(context.Context, string, factorysessions.EventReconnectRequest) (*interfaces.FactoryEventStream, error) {
	return nil, nil
}

func (fake *legacyHistoryFake) ProbeDurableFactorySessionEvents(context.Context, string, factorysessions.EventReconnectRequest) error {
	return nil
}

func (fake *legacyHistoryFake) GetDurableFactorySessionDispatch(context.Context, string, string) (factoryapi.FactoryDispatch, error) {
	return factoryapi.FactoryDispatch{}, factorysessions.ErrDispatchNotFound
}

func (fake *legacyHistoryFake) GetDurableFactorySessionArtifact(context.Context, string, string) (factoryapi.FactorySessionArtifactDetail, error) {
	return factoryapi.FactorySessionArtifactDetail{}, factorysessions.ErrArtifactNotFound
}
