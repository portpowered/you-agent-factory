package smoke

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/subagent"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
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

	mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		reviewerOutput: "accepted",
	})
	goalText := fmt.Sprintf("functional-smoke-goal-api-cli-response-event-parity-%d", time.Now().UnixNano())

	streamStdout, streamStderr, err := runNamedGoalResponseStreamInvocationCLI(t, mockWorkersPath, true, goalText)
	if err != nil {
		t.Fatalf("CLI JSON response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, streamStdout, streamStderr)
	}
	records, err := parseNamedGoalResponseStreamNDJSONRecords(streamStdout)
	if err != nil {
		t.Fatalf("parse response-stream NDJSON: %v\nstdout:\n%s", err, streamStdout)
	}
	cliEvents := extractResponseEventsFromCLIRecords(records)

	assertCLIResponseEventsMatchAPISSEPayloadEncoding(t, cliEvents)
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

	mockWorkersPath := writePackagedSubagentMockWorkers(t)
	requestText := fmt.Sprintf("functional-smoke-subagent-api-cli-response-event-parity-%d", time.Now().UnixNano())

	streamStdout, streamStderr, err := runNamedSubagentResponseStreamInvocationCLI(t, mockWorkersPath, true, requestText)
	if err != nil {
		t.Fatalf("CLI JSON response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, streamStdout, streamStderr)
	}
	records, err := parseNamedGoalResponseStreamNDJSONRecords(streamStdout)
	if err != nil {
		t.Fatalf("parse response-stream NDJSON: %v\nstdout:\n%s", err, streamStdout)
	}
	cliEvents := extractResponseEventsFromCLIRecords(records)

	assertCLIResponseEventsMatchAPISSEPayloadEncoding(t, cliEvents)
}

func materializeNamedSubagentFactoryForSmoke(t *testing.T) string {
	t.Helper()

	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), subagent.PackagedFactoryName, subagent.BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory(@you/subagent): %v", err)
	}
	return dir
}

func extractResponseEventsFromCLIRecords(
	records []namedGoalResponseStreamParsedRecord,
) []responseevents.FactoryResponseEvent {
	events := make([]responseevents.FactoryResponseEvent, 0, len(records))
	for _, record := range records {
		if record.RecordType != namedGoalResponseStreamJSONRecordResponseEvent {
			continue
		}
		events = append(events, record.Event)
	}
	return events
}

func assertCLIResponseEventsMatchAPISSEPayloadEncoding(
	t *testing.T,
	cliEvents []responseevents.FactoryResponseEvent,
) {
	t.Helper()

	for index, want := range cliEvents {
		payload, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("marshal CLI response_event[%d]: %v", index, err)
		}
		frame := fmt.Sprintf("id: %d\ndata: %s\n\n", want.Sequence, payload)
		got, err := readSessionResponseEventSSE(bufio.NewReader(bytes.NewReader([]byte(frame))))
		if err != nil {
			t.Fatalf("decode API SSE payload for CLI response_event[%d]: %v", index, err)
		}
		if got.Kind != want.Kind || got.Phase != want.Phase {
			t.Fatalf(
				"response_event[%d] kind/phase mismatch after SSE round-trip: got %s/%s, want %s/%s",
				index,
				got.Kind,
				got.Phase,
				want.Kind,
				want.Phase,
			)
		}
		if normalizedResponseEventPayload(got.Payload) != normalizedResponseEventPayload(want.Payload) {
			t.Fatalf(
				"response_event[%d] payload mismatch after SSE round-trip:\nwant=%s\ngot=%s",
				index,
				normalizedResponseEventPayload(want.Payload),
				normalizedResponseEventPayload(got.Payload),
			)
		}
	}
}

func readSessionResponseEventSSE(reader *bufio.Reader) (responseevents.FactoryResponseEvent, error) {
	var idLine, dataLine string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return responseevents.FactoryResponseEvent{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		switch {
		case strings.HasPrefix(line, "id: "):
			if idLine != "" {
				return responseevents.FactoryResponseEvent{}, fmt.Errorf("SSE message has multiple id lines")
			}
			idLine = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "data: "):
			if dataLine != "" {
				return responseevents.FactoryResponseEvent{}, fmt.Errorf("SSE message has multiple data lines")
			}
			dataLine = strings.TrimPrefix(line, "data: ")
		default:
			return responseevents.FactoryResponseEvent{}, fmt.Errorf("unexpected response-event SSE line %q", line)
		}
	}
	if idLine == "" || dataLine == "" {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("SSE message id=%q data=%q, want exactly one of each", idLine, dataLine)
	}

	var event responseevents.FactoryResponseEvent
	if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("decode response-event SSE data: %w", err)
	}
	if idLine != fmt.Sprint(event.Sequence) {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("SSE id = %q, want event sequence %d", idLine, event.Sequence)
	}
	if err := responseevents.ValidateEvent(event); err != nil {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("validate SSE response event: %w", err)
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
