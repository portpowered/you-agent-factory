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

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestSessionInvocationAPI_PackagedGoalReturnsExplicitSummaryPrimaryResult(t *testing.T) {
	dir := scaffoldPackagedGoalBuiltInFactory(t)
	server := startFunctionalServerWithArgs(t, dir, true, nil)

	submitted := "customer goal request text"
	response := postInvocation(t, server.URL(), textInvocationRequest(t, submitted, nil))
	assertPackagedGoalCompletedWithText(t, response, "mock worker accepted")
	if primaryResultText(t, response) == submitted {
		t.Fatal("primaryResult echoed submitted goal text")
	}

}

func TestSessionInvocationAPI_PackagedGoalContinueRepeatsBeforeCompletion(t *testing.T) {
	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("ordinary partial progress\n<CONTINUE>")},
		platformprocess.CommandResult{Stdout: []byte("finished after continue\n<COMPLETE>")},
	)
	server := startPackagedGoalInvocationServer(t, runner)

	response := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke packaged goal", nil))
	assertPackagedGoalCompletedWithText(t, response, "finished after continue")
	if got := runner.CallCount(); got != 2 {
		t.Fatalf("provider invocation count = %d, want 2 after continue", got)
	}
}

func TestSessionInvocationAPI_PackagedGoalRejectRepeatsBeforeCompletion(t *testing.T) {
	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("goal is not complete yet")},
		platformprocess.CommandResult{Stdout: []byte("finished after rejection\n<COMPLETE>")},
	)
	server := startPackagedGoalInvocationServer(t, runner)

	response := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke packaged goal", nil))
	assertPackagedGoalCompletedWithText(t, response, "finished after rejection")
	if got := runner.CallCount(); got != 2 {
		t.Fatalf("provider invocation count = %d, want 2 after rejection", got)
	}
}

func TestSessionInvocationAPI_PackagedGoalWorkerFailureReturnsFailedStatusDetails(t *testing.T) {
	runner := &packagedGoalFailingCommandRunner{}
	server := startPackagedGoalInvocationServer(t, runner)

	response := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke packaged goal", nil))
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
}

type packagedGoalFailingCommandRunner struct{}

func (r *packagedGoalFailingCommandRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, errors.New("mock provider failure")
}

func TestPackagedGoalBuiltInTopology_SubmitWhilePausedResumesThroughSessionControl(t *testing.T) {
	dir := scaffoldPackagedGoalBuiltInFactory(t)
	server := startFunctionalServerWithArgs(t, dir, true, nil)

	pause := postJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		server.URL()+"/factory-sessions/~default/pause",
		factoryapi.FactorySessionLifecycleControlRequest{},
		"pause packaged goal session",
	)
	if pause.Operation != factoryapi.FactorySessionLifecycleControlKindPause || pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause response = %#v, want accepted pause", pause)
	}
	pauseNoOp := postJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		server.URL()+"/factory-sessions/~default/pause",
		factoryapi.FactorySessionLifecycleControlRequest{},
		"repeat pause packaged goal session",
	)
	if pauseNoOp.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pauseNoOp.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("repeat pause response = %#v, want no-op pause", pauseNoOp)
	}

	submitted := submitGeneratedGoalWork(t, server.URL(), "paused-goal-submit", "customer goal request text")
	listed := support.ListDefaultSessionWork(t, server.URL())
	if support.HasWorkAtCustomerState(listed, stringPointerValue(submitted.WorkId), "goal:init") {
		t.Fatalf("paused submit reached goal:init while session was paused: %#v", listed.Results)
	}
	if support.HasWorkAtCustomerState(listed, stringPointerValue(submitted.WorkId), "goal:complete") {
		t.Fatalf("paused submit reached goal:complete before resume: %#v", listed.Results)
	}

	resume := postJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		server.URL()+"/factory-sessions/~default/resume",
		factoryapi.FactorySessionLifecycleControlRequest{},
		"resume packaged goal session",
	)
	if resume.Operation != factoryapi.FactorySessionLifecycleControlKindResume || resume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume response = %#v, want accepted resume", resume)
	}
	resumeNoOp := postJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		server.URL()+"/factory-sessions/~default/resume",
		factoryapi.FactorySessionLifecycleControlRequest{},
		"repeat resume packaged goal session",
	)
	if resumeNoOp.Operation != factoryapi.FactorySessionLifecycleControlKindResume ||
		resumeNoOp.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("repeat resume response = %#v, want no-op resume", resumeNoOp)
	}

	completed := waitForGeneratedWorkIDsComplete(t, server.URL(), []string{stringPointerValue(submitted.WorkId)}, 15*time.Second)
	if len(completed) != 1 || generatedWorkStateName(completed[0].State) != "complete" {
		t.Fatalf("completed work = %#v, want one completed goal after resume", completed)
	}
}

func scaffoldPackagedGoalBuiltInFactory(t *testing.T) string {
	t.Helper()

	dir := support.InstallPackagedFactory(t, t.TempDir(), "@you/goal")
	if _, err := support.LoadedFactory(t, dir); err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	return dir
}

func startPackagedGoalInvocationServer(t *testing.T, runner platformprocess.CommandRunner) *functionalAPIServer {
	t.Helper()

	dir := scaffoldPackagedGoalBuiltInFactory(t)
	return startFunctionalServerWithArgs(t, dir, false, nil, withWorkerCommands(runner, nil))
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
		"workTypeName": "goal",
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
