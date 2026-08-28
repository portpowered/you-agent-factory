package root_composition_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	forcedModelsCleanupChildEnv  = "YOU_MODELS_FORCED_CLEANUP_CHILD"
	forcedModelsCleanupReportEnv = "YOU_MODELS_FORCED_CLEANUP_REPORT"
	forcedModelsCleanupTimeout   = 30 * time.Second
)

var forcedModelsCleanupState *forcedModelsCleanupProbe

// TestModels_ForcedAssertionFailureCleansOwnedResources proves that the
// Models root-composition process closes a started host and its lease-backed
// invocation when the public scenario fails after acquisition. The child is
// required because the parent cannot inspect its own t.Cleanup/TestMain state
// after intentionally failing.
func TestModels_ForcedAssertionFailureCleansOwnedResources(t *testing.T) {
	if os.Getenv(forcedModelsCleanupChildEnv) == "1" {
		runForcedModelsCleanupChild(t)
		return
	}
	runForcedModelsCleanupParent(t)
}

func runForcedModelsCleanupParent(t *testing.T) {
	t.Helper()

	reportPath := filepath.Join(t.TempDir(), "forced-models-cleanup.json")
	command := exec.Command(os.Args[0], "-test.run=^TestModels_ForcedAssertionFailureCleansOwnedResources$")
	command.Env = append(os.Environ(),
		forcedModelsCleanupChildEnv+"=1",
		forcedModelsCleanupReportEnv+"="+reportPath,
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("forced Models cleanup child exited successfully; output=%q", output)
	}
	if command.Process == nil || command.ProcessState == nil || !command.ProcessState.Exited() {
		t.Fatalf("forced Models cleanup child did not exit; error=%v output=%q", err, output)
	}
	if command.ProcessState.ExitCode() == 0 {
		t.Fatalf("forced Models cleanup child exit code = 0; output=%q", output)
	}

	report := readForcedModelsCleanupReport(t, reportPath, output)
	assertForcedModelsCleanupReport(t, report, command.Process.Pid)
}

func readForcedModelsCleanupReport(
	t *testing.T,
	path string,
	childOutput []byte,
) forcedModelsCleanupReport {
	t.Helper()

	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read forced Models cleanup report %q: %v; child output=%q", path, err, childOutput)
	}
	var report forcedModelsCleanupReport
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("decode forced Models cleanup report %q: %v; child output=%q", path, err, childOutput)
	}
	return report
}

func assertForcedModelsCleanupReport(
	t *testing.T,
	report forcedModelsCleanupReport,
	childPID int,
) {
	t.Helper()

	if report.ApplicationPID != childPID {
		t.Fatalf("forced Models cleanup application PID = %d, want child PID %d", report.ApplicationPID, childPID)
	}
	if report.ProcessCloseError != "" || !report.ProcessClosed || !report.CommandClosed {
		t.Fatalf("forced Models cleanup process state = %#v, want closed process and command", report)
	}
	if report.HostStarts != 1 || report.HostStopCalls != 1 || report.HostActive {
		t.Fatalf("forced Models cleanup host state = %#v, want one stopped inactive host", report)
	}
	if !report.ProtocolStarted || !report.ProtocolCanceled {
		t.Fatalf("forced Models cleanup protocol state = %#v, want started and canceled protocol", report)
	}
	if !report.ListenerClosed {
		t.Fatal("forced Models cleanup host listener remained reachable after teardown")
	}
	if !report.Paths.FactoryAbsent || !report.Paths.ArtifactAbsent {
		t.Fatalf("forced Models cleanup owned paths remain: %#v", report.Paths)
	}
}

func runForcedModelsCleanupChild(t *testing.T) {
	t.Helper()

	reportPath := strings.TrimSpace(os.Getenv(forcedModelsCleanupReportEnv))
	if reportPath == "" {
		t.Fatal("forced Models cleanup child report path is required")
	}

	launcher := &forcedModelsHostLauncher{}
	protocol := newForcedModelsBlockingProtocol()
	process, factoryDir, environment := buildGenericCLIProcess(
		t,
		singleOutputModelFactoryConfig,
		nil,
		launcher,
		protocol,
		nil,
	)
	artifactPath := filepath.Join(factoryDir, "forced-models-cleanup-artifact.txt")
	if err := os.WriteFile(artifactPath, []byte("owned Models cleanup artifact"), 0o600); err != nil {
		t.Fatalf("create forced Models cleanup artifact: %v", err)
	}
	forcedModelsCleanupState = &forcedModelsCleanupProbe{
		process:      process,
		launcher:     launcher,
		protocol:     protocol,
		factoryDir:   factoryDir,
		artifactPath: artifactPath,
	}

	closer, ok := process.(interface{ Close(context.Context) error })
	if !ok {
		t.Fatal("forced Models cleanup process does not expose lifecycle close")
	}
	// Register this observable close before the command cleanup. The command
	// must receive cancellation first, then the process close can join the
	// host/lease teardown before TestMain writes the report.
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), forcedModelsCleanupTimeout)
		defer cancel()
		forcedModelsCleanupState.processCloseErr = closer.Close(closeCtx)
		forcedModelsCleanupState.processClosed = forcedModelsCleanupState.processCloseErr == nil
	})

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "models", "invoke", "llm", "--operation", "OMNI", "--text", "forced Models cleanup",
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = factoryDir
	command := support.StartProcessCommand(t, process, inputs.Input)
	forcedModelsCleanupState.command = command
	waitForForcedModelsProtocol(t, protocol, command)

	t.Fatal("intentional assertion failure after acquiring Models host, lease, command, and paths")
}

func waitForForcedModelsProtocol(
	t *testing.T,
	protocol *forcedModelsBlockingProtocol,
	command *support.ProcessCommand,
) {
	t.Helper()
	select {
	case <-protocol.started:
	case <-command.Done():
		t.Fatalf("forced Models cleanup command returned before host acquisition: %v", command.Err())
	case <-time.After(2 * time.Second):
		// The signal is emitted by the controlled protocol edge. The bounded
		// branch is only a deadlock ceiling for a broken fixture, not timing
		// used to establish readiness.
		t.Fatal("forced Models cleanup protocol did not reach its active boundary")
	}
}

type forcedModelsCleanupProbe struct {
	process      support.Process
	launcher     *forcedModelsHostLauncher
	protocol     *forcedModelsBlockingProtocol
	command      *support.ProcessCommand
	factoryDir   string
	artifactPath string

	processClosed   bool
	processCloseErr error
}

type forcedModelsCleanupReport struct {
	ApplicationPID    int                           `json:"application_pid"`
	ProcessClosed     bool                          `json:"process_closed"`
	ProcessCloseError string                        `json:"process_close_error,omitempty"`
	CommandClosed     bool                          `json:"command_closed"`
	HostStarts        int                           `json:"host_starts"`
	HostStopCalls     int                           `json:"host_stop_calls"`
	HostActive        bool                          `json:"host_active"`
	ProtocolStarted   bool                          `json:"protocol_started"`
	ProtocolCanceled  bool                          `json:"protocol_canceled"`
	ListenerClosed    bool                          `json:"listener_closed"`
	Paths             forcedModelsCleanupPathReport `json:"paths"`
}

type forcedModelsCleanupPathReport struct {
	FactoryAbsent  bool `json:"factory_absent"`
	ArtifactAbsent bool `json:"artifact_absent"`
}

func writeForcedModelsCleanupReport() error {
	reportPath := strings.TrimSpace(os.Getenv(forcedModelsCleanupReportEnv))
	if reportPath == "" {
		return nil
	}
	probe := forcedModelsCleanupState
	if probe == nil || probe.launcher == nil || probe.protocol == nil {
		return fmt.Errorf("forced Models cleanup probe was not acquired")
	}
	report := forcedModelsCleanupReport{
		ApplicationPID:   os.Getpid(),
		ProcessClosed:    probe.processClosed,
		CommandClosed:    forcedModelsCommandClosed(probe.command),
		HostStarts:       probe.launcher.Starts(),
		HostStopCalls:    probe.launcher.StopCalls(),
		HostActive:       probe.launcher.Active(),
		ProtocolStarted:  probe.protocol.Started(),
		ProtocolCanceled: probe.protocol.Canceled(),
		ListenerClosed:   forcedModelsListenerClosed(probe.launcher.Endpoint()),
		Paths: forcedModelsCleanupPathReport{
			FactoryAbsent:  forcedModelsPathAbsent(probe.factoryDir),
			ArtifactAbsent: forcedModelsPathAbsent(probe.artifactPath),
		},
	}
	if probe.processCloseErr != nil {
		report.ProcessCloseError = probe.processCloseErr.Error()
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal forced Models cleanup report: %w", err)
	}
	if err := os.WriteFile(reportPath, payload, 0o600); err != nil {
		return fmt.Errorf("write forced Models cleanup report: %w", err)
	}
	return nil
}

func forcedModelsCommandClosed(command *support.ProcessCommand) bool {
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

func forcedModelsListenerClosed(endpoint string) bool {
	if strings.TrimSpace(endpoint) == "" {
		return false
	}
	client := http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	response, err := client.Get(strings.TrimSuffix(endpoint, "/") + "/health")
	if err != nil {
		return true
	}
	response.Body.Close()
	return false
}

func forcedModelsPathAbsent(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

type forcedModelsHostLauncher struct {
	mu       sync.Mutex
	endpoint string
	starts   int
	stops    int
	active   bool
}

func (launcher *forcedModelsHostLauncher) Start(
	_ context.Context,
	spec serviceedges.HostProcessStartSpec,
) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	if launcher.active {
		return nil, fmt.Errorf("forced Models host is already active")
	}
	launcher.endpoint = spec.HealthEndpoint
	launcher.starts++
	launcher.active = true
	return &forcedModelsHostProcess{launcher: launcher, stopped: make(chan struct{})}, nil
}

func (launcher *forcedModelsHostLauncher) Starts() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.starts
}

func (launcher *forcedModelsHostLauncher) StopCalls() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.stops
}

func (launcher *forcedModelsHostLauncher) Active() bool {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.active
}

func (launcher *forcedModelsHostLauncher) Endpoint() string {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.endpoint
}

type forcedModelsHostProcess struct {
	launcher *forcedModelsHostLauncher
	stopped  chan struct{}
	once     sync.Once
}

func (process *forcedModelsHostProcess) HealthEndpoint() string { return process.launcher.Endpoint() }

func (process *forcedModelsHostProcess) Wait() error {
	<-process.stopped
	return nil
}

func (process *forcedModelsHostProcess) Stop(context.Context) error {
	process.once.Do(func() {
		process.launcher.mu.Lock()
		process.launcher.stops++
		process.launcher.active = false
		process.launcher.mu.Unlock()
		close(process.stopped)
	})
	return nil
}

type forcedModelsBlockingProtocol struct {
	started    chan struct{}
	canceled   chan struct{}
	startOnce  sync.Once
	cancelOnce sync.Once
}

func newForcedModelsBlockingProtocol() *forcedModelsBlockingProtocol {
	return &forcedModelsBlockingProtocol{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
	}
}

func (protocol *forcedModelsBlockingProtocol) Negotiate(
	ctx context.Context,
	_ string,
	_ serviceedges.ModelHostProtocolNegotiationRequest,
) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	protocol.startOnce.Do(func() { close(protocol.started) })
	<-ctx.Done()
	protocol.cancelOnce.Do(func() { close(protocol.canceled) })
	return serviceedges.ModelHostProtocolNegotiationResult{}, ctx.Err()
}

func (protocol *forcedModelsBlockingProtocol) Started() bool {
	select {
	case <-protocol.started:
		return true
	default:
		return false
	}
}

func (protocol *forcedModelsBlockingProtocol) Canceled() bool {
	select {
	case <-protocol.canceled:
		return true
	default:
		return false
	}
}
