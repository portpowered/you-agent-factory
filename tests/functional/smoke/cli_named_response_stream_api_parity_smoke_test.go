package smoke

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestNamedGoalResponseStream_APIInvocationMatchesCLIResponseStreamTerminal(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI/API named @you/goal response-stream terminal parity smoke")
	}

	factoryDir := materializeNamedGoalFactoryForRoutingSmoke(t)
	mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		reviewerOutput: "accepted",
	})
	server := startNamedGoalRoutingAPIServer(t, factoryDir, mockWorkersPath)

	goalText := fmt.Sprintf("functional-smoke-goal-api-cli-stream-parity-%d", time.Now().UnixNano())
	apiResponse := postNamedGoalRoutingInvocationOnServer(t, server, goalText)

	streamStdout, streamStderr, err := runNamedGoalResponseStreamInvocationCLI(t, mockWorkersPath, true, goalText)
	if err != nil {
		t.Fatalf("CLI JSON response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, streamStdout, streamStderr)
	}
	records, err := parseNamedGoalResponseStreamNDJSONRecords(streamStdout)
	if err != nil {
		t.Fatalf("parse response-stream NDJSON: %v\nstdout:\n%s", err, streamStdout)
	}
	streamTerminal, err := namedGoalResponseStreamTerminalInvocation(records)
	if err != nil {
		t.Fatalf("response-stream terminal invocation: %v\nstdout:\n%s", err, streamStdout)
	}

	assertNamedGoalInvocationTerminalOutcomeParity(t, apiResponse, streamTerminal)
}

func TestNamedGoalResponseStream_APISSEMatchesCLIResponseEventNDJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI/API named @you/goal response-event canonical parity smoke")
	}

	factoryDir := materializeNamedGoalFactoryForRoutingSmoke(t)
	mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		reviewerOutput: "accepted",
	})
	server := startNamedGoalRoutingAPIServer(t, factoryDir, mockWorkersPath)
	goalText := fmt.Sprintf("functional-smoke-goal-api-cli-response-event-parity-%d", time.Now().UnixNano())

	apiEvents := collectLiveAPISessionResponseEventsDuringInvocation(t, server, goalText)

	streamStdout, streamStderr, err := runNamedGoalResponseStreamInvocationCLI(t, mockWorkersPath, true, goalText)
	if err != nil {
		t.Fatalf("CLI JSON response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, streamStdout, streamStderr)
	}
	records, err := parseNamedGoalResponseStreamNDJSONRecords(streamStdout)
	if err != nil {
		t.Fatalf("parse response-stream NDJSON: %v\nstdout:\n%s", err, streamStdout)
	}
	cliEvents := extractResponseEventsFromCLIRecords(records)

	assertCLIResponseEventsMatchLiveAPISessionSSE(t, cliEvents, apiEvents)
}

func TestNamedSubagentResponseStream_APIInvocationMatchesCLIResponseStreamTerminal(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI/API named @you/subagent response-stream terminal parity smoke")
	}

	factoryDir := materializeNamedSubagentFactoryForSmoke(t)
	mockWorkersPath := writePackagedSubagentMockWorkers(t)
	server := startNamedGoalRoutingAPIServer(t, factoryDir, mockWorkersPath)

	requestText := fmt.Sprintf("functional-smoke-subagent-api-cli-stream-parity-%d", time.Now().UnixNano())
	apiResponse := postNamedGoalRoutingInvocationOnServer(t, server, requestText)

	streamStdout, streamStderr, err := runNamedSubagentResponseStreamInvocationCLI(t, mockWorkersPath, true, requestText)
	if err != nil {
		t.Fatalf("CLI JSON response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, streamStdout, streamStderr)
	}
	records, err := parseNamedGoalResponseStreamNDJSONRecords(streamStdout)
	if err != nil {
		t.Fatalf("parse response-stream NDJSON: %v\nstdout:\n%s", err, streamStdout)
	}
	streamTerminal, err := namedGoalResponseStreamTerminalInvocation(records)
	if err != nil {
		t.Fatalf("response-stream terminal invocation: %v\nstdout:\n%s", err, streamStdout)
	}

	assertNamedGoalInvocationTerminalOutcomeParity(t, apiResponse, streamTerminal)
}

func TestNamedSubagentResponseStream_APISSEMatchesCLIResponseEventNDJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI/API named @you/subagent response-event canonical parity smoke")
	}

	factoryDir := materializeNamedSubagentFactoryForSmoke(t)
	mockWorkersPath := writePackagedSubagentMockWorkers(t)
	server := startNamedGoalRoutingAPIServer(t, factoryDir, mockWorkersPath)
	requestText := fmt.Sprintf("functional-smoke-subagent-api-cli-response-event-parity-%d", time.Now().UnixNano())

	apiEvents := collectLiveAPISessionResponseEventsDuringInvocation(t, server, requestText)

	streamStdout, streamStderr, err := runNamedSubagentResponseStreamInvocationCLI(t, mockWorkersPath, true, requestText)
	if err != nil {
		t.Fatalf("CLI JSON response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, streamStdout, streamStderr)
	}
	records, err := parseNamedGoalResponseStreamNDJSONRecords(streamStdout)
	if err != nil {
		t.Fatalf("parse response-stream NDJSON: %v\nstdout:\n%s", err, streamStdout)
	}
	cliEvents := extractResponseEventsFromCLIRecords(records)

	assertCLIResponseEventsMatchLiveAPISessionSSE(t, cliEvents, apiEvents)
}

func materializeNamedSubagentFactoryForSmoke(t *testing.T) string {
	t.Helper()

	return support.InstallPackagedFactory(
		t,
		t.TempDir(),
		factorydefinitions.PackagedSubagentFactoryName,
	)
}

func collectLiveAPISessionResponseEventsDuringInvocation(
	t *testing.T,
	server *support.FunctionalAPIServer,
	goalText string,
) []factorysessions.FactoryResponseEvent {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	eventsCh := make(chan []factorysessions.FactoryResponseEvent, 1)
	errCh := make(chan error, 1)
	go func() {
		events, err := drainLiveSessionResponseEventsFromSSE(ctx, server.URL())
		if err != nil {
			errCh <- err
			return
		}
		eventsCh <- events
	}()

	time.Sleep(100 * time.Millisecond)
	postNamedGoalRoutingInvocationOnServer(t, server, goalText)
	time.Sleep(2 * time.Second)
	cancel()

	select {
	case events := <-eventsCh:
		return events
	case err := <-errCh:
		t.Fatalf("read live API response-event SSE: %v", err)
		return nil
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for live API response-event SSE drain")
		return nil
	}
}

func drainLiveSessionResponseEventsFromSSE(
	ctx context.Context,
	serverURL string,
) ([]factorysessions.FactoryResponseEvent, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		support.DefaultSessionResponseEventsURL(serverURL),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("new response-event request: %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("GET response-events: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("GET response-events status = %d: %s", response.StatusCode, string(payload))
	}
	if got := response.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		return nil, fmt.Errorf("Content-Type = %q, want text/event-stream", got)
	}

	reader := bufio.NewReader(response.Body)
	events := make([]factorysessions.FactoryResponseEvent, 0, 8)
	for {
		event, err := readSessionResponseEventSSE(reader)
		if err != nil {
			if err == io.EOF || errorsIsContextDone(err) {
				break
			}
			return events, err
		}
		events = append(events, event)
	}
	return events, nil
}

func errorsIsContextDone(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "context canceled") ||
		strings.Contains(err.Error(), "context deadline exceeded"))
}

func extractResponseEventsFromCLIRecords(
	records []namedGoalResponseStreamParsedRecord,
) []factorysessions.FactoryResponseEvent {
	events := make([]factorysessions.FactoryResponseEvent, 0, len(records))
	for _, record := range records {
		if record.RecordType != namedGoalResponseStreamJSONRecordResponseEvent {
			continue
		}
		events = append(events, record.Event)
	}
	return events
}

func assertCLIResponseEventsMatchLiveAPISessionSSE(
	t *testing.T,
	cliEvents []factorysessions.FactoryResponseEvent,
	apiEvents []factorysessions.FactoryResponseEvent,
) {
	t.Helper()

	if len(cliEvents) == 0 {
		t.Fatal("CLI response_event records = 0, want at least one canonical response event")
	}
	if len(apiEvents) == 0 {
		t.Fatalf(
			"live API response-event SSE records = 0, want at least one canonical response event (CLI had %d)",
			len(cliEvents),
		)
	}

	for index, apiEvent := range apiEvents {
		assertAPIResponseEventMatchesSSEFrameContract(t, index, apiEvent)
		assertAPIResponseEventMatchesCLIResponseEventSemantics(t, apiEvent, cliEvents)
	}
}

func assertAPIResponseEventMatchesSSEFrameContract(
	t *testing.T,
	index int,
	event factorysessions.FactoryResponseEvent,
) {
	t.Helper()

	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal API response_event[%d]: %v", index, err)
	}
	frame := fmt.Sprintf("id: %d\ndata: %s\n\n", event.Sequence, payload)
	got, err := readSessionResponseEventSSE(bufio.NewReader(bytes.NewReader([]byte(frame))))
	if err != nil {
		t.Fatalf("decode API SSE payload for response_event[%d]: %v", index, err)
	}
	if got.Kind != event.Kind || got.Phase != event.Phase {
		t.Fatalf(
			"API response_event[%d] kind/phase mismatch after SSE round-trip: got %s/%s, want %s/%s",
			index,
			got.Kind,
			got.Phase,
			event.Kind,
			event.Phase,
		)
	}
	if normalizedResponseEventPayload(got.Payload) != normalizedResponseEventPayload(event.Payload) {
		t.Fatalf(
			"API response_event[%d] payload mismatch after SSE round-trip:\nwant=%s\ngot=%s",
			index,
			normalizedResponseEventPayload(event.Payload),
			normalizedResponseEventPayload(got.Payload),
		)
	}
}

func assertAPIResponseEventMatchesCLIResponseEventSemantics(
	t *testing.T,
	apiEvent factorysessions.FactoryResponseEvent,
	cliEvents []factorysessions.FactoryResponseEvent,
) {
	t.Helper()

	apiFingerprint := responseEventSemanticsFingerprint(apiEvent)
	for _, cliEvent := range cliEvents {
		if responseEventSemanticsFingerprint(cliEvent) == apiFingerprint {
			return
		}
	}
	t.Fatalf(
		"live API response event kind=%s phase=%s payload=%s did not match any CLI response_event semantics",
		apiEvent.Kind,
		apiEvent.Phase,
		normalizedResponseEventPayload(apiEvent.Payload),
	)
}

func responseEventSemanticsFingerprint(event factorysessions.FactoryResponseEvent) string {
	return string(event.Kind) + "/" + string(event.Phase) + "/" + normalizedResponseEventPayload(event.Payload)
}

func readSessionResponseEventSSE(reader *bufio.Reader) (factorysessions.FactoryResponseEvent, error) {
	var idLine, dataLine string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return factorysessions.FactoryResponseEvent{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		switch {
		case strings.HasPrefix(line, "id: "):
			if idLine != "" {
				return factorysessions.FactoryResponseEvent{}, fmt.Errorf("SSE message has multiple id lines")
			}
			idLine = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "data: "):
			if dataLine != "" {
				return factorysessions.FactoryResponseEvent{}, fmt.Errorf("SSE message has multiple data lines")
			}
			dataLine = strings.TrimPrefix(line, "data: ")
		default:
			return factorysessions.FactoryResponseEvent{}, fmt.Errorf("unexpected response-event SSE line %q", line)
		}
	}
	if idLine == "" || dataLine == "" {
		return factorysessions.FactoryResponseEvent{}, fmt.Errorf("SSE message id=%q data=%q, want exactly one of each", idLine, dataLine)
	}

	var event factorysessions.FactoryResponseEvent
	if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
		return factorysessions.FactoryResponseEvent{}, fmt.Errorf("decode response-event SSE data: %w", err)
	}
	if idLine != fmt.Sprint(event.Sequence) {
		return factorysessions.FactoryResponseEvent{}, fmt.Errorf("SSE id = %q, want event sequence %d", idLine, event.Sequence)
	}
	if err := factorysessions.ValidateFactoryResponseEvent(event); err != nil {
		return factorysessions.FactoryResponseEvent{}, fmt.Errorf("validate SSE response event: %w", err)
	}
	return event, nil
}

func normalizedResponseEventPayload(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return string(payload)
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return string(payload)
	}
	return string(normalized)
}
