package inference_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	forcedInferenceCleanupChildEnv  = "YOU_WORKERS_FORCED_CLEANUP_CHILD"
	forcedInferenceCleanupReportEnv = "YOU_WORKERS_FORCED_CLEANUP_REPORT"
)

var forcedInferenceCleanupState *forcedInferenceCleanupProbe

// TestWorkers_ForcedAssertionFailureCleansOwnedResources proves the Workers
// inference fixture unwinds its package-local process, session, stream, route,
// command, and temporary-path resources when a scenario fails after acquiring
// them. The child process is required because a parent test cannot fail itself
// and then inspect the t.Cleanup and TestMain results it is meant to prove.
func TestWorkers_ForcedAssertionFailureCleansOwnedResources(t *testing.T) {
	if os.Getenv(forcedInferenceCleanupChildEnv) == "1" {
		runForcedInferenceCleanupChild(t)
		return
	}
	runForcedInferenceCleanupParent(t)
}

func runForcedInferenceCleanupParent(t *testing.T) {
	t.Helper()

	reportPath := filepath.Join(t.TempDir(), "forced-workers-cleanup.json")
	command := exec.Command(os.Args[0], "-test.run=^TestWorkers_ForcedAssertionFailureCleansOwnedResources$")
	command.Env = append(os.Environ(),
		forcedInferenceCleanupChildEnv+"=1",
		forcedInferenceCleanupReportEnv+"="+reportPath,
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("forced Workers cleanup child exited successfully; output=%q", output)
	}
	if command.Process == nil || command.ProcessState == nil || !command.ProcessState.Exited() {
		t.Fatalf("forced Workers cleanup child did not exit; error=%v output=%q", err, output)
	}
	if command.ProcessState.ExitCode() == 0 {
		t.Fatalf("forced Workers cleanup child exit code = 0; output=%q", output)
	}

	report := readForcedInferenceCleanupReport(t, reportPath, output)
	assertForcedInferenceCleanupReport(t, report, command.Process.Pid)
}

func readForcedInferenceCleanupReport(
	t *testing.T,
	path string,
	childOutput []byte,
) forcedInferenceCleanupReport {
	t.Helper()

	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read forced Workers cleanup report %q: %v; child output=%q", path, err, childOutput)
	}
	var report forcedInferenceCleanupReport
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("decode forced Workers cleanup report %q: %v; child output=%q", path, err, childOutput)
	}
	return report
}

func assertForcedInferenceCleanupReport(
	t *testing.T,
	report forcedInferenceCleanupReport,
	childPID int,
) {
	t.Helper()

	if report.ApplicationPID != childPID {
		t.Fatalf("forced Workers cleanup application PID = %d, want child PID %d", report.ApplicationPID, childPID)
	}
	if report.ProcessCloseError != "" || !report.ProcessClosed || !report.DaemonStopped {
		t.Fatalf("forced Workers cleanup process state = %#v, want clean process close", report)
	}
	if !report.ListenerClosed {
		t.Fatal("forced Workers cleanup listener remained reachable after process close")
	}
	if len(report.OpenedSessionIDs) != 1 || !sameStringSet(report.OpenedSessionIDs, report.DeletedSessionIDs) {
		t.Fatalf("forced Workers cleanup sessions opened=%v deleted=%v, want one deleted opened session", report.OpenedSessionIDs, report.DeletedSessionIDs)
	}
	if !report.SessionTerminal {
		t.Fatal("forced Workers cleanup session did not converge to durable terminal history after deletion")
	}
	if report.ActiveCommandRoutes != 0 || report.ActiveScriptRoutes != 0 {
		t.Fatalf("forced Workers cleanup routes commands=%d scripts=%d, want zero", report.ActiveCommandRoutes, report.ActiveScriptRoutes)
	}
	if report.CommandCalls != 1 || !report.CommandStarted || !report.CommandFinished {
		t.Fatalf("forced Workers cleanup command state = %#v, want one started and finished command", report)
	}
	if !report.Paths.RootAbsent || !report.Paths.FactoryAbsent || !report.Paths.WorkDirAbsent || !report.Paths.ArtifactAbsent {
		t.Fatalf("forced Workers cleanup owned paths remain: %#v", report.Paths)
	}
	if !report.ResponseStreamClosed {
		t.Fatal("forced Workers cleanup response stream did not close")
	}
}

func runForcedInferenceCleanupChild(t *testing.T) {
	t.Helper()

	reportPath := strings.TrimSpace(os.Getenv(forcedInferenceCleanupReportEnv))
	if reportPath == "" {
		t.Fatal("forced Workers cleanup child report path is required")
	}

	group := sharedInferenceGroup
	group.ensure(t)
	group.mu.Lock()
	defer group.mu.Unlock()

	runner := newForcedInferenceBlockingRunner()
	dir := group.hostDir
	paths := forcedInferenceCleanupPaths{
		Root:     group.rootDir,
		Factory:  dir,
		WorkDir:  filepath.Join(group.rootDir, "forced-assertion-workdir"),
		Artifact: filepath.Join(group.rootDir, "forced-assertion-artifact.json"),
	}
	if err := os.MkdirAll(paths.WorkDir, 0o755); err != nil {
		t.Fatalf("create forced Workers work directory: %v", err)
	}
	if err := os.WriteFile(paths.Artifact, []byte("owned Workers cleanup artifact"), 0o600); err != nil {
		t.Fatalf("create forced Workers cleanup artifact: %v", err)
	}

	probe := &forcedInferenceCleanupProbe{paths: paths, runner: runner}
	forcedInferenceCleanupState = probe
	scenario := sharedInferenceScenario{
		commandRunner: runner,
		scenarioName:  "forced-workers-cleanup",
	}
	releaseResponse := prepareSharedInferenceScenario(t, group, dir, scenario)
	defer releaseResponse()

	sessionID := openSharedInferenceSession(t, group, dir)
	probe.sessionID = sessionID
	t.Cleanup(func() {
		defer func() {
			group.commands.clear(dir)
			group.scripts.clear(dir)
			group.override.set(nil)
			group.workerRecordings.set(nil)
			group.setExternalRegistrations(nil)
		}()
		if probe.stream != nil {
			probe.stream.Close()
			probe.stream.WaitClosed(sharedInferenceScenarioTimeout)
			probe.responseStreamClosed = true
		}
		closeSharedInferenceSession(t, group, sessionID)
		probe.sessionTerminal = forcedInferenceSessionTerminal(group.baseURL, sessionID)
		probe.commandFinished = runner.finishedWithin(sharedInferenceScenarioTimeout)
	})
	updateSharedInferenceRouteContext(t, group, dir, scenario, sessionID)
	probe.stream = support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(group.baseURL, sessionID),
	)
	support.SubmitSessionWorkAt(t, group.baseURL, sessionID, factoryapi.SubmitWorkRequest{
		WorkTypeName: "task",
		Payload:      map[string]any{"title": "forced Workers cleanup"},
	})
	runner.waitStarted(t)

	t.Fatal("intentional assertion failure after acquiring Workers process, session, stream, route, command, and paths")
}

type forcedInferenceCleanupProbe struct {
	paths                forcedInferenceCleanupPaths
	sessionID            string
	stream               *support.FactoryResponseEventStream
	runner               *forcedInferenceBlockingRunner
	commandFinished      bool
	responseStreamClosed bool
	sessionTerminal      bool
}

type forcedInferenceCleanupPaths struct {
	Root     string
	Factory  string
	WorkDir  string
	Artifact string
}

type forcedInferenceCleanupReport struct {
	ApplicationPID       int                              `json:"application_pid"`
	ProcessClosed        bool                             `json:"process_closed"`
	ProcessCloseError    string                           `json:"process_close_error,omitempty"`
	DaemonStopped        bool                             `json:"daemon_stopped"`
	ListenerClosed       bool                             `json:"listener_closed"`
	OpenedSessionIDs     []string                         `json:"opened_session_ids"`
	DeletedSessionIDs    []string                         `json:"deleted_session_ids"`
	SessionTerminal      bool                             `json:"session_terminal"`
	ActiveCommandRoutes  int                              `json:"active_command_routes"`
	ActiveScriptRoutes   int                              `json:"active_script_routes"`
	CommandCalls         int32                            `json:"command_calls"`
	CommandStarted       bool                             `json:"command_started"`
	CommandFinished      bool                             `json:"command_finished"`
	ResponseStreamClosed bool                             `json:"response_stream_closed"`
	Paths                forcedInferenceCleanupPathReport `json:"paths"`
}

type forcedInferenceCleanupPathReport struct {
	RootAbsent     bool `json:"root_absent"`
	FactoryAbsent  bool `json:"factory_absent"`
	WorkDirAbsent  bool `json:"workdir_absent"`
	ArtifactAbsent bool `json:"artifact_absent"`
}

func writeForcedInferenceCleanupReport(group *inferenceProcessGroup, closeErr error) error {
	path := strings.TrimSpace(os.Getenv(forcedInferenceCleanupReportEnv))
	if path == "" {
		return nil
	}
	probe := forcedInferenceCleanupState
	if probe == nil {
		return fmt.Errorf("forced Workers cleanup probe was not acquired")
	}
	report := forcedInferenceCleanupReport{
		ApplicationPID:       os.Getpid(),
		ProcessClosed:        closeErr == nil && group.process != nil,
		ListenerClosed:       forcedInferenceListenerClosed(group.baseURL),
		DaemonStopped:        group.daemon == nil,
		OpenedSessionIDs:     append([]string(nil), group.openedSessionIDs...),
		DeletedSessionIDs:    append([]string(nil), group.deletedSessionIDs...),
		SessionTerminal:      probe.sessionTerminal,
		ActiveCommandRoutes:  group.commands.routeCount(),
		ActiveScriptRoutes:   group.scripts.routeCount(),
		CommandCalls:         probe.runner.calls.Load(),
		CommandStarted:       probe.runner.startedObserved(),
		CommandFinished:      probe.commandFinished && probe.runner.finishedObserved(),
		ResponseStreamClosed: probe.responseStreamClosed,
		Paths: forcedInferenceCleanupPathReport{
			RootAbsent:     forcedInferencePathAbsent(probe.paths.Root),
			FactoryAbsent:  forcedInferencePathAbsent(probe.paths.Factory),
			WorkDirAbsent:  forcedInferencePathAbsent(probe.paths.WorkDir),
			ArtifactAbsent: forcedInferencePathAbsent(probe.paths.Artifact),
		},
	}
	if closeErr != nil {
		report.ProcessCloseError = closeErr.Error()
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal forced Workers cleanup report: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write forced Workers cleanup report: %w", err)
	}
	return nil
}

func forcedInferenceListenerClosed(baseURL string) bool {
	if strings.TrimSpace(baseURL) == "" {
		return false
	}
	client := http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	response, err := client.Get(strings.TrimSuffix(baseURL, "/") + "/status")
	if err != nil {
		return true
	}
	defer response.Body.Close()
	return false
}

// forcedInferenceSessionTerminal distinguishes a deleted live session from
// its intentionally retained durable terminal history. A 404 is also clean
// for callers using a non-durable session store, while a durable 200 must be a
// terminal lifecycle state rather than an active runtime.
func forcedInferenceSessionTerminal(baseURL, sessionID string) bool {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(sessionID) == "" {
		return false
	}
	client := http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := client.Get(endpoint)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return true
	}
	if response.StatusCode != http.StatusOK {
		return false
	}
	var envelope factoryapi.FactorySessionGetResponse
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return false
	}
	durable, err := envelope.AsFactorySessionDurableReadModel()
	if err != nil {
		return false
	}
	switch durable.Status {
	case factoryapi.FactorySessionDurableLifecycleStatusCanceled,
		factoryapi.FactorySessionDurableLifecycleStatusFailed,
		factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		factoryapi.FactorySessionDurableLifecycleStatusTerminated,
		factoryapi.FactorySessionDurableLifecycleStatusTimedOut:
		return true
	default:
		return false
	}
}

func forcedInferencePathAbsent(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := seen[value]; !ok {
			return false
		}
	}
	return true
}

type forcedInferenceBlockingRunner struct {
	started    chan struct{}
	finished   chan struct{}
	startOnce  sync.Once
	finishOnce sync.Once
	calls      atomic.Int32
}

func newForcedInferenceBlockingRunner() *forcedInferenceBlockingRunner {
	return &forcedInferenceBlockingRunner{
		started:  make(chan struct{}),
		finished: make(chan struct{}),
	}
}

func (runner *forcedInferenceBlockingRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	runner.startOnce.Do(func() { close(runner.started) })
	defer runner.finishOnce.Do(func() { close(runner.finished) })
	<-ctx.Done()
	return platformprocess.CommandResult{}, ctx.Err()
}

func (runner *forcedInferenceBlockingRunner) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-runner.started:
	case <-time.After(sharedInferenceScenarioTimeout):
		t.Fatal("forced Workers cleanup command did not reach its deterministic active boundary")
	}
}

func (runner *forcedInferenceBlockingRunner) finishedWithin(timeout time.Duration) bool {
	select {
	case <-runner.finished:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (runner *forcedInferenceBlockingRunner) startedObserved() bool {
	select {
	case <-runner.started:
		return true
	default:
		return false
	}
}

func (runner *forcedInferenceBlockingRunner) finishedObserved() bool {
	select {
	case <-runner.finished:
		return true
	default:
		return false
	}
}

var _ platformprocess.CommandRunner = (*forcedInferenceBlockingRunner)(nil)
