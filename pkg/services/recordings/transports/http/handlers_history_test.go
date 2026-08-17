package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// TestWriteLegacyError_MapsEverySessionFailureSentinel pins the complete public
// error contract of the standalone durable-execution compatibility path. Each
// sentinel the legacy bridge can return has one status/code/message triple, and
// this table is what makes a later change to where those sentinels are named
// provably behavior preserving.
func TestWriteLegacyError_MapsEverySessionFailureSentinel(t *testing.T) {
	t.Parallel()

	adapter := NewLegacyAdapter(&legacyHistoryFake{}, nil)
	for _, tt := range []struct {
		name        string
		err         error
		wantStatus  int
		wantMessage string
		wantCode    string
		wantFamily  factoryapi.ErrorFamily
	}{
		{
			name: "durable session not found", err: factorysessions.ErrDurableSessionNotFound,
			wantStatus: http.StatusNotFound, wantMessage: "factory session not found",
			wantCode: "NOT_FOUND", wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name: "session not found", err: factorysessions.ErrSessionNotFound,
			wantStatus: http.StatusNotFound, wantMessage: "factory session not found",
			wantCode: "NOT_FOUND", wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name: "dispatch not found", err: factorysessions.ErrDispatchNotFound,
			wantStatus: http.StatusNotFound, wantMessage: "dispatch not found",
			wantCode: "NOT_FOUND", wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name: "artifact not found", err: factorysessions.ErrArtifactNotFound,
			wantStatus: http.StatusNotFound, wantMessage: "factory session artifact not found",
			wantCode: "NOT_FOUND", wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name: "reconnect cursor not found", err: factorysessions.ErrReconnectCursorNotFound,
			wantStatus: http.StatusBadRequest, wantMessage: "invalid event reconnect cursor",
			wantCode: "BAD_REQUEST", wantFamily: factoryapi.ErrorFamilyBadRequest,
		},
		{
			name: "recordings reconnect cursor not found", err: recordings.ErrReconnectCursorNotFound,
			wantStatus: http.StatusBadRequest, wantMessage: "invalid event reconnect cursor",
			wantCode: "BAD_REQUEST", wantFamily: factoryapi.ErrorFamilyBadRequest,
		},
		{
			name:       "wrapped dispatch not found stays classified",
			err:        fmt.Errorf("read durable dispatch: %w", factorysessions.ErrDispatchNotFound),
			wantStatus: http.StatusNotFound, wantMessage: "dispatch not found",
			wantCode: "NOT_FOUND", wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name: "unclassified failure uses the caller fallback", err: errors.New("boom"),
			wantStatus: http.StatusInternalServerError, wantMessage: "failed to read durable history",
			wantCode: "INTERNAL_ERROR", wantFamily: factoryapi.ErrorFamilyInternalServerError,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			if !adapter.writeLegacyError(recorder, tt.err, "failed to read durable history") {
				t.Fatalf("writeLegacyError(%v) = false, want the failure written", tt.err)
			}
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
			}
			if response.Message != tt.wantMessage ||
				string(response.Code) != tt.wantCode ||
				response.Family != tt.wantFamily {
				t.Fatalf("body = %#v, want message=%q code=%s family=%s",
					response, tt.wantMessage, tt.wantCode, tt.wantFamily)
			}
		})
	}
}

// TestWriteLegacyError_IgnoresSuccess pins that a nil failure writes nothing so
// the caller keeps ownership of the success response.
func TestWriteLegacyError_IgnoresSuccess(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	adapter := NewLegacyAdapter(&legacyHistoryFake{}, nil)
	if adapter.writeLegacyError(recorder, nil, "failed to read durable history") {
		t.Fatal("writeLegacyError(nil) = true, want no response written")
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", recorder.Body.String())
	}
}

// TestWriteLegacyStreamHeaders_PublishesRetainedCountAndStreamIdentity pins the
// SSE header contract of the legacy event path, including the retained-event
// count header consumed by dashboard reconnect logic.
func TestWriteLegacyStreamHeaders_PublishesRetainedCountAndStreamIdentity(t *testing.T) {
	t.Parallel()

	sessionID := "dur-sess-legacy-headers-001"
	stream := &interfaces.FactoryEventStream{
		History: []interfaces.FactoryEvent{
			{Id: "legacy-event-1"},
			{Id: "legacy-event-2"},
			{Id: "legacy-event-3"},
		},
		BackendScopeID:      "backend-scope-1",
		LogicalSessionKeyID: "logical-key-1",
		FactorySessionID:    sessionID,
		StreamGenerationID:  "generation-1",
	}

	recorder := httptest.NewRecorder()
	writeLegacyStreamHeaders(recorder, stream, sessionID)

	for header, want := range map[string]string{
		"Content-Type":  "text/event-stream",
		"Cache-Control": "no-cache",
		"Connection":    "keep-alive",
		factorysessions.SessionEventStreamRetainedCountHeader: "3",
		"X-Factory-Session-Backend-Scope-Id":                  "backend-scope-1",
		"X-Factory-Session-Logical-Session-Key-Id":            "logical-key-1",
		SessionEventStreamFactorySessionHeader:                sessionID,
		SessionEventStreamGenerationHeader:                    "generation-1",
	} {
		if got := recorder.Header().Get(header); got != want {
			t.Fatalf("header %s = %q, want %q", header, got, want)
		}
	}
}

// TestWriteLegacyStreamHeaders_OmitsAbsentStreamIdentity pins that blank stream
// identity values are not published as empty headers.
func TestWriteLegacyStreamHeaders_OmitsAbsentStreamIdentity(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	writeLegacyStreamHeaders(recorder, &interfaces.FactoryEventStream{}, "dur-sess-legacy-headers-002")

	if got := recorder.Header().Get(factorysessions.SessionEventStreamRetainedCountHeader); got != "0" {
		t.Fatalf("retained count header = %q, want 0", got)
	}
	for _, header := range []string{
		"X-Factory-Session-Backend-Scope-Id",
		"X-Factory-Session-Logical-Session-Key-Id",
		SessionEventStreamFactorySessionHeader,
		SessionEventStreamGenerationHeader,
	} {
		if _, present := recorder.Header()[http.CanonicalHeaderKey(header)]; present {
			t.Fatalf("header %s present, want omitted for a blank stream identity", header)
		}
	}
}

// historicalResultTestAdapter builds an adapter whose Recordings root replays
// one finalized recording with the supplied world state and dispatches.
func historicalResultTestAdapter(worldState string, dispatches []recordings.HistoricalDispatch) *Adapter {
	artifact := recordings.RecordingArtifactReference("artifact-history-projection-001")
	return NewAdapter(&rootFake{
		queryRecordingStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
				RecordingID: request.RecordingID, Artifact: artifact, State: recordings.RecordingFinalized,
			}}, nil
		},
		queryHistoricalRecording: func(request recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
			return recordings.HistoricalRecordingQueryResult{
				Recording: request.Recording, Status: recordings.RecordingStatusFacts{
					RecordingID: request.Recording.RecordingID, Artifact: artifact, State: recordings.RecordingFinalized,
				}, WorldState: recordings.WorldStateView{Payload: worldState}, Dispatches: dispatches,
			}, nil
		},
	})
}

// TestGetFactorySessionResults_ProjectsFailureAndPrimaryResult pins the full
// decoded result body for a failed session: the failure detail, the partial
// result flag, the primary result content, the requested mode, and the
// include-artifacts echo. The prior suite asserted only three substrings, so
// readFailedHistoricalResult drives the historical results handler over a
// failed-with-partial bracket and returns the decoded public projection, so
// each assertion below states one claim about that projection.
func readFailedHistoricalResult(t *testing.T, sessionID string) factoryapi.FactorySessionResult {
	t.Helper()

	includeArtifacts := true
	mode := factoryapi.FactorySessionResultModePartial
	worldState, err := json.Marshal(interfaces.FactoryWorldState{
		SessionBracket: &interfaces.FactoryWorldSessionBracketState{
			SessionID: sessionID, ResultStatus: "FAILED_WITH_PARTIAL", FinalStatus: "FAILED",
			ResultSummary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "partial result"}},
			ArtifactIDs:   []string{"artifact-1", "artifact-2"},
			FailureDetail: &workerexecution.FailureDetail{
				Reason: workerexecution.WorkFailureTypeTimeout, Message: "timed out",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal world state: %v", err)
	}

	recorder := httptest.NewRecorder()
	historicalResultTestAdapter(string(worldState), nil).GetFactorySessionResults(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/results", nil),
		factoryapi.SessionID(sessionID),
		factoryapi.GetFactorySessionResultsParams{IncludeArtifacts: &includeArtifacts, Mode: &mode},
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d %s, want 200", recorder.Code, recorder.Body.String())
	}

	var response factoryapi.FactorySessionResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
	}
	return response
}

// TestGetFactorySessionResults_ProjectsFailedIdentityAndEchoesRequest pins the
// identity half of the failed projection: the session identity and both status
// fields come from the recorded bracket, while mode and include-artifacts echo
// what the caller asked for.
func TestGetFactorySessionResults_ProjectsFailedIdentityAndEchoesRequest(t *testing.T) {
	t.Parallel()

	sessionID := "dur-sess-history-http-failure-001"
	response := readFailedHistoricalResult(t, sessionID)

	if response.SessionId != sessionID ||
		response.ResultStatus != factoryapi.FactorySessionResultStatus("FAILED_WITH_PARTIAL") {
		t.Fatalf("identity = %q %q, want the failed-with-partial projection", response.SessionId, response.ResultStatus)
	}
	if response.SessionStatus == nil || string(*response.SessionStatus) != "FAILED" {
		t.Fatalf("sessionStatus = %v, want FAILED", response.SessionStatus)
	}
	if response.Mode == nil || *response.Mode != factoryapi.FactorySessionResultModePartial {
		t.Fatalf("mode = %v, want the requested partial mode", response.Mode)
	}
	if response.IncludeArtifacts == nil || !*response.IncludeArtifacts {
		t.Fatalf("includeArtifacts = %v, want the requested true echo", response.IncludeArtifacts)
	}
}

// TestGetFactorySessionResults_ProjectsFailureDetailAndPartialPayload pins the
// payload half: the recorded failure reason and message survive to the public
// shape, and a partial result is reported alongside its artifact ids.
func TestGetFactorySessionResults_ProjectsFailureDetailAndPartialPayload(t *testing.T) {
	t.Parallel()

	response := readFailedHistoricalResult(t, "dur-sess-history-http-failure-002")

	if response.FailureDetail == nil ||
		string(response.FailureDetail.Reason) != string(workerexecution.WorkFailureTypeTimeout) ||
		response.FailureDetail.Message != "timed out" {
		t.Fatalf("failureDetail = %#v, want the timeout failure summary", response.FailureDetail)
	}
	if response.PartialResultAvailable == nil || !*response.PartialResultAvailable {
		t.Fatalf("partialResultAvailable = %v, want true alongside a result summary", response.PartialResultAvailable)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want the single encoded result part", response.PrimaryResult)
	}
	if response.ArtifactIds == nil || len(*response.ArtifactIds) != 2 {
		t.Fatalf("artifactIds = %#v, want both bracket artifact ids", response.ArtifactIds)
	}
}

// TestGetFactorySessionResults_DefaultsToFinalModeWithoutFailure pins the
// success projection: the default final mode, no failure detail, and no
// include-artifacts echo when the caller did not request one.
func TestGetFactorySessionResults_DefaultsToFinalModeWithoutFailure(t *testing.T) {
	t.Parallel()

	sessionID := "dur-sess-history-http-final-001"
	worldState, err := json.Marshal(interfaces.FactoryWorldState{
		SessionBracket: &interfaces.FactoryWorldSessionBracketState{
			SessionID: sessionID, ResultStatus: "FINAL", FinalStatus: "SUCCEEDED",
		},
	})
	if err != nil {
		t.Fatalf("marshal world state: %v", err)
	}

	recorder := httptest.NewRecorder()
	historicalResultTestAdapter(string(worldState), nil).GetFactorySessionResults(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/results", nil),
		factoryapi.SessionID(sessionID), factoryapi.GetFactorySessionResultsParams{},
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d %s, want 200", recorder.Code, recorder.Body.String())
	}

	var response factoryapi.FactorySessionResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
	}
	if response.Mode == nil || *response.Mode != factoryapi.FactorySessionResultModeFinal {
		t.Fatalf("mode = %v, want the default final mode", response.Mode)
	}
	if response.FailureDetail != nil || response.PartialResultAvailable != nil {
		t.Fatalf("failure projection = %#v / %v, want both omitted for a successful session",
			response.FailureDetail, response.PartialResultAvailable)
	}
	if response.IncludeArtifacts != nil {
		t.Fatalf("includeArtifacts = %v, want omitted when not requested", response.IncludeArtifacts)
	}
}

// TestListFactorySessionDispatches_FiltersByRequestedStatus pins the status
// query filter, which selects which historical dispatches reach the response.
func TestListFactorySessionDispatches_FiltersByRequestedStatus(t *testing.T) {
	t.Parallel()

	sessionID := "dur-sess-history-http-filter-001"
	dispatches := []recordings.HistoricalDispatch{
		{ID: "dispatch-completed", Status: recordings.FactoryDispatchStatusCompleted},
		{ID: "dispatch-queued", Status: recordings.FactoryDispatchStatusQueued},
	}
	adapter := historicalResultTestAdapter(`{}`, dispatches)

	completed := factoryapi.FactoryDispatchStatus(recordings.FactoryDispatchStatusCompleted)
	filtered := httptest.NewRecorder()
	adapter.ListFactorySessionDispatches(
		filtered,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/dispatches", nil),
		factoryapi.SessionID(sessionID),
		factoryapi.ListFactorySessionDispatchesParams{Status: &completed},
	)
	if filtered.Code != http.StatusOK {
		t.Fatalf("status = %d %s, want 200", filtered.Code, filtered.Body.String())
	}
	var filteredResponse factoryapi.ListFactorySessionDispatchesResponse
	if err := json.Unmarshal(filtered.Body.Bytes(), &filteredResponse); err != nil {
		t.Fatalf("decode filtered body %q: %v", filtered.Body.String(), err)
	}
	if filteredResponse.SessionId != sessionID || len(filteredResponse.Dispatches) != 1 ||
		filteredResponse.Dispatches[0].Id != "dispatch-completed" {
		t.Fatalf("filtered dispatches = %#v, want only dispatch-completed", filteredResponse.Dispatches)
	}

	unfiltered := httptest.NewRecorder()
	adapter.ListFactorySessionDispatches(
		unfiltered,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/dispatches", nil),
		factoryapi.SessionID(sessionID), factoryapi.ListFactorySessionDispatchesParams{},
	)
	var unfilteredResponse factoryapi.ListFactorySessionDispatchesResponse
	if err := json.Unmarshal(unfiltered.Body.Bytes(), &unfilteredResponse); err != nil {
		t.Fatalf("decode unfiltered body %q: %v", unfiltered.Body.String(), err)
	}
	if len(unfilteredResponse.Dispatches) != 2 {
		t.Fatalf("unfiltered dispatches = %#v, want both historical dispatches", unfilteredResponse.Dispatches)
	}

	missing := factoryapi.FactoryDispatchStatus("INTERRUPTED")
	empty := httptest.NewRecorder()
	adapter.ListFactorySessionDispatches(
		empty,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/dispatches", nil),
		factoryapi.SessionID(sessionID),
		factoryapi.ListFactorySessionDispatchesParams{Status: &missing},
	)
	var emptyResponse factoryapi.ListFactorySessionDispatchesResponse
	if err := json.Unmarshal(empty.Body.Bytes(), &emptyResponse); err != nil {
		t.Fatalf("decode empty body %q: %v", empty.Body.String(), err)
	}
	if emptyResponse.Dispatches == nil || len(emptyResponse.Dispatches) != 0 {
		t.Fatalf("unmatched filter dispatches = %#v, want an empty list", emptyResponse.Dispatches)
	}
}
