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

// TestPackagedGoalAcceptCompletesWithSummary proves packaged @you/goal invocation
// completes through the public session invocation API with an explicit summary
// primary result distinct from the submitted goal text.
func TestPackagedGoalAcceptCompletesWithSummary(t *testing.T) {
	dir := scaffoldPackagedGoalBuiltInFactory(t)
	mockWorkersPath := writePackagedGoalBuiltinMockWorkersConfig(t)

	submitted := "customer goal request text"
	response := postPackagedGoalInvocation(t, dir, mockWorkersPath, submitted)
	assertPackagedGoalCompletedWithSummary(t, response, packagedGoalMockWorkerAcceptedSummary)
	if primaryResultText(t, response) == submitted {
		t.Fatal("primaryResult echoed submitted goal text")
	}
}

// TestPackagedGoalContinueRepeatsThenCompletes proves a packaged @you/goal
// continue decision feeds back through the public session invocation API,
// triggers another executor dispatch on the built-in repeater workstation, and
// then completes with the post-continue primary result.
func TestPackagedGoalContinueRepeatsThenCompletes(t *testing.T) {
	dir := scaffoldPackagedGoalBuiltInFactory(t)
	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("ordinary partial progress\n<CONTINUE>")},
		platformprocess.CommandResult{Stdout: []byte(packagedGoalContinueThenCompleteSummary + "\n<COMPLETE>")},
	)

	_, response := invokePackagedGoalWithProviderRunner(t, dir, runner, "invoke packaged goal after continue")
	assertPackagedGoalCompletedWithSummary(t, response, packagedGoalContinueThenCompleteSummary)
	if got := runner.CallCount(); got != 2 {
		t.Fatalf("provider invocation count = %d, want 2 after continue", got)
	}
}

// TestPackagedGoalRejectRepeatsThenCompletes proves a packaged @you/goal reject
// decision feeds back through the public session invocation API, triggers another
// executor dispatch on the built-in repeater workstation, and then completes with
// the post-reject primary result.
func TestPackagedGoalRejectRepeatsThenCompletes(t *testing.T) {
	dir := scaffoldPackagedGoalBuiltInFactory(t)
	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("goal is not complete yet")},
		platformprocess.CommandResult{Stdout: []byte(packagedGoalRejectThenCompleteSummary + "\n<COMPLETE>")},
	)

	_, response := invokePackagedGoalWithProviderRunner(t, dir, runner, "invoke packaged goal after reject")
	assertPackagedGoalCompletedWithSummary(t, response, packagedGoalRejectThenCompleteSummary)
	if got := runner.CallCount(); got < 2 {
		t.Fatalf("provider invocation count = %d, want at least 2 after reject-then-complete", got)
	}
}

// TestPackagedGoalUnknownDecisionFails proves packaged @you/goal invocation through
// the public session invocation API fails with stable runtime-failure details and no
// success primary result when mock workers surface an invalid worker outcome on the
// built-in execute-goal topology.
func TestPackagedGoalUnknownDecisionFails(t *testing.T) {
	dir := scaffoldPackagedGoalBuiltInFactory(t)
	mockWorkersPath := writePackagedGoalFailingMockWorkersConfig(t)

	response := postPackagedGoalInvocation(t, dir, mockWorkersPath, "invoke packaged goal with failing worker")
	assertPackagedGoalInvocationFailedWithRuntimeDetails(t, response)
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
