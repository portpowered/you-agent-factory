package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// F7-R1/R2/R11: the same public spine proves native/default compatibility
// and fresh recorded admission, including a source without output or identity.
func TestInterruptExplicitModes(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct{ name, mode string }{
		{"provider", "provider"}, {"recorded", "recorded"}, {"empty", "recorded"}, {"bounded", "recorded"},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			runInterruptExplicitMode(t, cell.name, cell.mode)
		})
	}
}

func runInterruptExplicitMode(t *testing.T, name, mode string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	scenario := newS8InterruptScenario(t, ctx, "interrupt-mode-"+name)
	t.Cleanup(scenario.runner.releaseAll)
	initial := scenario.runner.callFor(scenario.repositoryA.path, s8InterruptCallAInitial)
	if name == "empty" {
		initial.omitInitialObservation = true
	}
	if name == "bounded" {
		initial.initialContext = strings.Repeat("世", 100000)
	}
	if name == "recorded" {
		initial.initialContext = "captured source context"
	}
	ids := scenario.ids
	if mode == "recorded" {
		scenario.runner.callFor(scenario.repositoryA.path, s8InterruptCallASuccessor).sessionID = "fresh-recorded-native"
	}
	invokeS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8RemoteWorkerInvocation{
		requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
		factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: ids.workA, message: s8MessageA,
	})
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallAInitial)
	if initial.initialContext != "" {
		minimumBytes := 1
		if name == "bounded" {
			minimumBytes = 150000
		}
		awaitInterruptCapturedContext(t, ctx, scenario, minimumBytes)
	}
	assertInterruptInvalidMode(t, ctx, scenario)
	args := []string{"you", "--remote", "--server", scenario.serverURL, "--json", "worker-sessions", "interrupt", ids.workerA,
		"--request-id", ids.interruptRequest, "--successor-worker-session-id", ids.successor,
		"--replacement-message", s8ReplacementMessage, "--resume-mode", mode, "--async"}
	request := support.FakeInputs(ctx, args)
	request.Input.Env, request.Input.WorkingDirectory = scenario.env, scenario.repositoryA.path
	if err := scenario.manager.Execute(request.Input); err != nil {
		t.Fatalf("CLI %s interrupt: %v stdout=%s stderr=%s", name, err, request.Stdout(), request.Stderr())
	}
	var first s8InterruptResult
	if err := json.Unmarshal([]byte(request.Stdout()), &first); err != nil {
		t.Fatal(err)
	}
	assertS8InterruptAdmission(t, first, ids)
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallASuccessor)
	scenario.runner.assertOrder(t, "start:"+s8InterruptCallAInitial, "cancel:"+s8InterruptCallAInitial, "start:"+s8InterruptCallASuccessor)
	assertInterruptModeExecution(t, scenario, name, mode, initial.initialContext)
	assertInterruptModeReplay(t, ctx, scenario, mode, first)
	assertInterruptModeLineage(t, ctx, scenario)
	scenario.runner.releaseAll()
	scenario.close(t)
}

func assertInterruptInvalidMode(t *testing.T, ctx context.Context, scenario s8InterruptScenario) {
	t.Helper()
	ids := scenario.ids
	// Invalid HTTP mode must not stop the source or admit a successor.
	status, body, _, err := sendS8InterruptHTTP(ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage, "invalid")
	if err != nil || status != http.StatusBadRequest || scenario.runner.cancellationCount(s8InterruptCallAInitial) != 0 || scenario.runner.CallCount() != 1 {
		t.Fatalf("invalid mode status=%d err=%v body=%s", status, err, body)
	}
}

func assertInterruptModeExecution(t *testing.T, scenario s8InterruptScenario, name, mode, initialContext string) {
	t.Helper()
	requests := scenario.runner.requests()
	if len(requests) != 2 {
		t.Fatalf("provider calls = %d", len(requests))
	}
	arguments := strings.Join(requests[1].Args, " ")
	input := string(requests[1].Stdin)
	if !strings.Contains(input, s8ReplacementMessage) || requests[1].WorkDir != scenario.repositoryA.path {
		t.Fatalf("lost input/settings: args=%s input=%s directory=%s", arguments, input, requests[1].WorkDir)
	}
	if mode == "recorded" {
		assertRecordedInterruptInput(t, name, arguments, input, initialContext)
	} else if !strings.Contains(arguments, s8InterruptProviderSessionA) || !strings.Contains(arguments, "resume") {
		t.Fatalf("native continuation = %s", input)
	}
}

func assertRecordedInterruptInput(t *testing.T, name, arguments, input, initialContext string) {
	t.Helper()
	if strings.Contains(arguments, "resume") || strings.Contains(arguments, s8InterruptProviderSessionA) || !strings.Contains(input, "Captured context (truncated=") {
		t.Fatalf("fresh recorded input = %s", input)
	}
	if name == "recorded" && !strings.Contains(input, initialContext) {
		t.Fatalf("lost captured context: %s", input)
	}
	if name == "bounded" {
		assertBoundedInterruptInput(t, input)
	}
	if name == "empty" && !strings.Contains(input, "Captured context (truncated=false):\n\nReplacement message:") {
		t.Fatalf("empty context = %s", input)
	}
}

func assertInterruptModeReplay(t *testing.T, ctx context.Context, scenario s8InterruptScenario, mode string, first s8InterruptResult) {
	t.Helper()
	ids := scenario.ids
	status, body, replay, err := sendS8InterruptHTTP(ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage, mode)
	if err != nil || status != http.StatusAccepted || !reflect.DeepEqual(s8InterruptResultFromAPI(replay), first) {
		t.Fatalf("exact HTTP replay = %d %s %v", status, body, err)
	}
	otherMode := "provider"
	if mode == "provider" {
		otherMode = "recorded"
	}
	status, body, _, err = sendS8InterruptHTTP(ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage, otherMode)
	if err != nil || status != http.StatusConflict || !strings.Contains(body, "WORKER_SESSION_INTERRUPT_REQUEST_ID_CONFLICT") || scenario.runner.CallCount() != 2 {
		t.Fatalf("changed-mode replay = %d %s %v", status, body, err)
	}
}

func assertInterruptModeLineage(t *testing.T, ctx context.Context, scenario s8InterruptScenario) {
	t.Helper()
	ids := scenario.ids
	listed := listS8RemoteWorkers(t, ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL)
	if source := findS8Observation(t, listed, ids.workerA); source.State != "CANCELED" {
		t.Fatalf("source = %#v", source)
	}
	if successor := findS8Observation(t, listed, ids.successor); successor.State != "RUNNING" {
		t.Fatalf("successor = %#v", successor)
	}
	sourceObservation := support.GetJSON[factoryapi.WorkerSessionObservation](t, scenario.serverURL+"/worker-sessions/"+ids.workerA)
	successorObservation := support.GetJSON[factoryapi.WorkerSessionObservation](t, scenario.serverURL+"/worker-sessions/"+ids.successor)
	if sourceObservation.SuccessorWorkerSessionId == nil || *sourceObservation.SuccessorWorkerSessionId != ids.successor || successorObservation.PredecessorWorkerSessionId == nil || *successorObservation.PredecessorWorkerSessionId != ids.workerA {
		t.Fatal("public observations lost interrupt lineage")
	}
}

// Provider callbacks enqueue capture asynchronously. Observe the public prefix
// before asking interrupt to freeze it, rather than guessing a scheduling delay.
func awaitInterruptCapturedContext(t *testing.T, ctx context.Context, scenario s8InterruptScenario, minimumBytes int) {
	t.Helper()
	for ctx.Err() == nil {
		found := 0
		next := ""
		for ctx.Err() == nil {
			page := support.GetJSON[factoryapi.WorkerSessionLogPage](t, scenario.serverURL+"/worker-sessions/"+scenario.ids.workerA+"/logs?limit=1000&nextToken="+url.QueryEscape(next))
			for _, event := range page.Events {
				if event.Event.Payload["kind"] == "MESSAGE" {
					payload, err := json.Marshal(event.Event.Payload)
					if err != nil {
						t.Fatal(err)
					}
					found += len(payload)
				}
			}
			if found >= minimumBytes {
				return
			}
			if len(page.Events) == 0 || page.NextToken == nil || *page.NextToken == "" {
				break
			}
			next = *page.NextToken
		}
	}
	t.Fatal("captured source context never reached the public log prefix")
}

func assertBoundedInterruptInput(t *testing.T, input string) {
	t.Helper()
	_, contextText, found := strings.Cut(input, "Captured context (truncated=true):\n")
	contextText, _, ended := strings.Cut(contextText, "\nReplacement message:\n")
	if !found || !ended || len(contextText) > 65536 || !utf8.ValidString(contextText) || !strings.Contains(contextText, "世") {
		t.Fatalf("bounded context bytes=%d found=%v ended=%v valid=%v input=%q", len(contextText), found, ended, utf8.ValidString(contextText), input[:min(len(input), 500)])
	}
}
