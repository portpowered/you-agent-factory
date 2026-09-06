package run

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestRunFactoryInvocation_LiveAndReplayPreserveCanonicalJavaScriptOrder(t *testing.T) {
	events := canonicalJavaScriptPhaseCheckpointPhaseEvents()

	outputs := make([]string, 0, 2)
	for _, source := range []struct {
		name       string
		replayPath string
	}{{name: "live"}, {name: "replay", replayPath: "recording.json"}} {
		t.Run(source.name, func(t *testing.T) {
			var output bytes.Buffer
			owner := newTestOpeningPresentationOwner()
			operation := testInvocationOperation{presentations: owner, invokeFactory: func(
				_ context.Context,
				target factorysessions.InvocationTarget,
				_ factorysessions.InvocationRequest,
				consume func([]interfaces.FactoryEvent),
			) (factorysessions.FactoryInvocationOutcome, error) {
				if target.ReplayPath != source.replayPath {
					t.Fatalf("ReplayPath = %q, want %q", target.ReplayPath, source.replayPath)
				}
				if source.name == "live" {
					consume(events[:2])
					consume(events[2:4])
					consume(events[4:])
				} else {
					consume(events)
				}
				return factorysessions.FactoryInvocationOutcome{Result: interfaces.FactoryInvocationResult{
					RequestID: "request-js", Status: interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{
						{Type: work.WorkContentPartTypeText, Text: "first streamed part"},
						{Type: work.WorkContentPartTypeText, Text: "second streamed part"},
					},
				}}, nil
			}}
			cfg := RunConfig{
				InvocationOutputMode: InvocationOutputResponseStream,
				JSONOutput:           true, Output: &output, ReplayPath: source.replayPath,
			}
			if err := runFactoryInvocation(
				context.Background(), cfg, invocationTarget(cfg, nil),
				factoryapi.InvocationRequest{}, operation, testResponsePresentation(), owner,
			); err != nil {
				t.Fatalf("run Factory invocation: %v", err)
			}
			outputs = append(outputs, output.String())
			assertPhaseCheckpointPhasePresentation(t, output.String())
		})
	}
	if outputs[0] != outputs[1] {
		t.Fatalf("live and replay presentation differ:\nlive=%s\nreplay=%s", outputs[0], outputs[1])
	}
}
func TestRunRemoteInvocationResponseStreamReconnectsFromCanonicalCursor(t *testing.T) {
	apiEvents := apiEventsFromDomain(t, canonicalJavaScriptFactoryEvents())
	operation := &scriptedRemoteResponseOperation{
		streams: []scriptedRemoteEventStream{
			{events: apiEvents[:1], terminalErr: errors.New("connection reset")},
			{events: apiEvents[1:]},
		},
		result: finalRemoteInvocationResult(t, "remote approved"),
	}
	var output bytes.Buffer
	err := RunRemoteInvocation(context.Background(), RunConfig{
		Dir:                     "factory",
		NamedFactoryName:        "@you/research",
		PreparedInvocationInput: preparedRemoteArguments("remote input"),
		InvocationOutputMode:    InvocationOutputResponseStream,
		Output:                  &output,
	}, "http://selected.test", operation, testResponsePresentation())
	if err != nil {
		t.Fatalf("RunRemoteInvocation: %v\noutput:\n%s", err, output.String())
	}
	if len(operation.eventRequests) != 2 {
		t.Fatalf("event stream opens = %d, want reconnect after first stream failure", len(operation.eventRequests))
	}
	first, second := operation.eventRequests[0], operation.eventRequests[1]
	if first.AfterEventID != "" || first.AfterSequence != nil {
		t.Fatalf("initial event cursor = %#v, want empty cursor", first)
	}
	if second.AfterEventID != apiEvents[0].Id || second.AfterSequence == nil || *second.AfterSequence != 1 {
		t.Fatalf("reconnect event cursor = %#v, want event=%q sequence=1", second, apiEvents[0].Id)
	}
	if eventIndex, resultIndex := strings.Index(output.String(), "factory started"), strings.Index(output.String(), "remote approved"); eventIndex < 0 || resultIndex < 0 || eventIndex >= resultIndex {
		t.Fatalf("human output ordering = %q, want canonical event before terminal result", output.String())
	}
}

func TestRunRemoteInvocationResponseStreamJSONEmitsEventsBeforeTerminalFailure(t *testing.T) {
	operation := &scriptedRemoteResponseOperation{
		openErr: errors.New("remote event stream unavailable"),
	}
	var output bytes.Buffer
	err := RunRemoteInvocation(context.Background(), RunConfig{
		Dir:                     "factory",
		NamedFactoryName:        "@you/research",
		PreparedInvocationInput: preparedRemoteArguments("remote input"),
		InvocationOutputMode:    InvocationOutputResponseStream,
		JSONOutput:              true,
		Output:                  &output,
	}, "http://selected.test", operation, testResponsePresentation())
	if err == nil || !strings.Contains(err.Error(), "remote event stream unavailable") {
		t.Fatalf("RunRemoteInvocation error = %v, want exhausted event-stream failure", err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], `"recordType":"invocation_result"`) {
		t.Fatalf("JSON failure output = %q, want one terminal invocation record", output.String())
	}
	var record remoteInvocationNDJSONRecord
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("decode terminal JSON record: %v", err)
	}
	if record.Response.Status != factoryapi.InvocationTerminalStatusFailed || record.Response.ErrorCode == nil || *record.Response.ErrorCode != RemoteDurableResultCode {
		t.Fatalf("terminal response = %#v, want remote durable failure", record.Response)
	}
}

func TestRemoteInvocationClientFactoryEventsUsesCanonicalCursor(t *testing.T) {
	event := apiEventsFromDomain(t, canonicalJavaScriptFactoryEvents()[:1])[0]
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/durable-remote/events" {
			t.Fatalf("event path = %q, want canonical session events path", r.URL.Path)
		}
		if r.URL.Query().Get("after_event_id") != "event-previous" || r.URL.Query().Get("after_sequence") != "7" {
			t.Fatalf("event cursor query = %s, want event-previous/7", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", mustJSON(event))
	}))
	defer server.Close()
	transport, err := clihttp.NewProtocol(server.Client(), platformclock.Real{})
	if err != nil {
		t.Fatalf("NewProtocol: %v", err)
	}
	operation := NewRemoteInvocation(transport)
	eventOperation, ok := operation.(RemoteInvocationEventOperation)
	if !ok {
		t.Fatal("NewRemoteInvocation does not expose canonical event operation")
	}
	sequence := 7
	stream, err := eventOperation.OpenFactorySessionEvents(context.Background(), RemoteInvocationEventRequest{
		Server:        server.URL,
		SessionID:     "durable-remote",
		AfterEventID:  "event-previous",
		AfterSequence: &sequence,
	})
	if err != nil {
		t.Fatalf("OpenFactorySessionEvents: %v", err)
	}
	defer stream.Close()
	got, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("event stream Next: %v", err)
	}
	if got.Id != event.Id || got.Type != event.Type {
		t.Fatalf("event = %#v, want %#v", got, event)
	}
	if _, err := stream.Next(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("event stream terminal read error = %v, want EOF", err)
	}
}

func TestRemoteInvocationClientFactoryEventReplayStopsAtCapturedRetainedHead(t *testing.T) {
	events := apiEventsFromDomain(t, canonicalJavaScriptFactoryEvents()[:2])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set(factorysessions.SessionEventStreamRetainedCountHeader, "1")
		for _, event := range events {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", mustJSON(event))
		}
	}))
	defer server.Close()
	transport, err := clihttp.NewProtocol(server.Client(), platformclock.Real{})
	if err != nil {
		t.Fatalf("NewProtocol: %v", err)
	}
	operation := NewRemoteInvocation(transport).(RemoteInvocationEventOperation)
	stream, err := operation.OpenFactorySessionEvents(context.Background(), RemoteInvocationEventRequest{
		Server: server.URL, SessionID: "session-1", ReplayOnly: true,
	})
	if err != nil {
		t.Fatalf("OpenFactorySessionEvents: %v", err)
	}
	defer stream.Close()
	if got, err := stream.Next(context.Background()); err != nil || got.Id != events[0].Id {
		t.Fatalf("first retained event = (%#v, %v), want %q", got, err, events[0].Id)
	}
	if _, err := stream.Next(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("read after retained head error = %v, want EOF", err)
	}
}

func TestRemoteInvocationClientFactoryEventReplayRejectsTruncatedRetainedHead(t *testing.T) {
	events := apiEventsFromDomain(t, canonicalJavaScriptFactoryEvents()[:1])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set(factorysessions.SessionEventStreamRetainedCountHeader, "2")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", mustJSON(events[0]))
	}))
	defer server.Close()
	transport, err := clihttp.NewProtocol(server.Client(), platformclock.Real{})
	if err != nil {
		t.Fatalf("NewProtocol: %v", err)
	}
	operation := NewRemoteInvocation(transport).(RemoteInvocationEventOperation)
	stream, err := operation.OpenFactorySessionEvents(context.Background(), RemoteInvocationEventRequest{
		Server: server.URL, SessionID: "session-1", ReplayOnly: true,
	})
	if err != nil {
		t.Fatalf("OpenFactorySessionEvents: %v", err)
	}
	defer stream.Close()
	if _, err := stream.Next(context.Background()); err != nil {
		t.Fatalf("first retained event = %v", err)
	}
	_, err = stream.Next(context.Background())
	var truncated *remoteFactoryEventReplayTruncatedError
	if !errors.As(err, &truncated) || truncated.remaining != 1 {
		t.Fatalf("truncated replay error = %v, want one remaining retained event", err)
	}
}

func TestRemoteInvocationClientFactoryEventsRejectsInvalidResponses(t *testing.T) {
	request := RemoteInvocationEventRequest{Server: "https://selected.test", SessionID: "durable-remote"}
	client := remoteInvocationClient{}
	if _, err := client.OpenFactorySessionEvents(nil, request); err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("nil context error = %v, want required-context error", err)
	}
	if _, err := client.OpenFactorySessionEvents(context.Background(), request); err == nil || !strings.Contains(err.Error(), "CLI HTTP protocol is required") {
		t.Fatalf("nil protocol error = %v, want required-protocol error", err)
	}
	if _, err := (remoteInvocationClient{transport: &remoteProtocolStub{}}).OpenFactorySessionEvents(
		context.Background(), RemoteInvocationEventRequest{Server: "http://[::1", SessionID: "durable-remote"},
	); err == nil || !strings.Contains(err.Error(), RemoteDurableResultCode) {
		t.Fatalf("invalid endpoint error = %v, want durable-result classification", err)
	}

	for _, test := range []struct {
		name     string
		response clihttp.Response
		want     string
	}{
		{name: "missing HTTP response", response: clihttp.Response{}, want: "HTTP response is unavailable"},
		{
			name: "server API error",
			response: clihttp.Response{HTTP: &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader(`{"message":"server unavailable"}`)),
			}},
			want: "server unavailable",
		},
		{
			name: "server status without API error",
			response: clihttp.Response{HTTP: &http.Response{
				StatusCode: http.StatusBadGateway,
				Body:       io.NopCloser(strings.NewReader("not JSON")),
			}},
			want: "(502)",
		},
		{
			name:     "missing event body",
			response: clihttp.Response{HTTP: &http.Response{StatusCode: http.StatusOK}},
			want:     "HTTP response has no body",
		},
		{
			name: "wrong content type",
			response: clihttp.Response{HTTP: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader("{}")),
			}},
			want: "content type",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := (remoteInvocationClient{transport: &remoteProtocolStub{response: test.response}}).OpenFactorySessionEvents(
				context.Background(), request,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRemoteFactoryEventStreamParsesFramesAndGuardsReads(t *testing.T) {
	var nilStream *remoteFactoryEventStream
	if _, err := nilStream.Next(context.Background()); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil stream Next error = %v, want unavailable error", err)
	}
	if err := nilStream.Close(); err != nil {
		t.Fatalf("nil stream Close error = %v, want nil", err)
	}

	stream := &remoteFactoryEventStream{reader: bufio.NewReader(strings.NewReader("data: not used\n\n"))}
	if _, err := stream.Next(nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("nil context error = %v, want context.Canceled", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := stream.Next(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error = %v, want context.Canceled", err)
	}

	event := apiEventsFromDomain(t, canonicalJavaScriptFactoryEvents()[:1])[0]
	payload := mustJSON(event)
	split := bytes.IndexByte(payload, ',') + 1
	if split <= 0 {
		t.Fatal("canonical event JSON has no safe multiline split")
	}
	framed := ": heartbeat\n\n" + "data: " + string(payload[:split]) + "\n" + "data: " + string(payload[split:]) + "\n\n"
	stream = &remoteFactoryEventStream{reader: bufio.NewReader(strings.NewReader(framed))}
	got, err := stream.Next(context.Background())
	if err != nil || got.Id != event.Id || got.Type != event.Type {
		t.Fatalf("framed event = %#v/%v, want event %q", got, err, event.Id)
	}
	if _, err := stream.Next(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("after-frame error = %v, want EOF", err)
	}

	for _, malformed := range []string{"data: {\"id\":\"event-only\"}\n\n", "data: {not-json}\n\n"} {
		_, err := readRemoteFactoryEventSSE(bufio.NewReader(strings.NewReader(malformed)))
		var malformedErr *remoteMalformedFactoryEventError
		if !errors.As(err, &malformedErr) {
			t.Fatalf("malformed SSE error = %v, want remote malformed event error", err)
		}
	}
}

func TestRemoteFactoryEventRetryClassificationAndReconnectCancellation(t *testing.T) {
	transportError := func(status int) error {
		return &remoteInvocationEventTransportError{status: status, message: "transport"}
	}
	for _, test := range []struct {
		name      string
		err       error
		wantRetry bool
	}{
		{name: "nil", wantRetry: false},
		{name: "EOF", err: io.EOF, wantRetry: false},
		{name: "canceled", err: context.Canceled, wantRetry: false},
		{name: "malformed", err: &remoteMalformedFactoryEventError{cause: errors.New("bad event")}, wantRetry: false},
		{name: "gateway", err: transportError(http.StatusBadGateway), wantRetry: true},
		{name: "client failure", err: transportError(http.StatusBadRequest), wantRetry: false},
		{name: "unknown", err: errors.New("unknown stream failure"), wantRetry: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := remoteFactoryEventRetryable(test.err); got != test.wantRetry {
				t.Fatalf("retryable(%v) = %t, want %t", test.err, got, test.wantRetry)
			}
		})
	}

	attempts := 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := retryRemoteFactoryEventStream(ctx, "https://selected.test", "durable-remote", &attempts, transportError(http.StatusInternalServerError))
	var invocationErr *InvocationError
	if !errors.As(err, &invocationErr) || !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("canceled reconnect = %v/attempts=%d, want typed canceled error after one attempt", err, attempts)
	}
}

type scriptedRemoteResponseOperation struct {
	streams       []scriptedRemoteEventStream
	openErr       error
	result        factoryapi.FactorySessionResult
	eventRequests []RemoteInvocationEventRequest
}

func (operation *scriptedRemoteResponseOperation) StartFactorySession(context.Context, RemoteInvocationRequest) (factoryapi.FactorySessionExecutionResponse, error) {
	return factoryapi.FactorySessionExecutionResponse{
		SessionId: "dur-sess-remote-stream",
		Status:    factoryapi.FactorySessionDurableLifecycleStatusQueued,
	}, nil
}

func (operation *scriptedRemoteResponseOperation) GetFactorySessionResult(context.Context, RemoteInvocationResultRequest) (factoryapi.FactorySessionResult, error) {
	return operation.result, nil
}

func (operation *scriptedRemoteResponseOperation) OpenFactorySessionEvents(_ context.Context, request RemoteInvocationEventRequest) (RemoteInvocationEventStream, error) {
	operation.eventRequests = append(operation.eventRequests, request)
	if operation.openErr != nil {
		return nil, operation.openErr
	}
	if len(operation.streams) == 0 {
		return nil, errors.New("no scripted remote event stream remains")
	}
	stream := operation.streams[0]
	operation.streams = operation.streams[1:]
	return &stream, nil
}

type scriptedRemoteEventStream struct {
	events      []factoryapi.FactoryEvent
	index       int
	terminalErr error
	errReturned bool
}

func (stream *scriptedRemoteEventStream) Next(context.Context) (factoryapi.FactoryEvent, error) {
	if stream.index < len(stream.events) {
		event := stream.events[stream.index]
		stream.index++
		return event, nil
	}
	if stream.terminalErr != nil && !stream.errReturned {
		stream.errReturned = true
		return factoryapi.FactoryEvent{}, stream.terminalErr
	}
	return factoryapi.FactoryEvent{}, io.EOF
}

func (stream *scriptedRemoteEventStream) Close() error { return nil }

func apiEventsFromDomain(t *testing.T, events []interfaces.FactoryEvent) []factoryapi.FactoryEvent {
	t.Helper()
	converted := make([]factoryapi.FactoryEvent, 0, len(events))
	for _, event := range events {
		var apiEvent factoryapi.FactoryEvent
		if err := event.Decode(&apiEvent); err != nil {
			t.Fatalf("decode domain Factory Event: %v", err)
		}
		converted = append(converted, apiEvent)
	}
	return converted
}

func finalRemoteInvocationResult(t *testing.T, text string) factoryapi.FactorySessionResult {
	if t != nil {
		t.Helper()
	}
	status := factoryapi.FactorySessionDurableLifecycleStatusSucceeded
	result := factoryapi.FactorySessionResult{
		SessionId:     "dur-sess-remote-stream",
		ResultStatus:  factoryapi.FactorySessionResultStatusFinal,
		SessionStatus: &status,
	}
	if t != nil {
		result.PrimaryResult = remoteTextContent(t, text)
	}
	return result
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func assertPhaseCheckpointPhasePresentation(t *testing.T, output string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 7 {
		t.Fatalf("records = %d, want six Factory Events and one terminal result:\n%s", len(lines), output)
	}
	wantTypes := []interfaces.FactoryEventType{
		interfaces.FactoryEventTypeSessionStarted,
		interfaces.FactoryEventTypeOrchestratorPhaseChanged,
		interfaces.FactoryEventTypeOrchestratorCheckpointWritten,
		interfaces.FactoryEventTypeOrchestratorPhaseChanged,
		interfaces.FactoryEventTypeOrchestratorPhaseChanged,
		interfaces.FactoryEventTypeOrchestratorPhaseChanged,
	}
	previousSequence := 0
	previousSessionSequence := 0
	for index, wantType := range wantTypes {
		var record factoryEventJSONRecord
		if err := json.Unmarshal([]byte(lines[index]), &record); err != nil {
			t.Fatalf("decode Factory Event record %d: %v", index, err)
		}
		if record.RecordType != factoryEventJSONRecordType || record.Event.Type != wantType {
			t.Fatalf("record %d = %#v, want %s %s", index, record, factoryEventJSONRecordType, wantType)
		}
		if record.Event.Context.Sequence <= previousSequence || record.Event.Context.SessionSequence == nil ||
			*record.Event.Context.SessionSequence <= previousSessionSequence {
			t.Fatalf("record %d sequence context is not strictly increasing: %#v", index, record.Event.Context)
		}
		previousSequence = record.Event.Context.Sequence
		previousSessionSequence = *record.Event.Context.SessionSequence
	}
	var terminalPhase interfaces.OrchestratorPhaseChangedEventPayload
	var terminalRecord factoryEventJSONRecord
	if err := json.Unmarshal([]byte(lines[len(wantTypes)-1]), &terminalRecord); err != nil {
		t.Fatalf("decode terminal phase record: %v", err)
	}
	if err := json.Unmarshal(terminalRecord.Event.Payload, &terminalPhase); err != nil {
		t.Fatalf("decode terminal phase payload: %v", err)
	}
	if terminalPhase.PhaseStatus != interfaces.OrchestratorPhaseStatusCompleted {
		t.Fatalf("terminal phase status = %q, want COMPLETED", terminalPhase.PhaseStatus)
	}
	if !strings.Contains(lines[6], `"recordType":"invocation_result"`) {
		t.Fatalf("terminal invocation record = %q", lines[6])
	}
	var terminal remoteInvocationNDJSONRecord
	if err := json.Unmarshal([]byte(lines[6]), &terminal); err != nil {
		t.Fatalf("decode terminal invocation record: %v", err)
	}
	assertGeneratedWorkContentPartsFromResponse(t, terminal.Response.PrimaryResult, []work.WorkContentPart{
		{Type: work.WorkContentPartTypeText, Text: "first streamed part"},
		{Type: work.WorkContentPartTypeText, Text: "second streamed part"},
	})
}

func TestRunFactoryInvocation_LiveEventIsWrittenBeforeOperationCompletes(t *testing.T) {
	var output lockedTestBuffer
	published := make(chan struct{})
	release := make(chan struct{})
	events := canonicalJavaScriptFactoryEvents()
	owner := newTestOpeningPresentationOwner()
	operation := testInvocationOperation{presentations: owner, invokeFactory: func(
		_ context.Context,
		_ factorysessions.InvocationTarget,
		_ factorysessions.InvocationRequest,
		consume func([]interfaces.FactoryEvent),
	) (factorysessions.FactoryInvocationOutcome, error) {
		consume(events[:1])
		close(published)
		<-release
		consume(events[1:])
		return factorysessions.FactoryInvocationOutcome{Result: interfaces.FactoryInvocationResult{
			RequestID: "request-live", Status: interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "complete"}},
		}}, nil
	}}
	cfg := RunConfig{
		InvocationOutputMode: InvocationOutputResponseStream,
		JSONOutput:           true, Output: &output,
	}
	done := make(chan error, 1)
	go func() {
		done <- runFactoryInvocation(
			context.Background(), cfg, invocationTarget(cfg, nil),
			factoryapi.InvocationRequest{}, operation, testResponsePresentation(), owner,
		)
	}()

	<-published
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(output.String(), `"recordType":"factory_event"`) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := output.String(); !strings.Contains(got, `"recordType":"factory_event"`) {
		t.Fatalf("live Factory Event was not written before operation completion: %q", got)
	}
	select {
	case err := <-done:
		t.Fatalf("invocation completed before release: %v", err)
	default:
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("run Factory invocation: %v", err)
	}
	assertCanonicalJavaScriptPresentation(t, output.String())
}

func TestHumanFactoryEventRenderer_WritesTerminalSuccessAndFailureLast(t *testing.T) {
	t.Parallel()

	t.Run("success after lifecycle", func(t *testing.T) {
		var output bytes.Buffer
		renderer := openTestHumanFactoryEventRenderer(t, &output, testResponsePresentation())
		renderer.PresentFactoryEvents(canonicalJavaScriptFactoryEvents()[:1])
		if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
			Status: interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText, Text: "complete",
			}},
		}); err != nil {
			t.Fatalf("write terminal success: %v", err)
		}
		if got := output.String(); !strings.HasSuffix(got, responseStreamPrimaryResultHeader+"\ncomplete") {
			t.Fatalf("success output does not end with primary result: %q", got)
		}
	})

	t.Run("failure includes public terminal context", func(t *testing.T) {
		var output bytes.Buffer
		renderer := openTestHumanFactoryEventRenderer(t, &output, testResponsePresentation())
		if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
			Status:    interfaces.InvocationTerminalStatusFailed,
			ErrorCode: "WORK_FAILED", Message: "worker stopped",
			SessionID: "session-1", WorkID: "work-1", WorkName: "research", WorkState: "FAILED",
		}); err != nil {
			t.Fatalf("write terminal failure: %v", err)
		}
		want := "--- invocation outcome ---\n" +
			"status: FAILED\nerror: WORK_FAILED\nmessage: worker stopped\n" +
			"session: session-1\nworkId: work-1\nworkName: research\nworkState: FAILED\n"
		if got := output.String(); got != want {
			t.Fatalf("failure output = %q, want %q", got, want)
		}
	})
}

func TestJSONFactoryEventRenderer_FinalizesTerminalRecordOnce(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	renderer := openTestJSONFactoryEventRenderer(t, &output, testResponsePresentation())
	result := apisurface.FactoryInvocationResult{
		Status: interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText, Text: "complete",
		}},
	}
	if err := renderer.WriteFinalInvocationResult(result); err != nil {
		t.Fatalf("write terminal record: %v", err)
	}
	if err := renderer.WriteFinalInvocationResult(result); err == nil {
		t.Fatal("duplicate terminal record write succeeded")
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], `"recordType":"invocation_result"`) {
		t.Fatalf("terminal output = %q, want one invocation_result record", output.String())
	}
}

func TestFactoryEventRenderers_RejectMissingPresentationEdges(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		open func() error
	}{
		{
			name: "human output",
			open: func() error {
				_, err := invocationFactoryEventRenderer(RunConfig{
					InvocationOutputMode: InvocationOutputResponseStream,
					Output:               nil,
				}, testResponsePresentation())
				return err
			},
		},
		{
			name: "human presentation",
			open: func() error {
				_, err := invocationFactoryEventRenderer(RunConfig{
					InvocationOutputMode: InvocationOutputResponseStream,
					Output:               &bytes.Buffer{},
				}, nil)
				return err
			},
		},
		{
			name: "json output",
			open: func() error {
				_, err := invocationFactoryEventRenderer(RunConfig{
					InvocationOutputMode: InvocationOutputResponseStream,
					JSONOutput:           true,
					Output:               nil,
				}, testResponsePresentation())
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.open(); err == nil {
				t.Fatal("constructor did not return error")
			}
		})
	}
}

type lockedTestBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (buffer *lockedTestBuffer) Write(payload []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.Write(payload)
}

func (buffer *lockedTestBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}

func assertCanonicalJavaScriptPresentation(t *testing.T, output string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 4 {
		t.Fatalf("records = %d, want three Factory Events and one terminal result:\n%s", len(lines), output)
	}
	wantTypes := []interfaces.FactoryEventType{
		interfaces.FactoryEventTypeSessionStarted,
		interfaces.FactoryEventTypeOrchestratorPhaseChanged,
		interfaces.FactoryEventTypeOrchestratorCheckpointWritten,
	}
	for i, wantType := range wantTypes {
		var record factoryEventJSONRecord
		if err := json.Unmarshal([]byte(lines[i]), &record); err != nil {
			t.Fatalf("decode Factory Event record %d: %v", i, err)
		}
		if record.RecordType != factoryEventJSONRecordType || record.Event.Type != wantType {
			t.Fatalf("record %d = %#v, want %s %s", i, record, factoryEventJSONRecordType, wantType)
		}
		if record.Event.Context.SessionSequence == nil || *record.Event.Context.SessionSequence != i+1 {
			t.Fatalf("record %d sessionSequence = %#v, want %d", i, record.Event.Context.SessionSequence, i+1)
		}
	}
	if !strings.Contains(lines[3], `"recordType":"invocation_result"`) {
		t.Fatalf("terminal record = %q", lines[3])
	}
}

func canonicalJavaScriptFactoryEvents() []interfaces.FactoryEvent {
	phaseName := "synthesize"
	events := []interfaces.FactoryEvent{
		canonicalFactoryEventWithPayload(1, interfaces.FactoryEventTypeSessionStarted, interfaces.FactorySessionStartedEventPayload{}),
		canonicalFactoryEventWithPayload(2, interfaces.FactoryEventTypeOrchestratorPhaseChanged, interfaces.OrchestratorPhaseChangedEventPayload{
			PhaseStatus: interfaces.OrchestratorPhaseStatusActive,
		}),
		canonicalFactoryEventWithPayload(3, interfaces.FactoryEventTypeOrchestratorCheckpointWritten, interfaces.OrchestratorCheckpointWrittenEventPayload{
			Label: "draft-ready", ResumabilityStatus: interfaces.CheckpointResumabilityStatusResumable,
		}),
	}
	events[1].Context.PhaseName = &phaseName
	return events
}

func canonicalJavaScriptPhaseCheckpointPhaseEvents() []interfaces.FactoryEvent {
	plan := "plan"
	execute := "execute"
	events := []interfaces.FactoryEvent{
		canonicalFactoryEventWithPayload(1, interfaces.FactoryEventTypeSessionStarted, interfaces.FactorySessionStartedEventPayload{}),
		canonicalFactoryEventWithPayload(2, interfaces.FactoryEventTypeOrchestratorPhaseChanged, interfaces.OrchestratorPhaseChangedEventPayload{PhaseStatus: interfaces.OrchestratorPhaseStatusActive}),
		canonicalFactoryEventWithPayload(3, interfaces.FactoryEventTypeOrchestratorCheckpointWritten, interfaces.OrchestratorCheckpointWrittenEventPayload{Label: "plan-ready", ResumabilityStatus: interfaces.CheckpointResumabilityStatusResumable}),
		canonicalFactoryEventWithPayload(4, interfaces.FactoryEventTypeOrchestratorPhaseChanged, interfaces.OrchestratorPhaseChangedEventPayload{PhaseStatus: interfaces.OrchestratorPhaseStatusCompleted}),
		canonicalFactoryEventWithPayload(5, interfaces.FactoryEventTypeOrchestratorPhaseChanged, interfaces.OrchestratorPhaseChangedEventPayload{PhaseStatus: interfaces.OrchestratorPhaseStatusActive}),
		canonicalFactoryEventWithPayload(6, interfaces.FactoryEventTypeOrchestratorPhaseChanged, interfaces.OrchestratorPhaseChangedEventPayload{PhaseStatus: interfaces.OrchestratorPhaseStatusCompleted}),
	}
	events[1].Context.PhaseName = &plan
	events[2].Context.PhaseName = &plan
	events[3].Context.PhaseName = &plan
	events[4].Context.PhaseName = &execute
	events[5].Context.PhaseName = &execute
	return events
}

func canonicalFactoryEventFixture(sequence int, eventType interfaces.FactoryEventType) interfaces.FactoryEvent {
	sessionID := "session-js"
	sessionSequence := sequence
	return interfaces.FactoryEvent{
		Id: fmt.Sprintf("factory-event-%d", sequence), SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type: eventType, Payload: json.RawMessage(`{}`),
		Context: interfaces.FactoryEventContext{
			EventTime: time.Unix(int64(sequence), 0).UTC(), Sequence: sequence,
			SessionID: &sessionID, SessionSequence: &sessionSequence,
		},
	}
}

func canonicalFactoryEventWithPayload(sequence int, eventType interfaces.FactoryEventType, payload any) interfaces.FactoryEvent {
	event := canonicalFactoryEventFixture(sequence, eventType)
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	event.Payload = encoded
	return event
}

func TestHumanFactoryEventRenderer_FailuresAreUnderstandable(t *testing.T) {
	t.Parallel()

	events := []interfaces.FactoryEvent{
		canonicalFactoryEventWithPayload(1, interfaces.FactoryEventTypeInferenceResponse, workerexecution.InferenceResponseEventPayload{
			Attempt: 2, Outcome: workerexecution.InferenceOutcomeFailed,
			FailureDetail: &workerexecution.InferenceResponseFailureDetail{Message: "model request timed out"},
		}),
		canonicalFactoryEventWithPayload(2, interfaces.FactoryEventTypeDispatchResponse, workerexecution.DispatchResponseEventPayload{
			TransitionID: "release review", Outcome: workerexecution.OutcomeFailed,
			FailureDetail: &workerexecution.FailureDetail{Message: "worker timed out"},
		}),
	}
	var output strings.Builder
	renderer := openTestHumanFactoryEventRenderer(t, &output, testResponsePresentation())
	renderer.PresentFactoryEvents(events)
	renderer.StopProgressRendering()
	want := "[1] inference failed (attempt 2) — model request timed out\n" +
		"[2] workstation failed: release review — worker timed out\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestInvocationFactoryEventRenderer_ColorsOnlyTTYHumanOutput(t *testing.T) {
	t.Parallel()

	outputs := make([]string, 0, 2)
	for _, outputIsTTY := range []bool{true, false} {
		var output strings.Builder
		renderer, err := invocationFactoryEventRenderer(RunConfig{
			InvocationOutputMode: InvocationOutputResponseStream,
			OutputIsTTY:          outputIsTTY,
			Output:               &output,
		}, testResponsePresentation())
		if err != nil {
			t.Fatalf("invocationFactoryEventRenderer(outputIsTTY=%t): %v", outputIsTTY, err)
		}
		renderer.PresentFactoryEvents(canonicalJavaScriptFactoryEvents())
		if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
			Status:        interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "complete"}},
		}); err != nil {
			t.Fatalf("writeFinalInvocationResult(outputIsTTY=%t): %v", outputIsTTY, err)
		}
		outputs = append(outputs, output.String())
	}
	if !strings.Contains(outputs[0], "\x1b[") {
		t.Fatalf("TTY human output omitted terminal colors: %q", outputs[0])
	}
	if strings.Contains(outputs[1], "\x1b[") {
		t.Fatalf("redirected human output contained terminal colors: %q", outputs[1])
	}
	plainTTY := strings.NewReplacer(
		"\x1b[32m", "", "\x1b[33m", "", "\x1b[34m", "", "\x1b[35m", "",
		"\x1b[36m", "", "\x1b[94m", "", "\x1b[95m", "", "\x1b[96m", "", "\x1b[0m", "",
	).Replace(outputs[0])
	if plainTTY != outputs[1] {
		t.Fatalf("TTY colors changed human content:\ntty=%q\nredirected=%q", outputs[0], outputs[1])
	}
}

// These fixtures are shared by the remote invocation input and response-stream
// tests so the protocol seam has one observable test double.
type remoteProtocolStub struct {
	response clihttp.Response
	err      error
	called   bool
	url      string
}

func (stub *remoteProtocolStub) Execute(request *http.Request) (clihttp.Response, error) {
	stub.called = true
	if request != nil && request.URL != nil {
		stub.url = request.URL.String()
	}
	return stub.response, stub.err
}

func (stub *remoteProtocolStub) GetJSON(context.Context, string, any) (clihttp.Response, error) {
	return stub.response, stub.err
}

func (stub *remoteProtocolStub) PostJSON(_ context.Context, url string, _ io.Reader, _ any) (clihttp.Response, error) {
	stub.called = true
	stub.url = url
	return stub.response, stub.err
}

func (stub *remoteProtocolStub) PostJSONCreated(context.Context, string, io.Reader, any) (clihttp.Response, error) {
	return stub.response, stub.err
}

func (stub *remoteProtocolStub) PutJSON(context.Context, string, io.Reader, any) (clihttp.Response, error) {
	return stub.response, stub.err
}

func (stub *remoteProtocolStub) PutJSONCreated(context.Context, string, io.Reader, any) (clihttp.Response, error) {
	return stub.response, stub.err
}

func preparedRemoteArguments(prompt string) *work.PreparedInvocationInput {
	return &work.PreparedInvocationInput{
		NormalizedArguments: &work.NormalizedArguments{
			Arguments: map[string]work.NormalizedArgument{
				"prompt": {Values: []string{prompt}},
			},
		},
	}
}

func remoteTextContent(t *testing.T, text string) *factoryapi.WorkContent {
	t.Helper()
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	}); err != nil {
		t.Fatalf("build remote text content: %v", err)
	}
	content := factoryapi.WorkContent{part}
	return &content
}

func boolPtr(value bool) *bool {
	return &value
}

func durableLifecycleStatusPtr(value factoryapi.FactorySessionDurableLifecycleStatus) *factoryapi.FactorySessionDurableLifecycleStatus {
	return &value
}

type remoteInvocationOperationFunc func(context.Context, RemoteInvocationRequest) (factoryapi.FactorySessionExecutionResponse, error)

func (fn remoteInvocationOperationFunc) StartFactorySession(ctx context.Context, request RemoteInvocationRequest) (factoryapi.FactorySessionExecutionResponse, error) {
	return fn(ctx, request)
}
