package stdio_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessionmcp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type selectedSyncPoll struct {
	delay time.Duration
	ticks <-chan time.Time
}

type selectedSyncScheduler struct {
	*platformclock.Deterministic
	polls chan selectedSyncPoll
	tick  int
}

func (scheduler *selectedSyncScheduler) After(delay time.Duration) <-chan time.Time {
	ticks := scheduler.Deterministic.After(delay)
	scheduler.polls <- selectedSyncPoll{delay: delay, ticks: ticks}
	return ticks
}

func (scheduler *selectedSyncScheduler) NewTimer(delay time.Duration) platformclock.Timer {
	// Startup readiness is independent of the operation waits held by this test.
	if delay == 10*time.Millisecond {
		readiness := platformclock.NewDeterministic(scheduler.Now(), delay)
		timer := readiness.NewTimer(delay)
		readiness.SetTick(1)
		return timer
	}
	return scheduler.Deterministic.NewTimer(delay)
}

func (scheduler *selectedSyncScheduler) poll(t *testing.T) selectedSyncPoll {
	t.Helper()
	select {
	case poll := <-scheduler.polls:
		if poll.delay != 10*time.Millisecond {
			t.Fatalf("sync poll = %s", poll.delay)
		}
		return poll
	//nolint:testsleep // Selected poll registration is the signal; host time only bounds a broken fixture.
	case <-time.After(30 * time.Second):
		t.Fatal("selected sync wait not registered")
		return selectedSyncPoll{}
	}
}

func (scheduler *selectedSyncScheduler) advance() {
	scheduler.tick++
	scheduler.SetTick(scheduler.tick)
}

// One immutable root serves independent connection-owned profiles/sessions.
// Cases are ordered because advancing a shared scheduler intentionally affects
// all its sessions. The whole cohort runs alongside unrelated functional tests.
func TestSelectedSchedulerControlsMCPSyncOperations(t *testing.T) {
	t.Parallel()
	base := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
	facts := platformclock.NewDeterministic(base, time.Second)
	scheduler := &selectedSyncScheduler{Deterministic: platformclock.NewDeterministic(base.Add(time.Hour), 10*time.Millisecond), polls: make(chan selectedSyncPoll, 8)}
	runner := &mcpRootResultRunner{}
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{Clock: facts, ProcessScheduler: scheduler, ProviderCommandRunner: runner})
	if err != nil {
		t.Fatal(err)
	}
	support.CleanupProcess(t, process)
	for _, name := range []string{"completion", "timeout keeps session running", "timeout cancels owned session"} {
		t.Run(name, func(t *testing.T) {
			cancelOnTimeout := name == "timeout cancels owned session"
			server := startComposedMemoryMCP(t, process)
			initializeMCPClient(t, server.client)
			gate := runner.gate(t, server.root)
			reply := make(chan mcpJSONRPCResponse, 1)
			timeout := int64(3600000)
			if name != "completion" {
				timeout = 20
			}
			go func() { reply <- selectedSyncCall(server.client, timeout, cancelOnTimeout) }()
			awaitComposedSignal(t, gate.entered)
			poll := scheduler.poll(t)
			facts.SetTick(3600)
			select {
			case <-poll.ticks:
				t.Fatal("fact clock released selected sync poll")
			default:
			}
			select {
			case response := <-reply:
				t.Fatalf("held sync returned: %#v", response)
			default:
			}
			if name == "completion" {
				close(gate.release)
			}
			scheduler.advance()
			response := advanceSelectedSyncToReply(t, scheduler, reply)
			result := decodeComposedTool[factoryapi.FactorySessionSyncExecutionResponse](t, response)
			if result.SessionId == "" {
				t.Fatal("sync response lost session identity")
			}
			if name == "timeout keeps session running" {
				assertSelectedSyncStillRunning(t, server, result)
				close(gate.release)
			} else if cancelOnTimeout {
				if result.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeTimedOut || result.SessionCanceledByTimeout == nil || !*result.SessionCanceledByTimeout {
					t.Fatalf("timeout cancellation = %#v", result)
				}
				t.Log("S03: selected timeout and cancellation-join polls retain timeout cancellation flags")
			} else {
				encoded, _ := json.Marshal(result.Result)
				if result.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted || !strings.Contains(string(encoded), "completed at ") {
					t.Fatalf("completed result = %#v", result)
				}
				t.Log("S01/S09 sync edge: only selected scheduler releases completion; independent fact timers preserve precedence")
			}
			server.closeInput(t)
		})
	}
	// A peer invocation remains usable after the timed-out connection closes.
	peer := startComposedMemoryMCP(t, process)
	initializeMCPClient(t, peer.client)
	assertComposedMCPSync(t, peer, "selected-peer")
	peer.closeInput(t)
}

func assertSelectedSyncStillRunning(t *testing.T, server *composedMemoryMCP, result factoryapi.FactorySessionSyncExecutionResponse) {
	t.Helper()
	if result.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeTimedOut || result.Result != nil ||
		result.SessionCanceledByTimeout != nil && *result.SessionCanceledByTimeout {
		t.Fatalf("non-canceling timeout = %#v", result)
	}
	inspected := decodeComposedTool[factoryapi.FactorySessionDurableReadModel](t, server.client.call("tools/call", map[string]any{
		"name": factorysessionmcp.ToolGetSession, "arguments": factorysessionmcp.GetSessionInput{SessionID: result.SessionId},
	}))
	if inspected.SessionId != result.SessionId || inspected.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("timed-out waiter changed running session: %#v", inspected)
	}
	t.Log("S02: selected timeout returns no result and retains the running session for subsequent public inspection")
}

func selectedSyncCall(client *stdioMCPClient, timeout int64, cancelOnTimeout bool) mcpJSONRPCResponse {
	return client.call("tools/call", map[string]any{"name": factorysessionmcp.ToolStartSync, "arguments": map[string]any{
		"requestId": uuid.NewString(),
		"source":    map[string]any{"kind": "INLINE_WORKFLOW", "inlineWorkflow": map[string]any{"inlineSource": map[string]any{"encoding": "utf-8", "inline": `return (async function () { return await agent.run({label: "selected", prompt: "selected", modelProvider: "codex", model: "gpt-5-codex"}); })();`}}},
		"wait":      map[string]any{"timeoutMillis": timeout, "cancelOnTimeout": cancelOnTimeout},
	}})
}

func advanceSelectedSyncToReply(t *testing.T, scheduler *selectedSyncScheduler, reply <-chan mcpJSONRPCResponse) mcpJSONRPCResponse {
	t.Helper()
	//nolint:testsleep // Selected polls synchronize progress; host time only bounds a missing protocol reply.
	ceiling, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for {
		select {
		case response := <-reply:
			return response
		case poll := <-scheduler.polls:
			if poll.delay != 10*time.Millisecond {
				t.Fatalf("poll delay = %s", poll.delay)
			}
			scheduler.advance()
		case <-ceiling.Done():
			t.Fatal("selected sync did not finish")
			return mcpJSONRPCResponse{}
		}
	}
}
