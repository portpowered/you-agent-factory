package lifecycle_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type forcedLifecycleCleanupReport struct {
	ApplicationPID    int    `json:"application_pid"`
	ProcessClosed     bool   `json:"process_closed"`
	ProcessCloseError string `json:"process_close_error,omitempty"`
	ProcessCloseTime  string `json:"process_close_time"`
	CommandDone       bool   `json:"command_done"`
	ProviderCalls     int    `json:"provider_calls"`
	ProviderStarted   bool   `json:"provider_started"`
	ProviderFinished  bool   `json:"provider_finished"`
	ProviderCanceled  bool   `json:"provider_canceled"`
	GatesReleased     bool   `json:"gates_released"`
	ListenerClosed    bool   `json:"listener_closed"`
	SessionObserved   bool   `json:"session_observed"`
	FactoryAbsent     bool   `json:"factory_absent"`
	ArtifactAbsent    bool   `json:"artifact_absent"`
}

func runForcedLifecycleCleanupParent(t *testing.T) {
	t.Helper()
	reportPath := filepath.Join(t.TempDir(), "forced-worker-cli-lifecycle-cleanup.json")
	command := exec.Command(os.Args[0], "-test.run=^TestCLIRunCleanInvocationFailurePreservesPublicError$")
	command.Env = append(os.Environ(),
		lifecycleForcedCleanupChildEnv+"=1",
		lifecycleForcedCleanupReportEnv+"="+reportPath,
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("forced lifecycle cleanup child exited successfully; output=%q", output)
	}
	if command.Process == nil || command.ProcessState == nil || !command.ProcessState.Exited() {
		t.Fatalf("forced lifecycle cleanup child did not exit; error=%v output=%q", err, output)
	}
	if command.ProcessState.ExitCode() == 0 {
		t.Fatalf("forced lifecycle cleanup child exit code = 0; output=%q", output)
	}
	payload, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read forced lifecycle cleanup report %q: %v; child output=%q", reportPath, err, output)
	}
	var report forcedLifecycleCleanupReport
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("decode forced lifecycle cleanup report %q: %v; child output=%q", reportPath, err, output)
	}
	if report.ApplicationPID != command.Process.Pid {
		t.Fatalf("forced lifecycle application PID = %d, want child PID %d", report.ApplicationPID, command.Process.Pid)
	}
	if !report.ProcessClosed || report.ProcessCloseError != "" || report.ProcessCloseTime == "" {
		t.Fatalf("forced lifecycle process close = %#v, want clean close", report)
	}
	if !report.CommandDone || report.ProviderCalls != 1 || !report.ProviderStarted || !report.ProviderFinished || !report.ProviderCanceled {
		t.Fatalf("forced lifecycle command/provider state = %#v, want one joined canceled command", report)
	}
	if !report.GatesReleased || !report.ListenerClosed || !report.SessionObserved {
		t.Fatalf("forced lifecycle public/resource state = %#v, want released gates, closed listener, observed session", report)
	}
	if !report.FactoryAbsent || !report.ArtifactAbsent {
		t.Fatalf("forced lifecycle owned paths remain: %#v", report)
	}
}

func runForcedLifecycleCleanupChild(t *testing.T) {
	t.Helper()
	reportPath := strings.TrimSpace(os.Getenv(lifecycleForcedCleanupReportEnv))
	if reportPath == "" {
		t.Fatal("forced lifecycle cleanup report path is required")
	}
	var coordinator *lifecycleCoordinator
	var command *lifecycleCancelableCommand
	var runner *blockingLifecycleRunner
	var baseURL string
	var listenerClose <-chan struct{}
	var factoryDir string
	var artifactPath string
	var sessionObserved atomic.Bool
	// Register this before every acquired resource so the report is written
	// after coordinator, command, artifact, and factory cleanup has run.
	t.Cleanup(func() {
		report := forcedLifecycleCleanupReport{ApplicationPID: os.Getpid()}
		if coordinator != nil {
			report.ProcessClosed, report.ProcessCloseTime = forcedLifecycleProcessClosed(coordinator)
			if closeErr, _ := coordinator.closeResult(); closeErr != nil {
				report.ProcessCloseError = closeErr.Error()
			}
			report.GatesReleased = len(coordinator.unreleasedGates()) == 0
		}
		if command != nil {
			select {
			case <-command.Done():
				report.CommandDone = true
			default:
			}
		}
		if runner != nil {
			report.ProviderCalls = int(runner.calls.Load())
			report.ProviderStarted = isClosed(runner.started)
			report.ProviderFinished = isClosed(runner.finished)
			report.ProviderCanceled = runner.CancellationCount() == 1
		}
		report.ListenerClosed = lifecycleListenerClosed(baseURL, listenerClose)
		report.SessionObserved = sessionObserved.Load()
		report.FactoryAbsent = pathAbsent(factoryDir)
		report.ArtifactAbsent = pathAbsent(artifactPath)
		payload, err := json.MarshalIndent(report, "", "  ")
		if err == nil {
			err = os.WriteFile(reportPath, payload, 0o600)
		}
		if err != nil {
			t.Errorf("write forced lifecycle cleanup report: %v", err)
		}
	})

	var err error
	factoryDir = scaffoldProviderBackedFactory(t)
	artifactPath = filepath.Join(factoryDir, "forced-cleanup-artifact.txt")
	if err := os.WriteFile(artifactPath, []byte("owned lifecycle artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(artifactPath) })

	runner = newBlockingLifecycleRunner()
	api := newLifecycleAPIServer()
	listenerClose = api.closed
	shutdownGate := newLifecycleGate("forced cleanup listener shutdown")
	api.HoldShutdownUntilSignaled(shutdownGate.channel())
	coordinator = buildLifecycleProcess(t, serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: runner,
	})
	coordinator.TrackGate(shutdownGate)
	inputs := coordinator.Inputs([]string{
		"you", "run",
		"--factory", filepath.Join(factoryDir, "factory.json"),
		"--with-server",
		"--no-record",
		"--quiet",
		"force lifecycle cleanup after acquisition",
	}, factoryDir)
	command = coordinator.StartCancelableCommand(inputs)
	baseURL, err = coordinator.WaitForReadiness(api.server)
	if err != nil {
		t.Fatal(err)
	}
	waitLifecycleSignal(t, runner.started, "forced-cleanup provider start")
	_, err = support.WaitForObservation(
		lifecycleAdverseSignalTimeout,
		func() (factoryapi.FactorySession, error) {
			session, ok, diagnostic := tryReadDefaultFactorySession(baseURL)
			if !ok {
				return factoryapi.FactorySession{}, errors.New(diagnostic)
			}
			return session, nil
		},
		func(session factoryapi.FactorySession) bool { return strings.TrimSpace(session.Id) != "" },
	)
	if err != nil {
		t.Fatal(err)
	}
	sessionObserved.Store(true)
	t.Fatal("intentional lifecycle assertion failure after process, session, listener, provider, gate, and artifact acquisition")
}

func forcedLifecycleProcessClosed(coordinator *lifecycleCoordinator) (bool, string) {
	if coordinator == nil {
		return false, ""
	}
	_, duration := coordinator.closeResult()
	return coordinator.closed(), duration.String()
}

func pathAbsent(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return errors.Is(err, os.ErrNotExist)
}
