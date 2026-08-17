package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
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

func TestHistoricalResultAndDispatchMapsSelectedWorldState(t *testing.T) {
	t.Parallel()

	sessionID := "dur-sess-history-http-rich-001"
	artifact := recordings.RecordingArtifactReference("artifact-history-http-rich-001")
	includeArtifacts := true
	mode := factoryapi.FactorySessionResultModePartial
	worldState, err := json.Marshal(interfaces.FactoryWorldState{
		SessionBracket: &interfaces.FactoryWorldSessionBracketState{
			SessionID: sessionID, ResultStatus: "FAILED_WITH_PARTIAL", FinalStatus: "FAILED",
			ResultSummary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "partial result"}},
			ArtifactIDs:   []string{"artifact-1"},
			FailureDetail: &workerexecution.FailureDetail{Reason: workerexecution.WorkFailureTypeTimeout, Message: "timed out"},
		},
	})
	if err != nil {
		t.Fatalf("marshal world state: %v", err)
	}
	dispatches := []recordings.HistoricalDispatch{
		{ID: "dispatch-js", Status: recordings.FactoryDispatchStatusCompleted, DispatchKind: recordings.FactoryDispatchKindJavaScriptScript},
		{ID: "dispatch-petri", Status: recordings.FactoryDispatchStatusQueued},
	}
	adapter := NewAdapter(&rootFake{
		queryRecordingStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
				RecordingID: request.RecordingID, Artifact: artifact, State: recordings.RecordingFailed,
			}}, nil
		},
		queryHistoricalRecording: func(request recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
			return recordings.HistoricalRecordingQueryResult{
				Recording: request.Recording, Status: recordings.RecordingStatusFacts{
					RecordingID: request.Recording.RecordingID, Artifact: artifact, State: recordings.RecordingFinalized,
				}, WorldState: recordings.WorldStateView{Payload: string(worldState)}, Dispatches: dispatches,
			}, nil
		},
	})

	result := httptest.NewRecorder()
	adapter.GetFactorySessionResults(
		result,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/results", nil),
		factoryapi.SessionID(sessionID), factoryapi.GetFactorySessionResultsParams{IncludeArtifacts: &includeArtifacts, Mode: &mode},
	)
	resultBody := result.Body.String()
	if result.Code != http.StatusOK || !strings.Contains(resultBody, `"resultStatus":"FAILED_WITH_PARTIAL"`) ||
		!strings.Contains(resultBody, `"sessionStatus":"FAILED"`) || !strings.Contains(resultBody, `"artifact-1"`) {
		t.Fatalf("result response = %d %s, want selected bracket projection", result.Code, resultBody)
	}

	list := httptest.NewRecorder()
	adapter.ListFactorySessionDispatches(
		list,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/dispatches", nil),
		factoryapi.SessionID(sessionID), factoryapi.ListFactorySessionDispatchesParams{},
	)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"id":"dispatch-petri"`) {
		t.Fatalf("dispatch list = %d %s, want default Petri kind", list.Code, list.Body.String())
	}

	detail := httptest.NewRecorder()
	adapter.GetFactorySessionDispatch(
		detail,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/dispatches/dispatch-js", nil),
		factoryapi.SessionID(sessionID), factoryapi.DispatchID("dispatch-js"),
	)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"orchestratorKind":"JAVASCRIPT"`) {
		t.Fatalf("dispatch detail = %d %s, want JavaScript orchestrator projection", detail.Code, detail.Body.String())
	}

	failedFallback := NewAdapter(&rootFake{
		queryRecordingStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
				RecordingID: request.RecordingID, Artifact: artifact, State: recordings.RecordingFinalized,
			}}, nil
		},
		queryHistoricalRecording: func(request recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
			return recordings.HistoricalRecordingQueryResult{Recording: request.Recording, Status: recordings.RecordingStatusFacts{
				RecordingID: request.Recording.RecordingID, State: recordings.RecordingFailed,
			}, WorldState: recordings.WorldStateView{Payload: `{}`}}, nil
		},
	})
	fallback := httptest.NewRecorder()
	failedFallback.GetFactorySessionResults(
		fallback,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/results", nil),
		factoryapi.SessionID(sessionID), factoryapi.GetFactorySessionResultsParams{},
	)
	if fallback.Code != http.StatusOK || !strings.Contains(fallback.Body.String(), `"resultStatus":"FAILED_WITH_PARTIAL"`) ||
		!strings.Contains(fallback.Body.String(), `"sessionStatus":"FAILED"`) {
		t.Fatalf("failed fallback result = %d %s, want failed status defaults", fallback.Code, fallback.Body.String())
	}
}

func TestLegacyHistoryDispatchAndArtifactDetailsRemainAvailable(t *testing.T) {
	t.Parallel()

	sessionID := "dur-sess-legacy-details-001"
	adapter := NewLegacyAdapter(&legacyHistoryFake{
		dispatch: func(_ context.Context, gotSessionID, dispatchID string) (factoryapi.FactoryDispatch, error) {
			if gotSessionID != sessionID || dispatchID != "dispatch-legacy-001" {
				return factoryapi.FactoryDispatch{}, errors.New("unexpected legacy dispatch request")
			}
			return factoryapi.FactoryDispatch{Id: dispatchID, SessionId: sessionID, Status: factoryapi.FactoryDispatchStatusCOMPLETED}, nil
		},
		artifact: func(_ context.Context, gotSessionID, artifactID string) (factoryapi.FactorySessionArtifactDetail, error) {
			if gotSessionID != sessionID || artifactID != "artifact-legacy-001" {
				return factoryapi.FactorySessionArtifactDetail{}, errors.New("unexpected legacy artifact request")
			}
			return factoryapi.FactorySessionArtifactDetail{Id: artifactID, SessionId: sessionID}, nil
		},
	}, nil)

	dispatch := httptest.NewRecorder()
	adapter.GetFactorySessionDispatch(
		dispatch,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/dispatches/dispatch-legacy-001", nil),
		factoryapi.SessionID(sessionID), factoryapi.DispatchID("dispatch-legacy-001"),
	)
	if dispatch.Code != http.StatusOK || !strings.Contains(dispatch.Body.String(), `"id":"dispatch-legacy-001"`) {
		t.Fatalf("legacy dispatch = %d %s, want stable detail", dispatch.Code, dispatch.Body.String())
	}

	artifact := httptest.NewRecorder()
	adapter.GetFactorySessionArtifact(
		artifact,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/artifacts/artifact-legacy-001", nil),
		factoryapi.SessionID(sessionID), factoryapi.ArtifactID("artifact-legacy-001"),
	)
	if artifact.Code != http.StatusOK || !strings.Contains(artifact.Body.String(), `"id":"artifact-legacy-001"`) {
		t.Fatalf("legacy artifact = %d %s, want stable detail", artifact.Code, artifact.Body.String())
	}
}

func TestLegacyHistoryRecoveryProbeMapsReadyUnknownAndStaleOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "ready", want: "STREAM_READY"},
		{name: "unknown", err: factorysessions.ErrSessionNotFound, want: "UNKNOWN_SESSION"},
		{name: "stale", err: factorysessions.ErrReconnectCursorNotFound, want: "CURSOR_STALE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got factorysessions.EventReconnectRequest
			adapter := NewLegacyAdapter(&legacyHistoryFake{
				probe: func(_ context.Context, _ string, request factorysessions.EventReconnectRequest) error {
					got = request
					return test.err
				},
			}, nil)
			request := httptest.NewRequest(http.MethodGet, "/factory-sessions/dur-sess-legacy-probe-001/events", nil)
			request.Header.Set("Accept", "application/json")
			recorder := httptest.NewRecorder()
			afterSequence := factoryapi.AfterSequence(7)
			adapter.GetEventsBySessionId(
				recorder, request, factoryapi.SessionID("dur-sess-legacy-probe-001"),
				factoryapi.GetEventsBySessionIdParams{AfterSequence: &afterSequence},
			)
			if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"outcome":"`+test.want+`"`) {
				t.Fatalf("probe = %d %s, want %s", recorder.Code, recorder.Body.String(), test.want)
			}
			if got.AfterSequence == nil || *got.AfterSequence != 7 {
				t.Fatalf("legacy reconnect request = %#v, want afterSequence=7", got)
			}
		})
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
	events     func(context.Context, string, factorysessions.EventReconnectRequest) (*interfaces.FactoryEventStream, error)
	probe      func(context.Context, string, factorysessions.EventReconnectRequest) error
	dispatches func(context.Context, string, factoryapi.ListFactorySessionDispatchesParams) (factoryapi.ListFactorySessionDispatchesResponse, error)
	artifacts  func(context.Context, string) (factoryapi.ListFactorySessionArtifactsResponse, error)
	dispatch   func(context.Context, string, string) (factoryapi.FactoryDispatch, error)
	artifact   func(context.Context, string, string) (factoryapi.FactorySessionArtifactDetail, error)
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

func (fake *legacyHistoryFake) ReadDurableFactorySessionEvents(ctx context.Context, sessionID string, request factorysessions.EventReconnectRequest) (*interfaces.FactoryEventStream, error) {
	if fake.events != nil {
		return fake.events(ctx, sessionID, request)
	}
	return nil, nil
}

func (fake *legacyHistoryFake) ProbeDurableFactorySessionEvents(ctx context.Context, sessionID string, request factorysessions.EventReconnectRequest) error {
	if fake.probe != nil {
		return fake.probe(ctx, sessionID, request)
	}
	return nil
}

func (fake *legacyHistoryFake) GetDurableFactorySessionDispatch(ctx context.Context, sessionID, dispatchID string) (factoryapi.FactoryDispatch, error) {
	if fake.dispatch != nil {
		return fake.dispatch(ctx, sessionID, dispatchID)
	}
	return factoryapi.FactoryDispatch{}, factorysessions.ErrDispatchNotFound
}

func (fake *legacyHistoryFake) GetDurableFactorySessionArtifact(ctx context.Context, sessionID, artifactID string) (factoryapi.FactorySessionArtifactDetail, error) {
	if fake.artifact != nil {
		return fake.artifact(ctx, sessionID, artifactID)
	}
	return factoryapi.FactorySessionArtifactDetail{}, factorysessions.ErrArtifactNotFound
}
