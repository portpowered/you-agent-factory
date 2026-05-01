package functional_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	runcli "github.com/portpowered/agent-factory/pkg/cli/run"
	"github.com/portpowered/agent-factory/pkg/interfaces"
)

func replayDispatchCompletedEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []factoryapi.DispatchResponseEventPayload {
	t.Helper()

	var out []factoryapi.DispatchResponseEventPayload
	for _, event := range artifact.Events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch completed event %q: %v", event.Id, err)
		}
		out = append(out, payload)
	}
	return out
}

type recordedFactoryWorkRequestEvent struct {
	RequestID string
	Source    string
	Payload   factoryapi.WorkRequestEventPayload
}

func replayEventCount(artifact *interfaces.ReplayArtifact, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range artifact.Events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func assertReplayWorkRequestRecorded(t *testing.T, artifact *interfaces.ReplayArtifact, requestID, source string, workItems int, relations int) {
	t.Helper()

	for _, record := range replayWorkRequestEvents(t, artifact) {
		if record.RequestID != requestID {
			continue
		}
		if record.Source != source {
			t.Fatalf("work request %s source = %q, want %q", requestID, record.Source, source)
		}
		if got := len(factoryWorksValue(record.Payload.Works)); got != workItems {
			t.Fatalf("work request %s work items = %d, want %d", requestID, got, workItems)
		}
		if got := len(factoryRelationsValue(record.Payload.Relations)); got != relations {
			t.Fatalf("work request %s relations = %d, want %d", requestID, got, relations)
		}
		return
	}
	t.Fatalf("replay artifact missing work request %s: %#v", requestID, replayWorkRequestEvents(t, artifact))
}

func replayWorkRequestEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []recordedFactoryWorkRequestEvent {
	if t != nil {
		t.Helper()
	}
	return replayWorkRequestEventsFromEvents(t, artifact.Events)
}

func replayWorkRequestEventsFromEvents(t *testing.T, events []factoryapi.FactoryEvent) []recordedFactoryWorkRequestEvent {
	if t != nil {
		t.Helper()
	}

	var out []recordedFactoryWorkRequestEvent
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			if t == nil {
				panic(err)
			}
			t.Fatalf("decode work request event %q: %v", event.Id, err)
		}
		source := stringPointerValue(payload.Source)
		if source == "" {
			source = stringPointerValue(event.Context.Source)
		}
		out = append(out, recordedFactoryWorkRequestEvent{
			RequestID: stringPointerValue(event.Context.RequestId),
			Source:    source,
			Payload:   payload,
		})
	}
	return out
}

func factoryWorksValue(value *[]factoryapi.Work) []factoryapi.Work {
	if value == nil {
		return nil
	}
	return *value
}

func factoryRelationsValue(value *[]factoryapi.Relation) []factoryapi.Relation {
	if value == nil {
		return nil
	}
	return *value
}

func stringPointerValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func lastFactoryEventTick(events []factoryapi.FactoryEvent) int {
	tick := 0
	for _, event := range events {
		if event.Context.Tick > tick {
			tick = event.Context.Tick
		}
	}
	return tick
}

func runRecordReplayCLIWithCapturedStdout(t *testing.T, cfg runcli.RunConfig) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}

	readCh := make(chan []byte, 1)
	readErrCh := make(chan error, 1)
	go func() {
		data, readErr := io.ReadAll(readPipe)
		readCh <- data
		readErrCh <- readErr
	}()

	os.Stdout = writePipe
	runErr := runcli.Run(context.Background(), cfg)
	os.Stdout = oldStdout

	if err := writePipe.Close(); err != nil {
		t.Fatalf("close captured stdout writer: %v", err)
	}
	output := <-readCh
	if err := <-readErrCh; err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if err := readPipe.Close(); err != nil {
		t.Fatalf("close captured stdout reader: %v", err)
	}

	return string(output), runErr
}

type scriptBoundaryEventIndices struct {
	dispatch  int
	request   int
	response  int
	completed int
}

func requireScriptResponseEventIndices(t *testing.T, events []factoryapi.FactoryEvent) scriptBoundaryEventIndices {
	t.Helper()

	indices := scriptBoundaryEventIndices{
		dispatch:  indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchRequest, 0),
		request:   indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeScriptRequest, 0),
		response:  indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeScriptResponse, 0),
		completed: indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchResponse, 0),
	}
	if indices.dispatch < 0 || indices.request < 0 || indices.response < 0 || indices.completed < 0 {
		t.Fatalf("event order = %v, want dispatch-request, script-request, script-response, dispatch-response", functionalEventTypes(events))
	}
	return indices
}

func assertFunctionalScriptEventDoesNotLeak(t *testing.T, event factoryapi.FactoryEvent, forbidden []string) {
	t.Helper()

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal script event: %v", err)
	}
	body := string(encoded)
	for _, value := range forbidden {
		if strings.Contains(body, value) {
			t.Fatalf("script event leaked %s: %s", value, body)
		}
	}
}

func assertScriptEventsRecordedInArtifact(t *testing.T, liveEvents []factoryapi.FactoryEvent, recordedEvents []factoryapi.FactoryEvent) {
	t.Helper()

	recordedByID := make(map[string]factoryapi.FactoryEvent, len(recordedEvents))
	for _, event := range recordedEvents {
		recordedByID[event.Id] = event
	}

	for _, live := range liveEvents {
		if live.Type != factoryapi.FactoryEventTypeScriptRequest && live.Type != factoryapi.FactoryEventTypeScriptResponse {
			continue
		}

		recorded, ok := recordedByID[live.Id]
		if !ok {
			t.Fatalf("recorded artifact missing script event %s from live history; artifact events=%v", live.Id, functionalEventTypes(recordedEvents))
		}
		if recorded.Type != live.Type {
			t.Fatalf("recorded script event %s = type %s, live type %s", live.Id, recorded.Type, live.Type)
		}

		liveJSON, err := json.Marshal(live)
		if err != nil {
			t.Fatalf("marshal live script event %s: %v", live.Id, err)
		}
		recordedJSON, err := json.Marshal(recorded)
		if err != nil {
			t.Fatalf("marshal recorded script event %s: %v", recorded.Id, err)
		}
		if string(recordedJSON) != string(liveJSON) {
			t.Fatalf("recorded script event %s does not match live history\nrecorded=%s\nlive=%s", live.Id, recordedJSON, liveJSON)
		}
	}
}

func normalizeFunctionalStdout(stdout string, trim bool) string {
	if trim {
		return strings.TrimSpace(stdout)
	}
	return stdout
}

func collectUnifiedSmokeEventsUntilRunResponse(t *testing.T, stream *factoryEventHTTPStream, initialEvents []factoryapi.FactoryEvent, timeout time.Duration) []factoryapi.FactoryEvent {
	t.Helper()

	events := append([]factoryapi.FactoryEvent(nil), initialEvents...)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		event := stream.next(time.Until(deadline))
		events = append(events, event)
		if event.Type == factoryapi.FactoryEventTypeRunResponse {
			return events
		}
	}
	t.Fatalf("timed out waiting for RUN_RESPONSE in live /events timeline: %#v", unifiedSmokeEventSummaries(events))
	return nil
}

func assertLiveEventsMatchRecordedArtifact(t *testing.T, liveEvents []factoryapi.FactoryEvent, artifact *interfaces.ReplayArtifact) {
	t.Helper()

	recordedByID := make(map[string]factoryapi.FactoryEvent, len(artifact.Events))
	for _, event := range artifact.Events {
		recordedByID[event.Id] = event
	}
	for _, live := range liveEvents {
		recorded, ok := recordedByID[live.Id]
		if !ok {
			t.Fatalf("live event %s (%s) missing from recorded artifact events: %#v", live.Id, live.Type, unifiedSmokeEventSummaries(artifact.Events))
		}
		if recorded.Type != live.Type || recorded.Context.Tick != live.Context.Tick {
			t.Fatalf("recorded event %s = type %s tick %d, live type %s tick %d", live.Id, recorded.Type, recorded.Context.Tick, live.Type, live.Context.Tick)
		}
		if unifiedSmokeDispatchID(recorded) != unifiedSmokeDispatchID(live) {
			t.Fatalf("recorded event %s dispatch id = %q, live dispatch id = %q", live.Id, unifiedSmokeDispatchID(recorded), unifiedSmokeDispatchID(live))
		}
		if strings.Join(unifiedSmokeWorkIDs(recorded), ",") != strings.Join(unifiedSmokeWorkIDs(live), ",") {
			t.Fatalf("recorded event %s work ids = %#v, live work ids = %#v", live.Id, unifiedSmokeWorkIDs(recorded), unifiedSmokeWorkIDs(live))
		}
	}
}

func maxUnifiedSmokeTick(events []factoryapi.FactoryEvent) int {
	maxTick := 0
	for _, event := range events {
		if event.Context.Tick > maxTick {
			maxTick = event.Context.Tick
		}
	}
	return maxTick
}

func unifiedSmokeDispatchID(event factoryapi.FactoryEvent) string {
	if event.Context.DispatchId != nil {
		return *event.Context.DispatchId
	}
	return ""
}

func unifiedSmokeWorkIDs(event factoryapi.FactoryEvent) []string {
	if event.Context.WorkIds == nil {
		return nil
	}
	out := make([]string, len(*event.Context.WorkIds))
	copy(out, *event.Context.WorkIds)
	return out
}

func unifiedSmokeEventSummaries(events []factoryapi.FactoryEvent) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, string(event.Type)+"@"+event.Id)
	}
	return out
}

func lastIndexOfFunctionalEventType(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == eventType {
			return i
		}
	}
	return -1
}
