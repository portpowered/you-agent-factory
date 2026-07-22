package runtime_api

import (
	"context"
	"testing"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// portos:func-length-exception owner=agent-factory reason=event-replay-functional-smoke review=2026-07-18 removal=split-runtime-recording-projection-and-api-assertions-before-next-event-replay-smoke-change
func TestAPIEventReplaySmoke_PublicEventsAndSessionProjectionExposeActiveAndCompletedTimeline(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	releaseDispatch := make(chan struct{})
	provider := &eventReplayBlockingProvider{release: releaseDispatch}
	server := startFunctionalServer(t, dir, false, withProvider(provider))

	stream := openDefaultSessionFactoryEventHTTPStream(t, server.URL())
	runStarted, first := requireFunctionalEventStreamPrelude(t, stream)

	traceID := submitGeneratedWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         stringPtr("Event Replay Story"),
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "event replay smoke",
		},
	})
	if traceID == "" {
		t.Fatal("POST /work returned an empty trace ID")
	}

	outcome := collectEventReplayOutcome(t, server, stream, releaseDispatch, runStarted, first)
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

	assertEventReplayActiveSession(t, outcome.activeSession)
	assertEventReplayCompletedSession(t, support.GetDefaultSession(t, server.URL()))

	work := server.ListWork(t)
	if len(work.Results) != 1 || stringPointerValue(work.Results[0].TraceId) != traceID {
		t.Fatalf("completed work = %#v, want one result for trace %q", work.Results, traceID)
	}
}

type eventReplayOutcome struct {
	events        []factoryapi.FactoryEvent
	workRequest   factoryapi.FactoryEvent
	request       factoryapi.FactoryEvent
	response      factoryapi.FactoryEvent
	activeSession factoryapi.FactorySession
}

func collectEventReplayOutcome(
	t *testing.T,
	server *functionalAPIServer,
	stream *factoryEventHTTPStream,
	releaseDispatch chan struct{},
	runStarted factoryapi.FactoryEvent,
	first factoryapi.FactoryEvent,
) eventReplayOutcome {
	t.Helper()

	outcome := eventReplayOutcome{events: []factoryapi.FactoryEvent{runStarted, first}}
	released := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && outcome.response.Id == "" {
		event := stream.next(time.Until(deadline))
		outcome.events = append(outcome.events, event)
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			outcome.workRequest = event
		case factoryapi.FactoryEventTypeDispatchRequest:
			outcome.request = event
			if !released {
				outcome.activeSession = support.GetDefaultSession(t, server.URL())
				close(releaseDispatch)
				released = true
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			outcome.response = event
		}
	}
	if !released {
		close(releaseDispatch)
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

func assertEventReplayActiveSession(t *testing.T, session factoryapi.FactorySession) {
	t.Helper()

	if session.Id == "" {
		t.Fatal("active Factory Session projection has an empty ID")
	}
}

func assertEventReplayCompletedSession(t *testing.T, session factoryapi.FactorySession) {
	t.Helper()

	if session.Runtime.Progress.Categories.Processing != 0 {
		t.Fatalf("completed Factory Session processing count = %d, want 0", session.Runtime.Progress.Categories.Processing)
	}
	if session.Runtime.Progress.Categories.Terminal != 1 {
		t.Fatalf("completed Factory Session terminal count = %d, want 1", session.Runtime.Progress.Categories.Terminal)
	}
}

type eventReplayBlockingProvider struct {
	release <-chan struct{}
}

func (p *eventReplayBlockingProvider) Infer(
	ctx context.Context,
	_ workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	select {
	case <-p.release:
	case <-ctx.Done():
		return workerexecution.InferenceResponse{}, ctx.Err()
	}

	return workerexecution.InferenceResponse{
		Content: "completed",
		ProviderSession: &workerexecution.ProviderSessionMetadata{
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
