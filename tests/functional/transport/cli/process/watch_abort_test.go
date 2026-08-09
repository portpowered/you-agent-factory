package process_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// TestCLIWorkWatchStreamAbortReturnsNonZeroExit proves a genuine Work-watch
// stream failure crosses the customer process boundary: complete transition
// lines remain on stdout, the abort is actionable on stderr, and the command
// is unsuccessful instead of being converted to a successful process result.
func TestCLIWorkWatchStreamAbortReturnsNonZeroExit(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)
	fixturePayloads := processWatchAbortPayloads(t)
	streamServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set(factorysessions.SessionEventStreamRetainedCountHeader, "0")
		writer.Header().Set("Cache-Control", "no-cache")
		flusher, ok := writer.(http.Flusher)
		if !ok {
			http.Error(writer, "watch fixture response writer does not support flushing", http.StatusInternalServerError)
			return
		}

		for _, payload := range fixturePayloads {
			_, _ = fmt.Fprintf(writer, "data: %s\n\n", payload)
			flusher.Flush()
		}
		_, _ = fmt.Fprint(writer, "data: {\"id\":\"stream-abort")
		flusher.Flush()
	}))
	defer streamServer.Close()
	session.ServerURL = streamServer.URL

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	args := append(session.ServerFlags(), "work", "watch", "--session", "session-abort")
	result, err := session.Run(ctx, args...)
	if err == nil {
		t.Fatalf("aborted Work watch result = %#v; want process failure", result)
	}
	if result.ExitCode == 0 {
		t.Fatalf("aborted Work watch exit code = %d; want non-zero", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "session-abort") ||
		!strings.Contains(result.Stderr, "decode canonical Factory Event SSE data") {
		t.Fatalf("stderr = %q; want session and stream-abort context", result.Stderr)
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
}

func processWatchAbortPayloads(t *testing.T) [][]byte {
	t.Helper()
	events := processWatchAbortEvents(t)
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
