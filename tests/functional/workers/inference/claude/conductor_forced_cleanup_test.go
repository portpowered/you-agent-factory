package claude

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	claudeForcedCleanupChildEnv  = "YOU_CLAUDE_FORCED_CLEANUP_CHILD"
	claudeForcedCleanupReportEnv = "YOU_CLAUDE_FORCED_CLEANUP_REPORT"
)

// TestClaudeForcedAssertionFailureCleansOwnedResources uses a child test
// process because a parent test cannot fail itself and then inspect the
// cleanup callbacks that the failure is intended to exercise.
func TestClaudeForcedAssertionFailureCleansOwnedResources(t *testing.T) {
	if os.Getenv(claudeForcedCleanupChildEnv) == "1" {
		runClaudeForcedAssertionFailureChild(t)
		return
	}

	reportPath := filepath.Join(t.TempDir(), "claude-forced-cleanup.json")
	command := exec.Command(os.Args[0], "-test.run=^TestClaudeForcedAssertionFailureCleansOwnedResources$")
	command.Env = append(
		os.Environ(),
		claudeForcedCleanupChildEnv+"=1",
		claudeForcedCleanupReportEnv+"="+reportPath,
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("forced Claude cleanup child exited successfully; output=%q", output)
	}
	if !strings.Contains(string(output), "intentional Claude cleanup assertion") {
		t.Fatalf("forced Claude cleanup child output omitted original assertion: %q", output)
	}

	report := readClaudeForcedCleanupReport(t, reportPath, output)
	if !report.ProcessClosed || !report.ListenerStopped {
		t.Fatalf("forced Claude cleanup process state = %#v, want closed process and listener; child output=%q", report, output)
	}
	if report.OpenedSessions != 1 || report.ClosedSessions != 1 {
		t.Fatalf("forced Claude cleanup sessions opened=%d closed=%d, want one each; child output=%q", report.OpenedSessions, report.ClosedSessions, output)
	}
	if report.OpenedStreams != 1 || report.ClosedStreams != 1 {
		t.Fatalf("forced Claude cleanup streams opened=%d closed=%d, want one each; child output=%q", report.OpenedStreams, report.ClosedStreams, output)
	}
	if report.ActiveCalls != 0 {
		t.Fatalf("forced Claude cleanup active command calls = %d, want 0", report.ActiveCalls)
	}
	if !report.HostDirectoryAbsent || !report.ScenarioDirectoriesAbsent {
		t.Fatalf("forced Claude cleanup owned directories remain: %#v", report)
	}
}

type claudeForcedCleanupReport struct {
	ProcessClosed             bool  `json:"process_closed"`
	ListenerStopped           bool  `json:"listener_stopped"`
	OpenedSessions            int32 `json:"opened_sessions"`
	ClosedSessions            int32 `json:"closed_sessions"`
	OpenedStreams             int32 `json:"opened_streams"`
	ClosedStreams             int32 `json:"closed_streams"`
	ActiveCalls               int   `json:"active_calls"`
	HostDirectoryAbsent       bool  `json:"host_directory_absent"`
	ScenarioDirectoriesAbsent bool  `json:"scenario_directories_absent"`
}

func runClaudeForcedAssertionFailureChild(t *testing.T) {
	t.Helper()
	reportPath := strings.TrimSpace(os.Getenv(claudeForcedCleanupReportEnv))
	if reportPath == "" {
		t.Fatal("forced Claude cleanup report path is required")
	}

	var fixture *claudeDefaultLaneFixture
	t.Cleanup(func() {
		if fixture == nil {
			return
		}
		report := claudeForcedCleanupReport{
			ProcessClosed:             fixture.processClosed.Load(),
			ListenerStopped:           claudeChannelClosed(fixture.apiStopped),
			OpenedSessions:            fixture.opened.Load(),
			ClosedSessions:            fixture.closed.Load(),
			OpenedStreams:             fixture.streamsOpened.Load(),
			ClosedStreams:             fixture.streamsClosed.Load(),
			HostDirectoryAbsent:       claudePathAbsent(fixture.hostDir),
			ScenarioDirectoriesAbsent: claudeScenarioDirectoriesAbsent(fixture),
		}
		for _, scenario := range fixture.scenarios {
			report.ActiveCalls += scenario.runner.ActiveCallCount()
		}
		payload, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Errorf("marshal forced Claude cleanup report: %v", err)
			return
		}
		if err := os.WriteFile(reportPath, payload, 0o600); err != nil {
			t.Errorf("write forced Claude cleanup report: %v", err)
		}
	})

	fixture = newClaudeDefaultLaneFixture(t)
	scenario := claudeScenarioNamed(t, fixture.scenarios, "Cancellation")
	scenario.forceAssertionFailure = true
	if t.Run("ForcedAssertion", func(t *testing.T) {
		fixture.runScenario(t, scenario)
	}) {
		t.Fatal("forced Claude cleanup scenario exited successfully")
	}
	t.Cleanup(func() {
		fixture.assertSharedProcessCleanup(t)
	})
}

func readClaudeForcedCleanupReport(t *testing.T, path string, childOutput []byte) claudeForcedCleanupReport {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read forced Claude cleanup report %q: %v; child output=%q", path, err, childOutput)
	}
	var report claudeForcedCleanupReport
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("decode forced Claude cleanup report %q: %v; child output=%q", path, err, childOutput)
	}
	return report
}

func claudeChannelClosed(channel <-chan struct{}) bool {
	select {
	case <-channel:
		return true
	default:
		return false
	}
}

func claudePathAbsent(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

func claudeScenarioDirectoriesAbsent(fixture *claudeDefaultLaneFixture) bool {
	for _, scenario := range fixture.scenarios {
		if !claudePathAbsent(scenario.factoryDir) {
			return false
		}
	}
	return true
}
