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

func TestMetricsSessionCostLensPreservesCostsAndCanonicalDetailIdentities(t *testing.T) {
	fixture := newMetricsSessionCostLensFixture(t)
	client := &metricsClientStub{response: &generatedclient.GetMetricsClientResponse{JSON200: &generatedclient.MetricsReport{
		Scope: generatedclient.MetricsScope{Kind: "FACTORY_SESSION", FactorySessionId: stringPointer("session-1")},
		UsageRows: []generatedclient.MetricsUsageRow{
			{DispatchId: stringPointer("dispatch-1"), WorkId: stringPointer(fixture.workID), WorkerSessionId: stringPointer("worker-session-1"), Provider: &fixture.provider, Model: &fixture.model},
		},
	}}}
	operation := NewOperation(func(string) (Client, error) { return client, nil })
	var gotCostRequest MetricsCostReportRequest
	var output bytes.Buffer
	err := RunMetricsOperation(context.Background(), operation, MetricsConfig{
		Server: "http://metrics.test", SessionID: "session-1", JSON: true, Output: &output,
		SessionReport: true, SessionEvents: metricsSessionEventOperationStub{events: fixture.events},
		SessionLens: "cost", SessionByWorker: true, SessionByDispatch: true,
		CostReport: func(_ context.Context, request MetricsCostReportRequest) (generatedclient.CostsReport, error) {
			gotCostRequest = request
			return fixture.costReport, nil
		},
	})
	if err != nil {
		t.Fatalf("RunMetricsOperation() error = %v", err)
	}
	assertMetricsSessionCostLensRequest(t, gotCostRequest)
	var document metricsSessionDocument
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("decode session cost report: %v\n%s", err, output.String())
	}
	assertMetricsSessionCostLensReport(t, document, fixture)
}

type metricsSessionCostLensFixture struct {
	events     []factoryapi.FactoryEvent
	costReport generatedclient.CostsReport
	workID     string
	provider   string
	model      string
	amount     string
}

func newMetricsSessionCostLensFixture(t *testing.T) metricsSessionCostLensFixture {
	t.Helper()
	base := time.Date(2026, 9, 5, 21, 0, 0, 0, time.UTC)
	workID := "work-1"
	provider := "codex"
	model := "gpt-5-codex"
	workerName := "reviewer"
	workstationID := "workstation-review"
	events := []factoryapi.FactoryEvent{
		metricsSessionEvent(t, factoryapi.FactoryEventTypeSessionStarted, "session-start", "", base,
			factoryapi.SessionStartedEventPayload{StartedAt: base}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "initial-structure", "", base.Add(time.Millisecond),
			factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{
				Workstations: &[]factoryapi.Workstation{{Id: &workstationID, Name: "review", Worker: &workerName}},
			}}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeWorkRequest, "work-request", "", base.Add(time.Millisecond),
			factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{{WorkId: &workID}}}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchQueued, "queued-1", "dispatch-1", base.Add(10*time.Millisecond),
			factoryapi.DispatchQueuedEventPayload{InputWorkIds: &[]string{workID}, Provider: &provider, Model: &model}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchRequest, "request-1", "dispatch-1", base.Add(20*time.Millisecond),
			factoryapi.DispatchRequestEventPayload{TransitionId: workstationID, Inputs: []factoryapi.DispatchConsumedWorkRef{{WorkId: workID}}}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeModelRequest, "model-request-1", "dispatch-1", base.Add(22*time.Millisecond),
			factoryapi.ModelRequestEventPayload{Attempt: 1, ModelRequestId: "model-request-1", Model: model, Operation: "TEXT", ProviderLocality: "CLOUD", Worker: workerName}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation, "worker-1", "dispatch-1", base.Add(21*time.Millisecond),
			factoryapi.DispatchWorkerSessionAssociationEventPayload{WorkerSessionId: "worker-session-1"}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchResponse, "response-1", "dispatch-1", base.Add(120*time.Millisecond),
			factoryapi.DispatchResponseEventPayload{Outcome: factoryapi.WorkOutcomeAccepted, DurationMillis: int64Pointer(100)}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchQueued, "queued-2", "dispatch-2", base.Add(130*time.Millisecond),
			factoryapi.DispatchQueuedEventPayload{InputWorkIds: &[]string{workID}, RetryOfDispatchId: stringPointer("dispatch-1")}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchResponse, "response-2", "dispatch-2", base.Add(230*time.Millisecond),
			factoryapi.DispatchResponseEventPayload{Outcome: factoryapi.WorkOutcomeFailed, DurationMillis: int64Pointer(100)}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeSessionCompleted, "session-complete", "", base.Add(300*time.Millisecond),
			factoryapi.SessionCompletedEventPayload{CompletedAt: base.Add(300 * time.Millisecond), FinalStatus: factoryapi.FactorySessionDurableLifecycleStatusSucceeded}),
	}
	amount := "0.1234"
	priceSource := generatedclient.CostsLineItemPriceSource("BUILT_IN")
	return metricsSessionCostLensFixture{
		events: events, workID: workID, provider: provider, model: model, amount: amount,
		costReport: generatedclient.CostsReport{
			Currency: generatedclient.CostsReportCurrency("USD"), Status: generatedclient.CostsReportStatus("PARTIAL"), KnownCost: &amount,
			Scope: generatedclient.CostsScope{Kind: generatedclient.CostsScopeKind("FACTORY_SESSION"), FactorySessionId: stringPointer("session-1")},
			LineItems: []generatedclient.CostsLineItem{
				{DispatchId: stringPointer("dispatch-1"), FactorySessionId: stringPointer("session-1"), WorkId: stringPointer(workID), WorkerSessionId: stringPointer("worker-session-1"), Provider: &provider, Model: &model, PricedAmount: &amount, PriceSource: &priceSource, Status: generatedclient.CostsLineItemStatus("PRICED")},
				{DispatchId: stringPointer("dispatch-2"), FactorySessionId: stringPointer("session-1"), WorkId: stringPointer(workID), Reason: stringPointer("model identity is unavailable"), Status: generatedclient.CostsLineItemStatus("UNPRICED")},
			},
		},
	}
}

func assertMetricsSessionCostLensRequest(t *testing.T, request MetricsCostReportRequest) {
	t.Helper()
	if request.Server != "http://metrics.test" {
		t.Fatalf("cost request server = %q, want selected server", request.Server)
	}
	if request.SessionID != "session-1" {
		t.Fatalf("cost request session = %q, want selected session", request.SessionID)
	}
}

func assertMetricsSessionCostLensReport(t *testing.T, document metricsSessionDocument, fixture metricsSessionCostLensFixture) {
	t.Helper()
	if document.Cost == nil || document.Cost.KnownCost == nil {
		t.Fatalf("cost report = %#v, want exact partial report", document.Cost)
	}
	if *document.Cost.KnownCost != fixture.amount || document.Cost.Status != "PARTIAL" {
		t.Fatalf("cost report = %#v, want exact amount and partial status", document.Cost)
	}
	if len(document.Cost.LineItems) != 2 || document.Cost.LineItems[1].Reason == nil || *document.Cost.LineItems[1].Reason != "model identity is unavailable" {
		t.Fatalf("cost line items = %#v, want preserved unpriced reason", document.Cost.LineItems)
	}
	if len(document.ByWorker) != 2 || len(document.ByDispatch) != 2 {
		t.Fatalf("detail rows = workers=%d dispatches=%d, want two canonical attempts/groups", len(document.ByWorker), len(document.ByDispatch))
	}
	assertMetricsSessionCanonicalWorker(t, findMetricsSessionWorker(document.ByWorker, "worker-session-1"), fixture)
	assertMetricsSessionCanonicalDispatch(t, findMetricsSessionDispatch(document.ByDispatch, "dispatch-1"), fixture)
	assertMetricsSessionUnpricedDispatch(t, findMetricsSessionDispatch(document.ByDispatch, "dispatch-2"))
}

func assertMetricsSessionCanonicalWorker(t *testing.T, worker *metricsSessionWorkerDocument, fixture metricsSessionCostLensFixture) {
	t.Helper()
	if worker == nil {
		t.Fatal("canonical worker detail is missing")
	}
	if worker.Worker != "reviewer" || worker.WorkerIdentity != "canonical" {
		t.Fatalf("worker detail = %#v, want canonical Worker", worker)
	}
	if worker.Provider == nil || *worker.Provider != fixture.provider || worker.Model == nil || *worker.Model != fixture.model {
		t.Fatalf("worker detail = %#v, want canonical provider/model", worker)
	}
}

func assertMetricsSessionCanonicalDispatch(t *testing.T, dispatch *metricsSessionDispatchDocument, fixture metricsSessionCostLensFixture) {
	t.Helper()
	if dispatch == nil {
		t.Fatal("canonical dispatch detail is missing")
	}
	if dispatch.Worker != "reviewer" || dispatch.WorkerIdentity != "canonical" {
		t.Fatalf("dispatch detail = %#v, want canonical Worker", dispatch)
	}
	if dispatch.Provider == nil || *dispatch.Provider != fixture.provider || dispatch.Workstation == nil || *dispatch.Workstation != "review" {
		t.Fatalf("dispatch detail = %#v, want canonical provider/workstation", dispatch)
	}
	if dispatch.Cost == nil || dispatch.Cost.KnownCost == nil || *dispatch.Cost.KnownCost != fixture.amount {
		t.Fatalf("dispatch cost = %#v, want exact cost", dispatch.Cost)
	}
}

func assertMetricsSessionUnpricedDispatch(t *testing.T, dispatch *metricsSessionDispatchDocument) {
	t.Helper()
	if dispatch == nil {
		t.Fatal("unpriced dispatch detail is missing")
	}
	if dispatch.Provider != nil || dispatch.Model != nil || dispatch.ProviderIdentity != "unavailable" || dispatch.ModelIdentity != "unavailable" {
		t.Fatalf("unpriced dispatch identities = %#v, want unavailable", dispatch)
	}
	if dispatch.Cost == nil || dispatch.Cost.KnownCost != nil || dispatch.Cost.Status != "UNPRICED" {
		t.Fatalf("unpriced dispatch cost = %#v, want null UNPRICED", dispatch.Cost)
	}
}
func findMetricsSessionWorker(rows []metricsSessionWorkerDocument, workerSessionID string) *metricsSessionWorkerDocument {
	for index := range rows {
		if rows[index].WorkerSessionID != nil && *rows[index].WorkerSessionID == workerSessionID {
			return &rows[index]
		}
	}
	return nil
}

func findMetricsSessionDispatch(rows []metricsSessionDispatchDocument, dispatchID string) *metricsSessionDispatchDocument {
	for index := range rows {
		if rows[index].DispatchID != nil && *rows[index].DispatchID == dispatchID {
			return &rows[index]
		}
	}
	return nil
}

func TestMetricsSessionDetailsMarkConflictingUsageIdentitiesUnavailable(t *testing.T) {
	base := time.Date(2026, 9, 5, 21, 0, 0, 0, time.UTC)
	workID := "work-conflict"
	dispatchID := "dispatch-conflict"
	events := []factoryapi.FactoryEvent{
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchQueued, "conflict-queued", dispatchID, base,
			factoryapi.DispatchQueuedEventPayload{InputWorkIds: &[]string{workID}, Provider: stringPointer("codex"), Model: stringPointer("gpt-5")}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchRequest, "conflict-request", dispatchID, base.Add(time.Millisecond),
			factoryapi.DispatchRequestEventPayload{TransitionId: "review", Inputs: []factoryapi.DispatchConsumedWorkRef{{WorkId: workID}}}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation, "conflict-association", dispatchID, base.Add(2*time.Millisecond),
			factoryapi.DispatchWorkerSessionAssociationEventPayload{WorkerSessionId: "worker-session-a"}),
	}
	client := &metricsClientStub{response: &generatedclient.GetMetricsClientResponse{JSON200: &generatedclient.MetricsReport{
		Scope: generatedclient.MetricsScope{Kind: "FACTORY_SESSION", FactorySessionId: stringPointer("session-conflict")},
		UsageRows: []generatedclient.MetricsUsageRow{
			{DispatchId: stringPointer(dispatchID), WorkId: stringPointer(workID), Provider: stringPointer("codex"), Model: stringPointer("gpt-5"), WorkerSessionId: stringPointer("worker-session-a")},
			{DispatchId: stringPointer(dispatchID), WorkId: stringPointer(workID), Provider: stringPointer("other"), Model: stringPointer("gpt-5"), WorkerSessionId: stringPointer("worker-session-b")},
		},
	}}}
	operation := NewOperation(func(string) (Client, error) { return client, nil })
	var output bytes.Buffer
	err := RunMetricsOperation(context.Background(), operation, MetricsConfig{
		Server: "http://metrics.test", SessionID: "session-conflict", JSON: true, Output: &output,
		SessionReport: true, SessionEvents: metricsSessionEventOperationStub{events: events},
		SessionByWorker: true, SessionByDispatch: true,
	})
	if err != nil {
		t.Fatalf("RunMetricsOperation() error = %v", err)
	}
	var document metricsSessionDocument
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("decode conflicting detail report: %v\n%s", err, output.String())
	}
	dispatch := findMetricsSessionDispatch(document.ByDispatch, dispatchID)
	if dispatch == nil || dispatch.Provider != nil || dispatch.ProviderIdentity != "unavailable" || dispatch.WorkerSessionID != nil {
		t.Fatalf("conflicting dispatch detail = %#v, want unavailable identities", dispatch)
	}
	if len(document.ByWorker) != 1 || document.ByWorker[0].WorkerSessionID != nil {
		t.Fatalf("conflicting worker detail = %#v, want one unavailable group", document.ByWorker)
	}
}

func TestMetricsSessionUnsupportedLensFailsBeforeQuery(t *testing.T) {
	called := false
	var output bytes.Buffer
	err := RunMetricsOperation(context.Background(), func(context.Context, MetricsConfig) error {
		called = true
		return nil
	}, MetricsConfig{
		Server: "http://metrics.test", SessionID: "session-unsupported", JSON: true, Output: &output,
		SessionReport: true, SessionLens: "forecast", SessionEvents: metricsSessionEventOperationStub{},
	})
	var metricsErr *MetricsError
	if !errors.As(err, &metricsErr) || metricsErr.CLIErrorCode() != MetricsUnsupportedSessionOptionCode {
		t.Fatalf("error = %#v, want unsupported-lens diagnostic", err)
	}
	if called || output.Len() != 0 {
		t.Fatalf("unsupported lens called operation or wrote output: called=%t output=%q", called, output.String())
	}
}

func TestMetricsSessionCostLensKeepsNoUsageAmountNull(t *testing.T) {
	base := time.Date(2026, 9, 5, 21, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchQueued, "no-usage-queued", "dispatch-no-usage", base,
			factoryapi.DispatchQueuedEventPayload{}),
		metricsSessionEvent(t, factoryapi.FactoryEventTypeDispatchResponse, "no-usage-response", "dispatch-no-usage", base.Add(time.Millisecond),
			factoryapi.DispatchResponseEventPayload{Outcome: factoryapi.WorkOutcomeCanceled}),
	}
	client := &metricsClientStub{response: &generatedclient.GetMetricsClientResponse{JSON200: &generatedclient.MetricsReport{
		Scope: generatedclient.MetricsScope{Kind: "FACTORY_SESSION", FactorySessionId: stringPointer("session-no-usage")},
	}}}
	operation := NewOperation(func(string) (Client, error) { return client, nil })
	var output bytes.Buffer
	err := RunMetricsOperation(context.Background(), operation, MetricsConfig{
		Server: "http://metrics.test", SessionID: "session-no-usage", JSON: true, Output: &output,
		SessionReport: true, SessionEvents: metricsSessionEventOperationStub{events: events}, SessionLens: "cost", SessionByDispatch: true,
		CostReport: func(context.Context, MetricsCostReportRequest) (generatedclient.CostsReport, error) {
			return generatedclient.CostsReport{
				Currency: generatedclient.CostsReportCurrency("USD"),
				Status:   generatedclient.CostsReportStatus("NO_USAGE"),
				Scope:    generatedclient.CostsScope{Kind: generatedclient.CostsScopeKind("FACTORY_SESSION"), FactorySessionId: stringPointer("session-no-usage")},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("RunMetricsOperation() error = %v", err)
	}
	var document metricsSessionDocument
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("decode no-usage report: %v\n%s", err, output.String())
	}
	if document.Cost == nil || document.Cost.Status != "NO_USAGE" || document.Cost.KnownCost != nil {
		t.Fatalf("top-level cost = %#v, want NO_USAGE with null amount", document.Cost)
	}
	if len(document.ByDispatch) != 1 || document.ByDispatch[0].Cost == nil || document.ByDispatch[0].Cost.Status != "NO_USAGE" || document.ByDispatch[0].Cost.KnownCost != nil {
		t.Fatalf("dispatch cost = %#v, want NO_USAGE with null amount", document.ByDispatch)
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
	case factoryapi.InitialStructureRequestEventPayload:
		err = eventPayload.FromInitialStructureRequestEventPayload(typed)
	case factoryapi.DispatchQueuedEventPayload:
		err = eventPayload.FromDispatchQueuedEventPayload(typed)
	case factoryapi.DispatchRequestEventPayload:
		err = eventPayload.FromDispatchRequestEventPayload(typed)
	case factoryapi.ModelRequestEventPayload:
		err = eventPayload.FromModelRequestEventPayload(typed)
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
