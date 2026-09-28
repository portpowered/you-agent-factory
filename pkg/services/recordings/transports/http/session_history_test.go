package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type sessionHistoryRootFake struct {
	factorysessions.Service
	readResult      func(factorysessions.SessionResultReadRequest) (factorysessions.SessionResultReadResult, error)
	queryDispatches func(factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error)
	queryEvents     func(factorysessions.SessionEventQueryRequest) (factorysessions.EventReadResult, error)
	probeEvents     func(factorysessions.SessionEventQueryRequest) error
	inspectDispatch func(factorysessions.SessionDispatchInspectRequest) (factorysessions.DispatchDetail, error)
	queryArtifacts  func(factorysessions.SessionArtifactQueryRequest) (factorysessions.ListArtifactsResult, error)
	inspectArtifact func(factorysessions.SessionArtifactInspectRequest) (factorysessions.ArtifactDetail, error)
}

func (f *sessionHistoryRootFake) ReadResult(_ context.Context, request factorysessions.SessionResultReadRequest) (factorysessions.SessionResultReadResult, error) {
	return f.readResult(request)
}
func (f *sessionHistoryRootFake) QueryDispatches(_ context.Context, request factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
	return f.queryDispatches(request)
}
func (f *sessionHistoryRootFake) QueryEvents(_ context.Context, request factorysessions.SessionEventQueryRequest) (factorysessions.EventReadResult, error) {
	return f.queryEvents(request)
}
func (f *sessionHistoryRootFake) ProbeEvents(_ context.Context, request factorysessions.SessionEventQueryRequest) error {
	return f.probeEvents(request)
}
func (f *sessionHistoryRootFake) InspectDispatch(_ context.Context, request factorysessions.SessionDispatchInspectRequest) (factorysessions.DispatchDetail, error) {
	return f.inspectDispatch(request)
}
func (f *sessionHistoryRootFake) QueryArtifacts(_ context.Context, request factorysessions.SessionArtifactQueryRequest) (factorysessions.ListArtifactsResult, error) {
	return f.queryArtifacts(request)
}
func (f *sessionHistoryRootFake) InspectArtifact(_ context.Context, request factorysessions.SessionArtifactInspectRequest) (factorysessions.ArtifactDetail, error) {
	return f.inspectArtifact(request)
}

func TestActiveDurableHistoryUsesCanonicalSessionCommands(t *testing.T) {
	t.Parallel()
	const sessionID = "dur-sess-direct-1"
	event, err := json.Marshal(interfaces.FactoryEvent{Id: "event-1", Type: interfaces.FactoryEventTypeWorkRequest})
	if err != nil {
		t.Fatal(err)
	}
	root := &sessionHistoryRootFake{
		readResult: func(request factorysessions.SessionResultReadRequest) (factorysessions.SessionResultReadResult, error) {
			if request.SessionID != sessionID || request.Mode != factorysessions.SessionOperationModeDurable {
				t.Fatalf("result request = %#v", request)
			}
			return factorysessions.SessionResultReadResult{Durable: &factorysessions.SessionDurableResult{SessionID: sessionID, Status: factorysessions.ResultStatus("NOT_READY")}}, nil
		},
		queryDispatches: func(request factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
			if request.SessionID != sessionID {
				t.Fatalf("dispatch query = %#v", request)
			}
			return factorysessions.ListDispatchesResult{SessionID: sessionID}, nil
		},
		queryEvents: func(request factorysessions.SessionEventQueryRequest) (factorysessions.EventReadResult, error) {
			if request.SessionID != sessionID {
				t.Fatalf("event query = %#v", request)
			}
			return factorysessions.EventReadResult{SessionID: sessionID, Events: []json.RawMessage{event}}, nil
		},
		probeEvents: func(request factorysessions.SessionEventQueryRequest) error {
			if request.SessionID != sessionID {
				t.Fatalf("event probe = %#v", request)
			}
			return nil
		},
		inspectDispatch: func(request factorysessions.SessionDispatchInspectRequest) (factorysessions.DispatchDetail, error) {
			if request.SessionID != sessionID || request.DispatchID != "dispatch-1" {
				t.Fatalf("dispatch inspect = %#v", request)
			}
			return factorysessions.DispatchDetail{SessionID: sessionID, DispatchSummary: factorysessions.DispatchSummary{ID: "dispatch-1"}}, nil
		},
		queryArtifacts: func(request factorysessions.SessionArtifactQueryRequest) (factorysessions.ListArtifactsResult, error) {
			if request.SessionID != sessionID {
				t.Fatalf("artifact query = %#v", request)
			}
			return factorysessions.ListArtifactsResult{SessionID: sessionID}, nil
		},
		inspectArtifact: func(request factorysessions.SessionArtifactInspectRequest) (factorysessions.ArtifactDetail, error) {
			if request.SessionID != sessionID || request.ArtifactID != "artifact-1" {
				t.Fatalf("artifact inspect = %#v", request)
			}
			return factorysessions.ArtifactDetail{SessionID: sessionID, ArtifactSummary: factorysessions.ArtifactSummary{ID: "artifact-1"}}, nil
		},
	}
	adapter := NewAdapterWithSessions(&rootFake{
		queryRecordingStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			if request.RecordingID != recordings.RecordingID(sessionID) {
				t.Fatalf("recording query = %#v", request)
			}
			return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{RecordingID: request.RecordingID}}, nil
		},
	}, root, root)
	assertRoute := func(label, path, want string, serve func(http.ResponseWriter, *http.Request)) {
		t.Helper()
		recorder := httptest.NewRecorder()
		serve(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("%s response = %d %s, want %q", label, recorder.Code, recorder.Body.String(), want)
		}
	}
	base := "/factory-sessions/" + sessionID
	assertRoute("result", base+"/results", `"resultStatus":"NOT_READY"`, func(w http.ResponseWriter, r *http.Request) {
		adapter.GetFactorySessionResults(w, r, sessionID, factoryapi.GetFactorySessionResultsParams{})
	})
	assertRoute("dispatches", base+"/dispatches", `"sessionId":"`+sessionID+`"`, func(w http.ResponseWriter, r *http.Request) {
		adapter.ListFactorySessionDispatches(w, r, sessionID, factoryapi.ListFactorySessionDispatchesParams{})
	})
	assertRoute("dispatch", base+"/dispatches/dispatch-1", `"id":"dispatch-1"`, func(w http.ResponseWriter, r *http.Request) {
		adapter.GetFactorySessionDispatch(w, r, sessionID, "dispatch-1")
	})
	assertRoute("artifacts", base+"/artifacts", `"sessionId":"`+sessionID+`"`, func(w http.ResponseWriter, r *http.Request) { adapter.ListFactorySessionArtifacts(w, r, sessionID) })
	assertRoute("artifact", base+"/artifacts/artifact-1", `"id":"artifact-1"`, func(w http.ResponseWriter, r *http.Request) {
		adapter.GetFactorySessionArtifact(w, r, sessionID, "artifact-1")
	})
	assertRoute("events", base+"/events", `"id":"event-1"`, func(w http.ResponseWriter, r *http.Request) {
		adapter.GetEventsBySessionId(w, r, sessionID, factoryapi.GetEventsBySessionIdParams{})
	})
	probe := httptest.NewRequest(http.MethodGet, base+"/events", nil)
	probe.Header.Set("Accept", "application/json")
	recorder := httptest.NewRecorder()
	adapter.GetEventsBySessionId(recorder, probe, sessionID, factoryapi.GetEventsBySessionIdParams{})
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "STREAM_READY") {
		t.Fatalf("probe response = %d %s", recorder.Code, recorder.Body.String())
	}
}
