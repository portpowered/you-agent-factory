package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type metricsClientStub struct {
	response *generatedclient.GetMetricsClientResponse
	err      error
	params   *generatedclient.GetMetricsParams
}

func (stub *metricsClientStub) GetMetricsWithResponse(
	_ context.Context,
	params *generatedclient.GetMetricsParams,
	_ ...generatedclient.RequestEditorFn,
) (*generatedclient.GetMetricsClientResponse, error) {
	stub.params = params
	return stub.response, stub.err
}

func TestMetricsOperationUsesGeneratedClientAndRendersCompleteReport(t *testing.T) {
	client := &metricsClientStub{response: &generatedclient.GetMetricsClientResponse{
		JSON200: &generatedclient.MetricsReport{
			Cost: generatedclient.MetricsCost{Availability: "UNAVAILABLE"},
			Scope: generatedclient.MetricsScope{
				Kind: "FACTORY_SESSION",
			},
			Totals: generatedclient.MetricsAggregate{
				InputTokens:         12,
				OutputTokens:        8,
				CompletedDispatches: 3,
				FailuresByReason:    map[string]float64{"timeout": 1},
				DispatchLatency:     generatedclient.MetricsDuration{Unit: "milliseconds", Samples: 1},
				ProviderLatency:     generatedclient.MetricsDuration{Unit: "milliseconds", Samples: 1},
			},
			Providers: []generatedclient.MetricsBreakdown{{
				Key: "provider-a",
				Aggregate: generatedclient.MetricsAggregate{
					InputTokens: 12,
				},
			}},
		}},
	}
	operation := NewOperation(func(server string) (Client, error) {
		if server != "http://metrics.test" {
			t.Fatalf("client server = %q, want http://metrics.test", server)
		}
		return client, nil
	})
	var output bytes.Buffer
	err := RunMetricsOperation(context.Background(), operation, MetricsConfig{
		Server:    " http://metrics.test ",
		GroupBy:   "provider",
		SessionID: "public-live-id",
		JSON:      true,
		Output:    &output,
	})
	if err != nil {
		t.Fatalf("RunMetricsOperation() error = %v", err)
	}
	if client.params == nil || client.params.SessionId == nil || *client.params.SessionId != "public-live-id" {
		t.Fatalf("generated client params = %#v, want session_id public-live-id", client.params)
	}
	for _, want := range []string{`"kind":"factory_session"`, `"group_by":"provider"`, `"provider-a"`, `"input_tokens":12`} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("metrics output missing %q:\n%s", want, output.String())
		}
	}
}

func TestMetricsOperationNormalizesAcceptedGroupBeforeRendering(t *testing.T) {
	client := &metricsClientStub{response: &generatedclient.GetMetricsClientResponse{
		JSON200: &generatedclient.MetricsReport{
			Cost:      generatedclient.MetricsCost{Availability: "UNAVAILABLE"},
			Providers: []generatedclient.MetricsBreakdown{{Key: "provider-a"}},
		},
	}}
	operation := NewOperation(func(string) (Client, error) { return client, nil })

	var output bytes.Buffer
	if err := RunMetricsOperation(context.Background(), operation, MetricsConfig{
		Server:  "http://metrics.test",
		GroupBy: " PROVIDER ",
		JSON:    false,
		Output:  &output,
	}); err != nil {
		t.Fatalf("RunMetricsOperation() error = %v", err)
	}
	if !strings.Contains(output.String(), "Breakdown by provider: 1 rows") ||
		!strings.Contains(output.String(), "provider-a:") {
		t.Fatalf("normalized provider output = %q", output.String())
	}
}

func TestMetricsOperationMapsTypedHTTPFailuresWithoutPartialOutput(t *testing.T) {
	tests := []struct {
		name     string
		response *generatedclient.GetMetricsClientResponse
		wantCode string
		wantFam  string
	}{
		{
			name: "session not found",
			response: &generatedclient.GetMetricsClientResponse{
				JSON404: &generatedclient.NotFound{Message: "Factory Session missing-live-id was not found; use `you session list --scope live`"},
			},
			wantCode: MetricsSessionNotFoundCode,
			wantFam:  "NOT_FOUND",
		},
		{
			name: "scope unavailable",
			response: &generatedclient.GetMetricsClientResponse{
				JSON503: &generatedclient.MetricsSessionScopeUnavailable{Message: "Factory Session known-live-id has no retained metrics scope; use `you session list --scope live`"},
			},
			wantCode: MetricsScopeUnavailableCode,
			wantFam:  "INTERNAL_SERVER_ERROR",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operation := NewOperation(func(string) (Client, error) {
				return &metricsClientStub{response: test.response}, nil
			})
			var output bytes.Buffer
			err := RunMetricsOperation(context.Background(), operation, MetricsConfig{
				Server: "http://metrics.test", GroupBy: "workstation", Output: &output,
			})
			if err == nil {
				t.Fatal("RunMetricsOperation() error = nil, want typed HTTP failure")
			}
			var metricsErr *MetricsError
			if !errors.As(err, &metricsErr) || metricsErr.CLIErrorCode() != test.wantCode || string(metricsErr.CLIErrorFamily()) != test.wantFam {
				t.Fatalf("error = %#v, want code %q family %q", err, test.wantCode, test.wantFam)
			}
			if !strings.Contains(metricsErr.CLIErrorMessage(), "you session list --scope live") {
				t.Fatalf("error message = %q, want live-session guidance", metricsErr.CLIErrorMessage())
			}
			if output.Len() != 0 {
				t.Fatalf("failed metrics request wrote partial output %q", output.String())
			}
		})
	}
}

func TestMetricsReportFromAPIPreservesMissingLatencyAsEmptySamples(t *testing.T) {
	result := metricsReportFromAPI(generatedclient.MetricsReport{
		Totals: generatedclient.MetricsAggregate{
			DispatchLatency: generatedclient.MetricsDuration{Unit: "milliseconds"},
			ProviderLatency: generatedclient.MetricsDuration{Unit: "milliseconds"},
		},
	})
	if result.Totals.DispatchDuration == nil || result.Totals.DispatchDuration.Samples != 0 || result.Totals.DispatchDuration.P50 != nil {
		t.Fatalf("dispatch duration = %#v, want explicit empty duration", result.Totals.DispatchDuration)
	}
	if result.Totals.ProviderDuration == nil || result.Totals.ProviderDuration.Samples != 0 || result.Totals.ProviderDuration.P95 != nil {
		t.Fatalf("provider duration = %#v, want explicit empty duration", result.Totals.ProviderDuration)
	}
	if result.Cost.Availability != factoryvisualization.RuntimeMetricsCostAvailability("") {
		t.Fatalf("cost availability = %q, want preserved empty value", result.Cost.Availability)
	}
}

type metricsSessionEventOperationStub struct {
	events []factoryapi.FactoryEvent
	err    error
}

func (stub metricsSessionEventOperationStub) OpenFactorySessionEvents(
	context.Context,
	SessionEventRequest,
) (SessionEventStream, error) {
	if stub.err != nil {
		return nil, stub.err
	}
	return &metricsSessionEventStreamStub{events: stub.events}, nil
}

type metricsSessionEventStreamStub struct {
	events []factoryapi.FactoryEvent
	index  int
}

func (stream *metricsSessionEventStreamStub) Next(context.Context) (factoryapi.FactoryEvent, error) {
	if stream.index >= len(stream.events) {
		return factoryapi.FactoryEvent{}, io.EOF
	}
	event := stream.events[stream.index]
	stream.index++
	return event, nil
}

func (stream *metricsSessionEventStreamStub) Close() error { return nil }

func TestMetricsSessionOperationReducesOneCanonicalAttemptPerDispatch(t *testing.T) {
	base := time.Date(2026, 9, 5, 21, 0, 0, 0, time.UTC)
	workID := "work-1"
	events := []factoryapi.FactoryEvent{
		metricsSessionEvent(t, factoryapi.FactoryEventTypeSessionStarted, "session-start", "", base,
			factoryapi.SessionStartedEventPayload{StartedAt: base}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeWorkRequest, "work-request", "", base.Add(time.Millisecond),
			factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{{WorkId: &workID}}}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchQueued, "queued-1", "dispatch-1", base.Add(10*time.Millisecond),
			factoryapi.DispatchQueuedEventPayload{InputWorkIds: &[]string{workID}}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchRequest, "request-1", "dispatch-1", base.Add(20*time.Millisecond),
			factoryapi.DispatchRequestEventPayload{Inputs: []factoryapi.DispatchConsumedWorkRef{{WorkId: workID}}}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation, "worker-1", "dispatch-1", base.Add(21*time.Millisecond),
			factoryapi.DispatchWorkerSessionAssociationEventPayload{WorkerSessionId: "worker-session-1"}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchResponse, "response-1", "dispatch-1", base.Add(120*time.Millisecond),
			factoryapi.DispatchResponseEventPayload{Outcome: factoryapi.WorkOutcomeAccepted, DurationMillis: int64Pointer(100)}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchResponse, "response-1", "dispatch-1", base.Add(120*time.Millisecond),
			factoryapi.DispatchResponseEventPayload{Outcome: factoryapi.WorkOutcomeAccepted, DurationMillis: int64Pointer(100)}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchQueued, "queued-2", "dispatch-2", base.Add(130*time.Millisecond),
			factoryapi.DispatchQueuedEventPayload{InputWorkIds: &[]string{workID}, RetryOfDispatchId: stringPointer("dispatch-1")}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchRequest, "request-2", "dispatch-2", base.Add(150*time.Millisecond),
			factoryapi.DispatchRequestEventPayload{Inputs: []factoryapi.DispatchConsumedWorkRef{{WorkId: workID}}}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchResponse, "response-2", "dispatch-2", base.Add(250*time.Millisecond),
			factoryapi.DispatchResponseEventPayload{Outcome: factoryapi.WorkOutcomeFailed, DurationMillis: int64Pointer(100)}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeSessionCompleted, "session-complete", "", base.Add(300*time.Millisecond),
			factoryapi.SessionCompletedEventPayload{CompletedAt: base.Add(300 * time.Millisecond), FinalStatus: factoryapi.FactorySessionDurableLifecycleStatusSucceeded}),
	}
	client := &metricsClientStub{response: &generatedclient.GetMetricsClientResponse{JSON200: &generatedclient.MetricsReport{
		Scope: generatedclient.MetricsScope{Kind: "FACTORY_SESSION", FactorySessionId: stringPointer("session-1")},
	}}}
	operation := NewOperation(func(string) (Client, error) { return client, nil })
	var output bytes.Buffer
	err := RunMetricsOperation(context.Background(), operation, MetricsConfig{
		Server: "http://metrics.test", SessionID: "session-1", JSON: true, Output: &output,
		SessionReport: true, SessionEvents: metricsSessionEventOperationStub{events: events},
	})
	if err != nil {
		t.Fatalf("RunMetricsOperation() error = %v", err)
	}
	var document metricsSessionDocument
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("decode session report: %v\n%s", err, output.String())
	}
	assertMetricsSessionTotals(t, document)
	assertMetricsSessionDurations(t, document)
	assertMetricsSessionAttempts(t, document)
	assertMetricsSessionOutputSafe(t, output.String())
}

func assertMetricsSessionTotals(t *testing.T, document metricsSessionDocument) {
	t.Helper()
	if document.DispatchAttempts != 2 {
		t.Fatalf("dispatch attempts = %d, want 2", document.DispatchAttempts)
	}
	if document.DistinctWorkItems != 1 || document.WorkerSessions != 1 || document.Retries != 1 {
		t.Fatalf("session totals = %#v, want one work, one Worker Session, one retry", document)
	}
	if document.AttemptOutcomes.Accepted != 1 || document.AttemptOutcomes.Failed != 1 {
		t.Fatalf("outcomes = %#v, want one accepted and one failed", document.AttemptOutcomes)
	}
}

func assertMetricsSessionDurations(t *testing.T, document metricsSessionDocument) {
	t.Helper()
	if document.QueueDuration.Samples != 2 || document.QueueDuration.Excluded != 0 {
		t.Fatalf("queue duration = %#v, want two samples and no exclusions", document.QueueDuration)
	}
	if document.QueueDuration.P50 == nil || *document.QueueDuration.P50 != 10 {
		t.Fatalf("queue p50 = %v, want 10ms", document.QueueDuration.P50)
	}
	if document.ExecutionDuration.Samples != 2 {
		t.Fatalf("execution duration = %#v, want two samples", document.ExecutionDuration)
	}
	if document.SummedExecutionTimeMillis == nil || *document.SummedExecutionTimeMillis != 200 {
		t.Fatalf("execution sum = %v, want 200ms", document.SummedExecutionTimeMillis)
	}
}

func assertMetricsSessionAttempts(t *testing.T, document metricsSessionDocument) {
	t.Helper()
	if len(document.Attempts) != 2 {
		t.Fatalf("attempts = %#v, want two deterministic identities", document.Attempts)
	}
	if document.Attempts[0].DispatchID == nil || *document.Attempts[0].DispatchID != "dispatch-1" {
		t.Fatalf("first attempt = %#v, want dispatch-1", document.Attempts[0])
	}
}

func assertMetricsSessionOutputSafe(t *testing.T, output string) {
	t.Helper()
	if strings.Contains(output, "prompt") {
		t.Fatalf("session report exposed prompt content: %s", output)
	}
	if strings.Contains(output, "output") {
		t.Fatalf("session report exposed output content: %s", output)
	}
}

func TestMetricsSessionOperationKeepsIncompleteDurationsNullAndAtomic(t *testing.T) {
	base := time.Date(2026, 9, 5, 21, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		metricsSessionEvent(t, factoryapi.FactoryEventTypeSessionStarted, "session-start", "", base,
			factoryapi.SessionStartedEventPayload{StartedAt: base}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchQueued, "queued", "queued-dispatch", base.Add(time.Second),
			factoryapi.DispatchQueuedEventPayload{}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchRequest, "running", "running-dispatch", base.Add(2*time.Second),
			factoryapi.DispatchRequestEventPayload{}),
	}
	client := &metricsClientStub{response: &generatedclient.GetMetricsClientResponse{JSON200: &generatedclient.MetricsReport{
		Scope: generatedclient.MetricsScope{Kind: "FACTORY_SESSION", FactorySessionId: stringPointer("session-1")},
	}}}
	operation := NewOperation(func(string) (Client, error) { return client, nil })
	var output bytes.Buffer
	err := RunMetricsOperation(context.Background(), operation, MetricsConfig{
		Server: "http://metrics.test", SessionID: "session-1", JSON: true, Output: &output,
		SessionReport: true, SessionEvents: metricsSessionEventOperationStub{events: events},
	})
	if err != nil {
		t.Fatalf("RunMetricsOperation() error = %v", err)
	}
	var document metricsSessionDocument
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("decode session report: %v\n%s", err, output.String())
	}
	if document.QueueDuration.Samples != 0 || document.QueueDuration.Excluded != 2 || document.ExecutionDuration.Samples != 0 || document.ExecutionDuration.Excluded != 2 {
		t.Fatalf("incomplete duration accounting = queue=%#v execution=%#v", document.QueueDuration, document.ExecutionDuration)
	}
	if document.Incomplete.Queued != 1 || document.Incomplete.Running != 1 {
		t.Fatalf("incomplete attempts = %#v, want one queued and one running", document.Incomplete)
	}
	if document.Attempts[0].QueueDurationMillis != nil || document.Attempts[1].ExecutionDurationMillis != nil {
		t.Fatalf("incomplete attempts invented durations: %#v", document.Attempts)
	}
}

func TestMetricsSessionOperationMapsReplayFailureWithoutPartialOutput(t *testing.T) {
	client := &metricsClientStub{response: &generatedclient.GetMetricsClientResponse{JSON200: &generatedclient.MetricsReport{
		Scope: generatedclient.MetricsScope{Kind: "FACTORY_SESSION", FactorySessionId: stringPointer("session-1")},
	}}}
	operation := NewOperation(func(string) (Client, error) { return client, nil })
	wantCause := errors.New("replay transport secret")
	var output bytes.Buffer
	err := RunMetricsOperation(context.Background(), operation, MetricsConfig{
		Server: "http://metrics.test", SessionID: "session-1", Output: &output,
		SessionReport: true, SessionEvents: metricsSessionEventOperationStub{err: wantCause},
	})
	var metricsErr *MetricsError
	if !errors.As(err, &metricsErr) || metricsErr.CLIErrorCode() != MetricsSessionEventsFailedCode || !errors.Is(err, wantCause) {
		t.Fatalf("error = %#v, want coded replay failure preserving cause", err)
	}
	if strings.Contains(err.Error(), "replay transport secret") || output.Len() != 0 {
		t.Fatalf("replay failure leaked data or wrote output: err=%v output=%q", err, output.String())
	}
}

func metricsSessionEvent(
	t *testing.T,
	eventType factoryapi.FactoryEventType,
	id, dispatchID string,
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
	case factoryapi.DispatchResponseEventPayload:
		err = eventPayload.FromDispatchResponseEventPayload(typed)
	default:
		t.Fatalf("unsupported metrics session payload %T", payload)
	}
	if err != nil {
		t.Fatalf("encode metrics session event payload: %v", err)
	}
	context := factoryapi.FactoryEventContext{EventTime: eventTime, Sequence: 1}
	if dispatchID != "" {
		context.DispatchId = stringPointer(dispatchID)
	}
	return factoryapi.FactoryEvent{
		Id: id, Type: eventType, SchemaVersion: factoryapi.AgentFactoryEventV1,
		Context: context, Payload: eventPayload,
	}
}

func stringPointer(value string) *string { return &value }

func int64Pointer(value int64) *int64 { return &value }
