package cli_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	forcedProviderSessionsCleanupChildEnv  = "YOU_PROVIDER_SESSIONS_FORCED_CLEANUP_CHILD"
	forcedProviderSessionsCleanupReportEnv = "YOU_PROVIDER_SESSIONS_FORCED_CLEANUP_REPORT"
	forcedProviderSessionsCleanupTimeout   = 30 * time.Second
)

var forcedProviderSessionsCleanupState *forcedProviderSessionsCleanupProbe

// TestProviderSessionsCLI_ForcedAssertionFailureCleansOwnedResources proves
// the Provider Sessions CLI fixture unwinds its process, Factory Session,
// provider route, stream command, listener, and owned paths after a child
// assertion failure. The child boundary is required because a parent test
// cannot fail itself and then inspect its own t.Cleanup and TestMain results.
func TestProviderSessionsCLI_ForcedAssertionFailureCleansOwnedResources(t *testing.T) {
	if os.Getenv(forcedProviderSessionsCleanupChildEnv) == "1" {
		runForcedProviderSessionsCleanupChild(t)
		return
	}
	runForcedProviderSessionsCleanupParent(t)
}

func runForcedProviderSessionsCleanupParent(t *testing.T) {
	t.Helper()

	reportPath := filepath.Join(t.TempDir(), "forced-provider-sessions-cleanup.json")
	command := exec.Command(os.Args[0], "-test.run=^TestProviderSessionsCLI_ForcedAssertionFailureCleansOwnedResources$")
	command.Env = append(os.Environ(),
		forcedProviderSessionsCleanupChildEnv+"=1",
		forcedProviderSessionsCleanupReportEnv+"="+reportPath,
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("forced Provider Sessions CLI cleanup child exited successfully; output=%q", output)
	}
	if command.Process == nil || command.ProcessState == nil || !command.ProcessState.Exited() {
		t.Fatalf("forced Provider Sessions CLI cleanup child did not exit; error=%v output=%q", err, output)
	}
	if command.ProcessState.ExitCode() == 0 {
		t.Fatalf("forced Provider Sessions CLI cleanup child exit code = 0; output=%q", output)
	}

	report := readForcedProviderSessionsCleanupReport(t, reportPath, output)
	assertForcedProviderSessionsCleanupReport(t, report, command.Process.Pid)
}

func readForcedProviderSessionsCleanupReport(
	t *testing.T,
	path string,
	childOutput []byte,
) forcedProviderSessionsCleanupReport {
	t.Helper()

	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read forced Provider Sessions CLI cleanup report %q: %v; child output=%q", path, err, childOutput)
	}
	var report forcedProviderSessionsCleanupReport
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("decode forced Provider Sessions CLI cleanup report %q: %v; child output=%q", path, err, childOutput)
	}
	return report
}

func assertForcedProviderSessionsCleanupReport(
	t *testing.T,
	report forcedProviderSessionsCleanupReport,
	childPID int,
) {
	t.Helper()

	if report.ApplicationPID != childPID {
		t.Fatalf("forced Provider Sessions CLI application PID = %d, want child PID %d", report.ApplicationPID, childPID)
	}
	if report.SharedProcessCleanupError != "" || report.BinaryCleanupError != "" || !report.SharedProcessClosed {
		t.Fatalf("forced Provider Sessions CLI process cleanup = %#v, want clean process and binary teardown", report)
	}
	if !report.APIStopped || !report.ListenerClosed {
		t.Fatalf("forced Provider Sessions CLI listener state = %#v, want stopped and unreachable", report)
	}
	if len(report.OpenedSessionIDs) != 1 || !sameForcedCleanupStringSet(report.OpenedSessionIDs, report.ClosedSessionIDs) {
		t.Fatalf("forced Provider Sessions CLI sessions opened=%v closed=%v, want one closed opened session", report.OpenedSessionIDs, report.ClosedSessionIDs)
	}
	if report.ProviderCommandCalls != 1 || report.ActiveProviderCalls != 0 || report.ActiveProviderRoutes != 0 {
		t.Fatalf("forced Provider Sessions CLI provider state = %#v, want one completed call and zero active calls/routes", report)
	}
	if !report.StreamCommandClosed {
		t.Fatal("forced Provider Sessions CLI stream command did not close after assertion cleanup")
	}
	if !report.Paths.RootAbsent || !report.Paths.CaseFactoryAbsent || !report.Paths.RecordAbsent {
		t.Fatalf("forced Provider Sessions CLI owned paths remain: %#v", report.Paths)
	}
}

func runForcedProviderSessionsCleanupChild(t *testing.T) {
	t.Helper()

	if strings.TrimSpace(os.Getenv(forcedProviderSessionsCleanupReportEnv)) == "" {
		t.Fatal("forced Provider Sessions CLI cleanup child report path is required")
	}

	caseFixture := newWorkerSessionsCLICase(t)
	fixture := caseFixture.fixture
	artifactPath := filepath.Join(caseFixture.factoryDir, "forced-assertion-artifact.txt")
	if err := os.WriteFile(artifactPath, []byte("owned Provider Sessions CLI cleanup artifact"), 0o600); err != nil {
		t.Fatalf("create forced Provider Sessions CLI cleanup artifact: %v", err)
	}
	forcedProviderSessionsCleanupState = &forcedProviderSessionsCleanupProbe{
		fixture:     fixture,
		caseFixture: caseFixture,
		paths: forcedProviderSessionsCleanupPaths{
			Root:        fixture.rootDir,
			CaseFactory: caseFixture.factoryDir,
			Record:      fixture.recordPath,
		},
	}

	caseFixture.registerRoutes(t, "worker-session-cli-success")
	sessionID := caseFixture.openSession(t)
	name := "worker-session-cli-success"
	submitted := support.SubmitSessionWorkAt(t, fixture.baseURL, sessionID, factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "forced Provider Sessions CLI cleanup"},
	})
	if submitted.WorkId == nil || strings.TrimSpace(*submitted.WorkId) == "" {
		t.Fatalf("forced Provider Sessions CLI submission = %#v, want Work identity", submitted)
	}
	fixtureContext, cancel := context.WithTimeout(context.Background(), forcedProviderSessionsCleanupTimeout)
	defer cancel()
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, sessionID, forcedProviderSessionsCleanupTimeout)

	streamInputs := support.FakeInputs(fixtureContext, []string{
		"you", "--server", fixture.baseURL, "worker-sessions", "stream",
		"--session", sessionID, "--provider", "codex", "--kind", "session_id",
		"--id", workerSessionsCodexSuccessID, "--output", "json",
	})
	streamInputs.Input.Env = functionalEnvironment(fixture.homeDir)
	streamInputs.Input.WorkingDirectory = caseFixture.factoryDir
	streamCommand := support.StartProcessCommand(t, fixture.process, streamInputs.Input)
	forcedProviderSessionsCleanupState.streamCommand = streamCommand
	select {
	case <-streamCommand.Done():
		if err := streamCommand.Err(); err != nil {
			t.Fatalf("forced Provider Sessions CLI stream command: %v; stdout=%q stderr=%q", err, streamInputs.Stdout(), streamInputs.Stderr())
		}
	case <-fixtureContext.Done():
		t.Fatalf("forced Provider Sessions CLI stream command did not finish: %v", fixtureContext.Err())
	}

	t.Fatal("intentional assertion failure after acquiring Provider Sessions CLI process, session, route, stream, and paths")
}

type forcedProviderSessionsCleanupProbe struct {
	fixture       *workerSessionsCLISharedFixture
	caseFixture   *workerSessionsCLICase
	paths         forcedProviderSessionsCleanupPaths
	streamCommand *support.ProcessCommand
}

type forcedProviderSessionsCleanupPaths struct {
	Root        string
	CaseFactory string
	Record      string
}

type forcedProviderSessionsCleanupReport struct {
	ApplicationPID            int                                     `json:"application_pid"`
	SharedProcessClosed       bool                                    `json:"shared_process_closed"`
	SharedProcessCleanupError string                                  `json:"shared_process_cleanup_error,omitempty"`
	BinaryCleanupError        string                                  `json:"binary_cleanup_error,omitempty"`
	APIStopped                bool                                    `json:"api_stopped"`
	ListenerClosed            bool                                    `json:"listener_closed"`
	OpenedSessionIDs          []string                                `json:"opened_session_ids"`
	ClosedSessionIDs          []string                                `json:"closed_session_ids"`
	ProviderCommandCalls      int                                     `json:"provider_command_calls"`
	ActiveProviderCalls       int                                     `json:"active_provider_calls"`
	ActiveProviderRoutes      int                                     `json:"active_provider_routes"`
	StreamCommandClosed       bool                                    `json:"stream_command_closed"`
	Paths                     forcedProviderSessionsCleanupPathReport `json:"paths"`
}

type forcedProviderSessionsCleanupPathReport struct {
	RootAbsent        bool `json:"root_absent"`
	CaseFactoryAbsent bool `json:"case_factory_absent"`
	RecordAbsent      bool `json:"record_absent"`
}

func writeForcedProviderSessionsCleanupReport(sharedErr, binaryErr error) error {
	path := strings.TrimSpace(os.Getenv(forcedProviderSessionsCleanupReportEnv))
	if path == "" {
		return nil
	}
	probe := forcedProviderSessionsCleanupState
	if probe == nil || probe.fixture == nil || probe.caseFixture == nil {
		return fmt.Errorf("forced Provider Sessions CLI cleanup probe was not acquired")
	}

	opened := copyForcedCleanupSessionIDs(probe.fixture.openedSessionIDs)
	closed := copyForcedCleanupSessionIDs(probe.fixture.closedSessionIDs)
	report := forcedProviderSessionsCleanupReport{
		ApplicationPID:       os.Getpid(),
		SharedProcessClosed:  sharedErr == nil,
		APIStopped:           channelClosed(probe.fixture.api.stopped),
		ListenerClosed:       assertForcedProviderSessionsListenerClosed(probe.fixture.baseURL),
		OpenedSessionIDs:     opened,
		ClosedSessionIDs:     closed,
		ProviderCommandCalls: probe.fixture.runner.CallCount(),
		ActiveProviderCalls:  probe.fixture.runner.ActiveCallCount(),
		ActiveProviderRoutes: probe.fixture.runner.routeCount(),
		StreamCommandClosed:  processCommandClosed(probe.streamCommand),
		Paths: forcedProviderSessionsCleanupPathReport{
			RootAbsent:        forcedProviderSessionsPathAbsent(probe.paths.Root),
			CaseFactoryAbsent: forcedProviderSessionsPathAbsent(probe.paths.CaseFactory),
			RecordAbsent:      forcedProviderSessionsPathAbsent(probe.paths.Record),
		},
	}
	if sharedErr != nil {
		report.SharedProcessCleanupError = sharedErr.Error()
	}
	if binaryErr != nil {
		report.BinaryCleanupError = binaryErr.Error()
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal forced Provider Sessions CLI cleanup report: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write forced Provider Sessions CLI cleanup report: %w", err)
	}
	return nil
}

func copyForcedCleanupSessionIDs(sessions map[string]struct{}) []string {
	ids := make([]string, 0, len(sessions))
	for sessionID := range sessions {
		ids = append(ids, sessionID)
	}
	sort.Strings(ids)
	return ids
}

func sameForcedCleanupStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func processCommandClosed(command *support.ProcessCommand) bool {
	if command == nil {
		return false
	}
	select {
	case <-command.Done():
		return true
	default:
		return false
	}
}

func channelClosed(channel <-chan struct{}) bool {
	if channel == nil {
		return false
	}
	select {
	case <-channel:
		return true
	default:
		return false
	}
}

func assertForcedProviderSessionsListenerClosed(baseURL string) bool {
	return assertWorkerSessionsCLIListenerClosed(baseURL) == nil
}

func forcedProviderSessionsPathAbsent(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}
