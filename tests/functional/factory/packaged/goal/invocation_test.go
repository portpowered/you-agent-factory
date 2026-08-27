package goal

import (
	"fmt"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPackagedGoalQuietCLIBatchReturnsPrimaryResultOnStdout proves packaged
// @you/goal batch invocation through the public you run CLI with --quiet writes
// only the primary result to stdout without echoing the submitted goal text.
func TestPackagedGoalQuietCLIBatchReturnsPrimaryResultOnStdout(t *testing.T) {
	scaffoldPackagedGoalBuiltInFactory(t)
	mockWorkersPath := writePackagedGoalBuiltinMockWorkersConfig(t)
	goalText := fmt.Sprintf("functional-packaged-goal-quiet-cli-%d", time.Now().UnixNano())

	stdout, stderr := runPackagedGoalQuietCLIBatch(t, mockWorkersPath, goalText)
	if stdout != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("stdout = %q, want only primary result %q", stdout, packagedGoalMockWorkerAcceptedSummary)
	}
	if strings.Contains(stdout, goalText) {
		t.Fatalf("stdout echoed submitted goal text %q", goalText)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty stderr on successful batch invocation", stderr)
	}
}

// TestPackagedGoalQuietCLIBatchExitsWithoutContinuousMode proves packaged
// @you/goal batch invocation through the public you run CLI exits after the
// invocation completes instead of staying in continuous service mode.
func TestPackagedGoalQuietCLIBatchExitsWithoutContinuousMode(t *testing.T) {
	scaffoldPackagedGoalBuiltInFactory(t)
	mockWorkersPath := writePackagedGoalBuiltinMockWorkersConfig(t)
	goalText := fmt.Sprintf("functional-packaged-goal-quiet-exit-%d", time.Now().UnixNano())

	if err := runPackagedGoalQuietCLIBatchWithTimeout(t, mockWorkersPath, goalText, 20*time.Second); err != nil {
		t.Fatalf("packaged goal quiet CLI batch invocation: %v", err)
	}
}

// TestPackagedGoalContinueRepeatsThenCompletes proves a packaged @you/goal
// continue decision feeds back through the public session invocation API,
// triggers another executor dispatch on the built-in repeater workstation, and
// then completes with the post-continue primary result.
func TestPackagedGoalContinueRepeatsThenCompletes(t *testing.T) {
	dir := scaffoldPackagedGoalBuiltInFactory(t)
	goalText := "invoke packaged goal after continue"
	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte(goalDecisionEnvelope("needs_changes", "continue with verification", "ordinary partial progress"))},
		platformprocess.CommandResult{Stdout: []byte(goalDecisionEnvelope("accepted", "", packagedGoalContinueThenCompleteSummary))},
	)

	_, response := invokePackagedGoalWithProviderRunner(t, dir, runner, goalText)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		errorCode, message := "", ""
		if response.ErrorCode != nil {
			errorCode = string(*response.ErrorCode)
		}
		if response.Message != nil {
			message = *response.Message
		}
		t.Logf("provider invocation count before failure = %d; response errorCode=%q message=%q", runner.CallCount(), errorCode, message)
	}
	assertPackagedGoalCompletedWithSummary(t, response, packagedGoalContinueThenCompleteSummary)
	if got := runner.CallCount(); got != 2 {
		t.Fatalf("provider invocation count = %d, want 2 after continue", got)
	}
	requests := runner.Requests()
	secondPrompt := string(requests[1].Stdin) + " " + strings.Join(requests[1].Args, " ")
	if !strings.Contains(secondPrompt, "state file's unchanged `objective` as authoritative") ||
		!strings.Contains(secondPrompt, "ordinary partial progress") {
		t.Fatalf("second attempt prompt does not preserve the durable objective contract and prior output: %s", secondPrompt)
	}
}

// TestPackagedGoalContinueExhaustsAtVisitBound proves the shipped loop breaker
// fails a perpetually continuing goal after exactly twelve executor visits and
// never launches a thirteenth attempt.
func TestPackagedGoalContinueExhaustsAtVisitBound(t *testing.T) {
	dir := scaffoldPackagedGoalBuiltInFactory(t)
	results := make([]platformprocess.CommandResult, 12)
	for index := range results {
		results[index] = platformprocess.CommandResult{Stdout: []byte(goalDecisionEnvelope(
			"needs_changes",
			"continue toward the visit bound",
			fmt.Sprintf("partial progress %d", index+1),
		))}
	}
	runner := support.NewShapedProviderCommandRunner(results...)

	_, response := invokePackagedGoalWithProviderRunner(t, dir, runner, "invoke packaged goal through visit exhaustion")
	assertPackagedGoalInvocationFailedWithRuntimeDetails(t, response)
	if got := runner.CallCount(); got != 12 {
		t.Fatalf("provider invocation count = %d, want exactly 12 before loop breaker", got)
	}
}

// TestPackagedGoalNeedsChangesRepeatsThenCompletes proves the packaged goal
// classifier's needs_changes decision feeds back into the same executor.
func TestPackagedGoalNeedsChangesRepeatsThenCompletes(t *testing.T) {
	dir := scaffoldPackagedGoalBuiltInFactory(t)
	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte(goalDecisionEnvelope("needs_changes", "finish the remaining work", "goal is not complete yet"))},
		platformprocess.CommandResult{Stdout: []byte(goalDecisionEnvelope("accepted", "", packagedGoalRejectThenCompleteSummary))},
	)

	_, response := invokePackagedGoalWithProviderRunner(t, dir, runner, "invoke packaged goal after reject")
	assertPackagedGoalCompletedWithSummary(t, response, packagedGoalRejectThenCompleteSummary)
	if got := runner.CallCount(); got < 2 {
		t.Fatalf("provider invocation count = %d, want at least 2 after reject-then-complete", got)
	}
}

// TestPackagedGoalBlockedDecisionStopsInInspectableBlockedState proves blocked
// is a classifier result while user/session termination remains a separate
// lifecycle control.
func TestPackagedGoalBlockedDecisionStopsInInspectableBlockedState(t *testing.T) {
	dir := scaffoldPackagedGoalBuiltInFactory(t)
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte(goalDecisionEnvelope("blocked", "requires operator credentials", "progress saved before blocker")),
	})

	_, response := invokePackagedGoalWithProviderRunner(t, dir, runner, "invoke blocked packaged goal")
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED blocked response", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.InvocationResponseErrorCode("INVOCATION_BLOCKED") {
		t.Fatalf("invocation errorCode = %#v, want INVOCATION_BLOCKED", response.ErrorCode)
	}
	if response.WorkState == nil || *response.WorkState != "goal:blocked" {
		t.Fatalf("invocation workState = %#v, want goal:blocked", response.WorkState)
	}
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("provider invocation count = %d, want 1 for blocked stop", got)
	}
}

// TestPackagedGoalPausedSubmissionResumes proves packaged @you/goal work submitted
// while the Factory Session is paused stays buffered through the public pause/resume
// control boundary and reaches the completed goal state only after resume.
func TestPackagedGoalPausedSubmissionResumes(t *testing.T) {
	dir := scaffoldPackagedGoalBuiltInFactory(t)
	server := startPackagedGoalSessionServer(t, dir)
	baseURL := strings.TrimSuffix(server.URL(), "/")
	sessionPath := "/factory-sessions/" + factorysessions.DefaultSessionID

	pause := postPackagedGoalJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		baseURL+sessionPath+"/pause",
		factoryapi.FactorySessionLifecycleControlRequest{},
		"pause packaged goal session",
	)
	if pause.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause response = %#v, want accepted pause", pause)
	}
	pauseNoOp := postPackagedGoalJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		baseURL+sessionPath+"/pause",
		factoryapi.FactorySessionLifecycleControlRequest{},
		"repeat pause packaged goal session",
	)
	if pauseNoOp.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pauseNoOp.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("repeat pause response = %#v, want no-op pause", pauseNoOp)
	}

	submitted := submitPackagedGoalWork(t, baseURL, "paused-goal-submit", "customer goal request text")
	workID := support.StringPointerValue(submitted.WorkId)
	listed := support.ListDefaultSessionWork(t, baseURL)
	if support.HasWorkAtCustomerState(listed, workID, "goal:init") {
		t.Fatalf("paused submit reached goal:init while session was paused: %#v", listed.Results)
	}
	if support.HasWorkAtCustomerState(listed, workID, "goal:complete") {
		t.Fatalf("paused submit reached goal:complete before resume: %#v", listed.Results)
	}

	resume := postPackagedGoalJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		baseURL+sessionPath+"/resume",
		factoryapi.FactorySessionLifecycleControlRequest{},
		"resume packaged goal session",
	)
	if resume.Operation != factoryapi.FactorySessionLifecycleControlKindResume ||
		resume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume response = %#v, want accepted resume", resume)
	}
	resumeNoOp := postPackagedGoalJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		baseURL+sessionPath+"/resume",
		factoryapi.FactorySessionLifecycleControlRequest{},
		"repeat resume packaged goal session",
	)
	if resumeNoOp.Operation != factoryapi.FactorySessionLifecycleControlKindResume ||
		resumeNoOp.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("repeat resume response = %#v, want no-op resume", resumeNoOp)
	}

	completed := waitForPackagedGoalWorkIDsComplete(t, baseURL, []string{workID}, 15*time.Second)
	if len(completed) != 1 || packagedGoalWorkStateName(completed[0].State) != "complete" {
		t.Fatalf("completed work = %#v, want one completed goal after resume", completed)
	}
}
