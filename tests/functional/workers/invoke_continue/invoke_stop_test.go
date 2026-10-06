package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// Direct attempts have no Factory origin. Each leaf owns two command routes
// and profiles; channel gates race natural completion against exact controls.
// Factory-origin sibling isolation remains the separate F6-04 witness.
func TestT7HostedStopAndCompletionKeepSiblingRunning(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.cancel", "cli/you.worker-sessions.terminate", "rest/readWorkerSessionLogs")
	fixture := ensureInvokeContinuePackageFixture(t)
	for _, mode := range []string{"cancel", "terminate", "race"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			t7StopWithSibling(t, fixture, mode)
		})
	}
}

func t7StopWithSibling(t *testing.T, fixture *invokeContinuePackageFixture, mode string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	target := fixture.scenario(t, "t7-stop-"+mode)
	defer target.close(t)
	peer := fixture.scenario(t, "t7-peer-"+mode)
	defer peer.close(t)
	targetRunner := target.providerRunner.(*t7GatedProviderRunner)
	peerRunner := peer.providerRunner.(*t7GatedProviderRunner)
	defer t7ReleaseAndJoin(t, ctx, targetRunner)()
	defer t7ReleaseAndJoin(t, ctx, peerRunner)()
	id, path := t7StartGatedAsync(t, ctx, fixture, target)
	peerID, peerPath := t7StartGatedAsync(t, ctx, fixture, peer)
	t19AwaitSignal(t, ctx, targetRunner.started, "target progress")
	t19AwaitSignal(t, ctx, peerRunner.started, "sibling progress")
	action := mode
	if mode == "race" {
		action = "terminate"
	}
	control := t7RemoteCLIInputs(target, ctx, fixture.baseURL, action, id)
	done := make(chan error, 1)
	go func() { done <- fixture.process.Execute(control.Input) }()
	if mode == "race" {
		close(targetRunner.release)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stop: %v: %s", err, control.Stderr())
		}
	case <-ctx.Done():
		t.Fatal("stop did not join")
	}
	t19AwaitSignal(t, ctx, targetRunner.stopped, "target joined")
	state := t7AssertStopResult(t, control.Stdout(), id, mode)
	repeated := t7RemoteCLIInputs(target, ctx, fixture.baseURL, action, id)
	if err := fixture.process.Execute(repeated.Input); err != nil || !strings.Contains(repeated.Stdout(), `"outcome":"NOOP"`) {
		t.Fatalf("repeat stop: %v: %s %s", err, repeated.Stdout(), repeated.Stderr())
	}
	status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+peerID, nil)
	if status != http.StatusOK || !strings.Contains(body, `"state":"RUNNING"`) {
		t.Fatalf("stop affected sibling: %d %s", status, body)
	}
	t7AssertSingleTerminal(t, ctx, fixture.baseURL, id, state)
	close(peerRunner.release)
	joined := t7RemoteCLIInputs(peer, ctx, fixture.baseURL, "invoke", "--execution", peerPath)
	if err := fixture.process.Execute(joined.Input); err != nil || !strings.Contains(joined.Stdout(), `"state":"COMPLETED"`) {
		t.Fatalf("sibling completion: %v: %s %s", err, joined.Stdout(), joined.Stderr())
	}
	if targetRunner.CallCount() != 1 || peerRunner.CallCount() != 1 {
		t.Fatalf("overlapping/repeated execution: target %d peer %d, document %s", targetRunner.CallCount(), peerRunner.CallCount(), path)
	}
}

func t7StartGatedAsync(t *testing.T, ctx context.Context, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario) (string, string) {
	t.Helper()
	id := scenarioScopedID(scenario, scenario.name+"-gated-session")
	path := filepath.Join(scenario.workingDirectory, "gated.json")
	writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{
		requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt",
		workingDirectory: scenario.workingDirectory, userMessage: "controlled gated invocation",
	})
	inputs := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--async", "--execution", path)
	if err := fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf("async admission: %v: %s", err, inputs.Stderr())
	}
	var result directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, inputs.Stdout(), &result)
	if !result.Accepted || result.WorkerSessionID != id {
		t.Fatalf("async identity = %#v", result)
	}
	return id, path
}

func t7AssertStopResult(t *testing.T, body, id, mode string) string {
	t.Helper()
	var result struct {
		WorkerSessionID string `json:"workerSessionId"`
		Outcome         string `json:"outcome"`
		State           string `json:"state"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatal(err)
	}
	want := "TERMINATED"
	if mode == "cancel" {
		want = "CANCELED"
	}
	if result.WorkerSessionID != id || (result.Outcome != "APPLIED" && result.Outcome != "NOOP") ||
		(result.State != want && !(mode == "race" && result.State == "COMPLETED")) {
		t.Fatalf("stop outcome = %s", body)
	}
	return result.State
}

func t7AssertSingleTerminal(t *testing.T, ctx context.Context, baseURL, id, state string) {
	t.Helper()
	status, body := t7HTTP(t, ctx, http.MethodGet, baseURL+"/worker-sessions/"+id+"/logs", nil)
	var logs struct {
		Health string `json:"health"`
		Events []struct {
			Event struct {
				Position int64 `json:"position"`
				Payload  struct {
					Payload struct {
						Status string `json:"status"`
					} `json:"payload"`
				} `json:"payload"`
			} `json:"event"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(body), &logs); err != nil {
		t.Fatal(err)
	}
	var previous int64
	terminalCount := 0
	for _, entry := range logs.Events {
		if entry.Event.Position <= previous {
			t.Fatalf("unordered stop capture: %s", body)
		}
		previous = entry.Event.Position
		switch entry.Event.Payload.Payload.Status {
		case "COMPLETED", "FAILED", "CANCELED", "TERMINATED":
			terminalCount++
			if entry.Event.Payload.Payload.Status != state {
				t.Fatalf("competing captured terminal: %s", body)
			}
		}
	}
	if status != http.StatusOK || logs.Health != "COMPLETE" || terminalCount != 1 || !strings.Contains(body, "T7 progress before detach") {
		t.Fatalf("terminal capture = %d %s", status, body)
	}
}
