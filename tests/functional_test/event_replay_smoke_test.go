package functional_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type factoryEventHTTPStream struct {
	t      *testing.T
	cancel context.CancelFunc
	done   chan struct{}
	events chan factoryapi.FactoryEvent
	errs   chan error
}

// portos:func-length-exception owner=agent-factory reason=event-replay-functional-smoke review=2026-07-18 removal=split-runtime-recording-projection-and-api-assertions-before-next-event-replay-smoke-change
func TestEndToEndEventReplaySmoke_BackendEventsReconstructSelectedTicksForWebsiteTimeline(t *testing.T) {
	dir := scaffoldFactory(t, simplePipelineConfig())
	releaseDispatch := make(chan struct{})
	executor := &eventReplayBlockingExecutor{release: releaseDispatch}
	server := StartFunctionalServer(t, dir, false, factory.WithServiceMode(), factory.WithWorkerExecutor("worker-a", executor))

	stream := openFactoryEventHTTPStream(t, server.URL()+"/events")
	runStarted, first := requireFunctionalEventStreamPrelude(t, stream)

	traceID := submitGeneratedWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         stringPointer("Event Replay Story"),
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "event replay smoke",
		},
	})
	if traceID == "" {
		t.Fatal("POST /work returned an empty trace ID")
	}

	events := []factoryapi.FactoryEvent{runStarted, first}
	var workRequest *factoryapi.FactoryEvent
	var request *factoryapi.FactoryEvent
	var response *factoryapi.FactoryEvent
	var activeView *DashboardResponse
	released := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && response == nil {
		event := stream.next(time.Until(deadline))
		events = append(events, event)
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			candidate := event
			workRequest = &candidate
		case factoryapi.FactoryEventTypeDispatchRequest:
			candidate := event
			request = &candidate
			if !released {
				view := server.GetDashboard(t)
				activeView = &view
				close(releaseDispatch)
				released = true
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			candidate := event
			response = &candidate
		}
	}
	if !released {
		close(releaseDispatch)
	}
	if workRequest == nil || request == nil || response == nil {
		t.Fatalf("event replay smoke missing required events: workRequest=%v request=%v response=%v", workRequest != nil, request != nil, response != nil)
	}
	if first.Context.Tick > request.Context.Tick {
		t.Fatalf("historical replay tick %d arrived after live dispatch tick %d", first.Context.Tick, request.Context.Tick)
	}
	if !(runStarted.Context.Sequence < first.Context.Sequence &&
		first.Context.Sequence < workRequest.Context.Sequence &&
		workRequest.Context.Sequence < request.Context.Sequence &&
		request.Context.Sequence < response.Context.Sequence) {
		t.Fatalf(
			"event sequences = run_request:%d initial_structure_request:%d work_request:%d dispatch_request:%d dispatch_response:%d, want increasing",
			runStarted.Context.Sequence,
			first.Context.Sequence,
			workRequest.Context.Sequence,
			request.Context.Sequence,
			response.Context.Sequence,
		)
	}
	workRequestPayload, err := workRequest.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("decode generated work request payload: %v", err)
	}
	if workRequestPayload.Works == nil || len(*workRequestPayload.Works) != 1 {
		t.Fatalf("generated WORK_REQUEST works = %#v, want one normalized work item", workRequestPayload.Works)
	}
	if len(uniqueEventTicks(events)) < 3 {
		t.Fatalf("event replay smoke used %d ticks, want at least 3: %#v", len(uniqueEventTicks(events)), eventTicks(events))
	}

	if activeView == nil {
		t.Fatal("active tick dashboard was not captured before dispatch release")
	}
	if activeView.Runtime.InFlightDispatchCount != 1 {
		t.Fatalf("active tick in-flight dispatch count = %d, want 1", activeView.Runtime.InFlightDispatchCount)
	}
	if activeView.Runtime.ActiveWorkstationNodeIds == nil || len(*activeView.Runtime.ActiveWorkstationNodeIds) == 0 {
		t.Fatal("active tick graph state missing active workstation nodes")
	}

	completedView := server.GetDashboard(t)
	if completedView.Runtime.InFlightDispatchCount != 0 {
		t.Fatalf("completed tick in-flight dispatch count = %d, want 0", completedView.Runtime.InFlightDispatchCount)
	}
	if completedView.Runtime.Session.CompletedCount != 1 {
		t.Fatalf("completed tick completed count = %d, want 1", completedView.Runtime.Session.CompletedCount)
	}
	if completedView.Runtime.Session.CompletedWorkLabels == nil || len(*completedView.Runtime.Session.CompletedWorkLabels) == 0 {
		t.Fatal("completed tick missing terminal work labels")
	}
	if completedView.Runtime.Session.ProviderSessions != nil && len(*completedView.Runtime.Session.ProviderSessions) != 0 {
		t.Fatalf("completed tick provider sessions = %#v, want no provider sessions without inference response events", completedView.Runtime.Session.ProviderSessions)
	}

	work := server.ListWork(t)
	if len(work.Results) != 1 || work.Results[0].TraceId != traceID {
		t.Fatalf("completed work = %#v, want one result for trace %q", work.Results, traceID)
	}
}

func openFactoryEventHTTPStream(t *testing.T, endpoint string) *factoryEventHTTPStream {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		cancel()
		t.Fatalf("build /events request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("GET /events: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		cancel()
		t.Fatalf("GET /events status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		defer resp.Body.Close()
		cancel()
		t.Fatalf("GET /events content type = %q, want text/event-stream", resp.Header.Get("Content-Type"))
	}

	stream := &factoryEventHTTPStream{
		t:      t,
		cancel: cancel,
		done:   make(chan struct{}),
		events: make(chan factoryapi.FactoryEvent, 4096),
		errs:   make(chan error, 1),
	}
	go stream.read(resp)
	t.Cleanup(stream.close)
	return stream
}

func (s *factoryEventHTTPStream) read(resp *http.Response) {
	defer close(s.done)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var dataLines []string
	flush := func() {
		if len(dataLines) == 0 {
			return
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &event); err != nil {
			select {
			case s.errs <- fmt.Errorf("decode /events payload: %w", err):
			default:
			}
			return
		}
		select {
		case s.events <- event:
		default:
			select {
			case s.errs <- fmt.Errorf("/events test buffer overflow"):
			default:
			}
		}
		dataLines = nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "event:") {
			select {
			case s.errs <- fmt.Errorf("/events emitted named SSE event line %q", line):
			default:
			}
			return
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	flush()
	if err := scanner.Err(); err != nil && !errorsIsContextCanceled(err) {
		select {
		case s.errs <- err:
		default:
		}
	}
}

func (s *factoryEventHTTPStream) next(timeout time.Duration) factoryapi.FactoryEvent {
	s.t.Helper()
	if timeout <= 0 {
		timeout = time.Nanosecond
	}
	select {
	case event := <-s.events:
		return event
	case err := <-s.errs:
		s.t.Fatalf("/events stream error: %v", err)
	case <-time.After(timeout):
		s.t.Fatalf("timed out waiting for /events payload within %s", timeout)
	}
	return factoryapi.FactoryEvent{}
}

func (s *factoryEventHTTPStream) close() {
	s.cancel()
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
}

type eventReplayBlockingExecutor struct {
	release <-chan struct{}
}

func (e *eventReplayBlockingExecutor) Execute(ctx context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	select {
	case <-e.release:
	case <-ctx.Done():
		return interfaces.WorkResult{}, ctx.Err()
	}

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-event-replay-smoke",
		},
	}, nil
}

func uniqueEventTicks(events []factoryapi.FactoryEvent) map[int]struct{} {
	ticks := make(map[int]struct{})
	for _, event := range events {
		ticks[event.Context.Tick] = struct{}{}
	}
	return ticks
}

func eventTicks(events []factoryapi.FactoryEvent) []int {
	ticks := make([]int, 0, len(events))
	for _, event := range events {
		ticks = append(ticks, event.Context.Tick)
	}
	return ticks
}

func stringPointer(value string) *string {
	return &value
}

func stringValueFromFunctionalPtr[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func requireFunctionalEventStreamPrelude(
	t *testing.T,
	stream *factoryEventHTTPStream,
) (factoryapi.FactoryEvent, factoryapi.FactoryEvent) {
	t.Helper()

	runStarted := stream.next(5 * time.Second)
	if runStarted.Type != factoryapi.FactoryEventTypeRunRequest || runStarted.Context.Tick != 0 {
		t.Fatalf("first /events payload = %#v, want run-request at tick 0", runStarted)
	}

	initialStructure := stream.next(5 * time.Second)
	if initialStructure.Type != factoryapi.FactoryEventTypeInitialStructureRequest || initialStructure.Context.Tick != 0 {
		t.Fatalf("second /events payload = %#v, want initial structure at tick 0", initialStructure)
	}
	if initialStructure.Context.Sequence <= runStarted.Context.Sequence {
		t.Fatalf(
			"/events prelude sequences = run_request:%d initial_structure_request:%d, want increasing order",
			runStarted.Context.Sequence,
			initialStructure.Context.Sequence,
		)
	}

	return runStarted, initialStructure
}
