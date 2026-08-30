package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestIntegrationSmoke_TimeoutCancelsProcessTreeAndClearsActiveExecution is
// isolated because its witness requires a real executable descendant, timeout
// propagation, process-tree termination, and OS-level cleanup.
func TestIntegrationSmoke_TimeoutCancelsProcessTreeAndClearsActiveExecution(t *testing.T) {
	support.SkipLongFunctional(t, "slow timeout cleanup smoke")
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	childPIDFile := filepath.Join(t.TempDir(), "descendant.pid")

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
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
  - '-test.run=TestIntegrationSmoke_ProcessTreeHelper'
  - '--'
  - 'spawn-child'
  - %s
timeout: 1500ms
---
Spawn a descendant and wait for the factory timeout to cancel it.
`, yamlSingleQuoted(os.Args[0]), yamlSingleQuoted(childPIDFile))
	if err := os.WriteFile(workerAgentsPath, []byte(workerAgents), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "work-timeout-cleanup-smoke",
		WorkTypeID: "task",
		TraceID:    "trace-timeout-cleanup-smoke",
		Payload:    []byte("spawn a descendant process"),
	})

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{FactoryDir: dir})
	defer server.Stop(t)
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)

	childPID := readTimeoutCleanupPID(t, childPIDFile)
	t.Cleanup(func() {
		timeoutCleanupTerminateProcess(childPID)
	})
	if !waitForTimeoutCleanupProcessExit(childPID, 3*time.Second) {
		t.Fatalf("spawned descendant process %d is still running after factory timeout", childPID)
	}

	listed := support.ListDefaultSessionWork(t, server.URL())
	assertSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
	assertDispatchOutcomeSequence(t, server.GetFactoryEvents(t), []factoryapi.WorkOutcome{
		factoryapi.WorkOutcomeFailed,
	}, "execution timeout")
}

// TestIntegrationSmoke_TimeoutRequeuesWorkAndSucceedsOnLaterAttempt is
// isolated because the real child owns attempt state and must be terminated on
// timeout before the later executable invocation can recover.
func TestIntegrationSmoke_TimeoutRequeuesWorkAndSucceedsOnLaterAttempt(t *testing.T) {
	support.SkipLongFunctional(t, "slow timeout retry smoke")
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	attemptFile := filepath.Join(t.TempDir(), "timeout-attempts.txt")

	workerAgentsPath := filepath.Join(dir, "workers", "script-worker", "AGENTS.md")
	workerAgents := fmt.Sprintf(`---
type: SCRIPT_WORKER
command: %s
args:
  - '-test.run=TestIntegrationSmoke_ProcessTreeHelper'
  - '--'
  - 'timeout-once'
  - %s
timeout: 1500ms
---
Timeout once, then succeed after the Agent Factory requeues the work.
`, yamlSingleQuoted(os.Args[0]), yamlSingleQuoted(attemptFile))
	if err := os.WriteFile(workerAgentsPath, []byte(workerAgents), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "work-timeout-requeue-smoke",
		WorkTypeID: "task",
		TraceID:    "trace-timeout-requeue-smoke",
		Payload:    []byte("timeout once and retry"),
	})

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{FactoryDir: dir})
	defer server.Stop(t)
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	assertSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})
	assertListedWorkIdentity(t, support.ListDefaultSessionWork(t, server.URL()), "done", "work-timeout-requeue-smoke", "task", "trace-timeout-requeue-smoke", nil)
	assertDispatchOutcomeSequence(t, server.GetFactoryEvents(t), []factoryapi.WorkOutcome{
		factoryapi.WorkOutcomeFailed,
		factoryapi.WorkOutcomeAccepted,
	}, "execution timeout")
}

// TestIntegrationSmoke_ProcessTreeHelper is isolated because it is the child
// executable entrypoint for the process-tree and attempt-state witnesses; an
// ordinary package invocation must remain inert.
func TestIntegrationSmoke_ProcessTreeHelper(t *testing.T) {
	if len(os.Args) < 2 {
		return
	}

	mode := os.Args[len(os.Args)-2]
	pidFile := os.Args[len(os.Args)-1]
	switch mode {
	case "spawn-child":
		spawnTimeoutCleanupChild(pidFile)
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "pid-sleep":
		writeTimeoutCleanupPID(pidFile)
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "timeout-once":
		runTimeoutOnceHelper(pidFile)
	default:
		return
	}
}

func spawnTimeoutCleanupChild(pidFile string) {
	child := exec.Command(os.Args[0],
		"-test.run=TestIntegrationSmoke_ProcessTreeHelper",
		"--",
		"pid-sleep",
		pidFile,
	)
	child.Env = os.Environ()
	if err := child.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start child: %v\n", err)
		os.Exit(2)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(pidFile); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "child did not write pid file")
	os.Exit(2)
}

func writeTimeoutCleanupPID(pidFile string) {
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write pid file: %v\n", err)
		os.Exit(2)
	}
}

func runTimeoutOnceHelper(attemptFile string) {
	attempt := readTimeoutAttempt(attemptFile) + 1
	if err := os.WriteFile(attemptFile, []byte(strconv.Itoa(attempt)), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write attempt file: %v\n", err)
		os.Exit(2)
	}
	if attempt == 1 {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	fmt.Println("recovered after timeout")
	os.Exit(0)
}

func readTimeoutAttempt(attemptFile string) int {
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

func readTimeoutCleanupPID(t *testing.T, pidFile string) int {
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

func waitForTimeoutCleanupProcessExit(pid int, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		if !timeoutCleanupProcessRunning(pid) {
			return true
		}
		select {
		case <-ctx.Done():
			return !timeoutCleanupProcessRunning(pid)
		case <-ticker.C:
		}
	}
}

func yamlSingleQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

const (
	forcedProviderCleanupChildEnv  = "YOU_PROVIDERS_FORCED_CLEANUP_CHILD"
	forcedProviderCleanupReportEnv = "YOU_PROVIDERS_FORCED_CLEANUP_REPORT"
)

// TestProviders_ForcedAssertionFailureCleansOwnedResources proves that the
// package-local process/session/router cleanup callbacks still run when a
// child test exits through an assertion failure. The child boundary is
// required because a parent test cannot intentionally fail itself and then
// inspect its own t.Cleanup results.
func TestProviders_ForcedAssertionFailureCleansOwnedResources(t *testing.T) {
	if os.Getenv(forcedProviderCleanupChildEnv) == "1" {
		runForcedProviderCleanupChild(t)
		return
	}
	runForcedProviderCleanupParent(t)
}

func runForcedProviderCleanupParent(t *testing.T) {
	t.Helper()

	reportPath := filepath.Join(t.TempDir(), "forced-provider-cleanup.json")
	command := exec.Command(os.Args[0], "-test.run=^TestProviders_ForcedAssertionFailureCleansOwnedResources$")
	command.Env = append(os.Environ(),
		forcedProviderCleanupChildEnv+"=1",
		forcedProviderCleanupReportEnv+"="+reportPath,
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("forced cleanup child exited successfully; output=%q", output)
	}
	if command.Process == nil || command.ProcessState == nil || !command.ProcessState.Exited() {
		t.Fatalf("forced cleanup child did not exit; error=%v output=%q", err, output)
	}
	if command.ProcessState.ExitCode() == 0 {
		t.Fatalf("forced cleanup child exit code = 0; output=%q", output)
	}

	report := readForcedProviderCleanupReport(t, reportPath, output)
	assertForcedProviderCleanupReport(t, report, command.Process.Pid)
}

func readForcedProviderCleanupReport(
	t *testing.T,
	path string,
	childOutput []byte,
) forcedProviderCleanupReport {
	t.Helper()

	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read forced cleanup report %q: %v; child output=%q", path, err, childOutput)
	}
	var report forcedProviderCleanupReport
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("decode forced cleanup report %q: %v; child output=%q", path, err, childOutput)
	}
	return report
}

func assertForcedProviderCleanupReport(
	t *testing.T,
	report forcedProviderCleanupReport,
	childPID int,
) {
	t.Helper()

	if report.ApplicationPID != childPID {
		t.Fatalf("forced cleanup application PID = %d, want child PID %d", report.ApplicationPID, childPID)
	}
	if !report.ProcessDone {
		t.Fatal("forced cleanup Process.Execute did not finish before cleanup report")
	}
	if !report.ListenerClosed {
		t.Fatal("forced cleanup listener remained reachable after process close")
	}
	if len(report.OpenedSessionIDs) != 1 || !reflect.DeepEqual(report.OpenedSessionIDs, report.DeletedSessionIDs) {
		t.Fatalf("forced cleanup sessions opened=%v deleted=%v, want one deleted opened session", report.OpenedSessionIDs, report.DeletedSessionIDs)
	}
	if report.ActiveRoutes != 0 {
		t.Fatalf("forced cleanup active routes = %d, want zero", report.ActiveRoutes)
	}
	if !report.Paths.RootAbsent || !report.Paths.FactoryAbsent || !report.Paths.WorkDirAbsent ||
		!report.Paths.ReplayAbsent || !report.Paths.RuntimeLogAbsent || !report.Paths.WorktreeAbsent {
		t.Fatalf("forced cleanup owned paths remain: %#v", report.Paths)
	}
}

func runForcedProviderCleanupChild(t *testing.T) {
	t.Helper()

	reportPath := strings.TrimSpace(os.Getenv(forcedProviderCleanupReportEnv))
	if reportPath == "" {
		t.Fatal("forced cleanup child report path is required")
	}

	var fixture *ProcessFixture
	var scenario *Scenario
	paths := forcedProviderCleanupPaths{}
	t.Cleanup(func() {
		if err := writeForcedProviderCleanupReport(reportPath, fixture, scenario, paths); err != nil {
			t.Errorf("write forced cleanup report: %v", err)
		}
	})

	var err error
	fixture, err = newSharedProviderProcessFixture(t)
	if err != nil {
		t.Fatalf("build isolated forced-cleanup fixture: %v", err)
	}
	t.Cleanup(func() {
		if err := fixture.close(); err != nil {
			t.Errorf("close isolated forced-cleanup fixture: %v", err)
		}
	})

	paths = prepareForcedProviderCleanupPaths(t, fixture)
	scenario = fixture.OpenScenario(
		t,
		paths.Factory,
		paths.WorkDir,
		support.NewStaticSuccessCommandRunner("forced-cleanup-output"),
	)
	t.Fatal("intentional assertion failure after acquiring process, session, route, and owned paths")
}

type forcedProviderCleanupPaths struct {
	Root       string
	Factory    string
	WorkDir    string
	Replay     string
	RuntimeLog string
	Worktree   string
}

func prepareForcedProviderCleanupPaths(
	t *testing.T,
	fixture *ProcessFixture,
) forcedProviderCleanupPaths {
	t.Helper()

	factoryDir := filepath.Join(fixture.rootDir, "forced-assertion-factory")
	if err := copySharedProviderDirectory(
		support.LegacyFixtureDir(t, "script_executor_dir"),
		factoryDir,
	); err != nil {
		t.Fatalf("copy forced-cleanup Factory: %v", err)
	}
	worktree := filepath.Join(factoryDir, "worktree")
	workDir := filepath.Join(factoryDir, "workdir")
	for _, path := range []string{worktree, workDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create forced-cleanup path %q: %v", path, err)
		}
	}
	replayPath := filepath.Join(fixture.rootDir, "forced-assertion.replay.json")
	runtimeLogPath := filepath.Join(fixture.rootDir, "forced-assertion.runtime.log")
	for _, path := range []string{replayPath, runtimeLogPath} {
		if err := os.WriteFile(path, []byte("owned test artifact"), 0o600); err != nil {
			t.Fatalf("create forced-cleanup artifact %q: %v", path, err)
		}
	}
	return forcedProviderCleanupPaths{
		Root: fixture.rootDir, Factory: factoryDir, WorkDir: workDir,
		Replay: replayPath, RuntimeLog: runtimeLogPath, Worktree: worktree,
	}
}

type forcedProviderCleanupReport struct {
	ApplicationPID    int                             `json:"application_pid"`
	ProcessDone       bool                            `json:"process_done"`
	ListenerClosed    bool                            `json:"listener_closed"`
	OpenedSessionIDs  []string                        `json:"opened_session_ids"`
	DeletedSessionIDs []string                        `json:"deleted_session_ids"`
	ActiveRoutes      int                             `json:"active_routes"`
	Paths             forcedProviderCleanupPathReport `json:"paths"`
}

type forcedProviderCleanupPathReport struct {
	RootAbsent       bool `json:"root_absent"`
	FactoryAbsent    bool `json:"factory_absent"`
	WorkDirAbsent    bool `json:"workdir_absent"`
	ReplayAbsent     bool `json:"replay_absent"`
	RuntimeLogAbsent bool `json:"runtime_log_absent"`
	WorktreeAbsent   bool `json:"worktree_absent"`
}

func writeForcedProviderCleanupReport(
	path string,
	fixture *ProcessFixture,
	scenario *Scenario,
	paths forcedProviderCleanupPaths,
) error {
	if scenario == nil {
		return fmt.Errorf("forced cleanup scenario was not opened")
	}
	opened, deleted := forcedProviderSessionIDs(fixture)
	report := forcedProviderCleanupReport{
		ApplicationPID:    os.Getpid(),
		ProcessDone:       forcedProviderChannelClosed(fixtureDone(fixture)),
		ListenerClosed:    forcedProviderListenerClosed(fixtureBaseURL(fixture)),
		OpenedSessionIDs:  opened,
		DeletedSessionIDs: deleted,
		ActiveRoutes:      forcedProviderRouteCount(fixture),
		Paths: forcedProviderCleanupPathReport{
			RootAbsent:       pathAbsent(paths.Root),
			FactoryAbsent:    pathAbsent(paths.Factory),
			WorkDirAbsent:    pathAbsent(paths.WorkDir),
			ReplayAbsent:     pathAbsent(paths.Replay),
			RuntimeLogAbsent: pathAbsent(paths.RuntimeLog),
			WorktreeAbsent:   pathAbsent(paths.Worktree),
		},
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

func forcedProviderSessionIDs(fixture *ProcessFixture) ([]string, []string) {
	if fixture == nil {
		return nil, nil
	}
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	return append([]string(nil), fixture.openedSessionIDs...), append([]string(nil), fixture.deletedSessionIDs...)
}

func fixtureDone(fixture *ProcessFixture) <-chan struct{} {
	if fixture == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return fixture.done
}

func fixtureBaseURL(fixture *ProcessFixture) string {
	if fixture == nil {
		return ""
	}
	return fixture.baseURL
}

func forcedProviderRouteCount(fixture *ProcessFixture) int {
	if fixture == nil || fixture.router == nil {
		return 0
	}
	return fixture.router.routeCount()
}

func forcedProviderChannelClosed(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func forcedProviderListenerClosed(baseURL string) bool {
	if strings.TrimSpace(baseURL) == "" {
		return false
	}
	// The fixture's shutdown signal proves Process.Execute returned; this
	// bounded HTTP probe additionally proves the public listener is no longer
	// reachable after the server's close path completed.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		strings.TrimSuffix(baseURL, "/")+"/status",
		nil,
	)
	if err != nil {
		return false
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return true
	}
	response.Body.Close()
	return false
}

func pathAbsent(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}
