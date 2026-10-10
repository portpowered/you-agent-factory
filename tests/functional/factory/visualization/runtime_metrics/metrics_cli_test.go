package runtime_metrics_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestMetricsInvalidGroupThroughRootProcessPreservesCodedDiagnostic proves
// the customer CLI process keeps the metrics-owned code and safe message at
// the production central-diagnostics boundary.
func TestMetricsInvalidGroupThroughRootProcessPreservesCodedDiagnostic(t *testing.T) {
	t.Parallel()

	process := runtimeMetricsProcess(t)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "metrics", "--group-by", "region",
	})
	inputs.Input.Env = []string{"HOME=" + t.TempDir(), "USERPROFILE=" + t.TempDir()}

	err := process.Execute(inputs.Input)
	if err == nil {
		t.Fatal("Process.Execute(metrics invalid group) error = nil, want coded failure")
	}
	if inputs.Stdout() != "" {
		t.Fatalf("metrics stdout = %q, want empty", inputs.Stdout())
	}
	assertMetricsDiagnostic(t, inputs.Stderr(), "METRICS_INVALID_GROUP_BY", `invalid --group-by "region": choose workstation, worker, or provider`)
}

// TestMetricsSuccessThroughRootProcessRendersQueryCostAvailability proves
// both public presenters consume the query result returned by the canonical
// process rather than relying on a presenter-local cost constant.
func TestMetricsSuccessThroughRootProcessRendersQueryCostAvailability(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	factoryDirectory := support.ScaffoldSingleStepFactory(t, "metrics-cost-availability")
	workingDirectory := t.TempDir()
	environment := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDirectory,
		WorkingDirectory:          workingDirectory,
		WaitForServiceModeRuntime: true,
		Env:                       environment,
	})

	process := runtimeMetricsProcess(t)

	human := support.FakeInputs(t.Context(), []string{"you", "--server", server.URL(), "metrics"})
	human.Input.Env = environment
	human.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(human.Input); err != nil {
		t.Fatalf("Process.Execute(metrics human) error = %v\nstdout:\n%s\nstderr:\n%s", err, human.Stdout(), human.Stderr())
	}
	if !strings.Contains(human.Stdout(), "Cost: unavailable\n") || human.Stderr() != "" {
		t.Fatalf("human metrics output = %q, stderr = %q", human.Stdout(), human.Stderr())
	}

	machine := support.FakeInputs(t.Context(), []string{"you", "--json", "--server", server.URL(), "metrics"})
	machine.Input.Env = environment
	machine.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(machine.Input); err != nil {
		t.Fatalf("Process.Execute(metrics JSON) error = %v\nstdout:\n%s\nstderr:\n%s", err, machine.Stdout(), machine.Stderr())
	}
	var document struct {
		Cost struct {
			Availability string `json:"availability"`
		} `json:"cost"`
	}
	if err := json.Unmarshal([]byte(machine.Stdout()), &document); err != nil {
		t.Fatalf("decode metrics JSON: %v\n%s", err, machine.Stdout())
	}
	if document.Cost.Availability != "unavailable" || machine.Stderr() != "" {
		t.Fatalf("JSON metrics cost = %#v, stderr = %q", document.Cost, machine.Stderr())
	}
}

func assertMetricsDiagnostic(t *testing.T, output, wantCode, wantMessage string) {
	t.Helper()
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || strings.Contains(trimmed, "\n") {
		t.Fatalf("metrics diagnostic = %q, want one JSON line", output)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
		t.Fatalf("decode metrics diagnostic: %v\n%s", err, output)
	}
	if response.Code != factoryapi.ErrorResponseCode(wantCode) || response.Message != wantMessage {
		t.Fatalf("metrics diagnostic = %#v, want code %q and message %q", response, wantCode, wantMessage)
	}
}

// TestMetricsSessionThroughRootProcessReadsOnlyTheSelectedRemoteReplay proves
// the new command through the reusable root process and a local real HTTP
// server. The empty temporary HOME has no metrics artifacts, so a successful
// report must come from the selected /metrics scope and retained event lane.
func TestMetricsSessionThroughRootProcessReadsOnlyTheSelectedRemoteReplay(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 9, 5, 21, 0, 0, 0, time.UTC)
	workID := "functional-work-1"
	events := []factoryapi.FactoryEvent{
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeSessionStarted, "functional-session-start", "", "session-functional", 1, base,
			factoryapi.SessionStartedEventPayload{StartedAt: base}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeWorkRequest, "functional-work-request", "", "session-functional", 2, base.Add(time.Millisecond),
			factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{{WorkId: &workID}}}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchQueued, "functional-queued", "functional-dispatch", "session-functional", 3, base.Add(10*time.Millisecond),
			factoryapi.DispatchQueuedEventPayload{InputWorkIds: &[]string{workID}}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchRequest, "functional-request", "functional-dispatch", "session-functional", 4, base.Add(20*time.Millisecond),
			factoryapi.DispatchRequestEventPayload{Inputs: []factoryapi.DispatchConsumedWorkRef{{WorkId: workID}}}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchResponse, "functional-response", "functional-dispatch", "session-functional", 5, base.Add(120*time.Millisecond),
			factoryapi.DispatchResponseEventPayload{Outcome: factoryapi.WorkOutcomeAccepted}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeSessionCompleted, "functional-session-complete", "", "session-functional", 6, base.Add(200*time.Millisecond),
			factoryapi.SessionCompletedEventPayload{CompletedAt: base.Add(200 * time.Millisecond), FinalStatus: factoryapi.FactorySessionDurableLifecycleStatusSucceeded}),
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/metrics":
			if request.URL.Query().Get("session_id") != "session-functional" {
				http.Error(writer, "wrong session scope", http.StatusBadRequest)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"cost":{"availability":"UNAVAILABLE"},"providers":[],"scope":{"kind":"FACTORY_SESSION","factory_session_id":"session-functional"},"totals":{"completed_dispatches":1,"dispatch_latency":{"unit":"milliseconds","samples":0,"p50":null,"p95":null},"failures_by_reason":{},"input_tokens":0,"output_tokens":0,"provider_latency":{"unit":"milliseconds","samples":0,"p50":null,"p95":null}},"usage_rows":[],"worker_types":[],"workstations":[]}`))
		case "/factory-sessions/session-functional/events":
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.Header().Set(factorysessions.SessionEventStreamRetainedCountHeader, strconv.Itoa(len(events)))
			for _, event := range events {
				encoded, err := json.Marshal(event)
				if err != nil {
					http.Error(writer, err.Error(), http.StatusInternalServerError)
					return
				}
				_, _ = writer.Write([]byte("data: " + string(encoded) + "\n\n"))
			}
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "--server", server.URL, "metrics", "session", "session-functional",
	})
	inputs.Input.Env = []string{"HOME=" + t.TempDir(), "USERPROFILE=" + t.TempDir()}
	if err := runtimeMetricsProcess(t).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(metrics session) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var document struct {
		FactorySessionID      string `json:"factory_session_id"`
		DispatchAttempts      int    `json:"dispatch_attempts"`
		DistinctWorkItems     int    `json:"distinct_work_items"`
		SummedQueueTimeMillis *int64 `json:"summed_queue_time_ms"`
		QueueDuration         struct {
			Samples int `json:"samples"`
		} `json:"queue_duration"`
	}
	if err := json.Unmarshal([]byte(inputs.Stdout()), &document); err != nil {
		t.Fatalf("decode metrics session JSON: %v\n%s", err, inputs.Stdout())
	}
	if document.FactorySessionID != "session-functional" || document.DispatchAttempts != 1 || document.DistinctWorkItems != 1 || document.QueueDuration.Samples != 1 {
		t.Fatalf("metrics session document = %#v, want selected session with one attempt/work/sample", document)
	}
	if document.SummedQueueTimeMillis == nil || *document.SummedQueueTimeMillis != 10 {
		t.Fatalf("summed queue time = %v, want 10ms", document.SummedQueueTimeMillis)
	}
	if inputs.Stderr() != "" {
		t.Fatalf("metrics session stderr = %q, want empty", inputs.Stderr())
	}
}

// TestMetricsSessionCostLensThroughRootProcessComposesCostsAndDetail proves
// the additive cost/detail journey through the reusable root process and a
// local real HTTP server. The response keeps the existing Costs JSON as the
// nested cost document and labels the authored worker as unavailable because
// the retained canonical facts do not prove that name.
func TestMetricsSessionCostLensThroughRootProcessComposesCostsAndDetail(t *testing.T) {
	t.Parallel()
	fixture := newBoundarySessionFixture(t, "functional-cost", "session-cost", "0.0042")
	server := newBoundaryServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/metrics":
			writeBoundaryMetricsReport(writer, fixture)
		case "/metrics/costs":
			writeBoundaryCostsReport(writer, fixture)
		case "/factory-sessions/session-cost/events":
			writeBoundaryEvents(writer, fixture.events)
		default:
			http.NotFound(writer, request)
		}
	})

	inputs := boundaryInputs(t, t.Context(),
		"you", "--json", "--server", server.URL(), "metrics", "session", fixture.sessionID,
		"--lens", "cost", "--by-worker", "--by-dispatch",
	)
	if err := runtimeMetricsProcess(t).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(metrics session cost) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var document struct {
		Cost     generatedclient.CostsReport `json:"cost"`
		ByWorker []struct {
			Worker          string  `json:"worker"`
			WorkerSessionID *string `json:"worker_session_id"`
			Provider        *string `json:"provider"`
			Model           *string `json:"model"`
		} `json:"by_worker"`
		ByDispatch []struct {
			DispatchID       *string `json:"dispatch_id"`
			DispatchIdentity string  `json:"dispatch_identity"`
			Workstation      *string `json:"workstation"`
			Provider         *string `json:"provider"`
			Model            *string `json:"model"`
			WorkerIdentity   string  `json:"worker_identity"`
		} `json:"by_dispatch"`
	}
	if err := json.Unmarshal([]byte(inputs.Stdout()), &document); err != nil {
		t.Fatalf("decode metrics session cost JSON: %v\n%s", err, inputs.Stdout())
	}
	if document.Cost.KnownCost == nil || *document.Cost.KnownCost != fixture.amount || document.Cost.Status != "PRICED" || document.Cost.Currency != "USD" {
		t.Fatalf("cost = %#v, want exact priced Costs response", document.Cost)
	}
	if document.Cost.TokenTotals.CachedInputTokens == nil || *document.Cost.TokenTotals.CachedInputTokens != 3 || document.Cost.TokenTotals.ReasoningOutputTokens == nil || *document.Cost.TokenTotals.ReasoningOutputTokens != 2 || document.Cost.Coverage.PricedRows != 1 {
		t.Fatalf("cost token/coverage facts = %#v, want cached/reasoning/provenance coverage", document.Cost)
	}
	if len(document.Cost.LineItems) != 1 || document.Cost.LineItems[0].PriceSource == nil || *document.Cost.LineItems[0].PriceSource != generatedclient.CostsLineItemPriceSource("BUILT_IN") {
		t.Fatalf("cost provenance = %#v, want BUILT_IN line-item provenance", document.Cost.LineItems)
	}
	if len(document.ByWorker) != 1 || document.ByWorker[0].Worker != "unavailable" || document.ByWorker[0].WorkerSessionID == nil || *document.ByWorker[0].WorkerSessionID != fixture.workerSession || document.ByWorker[0].Provider == nil || *document.ByWorker[0].Provider != fixture.provider || document.ByWorker[0].Model == nil || *document.ByWorker[0].Model != fixture.model {
		t.Fatalf("worker detail = %#v, want canonical Worker Session/provider/model and unavailable Worker", document.ByWorker)
	}
	if len(document.ByDispatch) != 1 || document.ByDispatch[0].DispatchID == nil || *document.ByDispatch[0].DispatchID != fixture.dispatchID || document.ByDispatch[0].DispatchIdentity != "canonical" || document.ByDispatch[0].Workstation == nil || *document.ByDispatch[0].Workstation != "review" || document.ByDispatch[0].WorkerIdentity != "unavailable" {
		t.Fatalf("dispatch detail = %#v, want canonical dispatch/workstation and unavailable Worker", document.ByDispatch)
	}
	if inputs.Stderr() != "" {
		t.Fatalf("metrics session stderr = %q, want no diagnostics", inputs.Stderr())
	}
	assertBoundaryRequestLog(t, server.log,
		"GET /metrics?session_id="+fixture.sessionID,
		"GET /factory-sessions/"+fixture.sessionID+"/events",
		"GET /metrics/costs?session_id="+fixture.sessionID,
	)
}

func TestMetricsSessionByWorkerAggregatesAuthoredWorkerAcrossSessions(t *testing.T) {
	t.Parallel()
	const sessionID = "worker-detail-session"
	base := time.Date(2026, 9, 5, 21, 0, 0, 0, time.UTC)
	dispatchOne, dispatchTwo := "worker-detail-dispatch-1", "worker-detail-dispatch-2"
	workOne, workTwo := "worker-detail-work-1", "worker-detail-work-2"
	workerSessionOne, workerSessionTwo := "worker-detail-session-1", "worker-detail-session-2"
	retryOf := dispatchOne
	events := []factoryapi.FactoryEvent{
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeSessionStarted, "worker-detail-start", "", sessionID, 1, base,
			factoryapi.SessionStartedEventPayload{StartedAt: base}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeWorkRequest, "worker-detail-work-request", "", sessionID, 2, base.Add(time.Millisecond),
			factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{
				{WorkId: stringPointer(workOne)}, {WorkId: stringPointer(workTwo)},
			}}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchQueued, "worker-detail-queued-1", dispatchOne, sessionID, 3, base.Add(10*time.Millisecond),
			factoryapi.DispatchQueuedEventPayload{InputWorkIds: &[]string{workOne}, Provider: stringPointer("codex"), Model: stringPointer("gpt-5-codex")}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchRequest, "worker-detail-request-1", dispatchOne, sessionID, 4, base.Add(20*time.Millisecond),
			factoryapi.DispatchRequestEventPayload{TransitionId: "review", Inputs: []factoryapi.DispatchConsumedWorkRef{{WorkId: workOne}}}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeModelRequest, "worker-detail-model-1", dispatchOne, sessionID, 5, base.Add(21*time.Millisecond),
			factoryapi.ModelRequestEventPayload{Attempt: 1, Model: "gpt-5-codex", ModelRequestId: "model-request-1", Operation: "TEXT", ProviderLocality: "CLOUD", Worker: "reviewer"}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation, "worker-detail-association-1", dispatchOne, sessionID, 6, base.Add(22*time.Millisecond),
			factoryapi.DispatchWorkerSessionAssociationEventPayload{WorkerSessionId: workerSessionOne}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchResponse, "worker-detail-response-1", dispatchOne, sessionID, 7, base.Add(40*time.Millisecond),
			factoryapi.DispatchResponseEventPayload{Outcome: factoryapi.WorkOutcomeAccepted, DurationMillis: int64Pointer(18)}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchQueued, "worker-detail-queued-2", dispatchTwo, sessionID, 8, base.Add(50*time.Millisecond),
			factoryapi.DispatchQueuedEventPayload{InputWorkIds: &[]string{workTwo}, Provider: stringPointer("codex"), Model: stringPointer("gpt-5-codex"), RetryOfDispatchId: &retryOf}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchRequest, "worker-detail-request-2", dispatchTwo, sessionID, 9, base.Add(60*time.Millisecond),
			factoryapi.DispatchRequestEventPayload{TransitionId: "review", Inputs: []factoryapi.DispatchConsumedWorkRef{{WorkId: workTwo}}}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeModelRequest, "worker-detail-model-2", dispatchTwo, sessionID, 10, base.Add(61*time.Millisecond),
			factoryapi.ModelRequestEventPayload{Attempt: 1, Model: "gpt-5-codex", ModelRequestId: "model-request-2", Operation: "TEXT", ProviderLocality: "CLOUD", Worker: "reviewer"}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation, "worker-detail-association-2", dispatchTwo, sessionID, 11, base.Add(62*time.Millisecond),
			factoryapi.DispatchWorkerSessionAssociationEventPayload{WorkerSessionId: workerSessionTwo}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchResponse, "worker-detail-response-2", dispatchTwo, sessionID, 12, base.Add(90*time.Millisecond),
			factoryapi.DispatchResponseEventPayload{Outcome: factoryapi.WorkOutcomeFailed, DurationMillis: int64Pointer(28)}),
		functionalMetricsSessionEvent(t, factoryapi.FactoryEventTypeSessionCompleted, "worker-detail-complete", "", sessionID, 13, base.Add(100*time.Millisecond),
			factoryapi.SessionCompletedEventPayload{CompletedAt: base.Add(100 * time.Millisecond), FinalStatus: factoryapi.FactorySessionDurableLifecycleStatusFailed}),
	}
	server := newBoundaryServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/metrics":
			writeBoundaryJSON(writer, http.StatusOK, map[string]any{
				"cost": map[string]any{"availability": "UNAVAILABLE"}, "providers": []any{},
				"scope":  map[string]any{"kind": "FACTORY_SESSION", "factory_session_id": sessionID},
				"totals": map[string]any{"completed_dispatches": 2, "dispatch_latency": map[string]any{"unit": "milliseconds", "samples": 0, "p50": nil, "p95": nil}, "failures_by_reason": map[string]any{}, "input_tokens": 0, "output_tokens": 0, "provider_latency": map[string]any{"unit": "milliseconds", "samples": 0, "p50": nil, "p95": nil}},
				"usage_rows": []any{
					map[string]any{"dispatch_id": dispatchOne, "factory_session_id": sessionID, "work_id": workOne, "worker_session_id": workerSessionOne, "provider": "codex", "model": "gpt-5-codex"},
					map[string]any{"dispatch_id": dispatchTwo, "factory_session_id": sessionID, "work_id": workTwo, "worker_session_id": workerSessionTwo, "provider": "codex", "model": "gpt-5-codex"},
				},
				"worker_types": []any{}, "workstations": []any{},
			})
		case "/factory-sessions/" + sessionID + "/events":
			writeBoundaryEvents(writer, events)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})
	inputs := boundaryInputs(t, t.Context(), "you", "--json", "--server", server.URL(), "metrics", "session", sessionID, "--by-worker")
	if err := runtimeMetricsProcess(t).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(metrics session by worker) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var document struct {
		ByWorker []struct {
			Worker           string   `json:"worker"`
			WorkerSessionID  *string  `json:"worker_session_id"`
			WorkerSessionIDs []string `json:"worker_session_ids"`
			Sessions         int      `json:"sessions"`
			Attempts         int      `json:"attempts"`
		} `json:"by_worker"`
	}
	if err := json.Unmarshal([]byte(inputs.Stdout()), &document); err != nil {
		t.Fatalf("decode worker detail JSON: %v\n%s", err, inputs.Stdout())
	}
	if len(document.ByWorker) != 1 {
		t.Fatalf("worker detail rows = %#v, want one authored Worker row", document.ByWorker)
	}
	row := document.ByWorker[0]
	if row.Worker != "reviewer" || row.Sessions != 2 || row.Attempts != 2 || row.WorkerSessionID != nil ||
		len(row.WorkerSessionIDs) != 2 || row.WorkerSessionIDs[0] != workerSessionOne || row.WorkerSessionIDs[1] != workerSessionTwo {
		t.Fatalf("worker detail row = %#v, want reviewer with two distinct sessions and two attempts", row)
	}
	if inputs.Stderr() != "" {
		t.Fatalf("worker detail stderr = %q, want empty", inputs.Stderr())
	}
}

func functionalMetricsSessionEvent(
	t *testing.T,
	eventType factoryapi.FactoryEventType,
	id, dispatchID, sessionID string,
	sequence int,
	eventTime time.Time,
	payload any,
) factoryapi.FactoryEvent {
	t.Helper()
	var eventPayload factoryapi.FactoryEvent_Payload
	var err error
	switch typed := payload.(type) {
	case factoryapi.SessionStartedEventPayload:
		err = eventPayload.FromSessionStartedEventPayload(typed)
	case factoryapi.SessionCompletedEventPayload:
		err = eventPayload.FromSessionCompletedEventPayload(typed)
	case factoryapi.WorkRequestEventPayload:
		err = eventPayload.FromWorkRequestEventPayload(typed)
	case factoryapi.DispatchQueuedEventPayload:
		err = eventPayload.FromDispatchQueuedEventPayload(typed)
	case factoryapi.DispatchRequestEventPayload:
		err = eventPayload.FromDispatchRequestEventPayload(typed)
	case factoryapi.DispatchWorkerSessionAssociationEventPayload:
		err = eventPayload.FromDispatchWorkerSessionAssociationEventPayload(typed)
	case factoryapi.ModelRequestEventPayload:
		err = eventPayload.FromModelRequestEventPayload(typed)
	case factoryapi.DispatchResponseEventPayload:
		err = eventPayload.FromDispatchResponseEventPayload(typed)
	default:
		t.Fatalf("unsupported functional metrics event payload %T", payload)
	}
	if err != nil {
		t.Fatalf("encode functional metrics event payload: %v", err)
	}
	eventContext := factoryapi.FactoryEventContext{
		EventTime: eventTime, Sequence: sequence, SessionId: stringPointer(sessionID),
	}
	if dispatchID != "" {
		eventContext.DispatchId = stringPointer(dispatchID)
	}
	return factoryapi.FactoryEvent{
		Id: id, Type: eventType, SchemaVersion: factoryapi.AgentFactoryEventV1,
		Context: eventContext, Payload: eventPayload,
	}
}

func stringPointer(value string) *string { return &value }

func int64Pointer(value int64) *int64 { return &value }

// M3-S: local retained history is read through the customer CLI, including
// compressed backups and an interrupted final write. Exact sums detect both
// dropped and duplicated records; the component reader owns ordering proof.
func TestMetricsRetainedHistoryThroughRootProcess(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	root := platformmetrics.RuntimeMetricsRoot(home)
	session := uuid.NewString()
	server := startRetainedMetricsHost(t, home, session)
	record := func(dispatch string, value int) map[string]any {
		return map[string]any{"metric_name": runtimeProviderInputTokens, "value": value,
			"session_id": session, "dispatch_id": dispatch, "provider": "codex"}
	}
	writeRuntimeMetricsArtifact(t, filepath.Join(root, "120000.000000000-runtime-metrics-history.log"), false,
		[]map[string]any{record("active", 3)}, `{"private-tail":`)
	writeRuntimeMetricsArtifact(t, filepath.Join(root, "120000.000000000-runtime-metrics-history-2026-08-20T12-01-00.000.log"), false,
		[]map[string]any{record("plain", 5)})
	writeRuntimeMetricsArtifact(t, filepath.Join(root, "120000.000000000-runtime-metrics-history-2026-08-20T12-02-00.000.log.gz"), true,
		[]map[string]any{record("gzip", 7)})
	writeRuntimeMetricsArtifact(t, filepath.Join(root, "130000.000000000-runtime-metrics-peer.log"), false,
		[]map[string]any{{"metric_name": runtimeProviderInputTokens, "value": 100,
			"session_id": "other", "dispatch_id": "foreign", "provider": "codex"}})
	assertRetainedMetricsTokens(t, home, server, session, 15)
}

// M3-F: a bad complete record or gzip backup fails atomically after a valid
// artifact. Public diagnostics fail safely; component tests own artifact/line context.
func TestMetricsRetainedHistoryFailuresThroughRootProcess(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, suffix, content string
	}{
		{"complete record", ".log", "{\"private-secret\":}\n"},
		{"gzip backup", "-2026-08-20T12-02-00.000.log.gz", "private-secret-invalid-gzip"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			root := platformmetrics.RuntimeMetricsRoot(home)
			server := startRetainedMetricsHost(t, home, uuid.NewString())
			writeRuntimeMetricsArtifact(t, filepath.Join(root, "110000.000000000-runtime-metrics-valid.log"), false,
				[]map[string]any{{"metric_name": runtimeProviderInputTokens, "value": 3}})
			path := filepath.Join(root, "120000.000000000-runtime-metrics-invalid"+test.suffix)
			writeFunctionalFile(t, path, test.content)
			inputs := retainedMetricsInputs(t, home, server)
			err := runtimeMetricsProcess(t).Execute(inputs.Input)
			assertBoundaryCodedFailure(t, err, inputs, "METRICS_QUERY_FAILED")
			assertMetricsDiagnostic(t, inputs.Stderr(), "METRICS_QUERY_FAILED", "query runtime metrics: server returned HTTP 500")
			if strings.Contains(inputs.Stderr(), "private-secret") {
				t.Fatalf("metrics diagnostic disclosed artifact contents: %s", inputs.Stderr())
			}
			assertFunctionalFileContents(t, path, test.content)
		})
	}
}

// Damaged backups and complete non-object records must fail the customer read
// atomically, even after a valid artifact. Repairing that same selected file
// restores the report without duplicating earlier facts or including a peer.
func TestMetricsDamagedHistoryFailsClosedAndRecoversThroughRootProcess(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		compressed bool
		damage     func([]byte) []byte
	}{
		{name: "gzip checksum", compressed: true, damage: func(data []byte) []byte {
			data[len(data)-8] ^= 1
			return data
		}},
		{name: "gzip truncated trailer", compressed: true, damage: func(data []byte) []byte {
			return data[:len(data)-4]
		}},
		{name: "complete null record", damage: func(data []byte) []byte {
			return append(data, []byte("null\n")...)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			root := platformmetrics.RuntimeMetricsRoot(home)
			session := uuid.NewString()
			server := startRetainedMetricsHost(t, home, session)
			record := func(id string, value int) map[string]any {
				return map[string]any{"metric_name": runtimeProviderInputTokens, "value": value,
					"session_id": id, "dispatch_id": "selected-history", "provider": "codex",
					"private_note": "selected-private-secret"}
			}
			prefix := filepath.Join(root, "110000.000000000-runtime-metrics-prefix.log")
			writeRuntimeMetricsArtifact(t, prefix, false, []map[string]any{record(session, 3)})
			suffix := ".log"
			if test.compressed {
				suffix = "-2026-08-20T12-02-00.000.log.gz"
			}
			path := filepath.Join(root, "120000.000000000-runtime-metrics-selected"+suffix)
			writeRuntimeMetricsArtifact(t, path, test.compressed,
				[]map[string]any{record(session, 7), record(uuid.NewString(), 100)})
			unknown := filepath.Join(root, "customer-note.txt")
			writeFunctionalFile(t, unknown, "customer content")
			assertRetainedMetricsTokens(t, home, server, session, 10)
			healthy, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			damaged := test.damage(append([]byte(nil), healthy...))
			writeFunctionalFile(t, path, string(damaged))
			inputs := retainedMetricsInputs(t, home, server, "--session", session)
			err = runtimeMetricsProcess(t).Execute(inputs.Input)
			assertBoundaryCodedFailure(t, err, inputs, "METRICS_QUERY_FAILED")
			assertMetricsDiagnostic(t, inputs.Stderr(), "METRICS_QUERY_FAILED", "query runtime metrics: server returned HTTP 500")
			if strings.Contains(inputs.Stderr(), "selected-private-secret") || strings.Contains(inputs.Stderr(), path) {
				t.Fatalf("metrics diagnostic disclosed artifact details: %s", inputs.Stderr())
			}
			assertFunctionalFileContents(t, path, string(damaged))
			assertFunctionalFileContents(t, unknown, "customer content")
			writeFunctionalFile(t, path, string(healthy))
			assertRetainedMetricsTokens(t, home, server, session, 10)
			assertFunctionalFileContents(t, path, string(healthy))
			assertFunctionalFileContents(t, unknown, "customer content")
		})
	}
}

func retainedMetricsInputs(t *testing.T, home, server string, selectors ...string) *support.CapturedInputs {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), append([]string{"you", "--json", "--server", server, "metrics"}, selectors...))
	inputs.Input.Env = []string{"HOME=" + home, "USERPROFILE=" + home}
	inputs.Input.WorkingDirectory = home
	return inputs
}

func assertRetainedMetricsTokens(t *testing.T, home, server, session string, want int) {
	t.Helper()
	inputs := retainedMetricsInputs(t, home, server, "--session", session)
	if err := runtimeMetricsProcess(t).Execute(inputs.Input); err != nil {
		t.Fatalf("metrics query: %v; stderr=%s", err, inputs.Stderr())
	}
	var document struct {
		Scope struct {
			FactorySessionID string `json:"factory_session_id"`
		} `json:"scope"`
		Totals struct {
			InputTokens int `json:"input_tokens"`
		} `json:"totals"`
	}
	if err := json.Unmarshal([]byte(inputs.Stdout()), &document); err != nil {
		t.Fatal(err)
	}
	if document.Scope.FactorySessionID != session || document.Totals.InputTokens != want || inputs.Stderr() != "" {
		t.Fatalf("metrics = %+v, stderr=%q; want session %q with %d tokens", document, inputs.Stderr(), session, want)
	}
}

// One immutable server edge routes each invocation to its own listener; the
// process graph is shared while sessions, homes and transport cleanup are owned
// by parallel scenarios. Empty factories keep provider facts out of history sums.
func startRetainedMetricsHost(t *testing.T, home, session string) string {
	t.Helper()
	server := support.NewProcessAPIServer()
	ctx := context.WithValue(t.Context(), retainedMetricsServerKey{}, server)
	factory := support.ScaffoldSingleStepFactory(t, "retained-metrics")
	inputs := support.FakeInputs(ctx, []string{"you", "run", "--dir", factory,
		"--session", session, "--continuously", "--with-server", "--server", "http://127.0.0.1:1",
		"--quiet", "--no-record"})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = home
	support.StartProcessCommand(t, runtimeMetricsProcess(t), inputs.Input)
	return server.WaitForURL(t)
}
