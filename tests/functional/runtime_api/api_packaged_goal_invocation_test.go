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
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/packages/goal"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestSessionInvocationAPI_PackagedGoalReturnsExplicitSummaryPrimaryResult(t *testing.T) {
	dir := scaffoldPackagedGoalBuiltInFactory(t)
	core, observedLogs := observer.New(zap.InfoLevel)
	server := startFunctionalServerWithConfig(t, dir, true, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.Logger = zap.New(core)
	})

	submitted := "customer goal request text"
	response := postInvocation(t, server.URL(), textInvocationRequest(t, submitted, nil))
	assertPackagedGoalCompletedWithText(t, response, "mock worker accepted")
	if primaryResultText(t, response) == submitted {
		t.Fatal("primaryResult echoed submitted goal text")
	}

	submittedLogs := observedLogs.FilterMessage("factory session invocation submitted").All()
	if len(submittedLogs) != 1 {
		t.Fatalf("submitted invocation log count = %d, want 1", len(submittedLogs))
	}
	submittedFields := submittedLogs[0].ContextMap()
	if got := submittedFields["invocation_return_policy_mode"]; got != "authored" {
		t.Fatalf("submitted invocation_return_policy_mode = %#v, want authored", got)
	}
	if got := submittedFields["policy_resolution_path"]; got != "explicit_scoped_terminal_match" {
		t.Fatalf("submitted policy_resolution_path = %#v, want explicit_scoped_terminal_match", got)
	}
}

func TestSessionInvocationAPI_PackagedGoalContinueRepeatsBeforeCompletion(t *testing.T) {
	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("ordinary partial progress\n<CONTINUE>")},
		workers.CommandResult{Stdout: []byte("finished after continue\n<COMPLETE>")},
	)
	server := startPackagedGoalInvocationServer(t, runner)

	response := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke packaged goal", nil))
	assertPackagedGoalCompletedWithText(t, response, "finished after continue")
	if got := runner.CallCount(); got != 2 {
		t.Fatalf("provider invocation count = %d, want 2 after continue", got)
	}
}

func TestSessionInvocationAPI_PackagedGoalRejectRepeatsBeforeCompletion(t *testing.T) {
	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("goal is not complete yet")},
		workers.CommandResult{Stdout: []byte("finished after rejection\n<COMPLETE>")},
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

func startPackagedGoalInvocationServer(t *testing.T, runner workers.CommandRunner) *functionalAPIServer {
	t.Helper()

	dir := scaffoldPackagedGoalBuiltInFactory(t)
	return startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		support.ConfigureWorkerCommands(t, cfg, runner, nil)
	})
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
