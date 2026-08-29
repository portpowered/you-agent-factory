package acceptance

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	invokeContinueForcedCleanupChildEnv  = "YOU_INVOKE_CONTINUE_FORCED_CLEANUP_CHILD"
	invokeContinueForcedCleanupReportEnv = "YOU_INVOKE_CONTINUE_FORCED_CLEANUP_REPORT"
)

// TestInvokeContinueForcedAssertionCleansOwnedResources uses a child test
// process so the parent can observe the original assertion failure and the
// package TestMain cleanup census independently.
func TestInvokeContinueForcedAssertionCleansOwnedResources(t *testing.T) {
	if os.Getenv(invokeContinueForcedCleanupChildEnv) == "1" {
		runInvokeContinueForcedAssertionChild(t)
		return
	}

	reportPath := filepath.Join(t.TempDir(), "invoke-continue-forced-cleanup.json")
	command := exec.Command(os.Args[0], "-test.run=^TestInvokeContinueForcedAssertionCleansOwnedResources$")
	command.Env = append(
		os.Environ(),
		invokeContinueForcedCleanupChildEnv+"=1",
		invokeContinueForcedCleanupReportEnv+"="+reportPath,
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("forced invoke/continue cleanup child exited successfully; output=%q", output)
	}
	if !strings.Contains(string(output), "intentional invoke/continue cleanup assertion") {
		t.Fatalf("forced invoke/continue cleanup child output omitted original assertion: %q", output)
	}

	report := readInvokeContinueForcedCleanupReport(t, reportPath, output)
	if report.ProcessBuilds != 1 || report.APIStarts != 1 {
		t.Fatalf("forced invoke/continue spine builds=%d starts=%d, want one each; child output=%q", report.ProcessBuilds, report.APIStarts, output)
	}
	if !report.ProcessClosed || !report.ListenerStopped || report.ListenerReachable || report.PortAvailable {
		t.Fatalf("forced invoke/continue process/listener census = %#v, want closed and unreachable; child output=%q", report, output)
	}
	if report.OpenedSessions != 1 || report.ClosedSessions != 1 || report.DeletedSessions != 1 {
		t.Fatalf("forced invoke/continue sessions opened=%d closed=%d deleted=%d, want one each; child output=%q", report.OpenedSessions, report.ClosedSessions, report.DeletedSessions, output)
	}
	if report.OpenedStreams != 1 || report.ClosedStreams != 1 {
		t.Fatalf("forced invoke/continue streams opened=%d closed=%d, want one each; child output=%q", report.OpenedStreams, report.ClosedStreams, output)
	}
	if report.ActiveCalls != 0 || report.ActiveProviderCalls != 0 || report.RoutesRemaining != 0 {
		t.Fatalf("forced invoke/continue calls=%d provider-calls=%d routes=%d, want zero; child output=%q", report.ActiveCalls, report.ActiveProviderCalls, report.RoutesRemaining, output)
	}
	if !report.OwnedRootAbsent {
		t.Fatalf("forced invoke/continue owned root remains: %#v; child output=%q", report, output)
	}
}

type invokeContinueForcedCleanupReport struct {
	ProcessBuilds       int32 `json:"process_builds"`
	APIStarts           int32 `json:"api_starts"`
	ProcessClosed       bool  `json:"process_closed"`
	ListenerStopped     bool  `json:"listener_stopped"`
	ListenerReachable   bool  `json:"listener_reachable"`
	PortAvailable       bool  `json:"port_available"`
	OpenedSessions      int   `json:"opened_sessions"`
	ClosedSessions      int   `json:"closed_sessions"`
	DeletedSessions     int   `json:"deleted_sessions"`
	OpenedStreams       int32 `json:"opened_streams"`
	ClosedStreams       int32 `json:"closed_streams"`
	ActiveCalls         int   `json:"active_calls"`
	ActiveProviderCalls int   `json:"active_provider_calls"`
	RoutesRemaining     int   `json:"routes_remaining"`
	OwnedRootAbsent     bool  `json:"owned_root_absent"`
}

func runInvokeContinueForcedAssertionChild(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv(invokeContinueForcedCleanupReportEnv)) == "" {
		t.Fatal("forced invoke/continue cleanup report path is required")
	}

	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "manager-isolation")
	t.Cleanup(fixture.managerRunner.releaseAll)
	ids := newS8ScenarioIdentities("manager", scenario.runNumber)

	ctx := t.Context()
	invokeS8RemoteWorker(t, ctx, fixture.process, invokeContinueEnvironment(fixture.homeDir), scenario.workingDirectory, fixture.baseURL, s8RemoteWorkerInvocation{
		requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
		factorySessionID: scenario.session.id, repository: fixture.managerRepositoryA.path, workID: ids.workA, message: s8MessageA,
	})
	fixture.managerRunner.waitStarted(t, fixture.managerRepositoryA.path, fixture.router.requests)
	stream := startS8LiveStream(t, fixture, ctx, fixture.process, invokeContinueEnvironment(fixture.homeDir), scenario.workingDirectory, fixture.baseURL, scenario.session.id, ids.workerA, s8ProviderSessionA)
	stream.writer.waitWorkerSessionFrame(t, ids.workerA)
	fixture.managerRunner.release(t, fixture.managerRepositoryA.path)
	waitS8Stream(t, stream, ids.workerA)

	t.Fatal("intentional invoke/continue cleanup assertion after acquiring session, stream, route, and command")
}

func readInvokeContinueForcedCleanupReport(t *testing.T, path string, childOutput []byte) invokeContinueForcedCleanupReport {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read forced invoke/continue cleanup report %q: %v; child output=%q", path, err, childOutput)
	}
	var report invokeContinueForcedCleanupReport
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("decode forced invoke/continue cleanup report %q: %v; child output=%q", path, err, childOutput)
	}
	return report
}

func writeInvokeContinueForcedCleanupReport(fixture *invokeContinuePackageFixture) error {
	if fixture == nil {
		return nil
	}
	reportPath := strings.TrimSpace(os.Getenv(invokeContinueForcedCleanupReportEnv))
	if reportPath == "" {
		return nil
	}

	fixture.sessionsMu.Lock()
	report := invokeContinueForcedCleanupReport{
		ProcessBuilds:   fixture.processBuilds.Load(),
		APIStarts:       fixture.apiStarts.Load(),
		ProcessClosed:   fixture.processClosed.Load(),
		ListenerStopped: invokeContinueChannelClosed(fixture.apiStopped),
		OpenedSessions:  len(fixture.openedSessionIDs),
		ClosedSessions:  len(fixture.closedSessionIDs),
		DeletedSessions: len(fixture.deletedSessionIDs),
	}
	fixture.sessionsMu.Unlock()

	report.OpenedStreams = fixture.streamsOpened.Load()
	report.ClosedStreams = fixture.streamsClosed.Load()
	report.ActiveCalls = fixture.router.activeCallCount()
	report.ActiveProviderCalls = fixture.activeProviderCallCount()
	report.RoutesRemaining = fixture.router.routeCount()
	report.OwnedRootAbsent = invokeContinuePathAbsent(fixture.rootDir)
	var err error
	if report.ListenerReachable, err = invokeContinueListenerReachable(fixture.baseURL); err != nil {
		return fmt.Errorf("probe forced cleanup listener: %w", err)
	}
	if report.PortAvailable, err = invokeContinuePortAvailable(fixture.baseURL); err != nil {
		return fmt.Errorf("probe forced cleanup port: %w", err)
	}

	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal forced invoke/continue cleanup report: %w", err)
	}
	if err := os.WriteFile(reportPath, payload, 0o600); err != nil {
		return fmt.Errorf("write forced invoke/continue cleanup report: %w", err)
	}
	return nil
}

func invokeContinueChannelClosed(channel <-chan struct{}) bool {
	select {
	case <-channel:
		return true
	default:
		return false
	}
}

func invokeContinuePathAbsent(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}
