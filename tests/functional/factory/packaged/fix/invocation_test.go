package fix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	packagedFixFactoryName           = "@you/fix"
	configuredPackagedFixFactoryName = "@test/fix"
)

var packagedFixPlanFile = regexp.MustCompile(`tasks/todo/([A-Za-z0-9._-]+)\.json`)

// Story 001 characterization map (reconciled against the recovered P012
// inventory):
//
//   - TestPackagedFixUsesNamedWorktreeAndIndependentReview -> CLI terminal
//     response, provider role/order/model prompts, real checkout existence and
//     preservation of an unrelated Git worktree, durable plan. Retained as
//     isolated-with-reason in packaged-fix-real-git-worktree; its fidelity is
//     local-real Git/filesystem plus a controlled ProviderCommandRunner.
//   - TestPackagedFixUsesOperatorDefaultsWhenOptionalRoleParametersAreOmitted,
//     TestPackagedFixCarriesIndependentRejectionFeedback, and
//     TestPackagedFixUsesConfiguredAndExplicitRoleModels -> CLI response and
//     exact controlled provider requests (including defaults, feedback, role
//     order, and model/provider selection). Eligible for
//     packaged-fix-public-outcomes; fidelity is local-real root composition and
//     test-owned filesystem with a controlled ProviderCommandRunner. The model
//     test has two eligible configuration cells.
//   - TestPackagedFixRejectsMissingAndUnsafeWorktreeNamesBeforeProviderExecution
//     -> public Execute error and zero provider calls. Retained as
//     isolated-with-reason in packaged-fix-worktree-validation because path
//     validation is the property under test; fidelity is local-real path
//     validation with a controlled runner that must remain unused.
//   - TestPackagedFixWorktreeCreationFailureIsStable -> failed CLI response,
//     fix:failed Work state, and zero provider calls. Retained as
//     isolated-with-reason in packaged-fix-real-filesystem-failure because a
//     valid name is deliberately run from a non-repository; fidelity is
//     local-real Git/filesystem failure plus a controlled runner.
//   - TestPackagedFixWorkerFailureIsStable -> failed CLI response, fix:failed
//     Work state, and planner-plus-failed-iterator request count. Retained as
//     isolated-with-reason in packaged-fix-command-failure because the command
//     runner error is the injected failure witness; fidelity is a controlled
//     ProviderCommandRunner over local-real root composition.
//   - TestPackagedFixReviewLoopExhaustionIsStable -> failed CLI response,
//     fix:failed Work state, eight reviewer visits, and 18 total requests.
//     Eligible for packaged-fix-public-outcomes because bounded rejection is a
//     controlled workflow outcome, not a command/process failure.
//
// Resource ownership: retained rows keep one root-built process per local-real
// edge and the testing package owns their temporary paths. Eligible rows share
// one continuous root-built host process, while each scenario owns a copied
// Factory, explicit Factory Session, selector, worktree, plan, and request
// identity. The CLI parity cell uses the existing root-built local CLI helper
// because a local CLI invocation cannot bind the host process's already-owned
// default runtime. Story 004 owns the direct census of process, session,
// worktree, and runtime cleanup. Existing assertions are characterization and
// must remain intact.

// TestPackagedFixUsesNamedWorktreeAndIndependentReview proves the public named
// route prepares the requested checkout before any provider call, preserves
// unrelated worktrees, repeats the Ralph iterator, and returns only after the
// independent reviewer approves the completed plan.
//
// Retained-edge fidelity: this is an isolated local-real Git/worktree witness,
// not a controlled worktree substitute. The t.TempDir-owned repository and
// worktrees are its cleanup boundary.
func TestPackagedFixUsesNamedWorktreeAndIndependentReview(t *testing.T) {
	workspace := initPackagedFixGitRepository(t)
	unrelated := createPackagedFixWorktree(t, workspace, "unrelated-work")
	runner := &packagedFixCommandRunner{firstIteratorContinue: true}
	requestText := "deliver the named Fix request"
	worktreeName := "customer-fix"

	response, stderr, err := runPackagedFixCLI(
		t,
		runner,
		workspace,
		"--provider", "CODEX", "--model", "operator-default-model",
		"--to", requestText,
		"--worktree-name", worktreeName,
	)
	if err != nil {
		t.Fatalf("Process.Execute(@you/fix) error = %v\nresponse = %#v\nstderr = %q", err, response, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", stderr)
	}
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response status = %q, want COMPLETED: %#v", response.Status, response)
	}
	if got := packagedFixPrimaryResultText(t, response); !strings.Contains(got, "approved Fix result") {
		t.Fatalf("primary result = %q, want independent reviewer output", got)
	}

	wantCheckout := filepath.Join(workspace, ".worktrees", worktreeName)
	requests := runner.Requests()
	if len(requests) != 4 {
		t.Fatalf("provider request count = %d, want planner, iterator, iterator, reviewer", len(requests))
	}
	if got := strings.Join(runner.Roles(), ","); got != "planner,iterator,iterator,reviewer" {
		t.Fatalf("stage order = %q, want planner,iterator,iterator,reviewer", got)
	}
	for index, request := range requests {
		if filepath.Clean(request.WorkDir) != filepath.Clean(wantCheckout) {
			t.Fatalf("request[%d] work dir = %q, want prepared checkout %q", index, request.WorkDir, wantCheckout)
		}
		prompt := packagedFixProviderPrompt(request)
		if !strings.Contains(prompt, requestText) || !strings.Contains(prompt, worktreeName) {
			t.Fatalf("request[%d] prompt = %q, want original request and worktree name", index, prompt)
		}
		if !packagedFixRequestIncludesModel(request, "operator-default-model") {
			t.Fatalf("request[%d] args = %#v, want operator model", index, request.Args)
		}
	}
	if _, err := os.Stat(filepath.Join(wantCheckout, ".git")); err != nil {
		t.Fatalf("prepared Fix worktree missing: %v", err)
	}
	plan := readPackagedFixPlan(t, runner.PlanPath())
	if len(plan.Stories) != 1 || !plan.Stories[0].Passes || plan.Stories[0].Notes == "" {
		t.Fatalf("durable Fix plan = %#v, want one verified story with notes", plan)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("unrelated worktree was altered or removed: %v", err)
	}
}

// TestPackagedFixSharedProcess proves compatible public-outcome scenarios can
// share one root-built process while retaining explicit Factory Session and
// selector isolation for each invocation.
func TestPackagedFixSharedProcess(t *testing.T) {
	fixture := sharedPackagedFixFixture(t)
	fixture.census.resetForRun()
	t.Run("UsesOperatorDefaultsWhenOptionalRoleParametersAreOmitted", testPackagedFixOperatorDefaults)

	t.Run("CarriesIndependentRejectionFeedback", testPackagedFixRejectionFeedback)

	t.Run("UsesConfiguredAndExplicitRoleModels", testPackagedFixConfiguredAndExplicitRoleModels)

	t.Run("ReviewLoopExhaustionIsStable", testPackagedFixReviewLoopExhaustion)

	t.Run("CLIResponseMatchesExplicitSession", testPackagedFixCLIResponseParity)
	t.Run("CleanupPathCensus", testPackagedFixCleanupPathCensus)
	t.Cleanup(func() {
		assertPackagedFixResourceCensus(t, fixture)
	})
	failPackagedFixForcedUnwindAfterAssertion(t)
}

func testPackagedFixOperatorDefaults(t *testing.T) {
	t.Parallel()
	runner := &packagedFixCommandRunner{}
	worktreeName := "shared-operator-default-fix"
	scenario := openPackagedFixScenario(t, runner, worktreeName, nil)
	response := invokePackagedFixSession(t, scenario, map[string]any{
		"request":      "complete Fix with operator defaults",
		"worktreeName": worktreeName,
	})
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response status = %q, want COMPLETED: %#v", response.Status, response)
	}
	for index, request := range runner.Requests() {
		if request.Command != "codex" || !packagedFixRequestIncludesModel(request, "operator-configured-model") {
			t.Fatalf("request[%d] = command %q args %#v, want operator CODEX/operator-configured-model", index, request.Command, request.Args)
		}
	}
	assertPackagedFixSharedEvidence(t, scenario, runner, "approved")
}

func testPackagedFixRejectionFeedback(t *testing.T) {
	t.Parallel()
	runner := &packagedFixCommandRunner{rejectFirstReview: true}
	worktreeName := "shared-reviewed-fix"
	scenario := openPackagedFixScenario(t, runner, worktreeName, nil)
	scenario.fixture.census.recordPath(packagedFixCleanupRejection)
	response := invokePackagedFixSession(t, scenario, map[string]any{
		"request":      "revise the requested fix",
		"worktreeName": worktreeName,
	})
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response status = %q, want COMPLETED: %#v", response.Status, response)
	}
	if got := strings.Join(runner.Roles(), ","); got != "planner,iterator,reviewer,iterator,reviewer" {
		t.Fatalf("stage order = %q, want rejection revision cycle", got)
	}
	requests := runner.Requests()
	if len(requests) != 5 {
		t.Fatalf("provider request count = %d, want five stage calls", len(requests))
	}
	revisionPrompt := packagedFixProviderPrompt(requests[3])
	if !strings.Contains(revisionPrompt, "reviewer requires the missing verification evidence") {
		t.Fatalf("revision iterator prompt = %q, want reviewer feedback", revisionPrompt)
	}
	if !strings.Contains(revisionPrompt, "first Fix candidate") {
		t.Fatalf("revision iterator prompt = %q, want prior candidate", revisionPrompt)
	}
	assertPackagedFixSharedEvidence(t, scenario, runner, "approved")
}

type packagedFixRoleModelScenario struct {
	name             string
	configure        func(*testing.T, string)
	args             []string
	plannerModel     string
	iteratorModel    string
	reviewerModel    string
	plannerProvider  string
	iteratorProvider string
	reviewerProvider string
}

func testPackagedFixConfiguredAndExplicitRoleModels(t *testing.T) {
	t.Parallel()
	tests := []packagedFixRoleModelScenario{
		{
			name: "installed worker configuration",
			configure: func(t *testing.T, factoryDir string) {
				configurePackagedFixWorkerModels(t, factoryDir, map[string]string{
					"fix-planner":  "configured-planner-model",
					"fix-iterator": "configured-iterator-model",
					"fix-reviewer": "configured-reviewer-model",
				})
			},
			plannerModel:     "configured-planner-model",
			iteratorModel:    "configured-iterator-model",
			reviewerModel:    "configured-reviewer-model",
			plannerProvider:  "codex",
			iteratorProvider: "codex",
			reviewerProvider: "codex",
		},
		{
			name: "explicit role flags",
			args: []string{
				"--planner-provider", "CODEX",
				"--planner-model", "flag-planner-model",
				"--iterator-provider", "CODEX",
				"--iterator-model", "flag-iterator-model",
				"--reviewer-provider", "CODEX",
				"--reviewer-model", "flag-reviewer-model",
			},
			plannerModel:     "flag-planner-model",
			iteratorModel:    "flag-iterator-model",
			reviewerModel:    "flag-reviewer-model",
			plannerProvider:  "codex",
			iteratorProvider: "codex",
			reviewerProvider: "codex",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			testPackagedFixRoleModelScenario(t, test)
		})
	}
}

func testPackagedFixRoleModelScenario(t *testing.T, test packagedFixRoleModelScenario) {
	worktreeName := "shared-configured-fix-" + strings.ReplaceAll(test.name, " ", "-")
	runner := &packagedFixCommandRunner{firstIteratorContinue: true}
	scenario := openPackagedFixScenario(t, runner, worktreeName, test.configure)
	invocationArgs := map[string]any{
		"request":      "complete a configured Fix request",
		"worktreeName": worktreeName,
	}
	for index := 0; index+1 < len(test.args); index += 2 {
		invocationArgs[strings.TrimPrefix(test.args[index], "--")] = test.args[index+1]
	}
	response := invokePackagedFixSession(t, scenario, invocationArgs)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response status = %q, want COMPLETED: %#v", response.Status, response)
	}
	requests := runner.Requests()
	if len(requests) != 4 {
		t.Fatalf("provider request count = %d, want planner, iterator, reviewer with one final iterator", len(requests))
	}
	for index, request := range requests {
		wantModel, wantProvider := test.plannerModel, test.plannerProvider
		if index == 1 || index == 2 {
			wantModel, wantProvider = test.iteratorModel, test.iteratorProvider
		}
		if index == 3 {
			wantModel, wantProvider = test.reviewerModel, test.reviewerProvider
		}
		if request.Command != wantProvider || !packagedFixRequestIncludesModel(request, wantModel) {
			t.Fatalf("request[%d] = command %q args %#v, want %s/%s", index, request.Command, request.Args, wantProvider, wantModel)
		}
	}
	assertPackagedFixSharedEvidence(t, scenario, runner, "approved")
}

func testPackagedFixReviewLoopExhaustion(t *testing.T) {
	t.Parallel()
	runner := &packagedFixCommandRunner{alwaysRejectReview: true}
	worktreeName := "shared-bounded-review"
	scenario := openPackagedFixScenario(t, runner, worktreeName, nil)
	response := invokePackagedFixSession(t, scenario, map[string]any{
		"request":      "keep rejecting this Fix request",
		"worktreeName": worktreeName,
	})
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("response status = %q, want FAILED: %#v", response.Status, response)
	}
	if response.WorkState == nil || *response.WorkState != "fix:failed" {
		t.Fatalf("response workState = %#v, want fix:failed", response.WorkState)
	}
	if got := runner.ReviewerCalls(); got != 8 {
		t.Fatalf("reviewer calls = %d, want eight bounded review visits", got)
	}
	if got := len(runner.Requests()); got != 18 {
		t.Fatalf("provider request count = %d, want planner, nine iterators, and eight reviewers", got)
	}
	assertPackagedFixSharedEvidence(t, scenario, runner, "failed")
}

func testPackagedFixCLIResponseParity(t *testing.T) {
	t.Parallel()
	request := "prove the packaged Fix CLI response"

	apiRunner := &packagedFixCommandRunner{}
	apiWorktreeName := "shared-cli-api-parity-fix"
	apiScenario := openPackagedFixScenario(t, apiRunner, apiWorktreeName, nil)
	apiResponse := invokePackagedFixSession(t, apiScenario, map[string]any{
		"request":      request,
		"worktreeName": apiWorktreeName,
	})
	if apiResponse.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("explicit-session response status = %q, want COMPLETED: %#v", apiResponse.Status, apiResponse)
	}
	assertPackagedFixSharedEvidence(t, apiScenario, apiRunner, "approved")

	cliRunner := &packagedFixCommandRunner{}
	cliWorkspace := initPackagedFixGitRepository(t)
	cliWorktreeName := "cli-parity-fix"
	cliResponse, stderr, err := runPackagedFixCLI(
		t,
		cliRunner,
		cliWorkspace,
		"--provider", "CODEX", "--model", "operator-configured-model",
		"--to", request,
		"--worktree-name", cliWorktreeName,
	)
	if err != nil {
		t.Fatalf("root-built customer CLI error = %v\nresponse = %#v\nstderr = %q", err, cliResponse, stderr)
	}
	if stderr != "" {
		t.Fatalf("root-built customer CLI stderr = %q, want empty successful-run stderr", stderr)
	}
	if cliResponse.Status != apiResponse.Status {
		t.Fatalf("customer CLI status = %q, explicit-session status = %q", cliResponse.Status, apiResponse.Status)
	}
	if got, want := packagedFixPrimaryResultText(t, cliResponse), packagedFixPrimaryResultText(t, apiResponse); got != want {
		t.Fatalf("customer CLI primary result = %q, explicit-session primary result = %q", got, want)
	}
	if got := strings.Join(cliRunner.Roles(), ","); got != "planner,iterator,reviewer" {
		t.Fatalf("customer CLI stage order = %q, want planner,iterator,reviewer", got)
	}
}

func invokePackagedFixSession(
	t *testing.T,
	scenario *packagedFixScenario,
	args map[string]any,
) factoryapi.InvocationResponse {
	t.Helper()
	// The packaged Petri Factory must run inside this already-open explicit
	// session so the assertions below can observe its canonical Work, Event,
	// dispatch, and replay history. The public run CLI has no session-target
	// flag, and its remote durable source only resolves JavaScript workflow
	// factories, so this is the narrowly scoped CLI-plus-API parity exception.
	// TestPackagedFixSharedProcess/CLIResponseMatchesExplicitSession still
	// executes the same invocation through the root-built customer CLI and
	// compares its terminal response with this explicit-session API path.
	payload, err := json.Marshal(factoryapi.InvocationRequest{
		RequestId: &scenario.requestID,
		Args:      &args,
	})
	if err != nil {
		t.Fatalf("marshal Fix invocation request: %v", err)
	}
	endpoint := strings.TrimSuffix(scenario.fixture.baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(scenario.sessionID) + "/invocations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode POST %s: %v", endpoint, err)
	}
	support.WaitForSessionTerminalStatus(t, scenario.fixture.baseURL, scenario.sessionID, packagedFixFixtureShutdownTimeout)
	return decoded
}

func assertPackagedFixSharedEvidence(
	t *testing.T,
	scenario *packagedFixScenario,
	runner *packagedFixCommandRunner,
	wantWorkStateName string,
) {
	t.Helper()
	workResponse := support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(scenario.fixture.baseURL, "/")+
			"/factory-sessions/"+url.PathEscape(scenario.sessionID)+"/work",
	)
	if len(workResponse.Results) != 1 {
		t.Fatalf("session Work results = %#v, want one Fix Work item", workResponse.Results)
	}
	work := workResponse.Results[0]
	if work.State == nil || work.State.Name != wantWorkStateName {
		t.Fatalf("session Work state = %#v, want name %q", work.State, wantWorkStateName)
	}
	if work.WorkId == nil || strings.TrimSpace(*work.WorkId) == "" {
		t.Fatalf("session Work identity = %#v, want non-empty Work ID", work.WorkId)
	}

	events := support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
	if len(events) < 2 {
		t.Fatalf("retained Factory Event history length = %d, want at least two events", len(events))
	}
	hasWorkRequest := false
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeWorkRequest {
			hasWorkRequest = true
			break
		}
	}
	if !hasWorkRequest {
		t.Fatal("retained Factory Event history has no Work Request event")
	}

	dispatches := support.ObserveDispatchEvents(t, events)
	roles := runner.Roles()
	if len(dispatches) < len(roles) || len(dispatches) > len(roles)+1 {
		t.Fatalf("dispatch count = %d, provider role count = %d; dispatches = %#v", len(dispatches), len(roles), dispatches)
	}
	wantTransitions := map[string]string{
		"planner":  "plan-fix",
		"iterator": "iterate-fix",
		"reviewer": "review-fix",
	}
	roleIndex := 0
	loopBreakerCount := 0
	for index, dispatch := range dispatches {
		transition := dispatch.Request.TransitionId
		if transition == "review-fix-loop-breaker" {
			loopBreakerCount++
			if loopBreakerCount > 1 {
				t.Fatalf("dispatch[%d] repeats the review loop breaker", index)
			}
			if index != len(dispatches)-1 {
				t.Fatalf("dispatch[%d] transition = %q, want loop breaker only after worker dispatches", index, transition)
			}
		} else {
			if roleIndex >= len(roles) {
				t.Fatalf("dispatch[%d] transition = %q has no matching provider role", index, transition)
			}
			role := roles[roleIndex]
			if got := transition; got != wantTransitions[role] {
				t.Fatalf("dispatch[%d] transition = %q for role %q, want %q", index, got, role, wantTransitions[role])
			}
			roleIndex++
		}
		if dispatch.Response == nil {
			t.Fatalf("dispatch[%d] = %#v, want retained dispatch response", index, dispatch)
		}
		if !support.DispatchObservationIncludesWork(dispatch, *work.WorkId) {
			t.Fatalf("dispatch[%d] = %#v, want Work ID %q", index, dispatch, *work.WorkId)
		}
	}
	if roleIndex != len(roles) {
		t.Fatalf("provider roles consumed by dispatches = %d, want %d", roleIndex, len(roles))
	}
	if len(dispatches) == len(roles)+1 && loopBreakerCount != 1 {
		t.Fatalf("extra dispatches = %d, want one review loop breaker", len(dispatches)-len(roles))
	}

	assertPackagedFixReplayAndRecord(t, scenario, runner, wantWorkStateName, *work.WorkId, events)
}

func assertPackagedFixReplayAndRecord(
	t *testing.T,
	scenario *packagedFixScenario,
	runner *packagedFixCommandRunner,
	wantWorkStateName, workID string,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()
	sequence := support.ReconnectSequenceForFactoryEvent(events[0])
	replayed := support.GetFactoryEventsAfterForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID, support.FactoryEventReadCursor{
		AfterEventID:  events[0].Id,
		AfterSequence: &sequence,
	})
	if len(replayed) != len(events)-1 {
		t.Fatalf("retained replay event count = %d, want %d", len(replayed), len(events)-1)
	}
	for index := range replayed {
		if replayed[index].Id != events[index+1].Id {
			t.Fatalf("retained replay event %d = %q, want %q", index, replayed[index].Id, events[index+1].Id)
		}
	}
	eventIDs := make([]string, 0, len(events))
	for _, event := range events {
		eventIDs = append(eventIDs, event.Id)
	}
	scenario.fixture.census.recordEvidence(
		scenario.requestID,
		scenario.worktreeName,
		workID,
		runner.PlanPath(),
		eventIDs,
	)
	if wantWorkStateName == "fix:failed" {
		scenario.fixture.census.recordPath(packagedFixCleanupFailure)
	} else {
		scenario.fixture.census.recordPath(packagedFixCleanupSuccess)
	}
	if runner.rejectFirstReview || runner.alwaysRejectReview {
		scenario.fixture.census.recordPath(packagedFixCleanupRejection)
	}
}

// TestPackagedFixRejectsMissingAndUnsafeWorktreeNamesBeforeProviderExecution
// proves required and path-traversing names fail at the public boundary without
// launching planner work.
//
// Retained-edge fidelity: path validation is the property under test, so this
// scenario remains isolated with the controlled runner only observing that no
// provider command is attempted. Its t.TempDir workspace owns cleanup.
func TestPackagedFixRejectsMissingAndUnsafeWorktreeNamesBeforeProviderExecution(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing", args: []string{"--to", "missing worktree name"}},
		{name: "path traversal", args: []string{
			"--provider", "CODEX", "--model", "validation-model",
			"--to", "unsafe worktree name", "--worktree-name", `..\escape`,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			runner := &packagedFixCommandRunner{}
			response, _, err := runPackagedFixCLI(t, runner, workspace, test.args...)
			if err == nil {
				t.Fatalf("Process.Execute error = nil, want worktree validation failure; response = %#v", response)
			}
			if got := len(runner.Requests()); got != 0 {
				t.Fatalf("provider request count = %d, want zero before worktree validation", got)
			}
		})
	}
}

// TestPackagedFixWorktreeCreationFailureIsStable proves a valid name still
// fails before provider execution when the caller directory is not a Git repo.
//
// Retained-edge fidelity: the non-repository Git/filesystem failure is a
// local-real witness and must not be replaced by a mocked worktree result. The
// t.TempDir workspace owns the failed-attempt cleanup.
func TestPackagedFixWorktreeCreationFailureIsStable(t *testing.T) {
	workspace := t.TempDir()
	runner := &packagedFixCommandRunner{}
	response, _, err := runPackagedFixCLI(
		t,
		runner,
		workspace,
		"--provider", "CODEX", "--model", "creation-failure-model",
		"--to", "fail worktree creation",
		"--worktree-name", "missing-repository",
	)
	if err == nil {
		t.Fatalf("Process.Execute error = nil, want worktree creation failure; response = %#v", response)
	}
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("response status = %q, want FAILED: %#v", response.Status, response)
	}
	if response.WorkState == nil || *response.WorkState != "fix:failed" {
		t.Fatalf("response workState = %#v, want fix:failed", response.WorkState)
	}
	if got := len(runner.Requests()); got != 0 {
		t.Fatalf("provider request count = %d, want zero after worktree creation failure", got)
	}
}

// TestPackagedFixWorkerFailureIsStable proves a provider failure routes the
// public invocation to the failed terminal state rather than approval.
//
// Retained-edge fidelity: the command-runner error is the failure property, so
// this remains an isolated root-built cell with a controlled
// ProviderCommandRunner and t.TempDir-owned filesystem.
func TestPackagedFixWorkerFailureIsStable(t *testing.T) {
	workspace := initPackagedFixGitRepository(t)
	runner := &packagedFixCommandRunner{failRole: "iterator"}
	response, _, err := runPackagedFixCLI(
		t,
		runner,
		workspace,
		"--provider", "CODEX", "--model", "worker-failure-model",
		"--to", "fail the Fix iterator",
		"--worktree-name", "worker-failure",
	)
	if err == nil {
		t.Fatal("Process.Execute error = nil, want worker failure")
	}
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("response status = %q, want FAILED: %#v", response.Status, response)
	}
	if response.WorkState == nil || *response.WorkState != "fix:failed" {
		t.Fatalf("response workState = %#v, want fix:failed", response.WorkState)
	}
	if got := len(runner.Requests()); got != 2 {
		t.Fatalf("provider request count = %d, want planner plus failed iterator", got)
	}
}

func runPackagedFixCLI(
	t *testing.T,
	runner platformprocess.CommandRunner,
	workspace string,
	args ...string,
) (factoryapi.InvocationResponse, string, error) {
	return runPackagedFixCLIWithFactorySetup(t, runner, workspace, nil, args...)
}

func runPackagedFixCLIWithFactorySetup(
	t *testing.T,
	runner platformprocess.CommandRunner,
	workspace string,
	configure func(*testing.T, string),
	args ...string,
) (factoryapi.InvocationResponse, string, error) {
	t.Helper()
	home := t.TempDir()
	factoryDir := support.InstallPackagedFactory(t, home, packagedFixFactoryName)
	factoryName := packagedFixFactoryName
	if configure != nil {
		configure(t, factoryDir)
		factoryDir = support.CopyFactoryAsNamed(t, factoryDir, home, configuredPackagedFixFactoryName)
		factoryName = configuredPackagedFixFactoryName
	}
	inputArgs := []string{
		"you", "--json", "run", "--named", factoryName, "--no-record",
	}
	inputArgs = append(inputArgs, args...)
	inputs := support.FakeInputs(t.Context(), inputArgs)
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = workspace
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	err := process.Execute(inputs.Input)
	var response factoryapi.InvocationResponse
	if strings.TrimSpace(inputs.Stdout()) != "" {
		response = support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	}
	return response, inputs.Stderr(), err
}

func configurePackagedFixWorkerModels(t *testing.T, factoryDir string, models map[string]string) {
	t.Helper()
	path := filepath.Join(factoryDir, "factory.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read materialized Fix factory: %v", err)
	}
	var factory map[string]any
	if err := json.Unmarshal(payload, &factory); err != nil {
		t.Fatalf("decode materialized Fix factory: %v", err)
	}
	workers, ok := factory["workers"].([]any)
	if !ok {
		t.Fatal("materialized Fix workers are not an array")
	}
	for _, raw := range workers {
		worker, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("materialized Fix worker = %#v, want object", raw)
		}
		model, ok := models[worker["name"].(string)]
		if !ok {
			continue
		}
		worker["modelProvider"] = "CODEX"
		worker["model"] = model
	}
	updated, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("encode materialized Fix factory: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write materialized Fix factory: %v", err)
	}
}

type packagedFixCommandRunner struct {
	mu                    sync.Mutex
	requests              []platformprocess.CommandRequest
	roles                 []string
	worktree              string
	planPath              string
	iteratorCalls         int
	reviewerCalls         int
	firstIteratorContinue bool
	rejectFirstReview     bool
	alwaysRejectReview    bool
	failRole              string
}

func (runner *packagedFixCommandRunner) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	prompt := packagedFixProviderPrompt(request)
	runner.mu.Lock()
	runner.requests = append(runner.requests, clonePackagedFixCommandRequest(request))
	runner.mu.Unlock()

	switch {
	case strings.Contains(prompt, "planning stage of @you/fix"):
		if runner.failForRole("planner") {
			return platformprocess.CommandResult{}, errors.New("mock Fix planner failure")
		}
		workName := packagedFixPlanWorkName(prompt)
		if workName == "" {
			return platformprocess.CommandResult{}, errors.New("planner prompt omitted durable Fix plan path")
		}
		planPath := filepath.Join(request.WorkDir, "tasks", "todo", workName+".json")
		if err := os.MkdirAll(filepath.Dir(planPath), 0o700); err != nil {
			return platformprocess.CommandResult{}, err
		}
		plan := packagedFixPlan{
			Project: "fix-functional",
			Stories: []packagedFixStory{{
				ID: "FIX-001", Description: "complete the requested fix",
				AcceptanceCriteria: []string{"the request is verified"},
				Tests:              []string{"the named CLI test"},
			}},
		}
		if err := writePackagedFixPlan(planPath, plan); err != nil {
			return platformprocess.CommandResult{}, err
		}
		if err := os.WriteFile(filepath.Join(filepath.Dir(planPath), workName+".md"), []byte("# Fix plan\n"), 0o600); err != nil {
			return platformprocess.CommandResult{}, err
		}
		runner.mu.Lock()
		runner.worktree = request.WorkDir
		runner.planPath = planPath
		runner.roles = append(runner.roles, "planner")
		runner.mu.Unlock()
		return packagedFixProviderResult("durable Fix plan prepared\n<COMPLETE>"), nil

	case strings.Contains(prompt, "iterative execution stage of @you/fix"):
		if runner.failForRole("iterator") {
			return platformprocess.CommandResult{}, errors.New("mock Fix iterator failure")
		}
		runner.mu.Lock()
		runner.iteratorCalls++
		call := runner.iteratorCalls
		planPath := runner.planPath
		firstContinue := runner.firstIteratorContinue
		runner.roles = append(runner.roles, "iterator")
		runner.mu.Unlock()
		if planPath == "" {
			return platformprocess.CommandResult{}, errors.New("iterator started before planner created durable Fix plan")
		}
		if firstContinue && call == 1 {
			return packagedFixProviderResult("one Fix story remains incomplete\n<CONTINUE>"), nil
		}
		plan := readPackagedFixPlanFromPath(planPath)
		plan.Stories[0].Passes = true
		plan.Stories[0].Notes = "verified by the named Fix CLI functional test"
		if err := writePackagedFixPlan(planPath, plan); err != nil {
			return platformprocess.CommandResult{}, err
		}
		candidate := "first Fix candidate"
		if call > 1 {
			candidate = "revised Fix candidate"
		}
		return packagedFixProviderResult(candidate + "\n<COMPLETE>"), nil

	case strings.Contains(prompt, "independent review stage of @you/fix"):
		if runner.failForRole("reviewer") {
			return platformprocess.CommandResult{}, errors.New("mock Fix reviewer failure")
		}
		runner.mu.Lock()
		runner.reviewerCalls++
		call := runner.reviewerCalls
		runner.roles = append(runner.roles, "reviewer")
		reject := runner.alwaysRejectReview || (runner.rejectFirstReview && call == 1)
		runner.mu.Unlock()
		if reject {
			return packagedFixProviderResult(`{"decision":"REJECTED","feedback":"reviewer requires the missing verification evidence"}`), nil
		}
		return packagedFixProviderResult(`{"decision":"ACCEPTED","output":"approved Fix result"}`), nil

	default:
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected Fix prompt: %s", prompt)
	}
}

func (runner *packagedFixCommandRunner) failForRole(role string) bool {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.failRole == role
}

func (runner *packagedFixCommandRunner) Requests() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(runner.requests))
	for index, request := range runner.requests {
		requests[index] = clonePackagedFixCommandRequest(request)
	}
	return requests
}

func (runner *packagedFixCommandRunner) Roles() []string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]string(nil), runner.roles...)
}

func (runner *packagedFixCommandRunner) PlanPath() string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.planPath
}

func (runner *packagedFixCommandRunner) ReviewerCalls() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.reviewerCalls
}

type packagedFixPlan struct {
	Project string             `json:"project"`
	Stories []packagedFixStory `json:"stories"`
}

type packagedFixStory struct {
	ID                 string   `json:"id"`
	Description        string   `json:"description"`
	AcceptanceCriteria []string `json:"acceptanceCriteria"`
	Tests              []string `json:"tests"`
	Passes             bool     `json:"passes"`
	Notes              string   `json:"notes"`
}

func writePackagedFixPlan(path string, plan packagedFixPlan) error {
	payload, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o600)
}

func readPackagedFixPlan(t *testing.T, path string) packagedFixPlan {
	t.Helper()
	return readPackagedFixPlanFromPath(path)
}

func readPackagedFixPlanFromPath(path string) packagedFixPlan {
	payload, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("read Fix plan: %v", err))
	}
	var plan packagedFixPlan
	if err := json.Unmarshal(payload, &plan); err != nil {
		panic(fmt.Sprintf("decode Fix plan: %v", err))
	}
	return plan
}

func packagedFixPlanWorkName(prompt string) string {
	matches := packagedFixPlanFile.FindStringSubmatch(prompt)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

func packagedFixProviderPrompt(request platformprocess.CommandRequest) string {
	if len(request.Stdin) > 0 {
		return string(request.Stdin)
	}
	return strings.Join(request.Args, " ")
}

func packagedFixProviderResult(output string) platformprocess.CommandResult {
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(output)}
}

func packagedFixRequestIncludesModel(request platformprocess.CommandRequest, model string) bool {
	for index := 0; index+1 < len(request.Args); index++ {
		if request.Args[index] == "--model" && request.Args[index+1] == model {
			return true
		}
	}
	return false
}

func packagedFixPrimaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	return part.Text
}

func clonePackagedFixCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func initPackagedFixGitRepository(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	runPackagedFixGit(t, workspace, "init")
	runPackagedFixGit(t, workspace, "config", "user.email", "fix-functional@example.com")
	runPackagedFixGit(t, workspace, "config", "user.name", "fix functional")
	runPackagedFixGit(t, workspace, "commit", "--allow-empty", "-m", "initial Fix functional repository")
	return workspace
}

func createPackagedFixWorktree(t *testing.T, workspace, name string) string {
	t.Helper()
	path := filepath.Join(workspace, ".worktrees", name)
	runPackagedFixGit(t, workspace, "worktree", "add", "--detach", path, "HEAD")
	return path
}

func runPackagedFixGit(t *testing.T, workspace string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = workspace
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}
