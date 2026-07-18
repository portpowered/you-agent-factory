package runtime_api

import (
	"context"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestAPIEventReplaySmoke_PublicSessionEventsPreserveOrderedTimeline(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot: dir,
		SystemRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, shutdownErr := host.Shutdown(shutdownCtx); shutdownErr != nil {
			t.Errorf("Shutdown() error = %v", shutdownErr)
		}
	})

	stream := openRootRunFactoryEventHTTPStream(t, host)
	runStarted, first := requireFunctionalEventStreamPrelude(t, stream)

	traceID := submitGeneratedWork(t, host.Endpoint(), factoryapi.SubmitWorkRequest{
		Name:         "Event Replay Story",
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "event replay smoke",
		},
	})
	if traceID == "" {
		t.Fatal("POST /work returned an empty trace ID")
	}

	outcome := collectEventReplayOutcome(t, stream, runStarted, first)
	assertEventReplayTimeline(t, outcome, runStarted, first)
	workRequestPayload, err := outcome.workRequest.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("decode generated work request payload: %v", err)
	}
	if workRequestPayload.Works == nil || len(*workRequestPayload.Works) != 1 {
		t.Fatalf("generated WORK_REQUEST works = %#v, want one normalized work item", workRequestPayload.Works)
	}
	if len(uniqueEventTicks(outcome.events)) < 3 {
		t.Fatalf("event replay smoke used %d ticks, want at least 3: %#v", len(uniqueEventTicks(outcome.events)), eventTicks(outcome.events))
	}

	work := waitForGeneratedWorkComplete(t, host.Endpoint(), traceID, 10*time.Second)
	if len(work.Results) != 1 || stringPointerValue(work.Results[0].TraceId) != traceID {
		t.Fatalf("completed work = %#v, want one result for trace %q", work.Results, traceID)
	}
}

type eventReplayOutcome struct {
	events      []factoryapi.FactoryEvent
	workRequest factoryapi.FactoryEvent
	request     factoryapi.FactoryEvent
	response    factoryapi.FactoryEvent
}

func collectEventReplayOutcome(
	t *testing.T,
	stream *factoryEventHTTPStream,
	runStarted factoryapi.FactoryEvent,
	first factoryapi.FactoryEvent,
) eventReplayOutcome {
	t.Helper()

	outcome := eventReplayOutcome{events: []factoryapi.FactoryEvent{runStarted, first}}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && outcome.response.Id == "" {
		event := stream.next(time.Until(deadline))
		outcome.events = append(outcome.events, event)
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			outcome.workRequest = event
		case factoryapi.FactoryEventTypeDispatchRequest:
			outcome.request = event
		case factoryapi.FactoryEventTypeDispatchResponse:
			outcome.response = event
		}
	}
	if outcome.workRequest.Id == "" || outcome.request.Id == "" || outcome.response.Id == "" {
		t.Fatalf("event replay smoke missing required events: workRequest=%v request=%v response=%v", outcome.workRequest.Id != "", outcome.request.Id != "", outcome.response.Id != "")
	}
	return outcome
}

func assertEventReplayTimeline(t *testing.T, outcome eventReplayOutcome, runStarted, first factoryapi.FactoryEvent) {
	t.Helper()

	if first.Context.Tick > outcome.request.Context.Tick {
		t.Fatalf("historical replay tick %d arrived after live dispatch tick %d", first.Context.Tick, outcome.request.Context.Tick)
	}
	if !(runStarted.Context.Sequence < first.Context.Sequence &&
		first.Context.Sequence < outcome.workRequest.Context.Sequence &&
		outcome.workRequest.Context.Sequence < outcome.request.Context.Sequence &&
		outcome.request.Context.Sequence < outcome.response.Context.Sequence) {
		t.Fatalf(
			"event sequences = run_request:%d initial_structure_request:%d work_request:%d dispatch_request:%d dispatch_response:%d, want increasing",
			runStarted.Context.Sequence,
			first.Context.Sequence,
			outcome.workRequest.Context.Sequence,
			outcome.request.Context.Sequence,
			outcome.response.Context.Sequence,
		)
	}
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
