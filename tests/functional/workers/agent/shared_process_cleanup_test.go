package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func (fixture *agentSharedProcessFixture) close(t testing.TB) {
	t.Helper()
	if fixture.process == nil {
		return
	}
	if fixture.command != nil {
		// StartProcessCommand registers its cleanup after this fixture cleanup,
		// so its LIFO cleanup stops the invocation before the root closes.
		// Calling Stop here also makes this boundary safe if setup ordering is
		// changed by a future package-level fixture.
		fixture.command.Stop(t)
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), agentSharedProcessTimeout)
	defer cancel()
	if err := fixture.process.Close(closeCtx); err != nil {
		fixture.processCloseMu.Lock()
		fixture.processCloseErr = err.Error()
		fixture.processCloseMu.Unlock()
		t.Errorf("close shared agent application process: %v", err)
	} else {
		fixture.processClosed.Store(true)
	}
	if fixture.command != nil {
		select {
		case <-fixture.apiClosed:
		case <-closeCtx.Done():
			t.Errorf("shared agent API server did not close: %v", closeCtx.Err())
		}
	}
	if got := fixture.processBuilds.Load(); got != 1 {
		t.Errorf("shared agent process builds = %d, want exactly one", got)
	}
	if fixture.command != nil && fixture.apiStarts.Load() != 1 {
		t.Errorf("shared agent API starts = %d, want exactly one", fixture.apiStarts.Load())
	}
	fixture.sessionsMu.Lock()
	if len(fixture.opened) != len(fixture.closed) {
		t.Errorf("closed shared Factory Sessions = %d, opened = %d; opened=%#v closed=%#v", len(fixture.closed), len(fixture.opened), fixture.opened, fixture.closed)
	}
	fixture.sessionsMu.Unlock()
	for _, scenario := range fixture.scenarios {
		if got := scenario.runner.activeCallCount(); got != 0 {
			t.Errorf("%s active agent command calls after process cleanup = %d, want zero", scenario.name, got)
		}
	}
	fixture.router.clearRoutes()
	for _, path := range fixture.ownedPaths() {
		if err := os.RemoveAll(path); err != nil {
			t.Errorf("remove test-owned agent path %q: %v", path, err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("test-owned agent path %q remains after cleanup: %v", path, err)
		}
	}
}

func (fixture *agentSharedProcessFixture) ownedPaths() []string {
	paths := make([]string, 0, len(fixture.scenarios)+1)
	paths = append(paths, fixture.hostDir)
	for _, scenario := range fixture.scenarios {
		paths = append(paths, scenario.factoryDir)
	}
	return paths
}

func runAgentForcedCleanupParent(t *testing.T) {
	t.Helper()
	reportPath := filepath.Join(t.TempDir(), "agent-forced-cleanup.json")
	command := exec.Command(os.Args[0], "-test.run=^TestAgentSharedProcess$")
	command.Env = append(
		os.Environ(),
		agentForcedCleanupChildEnv+"=1",
		agentForcedCleanupReportEnv+"="+reportPath,
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("forced agent cleanup child exited successfully; output=%q", output)
	}
	if command.Process == nil || command.ProcessState == nil || !command.ProcessState.Exited() || command.ProcessState.ExitCode() == 0 {
		t.Fatalf("forced agent cleanup child exit state = %#v; output=%q", command.ProcessState, output)
	}
	if !strings.Contains(string(output), "intentional agent cleanup assertion") {
		t.Fatalf("forced agent cleanup child output omitted original assertion: %q", output)
	}
	report := readAgentForcedCleanupReport(t, reportPath, output)
	assertAgentForcedCleanupReport(t, report, command.Process.Pid)
}

func readAgentForcedCleanupReport(t testing.TB, path string, childOutput []byte) agentForcedCleanupReport {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read forced agent cleanup report %q: %v; child output=%q", path, err, childOutput)
	}
	var report agentForcedCleanupReport
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("decode forced agent cleanup report %q: %v; child output=%q", path, err, childOutput)
	}
	return report
}

func assertAgentForcedCleanupReport(t *testing.T, report agentForcedCleanupReport, childPID int) {
	t.Helper()
	if report.ApplicationPID != childPID {
		t.Fatalf("forced agent cleanup application PID = %d, want child PID %d", report.ApplicationPID, childPID)
	}
	if report.ProcessCloseError != "" || !report.ProcessClosed || !report.DaemonStopped {
		t.Fatalf("forced agent cleanup process state = %#v, want clean process close", report)
	}
	if !report.ListenerClosed {
		t.Fatal("forced agent cleanup API listener remained active")
	}
	if len(report.OpenedSessionIDs) != 1 || !sameAgentStringSet(report.OpenedSessionIDs, report.DeletedSessionIDs) {
		t.Fatalf("forced agent cleanup sessions opened=%v deleted=%v, want one deleted opened session", report.OpenedSessionIDs, report.DeletedSessionIDs)
	}
	if report.ActiveCalls != 0 || report.CommandCalls != 1 || !report.CommandStarted || !report.CommandFinished || report.CanceledCalls != 1 {
		t.Fatalf("forced agent cleanup command state = %#v, want one canceled and finished call with no active calls", report)
	}
	if report.ActiveCommandRoutes != 0 {
		t.Fatalf("forced agent cleanup active command routes = %d, want zero", report.ActiveCommandRoutes)
	}
	if !report.ResponseStreamClosed {
		t.Fatal("forced agent cleanup response stream did not close")
	}
	if !report.HostDirectoryAbsent || !report.ScenarioDirectoriesAbsent {
		t.Fatalf("forced agent cleanup owned directories remain: %#v", report)
	}
}

type agentForcedCleanupReport struct {
	ApplicationPID            int      `json:"application_pid"`
	ProcessClosed             bool     `json:"process_closed"`
	ProcessCloseError         string   `json:"process_close_error,omitempty"`
	DaemonStopped             bool     `json:"daemon_stopped"`
	ListenerClosed            bool     `json:"listener_closed"`
	OpenedSessionIDs          []string `json:"opened_session_ids"`
	DeletedSessionIDs         []string `json:"deleted_session_ids"`
	ActiveCommandRoutes       int      `json:"active_command_routes"`
	ActiveCalls               int      `json:"active_calls"`
	CommandCalls              int      `json:"command_calls"`
	CommandStarted            bool     `json:"command_started"`
	CommandFinished           bool     `json:"command_finished"`
	CanceledCalls             int      `json:"canceled_calls"`
	ResponseStreamClosed      bool     `json:"response_stream_closed"`
	HostDirectoryAbsent       bool     `json:"host_directory_absent"`
	ScenarioDirectoriesAbsent bool     `json:"scenario_directories_absent"`
}

type agentForcedCleanupProbe struct {
	fixture              *agentSharedProcessFixture
	scenario             agentSharedScenario
	stream               *support.FactoryResponseEventStream
	responseStreamClosed bool
}

func runAgentForcedCleanupChild(t *testing.T) {
	t.Helper()
	reportPath := strings.TrimSpace(os.Getenv(agentForcedCleanupReportEnv))
	if reportPath == "" {
		t.Fatal("forced agent cleanup report path is required")
	}

	var probe *agentForcedCleanupProbe
	var fixture *agentSharedProcessFixture
	// This cleanup is registered before the fixture so it observes the state
	// after the fixture, command, session, and stream cleanups have run.
	t.Cleanup(func() {
		if fixture == nil || probe == nil {
			return
		}
		fixture.processCloseMu.Lock()
		closeErr := fixture.processCloseErr
		fixture.processCloseMu.Unlock()
		opened, deleted := fixture.sessionIDs()
		report := agentForcedCleanupReport{
			ApplicationPID:            os.Getpid(),
			ProcessClosed:             fixture.processClosed.Load(),
			ProcessCloseError:         closeErr,
			DaemonStopped:             agentChannelClosed(fixture.command.Done()),
			ListenerClosed:            agentChannelClosed(fixture.apiClosed),
			OpenedSessionIDs:          opened,
			DeletedSessionIDs:         deleted,
			ActiveCommandRoutes:       fixture.router.routeCount(),
			CommandCalls:              probe.scenario.runner.callCount(),
			CommandStarted:            agentChannelClosed(probe.scenario.runner.started),
			CommandFinished:           agentChannelClosed(probe.scenario.runner.finished),
			CanceledCalls:             probe.scenario.runner.canceledCount(),
			ResponseStreamClosed:      probe.responseStreamClosed,
			HostDirectoryAbsent:       agentPathAbsent(fixture.hostDir),
			ScenarioDirectoriesAbsent: agentScenarioDirectoriesAbsent(fixture),
		}
		for _, scenario := range fixture.scenarios {
			report.ActiveCalls += scenario.runner.activeCallCount()
		}
		payload, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Errorf("marshal forced agent cleanup report: %v", err)
			return
		}
		if err := os.WriteFile(reportPath, payload, 0o600); err != nil {
			t.Errorf("write forced agent cleanup report: %v", err)
		}
	})

	fixture = newAgentSharedProcessFixture(t)
	fixture.start(t)
	scenario := findAgentScenario(t, fixture.scenarios, "Cancel")
	probe = &agentForcedCleanupProbe{fixture: fixture, scenario: scenario}
	sessionID := fixture.openSession(t, scenario.factoryDir)
	probe.stream = support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(fixture.baseURL, sessionID))
	t.Cleanup(func() {
		if probe.stream == nil {
			return
		}
		probe.stream.Close()
		probe.stream.WaitClosed(agentSharedProcessTimeout)
		probe.responseStreamClosed = true
	})
	content := agentTextContent(t, scenario.inputMarker)
	support.SubmitSessionWorkAt(t, fixture.baseURL, sessionID, factoryapi.SubmitWorkRequest{
		Name:         stringPointer("agent-forced-cleanup"),
		TraceId:      stringPointer("trace-agent-forced-cleanup"),
		WorkTypeName: "task",
		Content:      &content,
	})
	scenario.runner.waitStarted(t, agentSharedProcessTimeout)
	t.Fatal("intentional agent cleanup assertion after acquiring process, session, stream, route, command, and paths")
}

func (fixture *agentSharedProcessFixture) sessionIDs() ([]string, []string) {
	fixture.sessionsMu.Lock()
	defer fixture.sessionsMu.Unlock()
	opened := make([]string, 0, len(fixture.opened))
	for sessionID := range fixture.opened {
		opened = append(opened, sessionID)
	}
	deleted := make([]string, 0, len(fixture.closed))
	for sessionID := range fixture.closed {
		deleted = append(deleted, sessionID)
	}
	return opened, deleted
}

func stringPointer(value string) *string {
	return &value
}

func agentChannelClosed(channel <-chan struct{}) bool {
	select {
	case <-channel:
		return true
	default:
		return false
	}
}

func agentPathAbsent(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

func agentScenarioDirectoriesAbsent(fixture *agentSharedProcessFixture) bool {
	for _, scenario := range fixture.scenarios {
		if !agentPathAbsent(scenario.factoryDir) {
			return false
		}
	}
	return true
}

func sameAgentStringSet(left, right []string) bool {
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
