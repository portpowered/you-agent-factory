package runtime_api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/goal"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestSessionInvocationAPI_PackagedGoalReturnsExplicitSummaryPrimaryResult(t *testing.T) {
	host, stream := startPackagedGoalInvocationHost(t, nil)

	submitted := "customer goal request text"
	response := postInvocation(t, host.Endpoint(), textInvocationRequest(t, submitted, nil))
	assertPackagedGoalCompletedWithText(t, response, "mock worker accepted")
	if primaryResultText(t, response) == submitted {
		t.Fatal("primaryResult echoed submitted goal text")
	}
	assertPackagedGoalDispatches(t, stream, response.TraceId, []factoryapi.WorkOutcome{factoryapi.WorkOutcomeAccepted})
	assertInvocationTraceWorkState(t, host.Endpoint(), response.TraceId, "complete", factoryapi.WorkStateTypeTERMINAL)
}

func TestSessionInvocationAPI_PackagedGoalContinueRepeatsBeforeCompletion(t *testing.T) {
	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("ordinary partial progress\n<CONTINUE>")},
		workers.CommandResult{Stdout: []byte("finished after continue\n<COMPLETE>")},
	)
	host, stream := startPackagedGoalInvocationHost(t, runner)

	response := postInvocation(t, host.Endpoint(), textInvocationRequest(t, "invoke packaged goal", nil))
	assertPackagedGoalCompletedWithText(t, response, "finished after continue")
	assertPackagedGoalDispatches(t, stream, response.TraceId, []factoryapi.WorkOutcome{
		factoryapi.WorkOutcomeContinue,
		factoryapi.WorkOutcomeAccepted,
	})
	assertInvocationTraceWorkState(t, host.Endpoint(), response.TraceId, "complete", factoryapi.WorkStateTypeTERMINAL)
}

func TestSessionInvocationAPI_PackagedGoalRejectRepeatsBeforeCompletion(t *testing.T) {
	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("goal is not complete yet")},
		workers.CommandResult{Stdout: []byte("finished after rejection\n<COMPLETE>")},
	)
	host, stream := startPackagedGoalInvocationHost(t, runner)

	response := postInvocation(t, host.Endpoint(), textInvocationRequest(t, "invoke packaged goal", nil))
	assertPackagedGoalCompletedWithText(t, response, "finished after rejection")
	assertPackagedGoalDispatches(t, stream, response.TraceId, []factoryapi.WorkOutcome{
		factoryapi.WorkOutcomeRejected,
		factoryapi.WorkOutcomeAccepted,
	})
	assertInvocationTraceWorkState(t, host.Endpoint(), response.TraceId, "complete", factoryapi.WorkStateTypeTERMINAL)
}

func TestSessionInvocationAPI_PackagedGoalWorkerFailureReturnsFailedStatusDetails(t *testing.T) {
	runner := &packagedGoalFailingCommandRunner{}
	host, stream := startPackagedGoalInvocationHost(t, runner)

	response := postInvocation(t, host.Endpoint(), textInvocationRequest(t, "invoke packaged goal", nil))
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.InvocationResponseErrorCode("INVOCATION_RUNTIME_FAILURE") {
		t.Fatalf("invocation errorCode = %#v, want INVOCATION_RUNTIME_FAILURE", response.ErrorCode)
	}
	if response.Message == nil || !strings.Contains(*response.Message, "invocation failed") || !strings.Contains(*response.Message, `state "goal:failed"`) {
		t.Fatalf("invocation message = %#v, want failed goal explanation", response.Message)
	}
	if response.WorkState == nil || *response.WorkState != "goal:failed" {
		t.Fatalf("invocation workState = %#v, want goal:failed", response.WorkState)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("invocation primaryResult = %#v, want nil on failed output", response.PrimaryResult)
	}
	assertPackagedGoalDispatches(t, stream, response.TraceId, []factoryapi.WorkOutcome{factoryapi.WorkOutcomeFailed})
	assertInvocationWorkFailedPublicly(t, host.Endpoint(), response)
}

type packagedGoalFailingCommandRunner struct{}

func (r *packagedGoalFailingCommandRunner) Run(_ context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, errors.New("mock provider failure")
}

func TestPackagedGoalBuiltInTopology_SubmitWhilePausedResumesThroughSessionControl(t *testing.T) {
	dir := scaffoldPackagedGoalBuiltInFactory(t)
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
	requireFunctionalEventStreamPrelude(t, stream)

	pause := postJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		host.Endpoint()+"/factory-sessions/~default/pause",
		factoryapi.FactorySessionLifecycleControlRequest{},
		"pause packaged goal session",
	)
	if pause.Operation != factoryapi.FactorySessionLifecycleControlKindPause || pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause response = %#v, want accepted pause", pause)
	}

	submitted := submitGeneratedGoalWork(t, host.Endpoint(), "paused-goal-submit", "customer goal request text")
	workID := stringPointerValue(submitted.WorkId)
	assertPausedGoalSubmitHasNoTerminalPublicResult(t, host.Endpoint(), workID)

	resume := postJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		host.Endpoint()+"/factory-sessions/~default/resume",
		factoryapi.FactorySessionLifecycleControlRequest{},
		"resume packaged goal session",
	)
	if resume.Operation != factoryapi.FactorySessionLifecycleControlKindResume || resume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume response = %#v, want accepted resume", resume)
	}

	assertTerminalDispatchForTrace(t, stream, submitted.TraceId)
	completed := waitForGeneratedWorkIDsComplete(t, host.Endpoint(), []string{workID}, 15*time.Second)
	if len(completed) != 1 || generatedWorkStateName(completed[0].State) != "complete" {
		t.Fatalf("completed work = %#v, want one completed goal after resume", completed)
	}
}

func assertPausedGoalSubmitHasNoTerminalPublicResult(t *testing.T, baseURL, workID string) {
	t.Helper()

	works := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(baseURL, "/work"))
	for _, candidate := range works.Results {
		if stringPointerValue(candidate.WorkId) == workID && generatedWorkStateType(candidate.State) == factoryapi.WorkStateTypeTERMINAL {
			t.Fatalf("GET /work paused submission state = %#v, want no terminal result before resume", candidate.State)
		}
	}
}

func scaffoldPackagedGoalBuiltInFactory(t *testing.T) string {
	t.Helper()

	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), goal.PackagedFactoryName, goal.BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	if _, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(dir, nil); err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	return dir
}

func startPackagedGoalInvocationHost(t *testing.T, runner workers.CommandRunner) (*support.RootRunFunctionalHost, *factoryEventHTTPStream) {
	t.Helper()

	dir := scaffoldPackagedGoalBuiltInFactory(t)
	edges := wire.FunctionalEdges{}
	if runner != nil {
		edges.ProviderCommandRunner = runner
	}
	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot:        dir,
		SystemRoot:         t.TempDir(),
		DisableMockWorkers: runner != nil,
		FunctionalEdges:    edges,
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
	requireFunctionalEventStreamPrelude(t, stream)
	return host, stream
}

func assertPackagedGoalDispatches(t *testing.T, stream *factoryEventHTTPStream, traceID string, want []factoryapi.WorkOutcome) {
	t.Helper()

	for index, expectedOutcome := range want {
		for {
			event := stream.next(10 * time.Second)
			if event.Type != factoryapi.FactoryEventTypeDispatchResponse || !packagedGoalEventHasTrace(event, traceID) {
				continue
			}
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode packaged goal DISPATCH_RESPONSE %d: %v", index, err)
			}
			if payload.TransitionId != goal.PackagedExecuteWorkstationName || payload.Outcome != expectedOutcome {
				t.Fatalf("packaged goal DISPATCH_RESPONSE %d = transition %q outcome %q, want transition %q outcome %q", index, payload.TransitionId, payload.Outcome, goal.PackagedExecuteWorkstationName, expectedOutcome)
			}
			break
		}
	}
}

func packagedGoalEventHasTrace(event factoryapi.FactoryEvent, traceID string) bool {
	if event.Context.TraceIds == nil {
		return false
	}
	for _, candidate := range *event.Context.TraceIds {
		if candidate == traceID {
			return true
		}
	}
	return false
}

func assertPackagedGoalCompletedWithText(t *testing.T, response factoryapi.InvocationResponse, want string) {
	t.Helper()

	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	if got := primaryResultText(t, response); got != want {
		t.Fatalf("primaryResult text = %q, want %q", got, want)
	}
}

func primaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()

	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("invocation primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	return part.Text
}

func submitGeneratedGoalWork(t *testing.T, baseURL, name, text string) factoryapi.SubmitWorkResponse {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"name":         name,
		"workTypeName": goal.PackagedGoalWorkTypeName,
		"items": []map[string]any{{
			"type": "text",
			"text": text,
		}},
	})
	if err != nil {
		t.Fatalf("marshal generated goal submit request: %v", err)
	}
	resp, err := http.Post(support.DefaultSessionWorkURL(baseURL, "/work"), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /work goal submit: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /work goal submit status = %d, want 201: %s", resp.StatusCode, string(payload))
	}
	var submitted factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&submitted); err != nil {
		t.Fatalf("decode goal submit response: %v", err)
	}
	if strings.TrimSpace(stringPointerValue(submitted.WorkId)) == "" {
		t.Fatalf("goal submit response = %#v, want work id", submitted)
	}
	return submitted
}
