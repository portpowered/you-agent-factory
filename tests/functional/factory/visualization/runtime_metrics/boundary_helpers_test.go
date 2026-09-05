package runtime_metrics_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type boundaryRequestLog struct {
	mu       sync.Mutex
	requests []string
}

func (log *boundaryRequestLog) add(request *http.Request) {
	if log == nil || request == nil {
		return
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	log.requests = append(log.requests, request.Method+" "+request.URL.RequestURI())
}

func (log *boundaryRequestLog) snapshot() []string {
	if log == nil {
		return nil
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	return append([]string(nil), log.requests...)
}

type boundaryServer struct {
	server *httptest.Server
	log    *boundaryRequestLog
}

func newBoundaryServer(t *testing.T, handler http.HandlerFunc) boundaryServer {
	t.Helper()
	log := &boundaryRequestLog{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		log.add(request)
		handler(writer, request)
	}))
	t.Cleanup(server.Close)
	return boundaryServer{server: server, log: log}
}

func (server boundaryServer) URL() string {
	if server.server == nil {
		return ""
	}
	return server.server.URL
}

func boundaryInputs(t *testing.T, ctx context.Context, args ...string) *support.CapturedInputs {
	t.Helper()
	if ctx == nil {
		ctx = t.Context()
	}
	inputs := support.FakeInputs(ctx, args)
	home := t.TempDir()
	inputs.Input.Env = []string{"HOME=" + home, "USERPROFILE=" + home}
	inputs.Input.WorkingDirectory = t.TempDir()
	return inputs
}

type boundarySessionFixture struct {
	sessionID     string
	dispatchID    string
	workID        string
	workerSession string
	provider      string
	model         string
	amount        string
	events        []factoryapi.FactoryEvent
}

func newBoundarySessionFixture(t *testing.T, prefix, sessionID, amount string) boundarySessionFixture {
	t.Helper()
	base := time.Date(2026, 9, 5, 21, 0, 0, 0, time.UTC)
	workID := prefix + "-work"
	dispatchID := prefix + "-dispatch"
	workerSessionID := prefix + "-worker-session"
	provider := "codex"
	model := "gpt-5-codex"
	events := []factoryapi.FactoryEvent{
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeSessionStarted, prefix+"-session-start", "", sessionID, 1, base,
			factoryapi.SessionStartedEventPayload{StartedAt: base}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeWorkRequest, prefix+"-work-request", "", sessionID, 2, base.Add(time.Millisecond),
			factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{{WorkId: stringPointer(workID)}}}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchQueued, prefix+"-queued", dispatchID, sessionID, 3, base.Add(10*time.Millisecond),
			factoryapi.DispatchQueuedEventPayload{InputWorkIds: &[]string{workID}, Provider: stringPointer(provider), Model: stringPointer(model)}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchRequest, prefix+"-request", dispatchID, sessionID, 4, base.Add(20*time.Millisecond),
			factoryapi.DispatchRequestEventPayload{TransitionId: "review", Inputs: []factoryapi.DispatchConsumedWorkRef{{WorkId: workID}}}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation, prefix+"-association", dispatchID, sessionID, 5, base.Add(21*time.Millisecond),
			factoryapi.DispatchWorkerSessionAssociationEventPayload{WorkerSessionId: workerSessionID}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchResponse, prefix+"-response", dispatchID, sessionID, 6, base.Add(120*time.Millisecond),
			factoryapi.DispatchResponseEventPayload{Outcome: factoryapi.WorkOutcomeAccepted, DurationMillis: int64Pointer(100)}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeSessionCompleted, prefix+"-session-complete", "", sessionID, 7, base.Add(200*time.Millisecond),
			factoryapi.SessionCompletedEventPayload{CompletedAt: base.Add(200 * time.Millisecond), FinalStatus: factoryapi.FactorySessionDurableLifecycleStatusSucceeded}),
	}
	return boundarySessionFixture{
		sessionID: sessionID, dispatchID: dispatchID, workID: workID,
		workerSession: workerSessionID, provider: provider, model: model,
		amount: amount, events: events,
	}
}

func writeBoundaryMetricsReport(writer http.ResponseWriter, fixture boundarySessionFixture) {
	writeBoundaryJSON(writer, http.StatusOK, map[string]any{
		"cost":      map[string]any{"availability": "UNAVAILABLE"},
		"providers": []any{},
		"scope": map[string]any{
			"kind": "FACTORY_SESSION", "factory_session_id": fixture.sessionID,
		},
		"totals": map[string]any{
			"completed_dispatches": 1,
			"dispatch_latency":     map[string]any{"unit": "milliseconds", "samples": 0, "p50": nil, "p95": nil},
			"failures_by_reason":   map[string]any{},
			"input_tokens":         10, "output_tokens": 5,
			"provider_latency": map[string]any{"unit": "milliseconds", "samples": 0, "p50": nil, "p95": nil},
		},
		"usage_rows": []any{map[string]any{
			"dispatch_id": fixture.dispatchID, "factory_session_id": fixture.sessionID,
			"input_tokens": 10, "output_tokens": 5, "work_id": fixture.workID,
			"worker_session_id": fixture.workerSession, "provider": fixture.provider, "model": fixture.model,
		}},
		"worker_types": []any{}, "workstations": []any{},
	})
}

func writeBoundaryCostsReport(writer http.ResponseWriter, fixture boundarySessionFixture) {
	writeBoundaryJSON(writer, http.StatusOK, map[string]any{
		"coverage": map[string]any{
			"encountered_provider_models": 1, "encountered_rows": 1,
			"priced_provider_models": 1, "priced_rows": 1,
			"unpriced_provider_models": 0, "unpriced_rows": 0,
		},
		"currency": "USD", "factory_sessions": []any{}, "known_cost": fixture.amount,
		"line_items": []any{map[string]any{
			"cached_input_tokens": 3, "dispatch_id": fixture.dispatchID,
			"factory_session_id": fixture.sessionID, "input_tokens": 10,
			"model": fixture.model, "output_tokens": 5, "priced_amount": fixture.amount,
			"price_source": "BUILT_IN", "provider": fixture.provider,
			"reasoning_output_tokens": 2, "status": "PRICED", "work_id": fixture.workID,
			"worker_session_id": fixture.workerSession,
		}},
		"provider_models": []any{},
		"scope":           map[string]any{"factory_session_id": fixture.sessionID, "kind": "FACTORY_SESSION"},
		"status":          "PRICED",
		"token_totals": map[string]any{
			"cached_input_tokens": 3, "input_tokens": 10, "output_tokens": 5,
			"reasoning_output_tokens": 2, "total_tokens": 15,
		},
		"unpriced_dispatch_count": 0, "unpriced_pairs": []any{},
		"work_items": []any{}, "worker_sessions": []any{},
	})
}

func writeBoundaryEvents(writer http.ResponseWriter, events []factoryapi.FactoryEvent) {
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set(factorysessions.SessionEventStreamRetainedCountHeader, strconv.Itoa(len(events)))
	for _, event := range events {
		encoded, err := json.Marshal(event)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(writer, "data: %s\n\n", encoded)
	}
}

func writeBoundaryJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeBoundaryError(writer http.ResponseWriter, status int, code factoryapi.ErrorResponseCode, family factoryapi.ErrorFamily, message string) {
	writeBoundaryJSON(writer, status, factoryapi.ErrorResponse{Code: code, Family: family, Message: message})
}

type boundarySessionDocument struct {
	FactorySessionID string `json:"factory_session_id"`
	Attempts         []struct {
		DispatchID *string  `json:"dispatch_id"`
		WorkIDs    []string `json:"work_ids"`
	} `json:"attempts"`
	Cost *struct {
		KnownCost *string `json:"known_cost"`
		Status    string  `json:"status"`
	} `json:"cost"`
}

func assertBoundaryRequestLog(t *testing.T, log *boundaryRequestLog, want ...string) {
	t.Helper()
	got := log.snapshot()
	if len(got) != len(want) {
		t.Fatalf("requests = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("requests = %#v, want %#v", got, want)
		}
	}
}
