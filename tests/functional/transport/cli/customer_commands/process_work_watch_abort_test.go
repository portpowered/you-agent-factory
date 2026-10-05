package customer_commands_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// TestCLIWorkWatchStreamAbortReturnsNonZeroExit proves a genuine Work-watch
// stream failure crosses the in-process customer process boundary: complete
// transition lines remain on stdout, the abort is actionable on stderr, and
// the command is unsuccessful instead of being converted to a success.
func testProcessCLIWorkWatchStreamAbortReturnsNonZeroExit(t *testing.T) {
	t.Parallel()
	process := plainProcess(t)
	session := newCLIHome(t)
	successSession := newCLIHome(t)
	fixturePayloads := processWatchAbortPayloads(t)
	finitePayloads := processWatchFinitePayloads(t)
	streamServer := newProcessWatchAbortServer(t, fixturePayloads, finitePayloads)
	defer streamServer.Close()

	result := session.run(t, process, nil, "--server", streamServer.URL, "work", "watch", "--session", "session-abort")
	if result.Err == nil {
		t.Fatalf("aborted Work watch result = %#v; want process failure", result)
	}
	var diagnostic struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stderr)), &diagnostic); err != nil {
		t.Fatalf("decode safe Work-watch diagnostic: %v; stderr=%q", err, result.Stderr)
	}
	if diagnostic.Code != "CLI_COMMAND_FAILED" || diagnostic.Message != "command failed" {
		t.Fatalf("Work-watch diagnostic = %#v, want safe CLI_COMMAND_FAILED diagnostic", diagnostic)
	}
	if strings.Count(result.Stderr, "CLI_COMMAND_FAILED") != 1 {
		t.Fatalf("stderr = %q; want exactly one coded diagnostic", result.Stderr)
	}
	if strings.Contains(result.Stdout, "stream-abort") {
		t.Fatalf("stdout contains incomplete abort payload: %q", result.Stdout)
	}

	scanner := bufio.NewScanner(strings.NewReader(result.Stdout))
	var line struct {
		SchemaVersion string `json:"schemaVersion"`
		EventID       string `json:"eventId"`
		Terminal      bool   `json:"terminal"`
	}
	if !scanner.Scan() {
		t.Fatalf("stdout = %q; want the complete transition emitted before the abort", result.Stdout)
	}
	if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
		t.Fatalf("decode complete stdout transition: %v; stdout=%q", err, result.Stdout)
	}
	if scanner.Scan() {
		t.Fatalf("stdout = %q; want exactly one complete transition before abort", result.Stdout)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan Work-watch stdout: %v", err)
	}
	if line.SchemaVersion != "you.work.watch.v1" || line.EventID != "move-before-abort" || line.Terminal {
		t.Fatalf("stdout transition = %#v; want non-terminal you.work.watch.v1 move-before-abort", line)
	}

	successResult := successSession.run(t, process, nil, "--server", streamServer.URL, "work", "watch", "--session", "session-success")
	if successResult.Err != nil {
		t.Fatalf("successful finite Work watch result = %#v; want success", successResult)
	}
}

func processWatchAbortPayloads(t *testing.T) [][]byte {
	t.Helper()
	events := processWatchAbortEvents(t)
	return marshalProcessWatchEvents(t, events)
}

func processWatchFinitePayloads(t *testing.T) [][]byte {
	t.Helper()
	events := processWatchAbortEvents(t)
	events[2] = processWatchFactoryEvent(t, factoryapi.FactoryEventTypeWorkStateChange, "move-finite", 3, factoryapi.WorkStateChangeEventPayload{
		WorkId:       "work-before-abort",
		WorkTypeName: "task",
		FromState:    "ready",
		ToState:      "done",
		Source:       factoryapi.WorkStateChangeSourceCLI,
		Reason:       processWatchStringPtr("finished"),
	})
	return marshalProcessWatchEvents(t, events)
}

func marshalProcessWatchEvents(t *testing.T, events []factoryapi.FactoryEvent) [][]byte {
	t.Helper()
	payloads := make([][]byte, 0, len(events))
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal watch fixture event %q: %v", event.Id, err)
		}
		payloads = append(payloads, payload)
	}
	return payloads
}

func processWatchAbortEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	metadataPayload := factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{
		WorkTypes: &[]factoryapi.WorkType{{Name: "task", States: []factoryapi.WorkState{
			{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL},
			{Name: "processing", Type: factoryapi.WorkStateTypePROCESSING},
			{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL},
		}}},
	}}
	requestPayload := factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{{
		WorkId:       processWatchStringPtr("work-before-abort"),
		WorkTypeName: processWatchStringPtr("task"),
		State:        &factoryapi.WorkState{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL},
	}}}
	transitionPayload := factoryapi.WorkStateChangeEventPayload{
		WorkId:       "work-before-abort",
		WorkTypeName: "task",
		FromState:    "ready",
		ToState:      "processing",
		Source:       factoryapi.WorkStateChangeSourceCLI,
	}
	return []factoryapi.FactoryEvent{
		processWatchFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory", 1, metadataPayload),
		processWatchFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "request", 2, requestPayload),
		processWatchFactoryEvent(t, factoryapi.FactoryEventTypeWorkStateChange, "move-before-abort", 3, transitionPayload),
	}
}

func processWatchFactoryEvent(t *testing.T, eventType factoryapi.FactoryEventType, id string, sequence int, payload any) factoryapi.FactoryEvent {
	t.Helper()
	var union factoryapi.FactoryEvent_Payload
	var err error
	switch typed := payload.(type) {
	case factoryapi.InitialStructureRequestEventPayload:
		err = union.FromInitialStructureRequestEventPayload(typed)
	case factoryapi.WorkRequestEventPayload:
		err = union.FromWorkRequestEventPayload(typed)
	case factoryapi.WorkStateChangeEventPayload:
		err = union.FromWorkStateChangeEventPayload(typed)
	default:
		t.Fatalf("unsupported Work-watch fixture payload %T", payload)
	}
	if err != nil {
		t.Fatalf("encode Work-watch fixture event %q: %v", id, err)
	}
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          eventType,
		Id:            id,
		Context: factoryapi.FactoryEventContext{
			EventTime: time.Date(2026, time.August, 9, 13, 0, sequence, 0, time.UTC),
			Sequence:  sequence,
		},
		Payload: union,
	}
}

func processWatchStringPtr(value string) *string { return &value }

func newProcessWatchAbortServer(t *testing.T, fixturePayloads, finitePayloads [][]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Cache-Control", "no-cache")
		flusher, ok := writer.(http.Flusher)
		if !ok {
			http.Error(writer, "watch fixture response writer does not support flushing", http.StatusInternalServerError)
			return
		}

		payloads := fixturePayloads
		if strings.Contains(request.URL.Path, "session-success") {
			payloads = finitePayloads
		}
		writer.Header().Set(factorysessions.SessionEventStreamRetainedCountHeader, strconv.Itoa(len(payloads)))
		for _, payload := range payloads {
			_, _ = fmt.Fprintf(writer, "data: %s\n\n", payload)
			flusher.Flush()
		}
		if strings.Contains(request.URL.Path, "session-abort") {
			_, _ = fmt.Fprint(writer, "data: {\"id\":\"stream-abort")
			flusher.Flush()
		}
	}))
}
