package runtime_metrics_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestMetricsInvalidGroupThroughRootProcessPreservesCodedDiagnostic proves
// the customer CLI process keeps the metrics-owned code and safe message at
// the production central-diagnostics boundary.
func TestMetricsInvalidGroupThroughRootProcessPreservesCodedDiagnostic(t *testing.T) {
	t.Parallel()

	process := runtimeMetricsCLIProcess
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

	process := runtimeMetricsCLIProcess

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
	if err := runtimeMetricsCLIProcess.Execute(inputs.Input); err != nil {
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
