package inference_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const processCleanupWorkerTimeout = 5 * time.Second

// TestProcessGoneReleasesSameRouteAdmissionThroughRootProcess proves the
// parent-process observation closes the gap between a dead command leader and
// an inherited output pipe. The first Work leaves a descendant holding the
// pipe; the second same-route Work can complete only after the first
// workstation admission is released by PROCESS_GONE reconciliation.
func TestProcessGoneReleasesSameRouteAdmissionThroughRootProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-gone inherited-pipe observation is covered on the Unix process boundary")
	}
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	childPIDFile := filepath.Join(t.TempDir(), "process-gone-child.pid")

	workerAgentsPath := filepath.Join(dir, "workers", "script-worker", "AGENTS.md")
	workerAgents := fmt.Sprintf(`---
type: SCRIPT_WORKER
command: %s
args:
  - '-test.run=TestProcessTreeHelper'
  - '--'
  - 'process-gone'
  - '{{ (index .Inputs 0).WorkID }}'
  - %s
---
Exit the first process while a descendant holds the inherited output pipe,
then complete the second same-route Work after PROCESS_GONE reconciliation.
`, yamlSingleQuoted(os.Args[0]), yamlSingleQuoted(childPIDFile))
	if err := os.WriteFile(workerAgentsPath, []byte(workerAgents), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}

	const (
		firstWorkID  = "work-process-gone-first"
		secondWorkID = "work-process-gone-second"
	)
	for _, workID := range []string{firstWorkID, secondWorkID} {
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkID:     workID,
			WorkTypeID: "task",
			TraceID:    "trace-" + workID,
			Payload:    []byte("process gone route release"),
		})
	}

	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		processCleanupScriptEdges(t),
		20*time.Second,
	)

	childPID := readProcessCleanupPID(t, childPIDFile)
	t.Cleanup(func() {
		processCleanupTerminateProcess(childPID)
	})
	if processCleanupProcessRunning(childPID) {
		t.Fatalf("inherited-pipe descendant process %d is still running after PROCESS_GONE reconciliation", childPID)
	}
	assertProcessCleanupListedWorkIdentity(t, listed, "failed", firstWorkID, "task", "trace-"+firstWorkID, nil)
	assertProcessCleanupListedWorkIdentity(t, listed, "done", secondWorkID, "task", "trace-"+secondWorkID, nil)
	if session.Runtime.Progress.Categories.Failed != 1 || session.Runtime.Progress.Categories.Terminal != 1 {
		t.Fatalf(
			"session progress categories = %+v, want one PROCESS_GONE failure and one completed same-route Work",
			session.Runtime.Progress.Categories,
		)
	}
	if session.Runtime.Progress.Categories.Processing != 0 {
		t.Fatalf("session processing count = %d, want zero after route release", session.Runtime.Progress.Categories.Processing)
	}
	assertProcessGoneDispatchOutcomes(t, events, firstWorkID, secondWorkID)
}

// TestProcessGoneReconciliationThroughRootProcess exercises the public root
// composition with a deterministic command edge. The edge reports a started
// then exited process before returning, so this remains a cross-platform
// observable witness for workstation reconciliation and route release while
// the Unix test above covers the real inherited-pipe process boundary.
func TestProcessGoneReconciliationThroughRootProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	const (
		workID  = "work-process-gone-functional"
		traceID = "trace-process-gone-functional"
	)
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     workID,
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte("deterministic process gone reconciliation"),
	})

	session, listed, events := runSharedInferenceFactoryToCompletion(t, dir, sharedInferenceScenario{
		scriptRunner: processGoneFunctionalCommandRunner{},
	}, 20*time.Second)

	assertProcessCleanupListedWorkIdentity(t, listed, "failed", workID, "task", traceID, nil)
	if session.Runtime.Progress.Categories.Failed != 1 || session.Runtime.Progress.Categories.Terminal != 0 {
		t.Fatalf(
			"session progress categories = %+v, want one PROCESS_GONE failure and no successful terminal Work",
			session.Runtime.Progress.Categories,
		)
	}
	if session.Runtime.Progress.Categories.Processing != 0 {
		t.Fatalf("session processing count = %d, want zero after route release", session.Runtime.Progress.Categories.Processing)
	}
	assertProcessCleanupDispatchOutcomeSequence(t, events, []factoryapi.WorkOutcome{
		factoryapi.WorkOutcomeFailed,
	}, "process")
	assertDirectProcessGoneWorkerSession(t)
}

// processGoneFunctionalCommandRunner is a root.BuildProcess edge, not a
// service-level fake. It preserves the platform command contract and reports
// lifecycle facts through the request-scoped observer installed by Workers.
type processGoneFunctionalCommandRunner struct{}

func (processGoneFunctionalCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return processGoneFunctionalCommandRunner{}.RunStreaming(ctx, request, nil)
}

func (processGoneFunctionalCommandRunner) RunStreaming(
	ctx context.Context,
	request platformprocess.CommandRequest,
	_ platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	if observer := request.ProcessLifecycleObserver; observer != nil {
		observer.ProcessStarted(platformprocess.ProcessInfo{PID: 1})
		observer.ProcessExited(platformprocess.ProcessInfo{PID: 1})
	}
	return platformprocess.CommandResult{}, ctx.Err()
}

type processGoneFunctionalProvider struct {
	testutil.NativeProvider
}

func (provider processGoneFunctionalProvider) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	if observer := request.ProcessLifecycleObserver; observer != nil {
		observer.ProcessStarted(platformprocess.ProcessInfo{PID: 1})
		observer.ProcessExited(platformprocess.ProcessInfo{PID: 1})
	}
	return providers.ExecuteResult{}, ctx.Err()
}

func assertDirectProcessGoneWorkerSession(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	withSharedInferenceProcess(t, sharedInferenceScenario{
		providerOverride: processGoneFunctionalProvider{},
	}, func(process support.ApplicationProcess) {
		homeDir := t.TempDir()
		inputs := support.FakeInputs(ctx, []string{
			"you", "--json", "worker-sessions", "invoke",
			"--request-id", "process-gone-direct-request",
			"--worker-session-id", "process-gone-direct-session",
			"--dispatch-id", "process-gone-direct-dispatch",
			"--workstation", "direct",
			"--worker-type", "direct-worker",
			"--runner", "codex",
			"--provider", "codex",
			"--model", "functional-model",
			"--user-message", "reconcile the gone process",
		})
		inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
		inputs.Input.WorkingDirectory = t.TempDir()
		if err := process.Execute(inputs.Input); err == nil {
			t.Fatal("direct Worker Session process-gone invocation succeeded, want terminal failure")
		}

		var response factoryapi.ErrorResponse
		for _, output := range []string{inputs.Stderr(), inputs.Stdout()} {
			if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &response); err == nil && response.Code != "" {
				if response.Code != factoryapi.ErrorResponseCode("WORKER_SESSION_FAILED") {
					t.Fatalf("direct process-gone Worker Session code = %q, want WORKER_SESSION_FAILED; stderr=%s stdout=%s", response.Code, inputs.Stderr(), inputs.Stdout())
				}
				if !strings.Contains(strings.ToLower(response.Message), "process exited") {
					t.Fatalf("direct process-gone Worker Session message = %q, want process-exited diagnostic", response.Message)
				}
				return
			}
		}
		t.Fatalf("direct process-gone Worker Session emitted no typed failure; stderr=%s stdout=%s", inputs.Stderr(), inputs.Stdout())
	})
}

// TestProviderTimeoutTerminatesChildProcessTree proves a timed-out script-worker
// invocation tears down its spawned descendant process tree and clears active
// execution so the public Work listing and Factory Event stream show a terminal
// timeout failure with no lingering in-progress dispatch.
func TestProviderTimeoutTerminatesChildProcessTree(t *testing.T) {
	support.SkipLongFunctional(t, "slow timeout process-tree cleanup proof")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	childPIDFile := filepath.Join(t.TempDir(), "descendant.pid")

	support.UpdateFactoryConfig(t, dir, func(cfg map[string]any) {
		cfg["workstations"] = append(cfg["workstations"].([]any), map[string]any{
			"name":     "timeout-cleanup-loop-breaker",
			"behavior": "STANDARD",
			"type":     "LOGICAL_MOVE",
			"inputs":   []map[string]any{{"workType": "task", "state": "init"}},
			"outputs":  []map[string]any{{"workType": "task", "state": "failed"}},
			"guards": []map[string]any{{
				"type":        "VISIT_COUNT",
				"workstation": "run-script",
				"maxVisits":   float64(1),
			}},
		})
	})

	workerAgentsPath := filepath.Join(dir, "workers", "script-worker", "AGENTS.md")
	workerAgents := fmt.Sprintf(`---
type: SCRIPT_WORKER
command: %s
args:
  - '-test.run=TestProcessTreeHelper'
%s  - '--'
  - 'spawn-child'
  - %s
timeout: %s
---
Spawn a descendant and wait for the factory timeout to cancel it.
`, yamlSingleQuoted(os.Args[0]), processCleanupCoverageTestArg(), yamlSingleQuoted(childPIDFile), processCleanupWorkerTimeout.String())
	if err := os.WriteFile(workerAgentsPath, []byte(workerAgents), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "work-timeout-cleanup-smoke",
		WorkTypeID: "task",
		TraceID:    "trace-timeout-cleanup-smoke",
		Payload:    []byte("spawn a descendant process"),
	})

	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		processCleanupScriptEdges(t),
		20*time.Second,
	)

	childPID := readProcessCleanupPID(t, childPIDFile)
	t.Cleanup(func() {
		processCleanupTerminateProcess(childPID)
	})
	if !waitForProcessCleanupExit(childPID, 3*time.Second) {
		t.Fatalf("spawned descendant process %d is still running after factory timeout", childPID)
	}

	assertProcessCleanupSessionPlaces(t, listed, map[string]int{
		"task:failed": 1,
		"task:init":   0,
		"task:done":   0,
	})
	assertProcessCleanupDispatchOutcomeSequence(t, events, []factoryapi.WorkOutcome{
		factoryapi.WorkOutcomeFailed,
	}, "execution timeout")
	if session.Runtime.Progress.Categories.Failed != 1 {
		t.Fatalf(
			"session progress categories = %+v, want one failed work item and cleared active execution",
			session.Runtime.Progress.Categories,
		)
	}
	if session.Runtime.Progress.Categories.Processing != 0 {
		t.Fatalf(
			"session processing count = %d, want 0 after timeout cleanup",
			session.Runtime.Progress.Categories.Processing,
		)
	}
}

// TestProcessTreeHelper is invoked as the external script command for timeout
// cleanup proofs. It spawns a descendant process, records its PID, and blocks
// until the factory timeout cancels the process tree.
func TestProcessTreeHelper(t *testing.T) {
	mode, args := processCleanupHelperArgs()
	if mode == "" {
		return
	}

	switch mode {
	case "spawn-child":
		if len(args) < 1 {
			return
		}
		pidFile := args[0]
		spawnProcessCleanupChild(pidFile)
		time.Sleep(30 * time.Second)
		finishProcessCleanupHelper()
		return
	case "pid-sleep":
		time.Sleep(30 * time.Second)
		finishProcessCleanupHelper()
		return
	case "companion-timeout-once":
		if len(args) < 1 {
			return
		}
		pidFile := args[0]
		runCompanionTimeoutOnceHelper(pidFile)
		finishProcessCleanupHelper()
		return
	case "process-gone":
		if len(args) < 2 {
			return
		}
		runProcessGoneHelper(args[0], args[1])
		finishProcessCleanupHelper()
		return
	case "timeout-once":
		if len(args) < 1 {
			return
		}
		pidFile := args[0]
		runTimeoutOnceHelper(pidFile)
		finishProcessCleanupHelper()
		return
	default:
		return
	}
}

func processCleanupHelperArgs() (string, []string) {
	separator := -1
	for index := len(os.Args) - 1; index >= 0; index-- {
		if os.Args[index] == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		return "", nil
	}
	return os.Args[separator+1], os.Args[separator+2:]
}

// TestProviderSuccessWaitsForProcessAndStreamClosure proves a successful script-worker
// invocation waits for the provider process to exit and its stdout stream to close
// before surfacing a public terminal success outcome, and that later success after a
// prior timeout only settles once timeout cleanup has finished.
func TestProviderSuccessWaitsForProcessAndStreamClosure(t *testing.T) {
	support.SkipLongFunctional(t, "slow timeout retry and success cleanup proof")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	attemptFile := filepath.Join(t.TempDir(), "timeout-attempts.txt")
	providerPIDFile := attemptFile + ".provider.pid"
	traceID := "trace-timeout-requeue-smoke"
	workID := "work-timeout-requeue-smoke"
	const successStdout = "recovered after timeout"

	workerAgentsPath := filepath.Join(dir, "workers", "script-worker", "AGENTS.md")
	workerAgents := fmt.Sprintf(`---
type: SCRIPT_WORKER
command: %s
args:
  - '-test.run=TestProcessTreeHelper'
%s  - '--'
  - 'timeout-once'
  - %s
timeout: %s
---
Timeout once, then succeed after the Agent Factory requeues the work.
`, yamlSingleQuoted(os.Args[0]), processCleanupCoverageTestArg(), yamlSingleQuoted(attemptFile), processCleanupWorkerTimeout.String())
	if err := os.WriteFile(workerAgentsPath, []byte(workerAgents), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     workID,
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte("timeout once and retry"),
	})

	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		processCleanupScriptEdges(t),
		20*time.Second,
	)

	assertProcessCleanupSessionPlaces(t, listed, map[string]int{
		"task:done":   1,
		"task:init":   0,
		"task:failed": 0,
	})
	assertProcessCleanupListedWorkIdentity(t, listed, "done", workID, "task", traceID, nil)
	assertProcessCleanupDispatchOutcomeSequence(t, events, []factoryapi.WorkOutcome{
		factoryapi.WorkOutcomeFailed,
		factoryapi.WorkOutcomeAccepted,
	}, "execution timeout")
	assertProcessCleanupScriptResponseBeforeAcceptedDispatch(t, events, successStdout)
	if _, err := os.Stat(providerPIDFile); err != nil {
		t.Fatalf("provider pid file missing after successful attempt: %v", err)
	}
	providerPID := readProcessCleanupPID(t, providerPIDFile)
	t.Cleanup(func() {
		processCleanupTerminateProcess(providerPID)
	})
	if processCleanupProcessRunning(providerPID) {
		t.Fatalf("provider process %d is still running after terminal success", providerPID)
	}
	if session.Runtime.Progress.Categories.Terminal != 1 {
		t.Fatalf(
			"session progress categories = %+v, want one terminal work item after timeout requeue success",
			session.Runtime.Progress.Categories,
		)
	}
	if session.Runtime.Progress.Categories.Processing != 0 {
		t.Fatalf(
			"session processing count = %d, want 0 after success-path cleanup",
			session.Runtime.Progress.Categories.Processing,
		)
	}
}

func runTimeoutOnceHelper(attemptFile string) {
	attempt := readProcessCleanupAttempt(attemptFile) + 1
	if err := os.WriteFile(attemptFile, []byte(strconv.Itoa(attempt)), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write attempt file: %v\n", err)
		os.Exit(2)
	}
	if attempt == 1 {
		time.Sleep(30 * time.Second)
		return
	}
	writeProcessCleanupPID(attemptFile + ".provider.pid")
	fmt.Println("recovered after timeout")
}

func runCompanionTimeoutOnceHelper(attemptFile string) {
	attempt := readProcessCleanupAttempt(attemptFile) + 1
	if err := os.WriteFile(attemptFile, []byte(strconv.Itoa(attempt)), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write attempt file: %v\n", err)
		os.Exit(2)
	}
	if attempt == 1 {
		spawnProcessCleanupChild(attemptFile + ".companion.pid")
		time.Sleep(30 * time.Second)
		return
	}
	fmt.Println("recovered after companion timeout")
}

func runProcessGoneHelper(workID, childPIDFile string) {
	if strings.HasSuffix(workID, "-first") {
		spawnProcessCleanupChild(childPIDFile)
		return
	}
	fmt.Println("recovered after process gone COMPLETE")
}

func finishProcessCleanupHelper() {
	if os.Getenv("GOCOVERDIR") == "" {
		os.Exit(0)
	}
	// The coverage-instrumented test binary is reused as a child command, but
	// its external test package has no coverable statements. Returning normally
	// avoids the coverage exit-hook diagnostic; redirecting the harness tail
	// keeps the child protocol's intended stdout unchanged.
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open helper output sink: %v\n", err)
		os.Exit(2)
	}
	os.Stdout = null
}

func readProcessCleanupAttempt(attemptFile string) int {
	raw, err := os.ReadFile(attemptFile)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "read attempt file: %v\n", err)
		os.Exit(2)
	}
	attempt, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse attempt file %q: %v\n", raw, err)
		os.Exit(2)
	}
	return attempt
}

func spawnProcessCleanupChild(pidFile string) {
	args := []string{"-test.run=TestProcessTreeHelper"}
	if coverageArg := processCleanupCoverageTestArgValue(); coverageArg != "" {
		args = append(args, coverageArg)
	}
	args = append(args, "--", "pid-sleep", pidFile)
	child := exec.Command(os.Args[0], args...)
	child.Env = os.Environ()
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start child: %v\n", err)
		os.Exit(2)
	}
	if err := publishProcessCleanupPID(pidFile, child.Process.Pid); err != nil {
		fmt.Fprintf(os.Stderr, "publish child pid file: %v\n", err)
		_ = child.Process.Kill()
		os.Exit(2)
	}
}

func writeProcessCleanupPID(pidFile string) {
	if err := publishProcessCleanupPID(pidFile, os.Getpid()); err != nil {
		fmt.Fprintf(os.Stderr, "write pid file: %v\n", err)
		os.Exit(2)
	}
}

func publishProcessCleanupPID(pidFile string, pid int) error {
	// Publish the complete PID atomically so the parent can use file existence
	// as an authoritative readiness signal instead of observing an empty file
	// between creation and write completion.
	temporaryPIDFile := pidFile + ".tmp"
	if err := os.WriteFile(temporaryPIDFile, []byte(strconv.Itoa(pid)), 0o600); err != nil {
		return err
	}
	return os.Rename(temporaryPIDFile, pidFile)
}

func readProcessCleanupPID(t *testing.T, pidFile string) int {
	t.Helper()

	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read descendant pid file: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("parse descendant pid %q: %v", raw, err)
	}
	return pid
}

func waitForProcessCleanupExit(pid int, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		if !processCleanupProcessRunning(pid) {
			return true
		}
		select {
		case <-ctx.Done():
			return !processCleanupProcessRunning(pid)
		case <-ticker.C:
		}
	}
}

func assertProcessCleanupSessionPlaces(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}

func assertProcessCleanupDispatchOutcomeSequence(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wants []factoryapi.WorkOutcome,
	firstError string,
) {
	t.Helper()
	responses := processCleanupDispatchResponses(t, events)
	if len(responses) < len(wants) {
		t.Fatalf("dispatch response count = %d, want at least %d", len(responses), len(wants))
	}
	for index, want := range wants {
		if responses[index].Outcome != want {
			t.Errorf("dispatch response %d outcome = %s, want %s", index, responses[index].Outcome, want)
		}
	}
	if firstError != "" && (responses[0].Error == nil || !strings.Contains(*responses[0].Error, firstError)) {
		t.Errorf("first dispatch error = %#v, want text %q", responses[0].Error, firstError)
	}
}

func assertProcessGoneDispatchOutcomes(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	firstWorkID, secondWorkID string,
) {
	t.Helper()
	var first, second *support.DispatchEventObservation
	observations := support.ObserveDispatchEvents(t, events)
	for index := range observations {
		dispatch := &observations[index]
		if support.DispatchObservationIncludesWork(*dispatch, firstWorkID) {
			first = dispatch
		}
		if support.DispatchObservationIncludesWork(*dispatch, secondWorkID) {
			second = dispatch
		}
	}
	if first == nil || first.Response == nil {
		t.Fatalf("PROCESS_GONE dispatch for work %q = %#v, want terminal response", firstWorkID, first)
	}
	if first.Response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("PROCESS_GONE dispatch outcome = %s, want FAILED", first.Response.Outcome)
	}
	if !strings.Contains(strings.ToLower(support.StringPointerValue(first.Response.Error)), "process") {
		t.Fatalf("PROCESS_GONE dispatch error = %q, want process classification", support.StringPointerValue(first.Response.Error))
	}
	if second == nil || second.Response == nil {
		t.Fatalf("same-route follow-up dispatch for work %q = %#v, want terminal response", secondWorkID, second)
	}
	if second.Response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("same-route follow-up dispatch outcome = %s, want ACCEPTED", second.Response.Outcome)
	}
}

func assertProcessCleanupScriptResponseBeforeAcceptedDispatch(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wantStdout string,
) {
	t.Helper()

	scriptResponseIndex, scriptResponse := processCleanupSucceededScriptResponse(t, events)
	if scriptResponse.Outcome != factoryapi.ScriptExecutionOutcomeSucceeded {
		t.Fatalf("script response outcome = %s, want succeeded", scriptResponse.Outcome)
	}
	if scriptResponse.ExitCode == nil || *scriptResponse.ExitCode != 0 {
		t.Fatalf("script response exit code = %#v, want 0", scriptResponse.ExitCode)
	}
	if strings.TrimSpace(scriptResponse.Stdout) != wantStdout {
		t.Fatalf("script response stdout = %q, want %q", scriptResponse.Stdout, wantStdout)
	}
	if scriptResponse.Stderr != "" {
		t.Fatalf("script response stderr = %q, want empty", scriptResponse.Stderr)
	}

	dispatchResponseIndex := processCleanupAcceptedDispatchIndexForDispatchID(t, events, scriptResponse.DispatchId)
	if dispatchResponseIndex <= scriptResponseIndex {
		t.Fatalf(
			"dispatch response index = %d, script response index = %d; want script stream closure before accepted dispatch",
			dispatchResponseIndex,
			scriptResponseIndex,
		)
	}
}

func processCleanupSucceededScriptResponse(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) (int, factoryapi.ScriptResponseEventPayload) {
	t.Helper()

	for index, event := range events {
		if event.Type != factoryapi.FactoryEventTypeScriptResponse {
			continue
		}
		payload, err := event.Payload.AsScriptResponseEventPayload()
		if err != nil {
			t.Fatalf("decode script response: %v", err)
		}
		if payload.Outcome != factoryapi.ScriptExecutionOutcomeSucceeded {
			continue
		}
		return index, payload
	}
	t.Fatalf("factory events do not contain a succeeded script response")
	return -1, factoryapi.ScriptResponseEventPayload{}
}

func processCleanupAcceptedDispatchIndexForDispatchID(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	dispatchID string,
) int {
	t.Helper()

	for index, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		if support.StringPointerValue(event.Context.DispatchId) != dispatchID {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Outcome != factoryapi.WorkOutcomeAccepted {
			continue
		}
		return index
	}
	t.Fatalf("factory events do not contain accepted dispatch response for dispatch %s", dispatchID)
	return -1
}

func assertProcessCleanupListedWorkIdentity(
	t *testing.T,
	response factoryapi.ListWorkResponse,
	stateName, workID, workType, traceID string,
	tags map[string]string,
) {
	t.Helper()
	for _, item := range response.Results {
		if item.State == nil || item.State.Name != stateName {
			continue
		}
		if workID != "" && (item.WorkId == nil || *item.WorkId != workID) {
			t.Errorf("listed Work ID = %#v, want %q", item.WorkId, workID)
		}
		if item.WorkTypeName == nil || *item.WorkTypeName != workType {
			t.Errorf("listed Work type = %#v, want %q", item.WorkTypeName, workType)
		}
		if traceID != "" && (item.TraceId == nil || *item.TraceId != traceID) {
			t.Errorf("listed Work trace ID = %#v, want %q", item.TraceId, traceID)
		}
		for key, want := range tags {
			if item.Tags == nil || (*item.Tags)[key] != want {
				t.Errorf("listed Work tag %q = %#v, want %q", key, item.Tags, want)
			}
		}
		return
	}
	t.Fatalf("listed Work has no item in state %q: %#v", stateName, response.Results)
}

func processCleanupDispatchResponses(t *testing.T, events []factoryapi.FactoryEvent) []factoryapi.DispatchResponseEventPayload {
	t.Helper()
	var responses []factoryapi.DispatchResponseEventPayload
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		responses = append(responses, payload)
	}
	return responses
}

func processCleanupScriptEdges(t *testing.T) serviceedges.Edges {
	t.Helper()
	stateReader := platformprocess.NewProcfsProcessStateReader(os.ReadFile)
	runner, err := platformprocess.NewExecCommandRunner(exec.Command, platformclock.Real{}, nil, stateReader)
	if err != nil {
		t.Fatalf("construct process cleanup script command runner: %v", err)
	}
	return serviceedges.Edges{ScriptCommandRunner: runner}
}

func yamlSingleQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func processCleanupCoverageTestArg() string {
	coverageArg := processCleanupCoverageTestArgValue()
	if coverageArg == "" {
		return ""
	}
	return fmt.Sprintf("  - %s\n", yamlSingleQuoted(coverageArg))
}

func processCleanupCoverageTestArgValue() string {
	coverageDir := os.Getenv("GOCOVERDIR")
	if coverageDir == "" {
		return ""
	}
	return "-test.gocoverdir=" + coverageDir
}
