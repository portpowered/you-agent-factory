package workersessions_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

func TestDirectCapacitySuccessors(t *testing.T) {
	for _, mode := range []string{"continue", "interrupt"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			client := newSelectedHostClientProcess(t)
			runner := &capacitySuccessorRunner{output: capturedProviderFixtureOutput(t), started: make(chan struct{}, 4), canceled: make(chan struct{}), sourceRelease: make(chan struct{}), successorsRelease: make(chan struct{}), sourceJoined: make(chan struct{}), interrupt: mode == "interrupt"}
			host, dir, _ := newCapacityProfileHost(t, runner, `{"defaults":{"directSessionCapacity":1}}`, nil)
			t.Cleanup(runner.unblock)
			session, ctx := startMCP(t, client, host.URL())
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			assertSubagentAdmission(t, callTool(t, ctx, session, "you.subagent", capacityLifetimeArgs("source", dir)), "source")
			waitCapacityGate(t, ctx, runner.started)
			if mode == "continue" {
				runCapacityContinue(t, ctx, client, session, host.URL(), dir, runner)
			} else {
				runCapacityInterrupt(t, ctx, session, dir, runner)
			}
			waitCapacityGate(t, ctx, runner.started)
			select {
			case <-runner.sourceJoined:
			default:
				t.Fatal("successor admitted before source physical join")
			}
			assertToolError(t, callTool(t, ctx, session, "you.subagent", capacityLifetimeArgs("successor-excess", dir)), "worker_session.capacity_exceeded", true)
			runner.successorsOnce.Do(func() { close(runner.successorsRelease) })
			followCapacityTerminal(t, ctx, client, host.URL(), dir, "successor", "COMPLETED")
			assertSubagentAdmission(t, callTool(t, ctx, session, "you.subagent", capacityLifetimeArgs("replacement", dir)), "replacement")
			followCapacityTerminal(t, ctx, client, host.URL(), dir, "replacement", "COMPLETED")
		})
	}
}

func runCapacityContinue(t *testing.T, ctx context.Context, client selectedHostClientProcess, session *mcp.ClientSession, host, dir string, runner *capacitySuccessorRunner) {
	t.Helper()
	runner.sourceOnce.Do(func() { close(runner.sourceRelease) })
	followCapacityTerminal(t, ctx, client, host, dir, "source", "COMPLETED")
	assertSubagentAdmission(t, callTool(t, ctx, session, "you.subagent", capacityLifetimeArgs("occupied", dir)), "occupied")
	waitCapacityGate(t, ctx, runner.started)
	payload := map[string]any{"requestId": "continue-request", "successorWorkerSessionId": "successor", "followUpInput": "controlled follow-up"}
	refused := postHostJSON(t, ctx, host+"/worker-sessions/source/continue", payload, 503).(map[string]any)
	if refused["code"] != "WORKER_SESSION_CONTINUATION_ADMISSION_FAILED" {
		t.Fatal("occupied continuation lost admission failure classification")
	}
	assertToolError(t, callAction(t, ctx, session, "READ", map[string]any{"workerSessionId": "successor"}), "worker_session.not_found", false)
	result := callAction(t, ctx, session, "CONTROL", map[string]any{"workerSessionId": "occupied", "operation": "CANCEL"})
	if result.IsError {
		t.Fatal("occupied session cancellation failed")
	}
	followCapacityTerminal(t, ctx, client, host, dir, "occupied", "CANCELED")
	accepted := postHostJSON(t, ctx, host+"/worker-sessions/source/continue", payload, 202).(map[string]any)
	if accepted["accepted"] != true || accepted["successorWorkerSessionId"] != "successor" {
		t.Fatal("continuation did not admit successor")
	}
	assertJSONEqual(t, accepted, postHostJSON(t, ctx, host+"/worker-sessions/source/continue", payload, 202))
	payload["followUpInput"] = "conflicting follow-up"
	postHostJSON(t, ctx, host+"/worker-sessions/source/continue", payload, 409)
	if runner.calls.Load() != 3 {
		t.Fatal("refused/replayed continuation executed an extra provider")
	}
}

func runCapacityInterrupt(t *testing.T, ctx context.Context, session *mcp.ClientSession, dir string, runner *capacitySuccessorRunner) {
	t.Helper()
	args := map[string]any{"action": "CONTROL", "workerSessionId": "source", "operation": "INTERRUPT", "requestId": "interrupt-request", "successorWorkerSessionId": "successor", "replacementMessage": "controlled replacement"}
	type response struct {
		result *mcp.CallToolResult
		err    error
	}
	done := make(chan response, 1)
	go func() {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: args})
		done <- response{result, err}
	}()
	waitCapacityGate(t, ctx, runner.canceled)
	assertToolError(t, callTool(t, ctx, session, "you.subagent", capacityLifetimeArgs("before-source-join", dir)), "worker_session.capacity_exceeded", true)
	runner.sourceOnce.Do(func() { close(runner.sourceRelease) })
	select {
	case r := <-done:
		if r.err != nil || r.result == nil || r.result.IsError {
			t.Fatalf("interrupt admission: %v %+v", r.err, r.result)
		}
		assertJSONEqual(t, r.result.StructuredContent, callTool(t, ctx, session, "you.subagent", args).StructuredContent)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	args["replacementMessage"] = "conflicting replacement"
	assertToolError(t, callTool(t, ctx, session, "you.subagent", args), "worker_session.conflict", false)
	if runner.calls.Load() != 2 {
		t.Fatal("interrupt replay executed an extra provider")
	}
}

type capacitySuccessorRunner struct {
	output                                                            []byte
	started, canceled, sourceRelease, successorsRelease, sourceJoined chan struct{}
	sourceOnce, successorsOnce                                        sync.Once
	interrupt                                                         bool
	calls                                                             atomic.Int32
}

func (r *capacitySuccessorRunner) unblock() {
	r.sourceOnce.Do(func() { close(r.sourceRelease) })
	r.successorsOnce.Do(func() { close(r.successorsRelease) })
}

func (r *capacitySuccessorRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return r.RunStreaming(ctx, req, nil)
}

func (r *capacitySuccessorRunner) RunStreaming(ctx context.Context, req platformprocess.CommandRequest, observer platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	r.calls.Add(1)
	source := strings.TrimSpace(string(req.Stdin)) == "source"
	output := r.output
	if strings.TrimSpace(string(req.Stdin)) == "occupied" || strings.TrimSpace(string(req.Stdin)) == "replacement" {
		output = bytes.ReplaceAll(output, []byte(interruptProviderID), []byte("capacity-"+strings.TrimSpace(string(req.Stdin))))
	}
	if observer != nil {
		end := bytes.IndexByte(output, '\n') + 1
		observer(platformprocess.OutputStreamStdout, output[:end])
	}
	r.started <- struct{}{}
	if source {
		if r.interrupt {
			<-ctx.Done()
			close(r.canceled)
		}
		<-r.sourceRelease
		close(r.sourceJoined)
		if ctx.Err() != nil {
			return platformprocess.CommandResult{}, ctx.Err()
		}
	} else {
		select {
		case <-r.successorsRelease:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	if observer != nil {
		end := bytes.IndexByte(output, '\n') + 1
		observer(platformprocess.OutputStreamStdout, output[end:])
	}
	return platformprocess.CommandResult{Stdout: output}, nil
}
