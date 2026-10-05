package response_events

import (
	"context"
	"fmt"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// One inert canonical graph serves the four public response-event cases.
// The HTTP boundary owns SSE replay and observer cancellation; the command
// and logger edges are controlled, with isolated local customer profiles.
func TestFactoryResponseEventsSurviveTheEventsAuthoritativePublishPath(t *testing.T) {
	t.Parallel()
	core, diagnostics := observer.New(zapcore.DebugLevel)
	runner := &eventsLoggerCodexRunner{}
	host := support.ScaffoldFactory(t, concurrentIsolationFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: host, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, ProcessLogger: zap.New(core)},
		BeforeStart: func(tb testing.TB, process support.Process, input root.Input) {
			support.InitializeCustomerHomeWithProcess(tb, process, input.Env, input.WorkingDirectory)
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	for _, mode := range []string{"success", "failure"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			assertSelectedEventsLoggerJourney(t, server.URL(), mode, diagnostics)
		})
	}
}

// The immutable prompt markers route effects to the owned session. No mutable
// package switch or lock covers a customer invocation.
type eventsLoggerCodexRunner struct{}

func (*eventsLoggerCodexRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	observed := string(request.Stdin) + "\n" + strings.Join(request.Args, "\n")
	for _, mode := range []string{"success", "failure"} {
		for _, role := range []string{"first", "peer"} {
			marker := "events-logger-" + mode + "-" + role
			if !strings.Contains(observed, marker) {
				continue
			}
			if mode == "failure" && role == "first" {
				return platformprocess.CommandResult{}, fmt.Errorf("events-private-failure-content")
			}
			return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(mode + " " + role + " private-result COMPLETE")}, nil
		}
	}
	return platformprocess.CommandResult{}, fmt.Errorf("provider received no owned Events logger prompt")
}

func (r *eventsLoggerCodexRunner) RunStreaming(ctx context.Context, request platformprocess.CommandRequest, observer platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	result, err := r.Run(ctx, request)
	if observer != nil && len(result.Stdout) > 0 {
		observer(platformprocess.OutputStreamStdout, result.Stdout)
	}
	return result, err
}

func assertSelectedEventsLoggerJourney(t *testing.T, baseURL, mode string, diagnostics *observer.ObservedLogs) {
	t.Helper()
	markers := [2]string{"events-logger-" + mode + "-first", "events-logger-" + mode + "-peer"}
	outputs := [2]string{mode + " first private-result", mode + " peer private-result"}
	var ids [2]string
	var streams [2]*support.FactoryResponseEventStream
	for i, marker := range markers {
		opened := support.OpenFactorySessionAt(t, baseURL, scaffoldConcurrentIsolationFactory(t, marker))
		ids[i] = opened.Session.Id
		if ids[i] == "" || ids[i] == "~default" || (i > 0 && ids[0] == ids[i]) {
			t.Fatalf("expected distinct explicit sessions: %v", ids)
		}
		id := ids[i]
		t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, id) })
		streams[i] = support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(baseURL, id))
	}
	invocations := startConcurrentIsolationInvocations(t, baseURL, map[string]string{ids[0]: markers[0], ids[1]: markers[1]})
	first := awaitConcurrentIsolationInvocation(t, invocations[ids[0]])
	if mode == "failure" {
		assertEventsLoggerFailedInvocation(t, baseURL, ids[0], first)
	} else {
		assertConcurrentIsolationInvocationCompleted(t, first, outputs[0])
		assertEventsLoggerCompletedWork(t, baseURL, ids[0])
	}
	retained := retainedFactoryResponseEventsWithoutGaps(support.GetFactoryResponseEventsAt(t, baseURL, ids[0]))
	firstFrames := collectResponseEventStreamUntilCount(t, streams[0], len(retained), concurrentIsolationTimeout)
	assertResponseEventFramesMatchRetainedCatchUp(t, retained, firstFrames)
	// Cancel only this observer, leaving its history and the peer runtime intact.
	streams[0].Close()
	streams[0].WaitClosed(concurrentIsolationTimeout)
	canceled := false
	for range len(firstFrames) + 1 {
		result := streams[0].TryNextFrameResult(concurrentIsolationTimeout)
		if result.Outcome == support.FactoryResponseEventStreamOutcomeCanceled {
			canceled = true
			break
		}
		if result.Outcome != support.FactoryResponseEventStreamOutcomeFrame {
			t.Fatalf("observer cancel = %+v", result)
		}
	}
	if !canceled {
		t.Fatal("closed observer never reported client cancellation")
	}
	if mode == "success" {
		assertResponseEventStreamResumesFromCursor(t, baseURL, ids[0], firstFrames)
	}
	peer := awaitConcurrentIsolationInvocation(t, invocations[ids[1]])
	assertConcurrentIsolationInvocationCompleted(t, peer, outputs[1])
	assertEventsLoggerCompletedWork(t, baseURL, ids[1])
	peerRetained := retainedFactoryResponseEventsWithoutGaps(support.GetFactoryResponseEventsAt(t, baseURL, ids[1]))
	peerFrames := collectResponseEventStreamUntilCount(t, streams[1], len(peerRetained), concurrentIsolationTimeout)
	assertResponseEventFramesMatchRetainedCatchUp(t, peerRetained, peerFrames)
	assertSessionScopedOrderedTypedPayloads(t, ids[1], responseEventsFromFrames(peerFrames), outputs[1], outputs[0])
	if mode == "success" {
		assertSessionScopedOrderedTypedPayloads(t, ids[0], retained, outputs[0], outputs[1])
	}
	assertResponseEventStreamResumesFromCursor(t, baseURL, ids[1], peerFrames)
	for _, id := range ids {
		assertSelectedEventsDiagnostics(t, id, retainedFactoryResponseEventsWithoutGaps(support.GetFactoryResponseEventsAt(t, baseURL, id)), diagnostics)
	}
}

func assertEventsLoggerCompletedWork(t *testing.T, baseURL, id string) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, id, "/work"))
	if len(listed.Results) != 1 || listed.Results[0].WorkId == nil || listed.Results[0].State == nil || listed.Results[0].State.Name != "complete" {
		t.Fatalf("completed Work = %#v", listed)
	}
}

func assertEventsLoggerFailedInvocation(t *testing.T, baseURL, id string, result concurrentIsolationInvocation) {
	t.Helper()
	if result.err != nil || result.response.Status != factoryapi.InvocationTerminalStatusFailed || result.response.PrimaryResult != nil {
		t.Fatalf("failed invocation = %+v", result)
	}
	retained := retainedFactoryResponseEventsWithoutGaps(support.GetFactoryResponseEventsAt(t, baseURL, id))
	assertResponseEventsAscendingSequence(t, retained)
	var failed bool
	for _, event := range retained {
		if event.FactorySessionId != id {
			t.Fatalf("failed event leaked peer identity: %+v", event)
		}
		if event.Kind == factoryapi.FactoryResponseEventKindError && event.Phase == factoryapi.FactoryResponseEventPhaseFailed {
			payload, err := event.Payload.AsFactoryResponseEventErrorPayload()
			if err != nil || payload.Code == "" {
				t.Fatalf("failed error payload = %+v, %v", payload, err)
			}
			failed = true
		}
	}
	if !failed {
		t.Fatal("failed invocation omitted typed terminal response error")
	}
}

func assertSelectedEventsDiagnostics(t *testing.T, id string, retained []factoryapi.FactoryResponseEvent, diagnostics *observer.ObservedLogs) {
	t.Helper()
	topic := "factory-session/" + id + "/response-events"
	accepted := map[string]uint64{}
	for _, entry := range diagnostics.All() {
		if !strings.HasPrefix(entry.Message, "events ") {
			continue
		}
		fields := entry.ContextMap()
		assertEventsDiagnosticPrivacy(t, entry)
		if entry.Message != "events append outcome" || fields["topic"] != topic || fields["outcome"] != "accepted" {
			continue
		}
		if fields["source_id"] != id || fields["source_type"] != "factory-session-response-event" {
			t.Fatalf("missing safe Events correlation: %v", fields)
		}
		eventID, ok := fields["source_event_id"].(string)
		position, positionOK := fields["position"].(uint64)
		if !ok || !positionOK {
			t.Fatalf("missing event identity/position: %v", fields)
		}
		accepted[eventID] = position
	}
	if len(accepted) == 0 {
		t.Fatalf("selected process logger received no accepted outcomes for %q", topic)
	}
	assertEventsDiagnosticIdentities(t, id, retained, accepted)
}

func assertEventsDiagnosticIdentities(t *testing.T, id string, retained []factoryapi.FactoryResponseEvent, accepted map[string]uint64) {
	t.Helper()
	seen := map[string]bool{}
	for _, event := range retained {
		if event.FactorySessionId != id || event.EventId == "" || seen[event.EventId] || accepted[event.EventId] != uint64(event.Sequence) {
			t.Fatalf("public event identity/sequence does not match selected logger: %+v", event)
		}
		seen[event.EventId] = true
	}
}

func assertEventsDiagnosticPrivacy(t *testing.T, entry observer.LoggedEntry) {
	t.Helper()
	rendered := fmt.Sprint(entry.Message, entry.ContextMap())
	for _, secret := range []string{"private-result", "events-private-failure-content", "events-logger-success", "events-logger-failure"} {
		if strings.Contains(rendered, secret) {
			t.Fatalf("Events diagnostic leaked source content: %s", rendered)
		}
	}
}
