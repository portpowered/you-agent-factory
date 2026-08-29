package lifecycle_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	lifecycleReadinessTimeout    = 15 * time.Second
	lifecycleObservationTimeout  = 15 * time.Second
	lifecycleCommandDoneTimeout  = 15 * time.Second
	lifecycleHTTPTimeout         = 2 * time.Second
	lifecyclePollInterval        = 10 * time.Millisecond
	lifecycleProcessCloseTimeout = 5 * time.Second
)

// A shared client is safe for concurrent use and keeps a stalled public
// projection from preventing the coordinator's phase deadline from firing.
var lifecycleHTTPClient = &http.Client{Timeout: lifecycleHTTPTimeout}

type lifecyclePhase string

const (
	lifecyclePhaseInputs          lifecyclePhase = "inputs"
	lifecyclePhaseExecute         lifecyclePhase = "execute"
	lifecyclePhaseReadiness       lifecyclePhase = "readiness"
	lifecyclePhaseActive          lifecyclePhase = "active observation"
	lifecyclePhaseProviderRelease lifecyclePhase = "provider release"
	lifecyclePhaseTerminal        lifecyclePhase = "terminal state"
	lifecyclePhaseCommandDone     lifecyclePhase = "command done"
	lifecyclePhaseProcessClose    lifecyclePhase = "process close"
)

type lifecycleGate struct {
	name string
	ch   chan struct{}
	once sync.Once
}

func newLifecycleGate(name string) *lifecycleGate {
	return &lifecycleGate{name: name, ch: make(chan struct{})}
}

func (gate *lifecycleGate) channel() <-chan struct{} {
	if gate == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return gate.ch
}

func (gate *lifecycleGate) release() bool {
	if gate == nil {
		return false
	}
	released := false
	gate.once.Do(func() {
		close(gate.ch)
		released = true
	})
	return released
}

type lifecycleCoordinator struct {
	t       *testing.T
	process support.ApplicationProcess
	homeDir string
	started time.Time

	mu                    sync.Mutex
	phase                 lifecyclePhase
	lastPublicObservation string
	transitions           []string
	gates                 []*lifecycleGate
	closeOnce             sync.Once
}

func newLifecycleCoordinator(t *testing.T, process support.ApplicationProcess) *lifecycleCoordinator {
	t.Helper()
	if process == nil {
		t.Fatal("lifecycle coordinator requires a process")
	}
	coordinator := &lifecycleCoordinator{
		t:       t,
		process: process,
		homeDir: t.TempDir(),
		started: time.Now(),
	}
	t.Cleanup(coordinator.close)
	return coordinator
}

func buildLifecycleProcess(t *testing.T, edges serviceedges.Edges) *lifecycleCoordinator {
	t.Helper()
	return newLifecycleCoordinator(t, support.BuildProcess(t, edges))
}

func (coordinator *lifecycleCoordinator) Inputs(args []string, workingDirectory string) *support.CapturedInputs {
	coordinator.t.Helper()
	inputs := support.FakeInputs(coordinator.t.Context(), args)
	inputs.Input.WorkingDirectory = workingDirectory
	inputs.Input.Env = isolatedLifecycleEnvironment(inputs.Input.Env, coordinator.homeDir)
	coordinator.recordPhase(lifecyclePhaseInputs, "invocation inputs prepared with an isolated operator home", false)
	return inputs
}

func isolatedLifecycleEnvironment(environment []string, home string) []string {
	filtered := make([]string, 0, len(environment)+2)
	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if ok && (strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE")) {
			continue
		}
		filtered = append(filtered, entry)
	}
	filtered = append(filtered, "HOME="+home, "USERPROFILE="+home)
	return filtered
}

func (coordinator *lifecycleCoordinator) TrackGate(gate *lifecycleGate) {
	if coordinator == nil || gate == nil {
		return
	}
	coordinator.mu.Lock()
	coordinator.gates = append(coordinator.gates, gate)
	coordinator.mu.Unlock()
}

func (coordinator *lifecycleCoordinator) ReleaseGate(
	gate *lifecycleGate,
	phase lifecyclePhase,
	observation string,
) {
	if coordinator == nil || gate == nil || !gate.release() {
		return
	}
	coordinator.recordPhase(phase, fmt.Sprintf("%s gate %q released", observation, gate.name), false)
}

func (coordinator *lifecycleCoordinator) StartCommand(inputs *support.CapturedInputs) *support.ProcessCommand {
	coordinator.t.Helper()
	coordinator.recordPhase(lifecyclePhaseExecute, "Process.Execute started", false)
	command := support.StartProcessCommand(coordinator.t, coordinator.process, inputs.Input)
	// This cleanup is registered after StartProcessCommand's cleanup. On a
	// failed assertion it therefore releases gates before command.Stop waits
	// for Process.Execute, and cannot strand a gated provider or listener.
	coordinator.t.Cleanup(coordinator.releaseAll)
	return command
}

func (coordinator *lifecycleCoordinator) Execute(inputs *support.CapturedInputs) error {
	coordinator.t.Helper()
	coordinator.recordPhase(lifecyclePhaseExecute, "Process.Execute started", false)
	err := coordinator.process.Execute(inputs.Input)
	if err == nil {
		coordinator.publicObservation(lifecyclePhaseTerminal, "Process.Execute returned successful terminal outcome")
	} else {
		coordinator.publicObservation(lifecyclePhaseTerminal, fmt.Sprintf("Process.Execute returned terminal error: %v", err))
	}
	coordinator.recordPhase(lifecyclePhaseCommandDone, "Process.Execute returned to the caller", false)
	return err
}

func (coordinator *lifecycleCoordinator) WaitCommand(command *support.ProcessCommand) error {
	coordinator.t.Helper()
	if command == nil {
		return coordinator.phaseFailure(lifecyclePhaseCommandDone, "command is nil")
	}
	doneTimer := time.NewTimer(lifecycleCommandDoneTimeout)
	defer doneTimer.Stop()
	select {
	case <-command.Done():
	case <-doneTimer.C:
		return coordinator.phaseFailure(lifecyclePhaseCommandDone, "command completion deadline expired")
	}
	err := command.Err()
	if err == nil {
		coordinator.recordPhase(lifecyclePhaseCommandDone, "asynchronous Process.Execute completed successfully", false)
	} else {
		coordinator.recordPhase(lifecyclePhaseCommandDone, fmt.Sprintf("asynchronous Process.Execute completed with error: %v", err), false)
	}
	return err
}

func (coordinator *lifecycleCoordinator) WaitForReadiness(server *support.ProcessAPIServer) (string, error) {
	coordinator.t.Helper()
	coordinator.recordPhase(lifecyclePhaseReadiness, "waiting for the invocation-owned API listener to bind", false)
	if server == nil {
		return "", coordinator.phaseFailure(lifecyclePhaseReadiness, "process API server is nil")
	}
	started := time.Now()
	baseURL, err := server.WaitForBaseURL(lifecycleReadinessTimeout)
	if err != nil {
		return "", coordinator.phaseFailure(
			lifecyclePhaseReadiness,
			fmt.Sprintf("wait for API listener after %s: %v", time.Since(started), err),
		)
	}
	coordinator.publicObservation(
		lifecyclePhaseReadiness,
		fmt.Sprintf("API listener ready at %s after %s", baseURL, time.Since(started)),
	)
	return baseURL, nil
}

func (coordinator *lifecycleCoordinator) publicObservation(phase lifecyclePhase, observation string) {
	coordinator.recordPhase(phase, observation, true)
}

func (coordinator *lifecycleCoordinator) recordPhase(
	phase lifecyclePhase,
	observation string,
	public bool,
) {
	if coordinator == nil {
		return
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.phase = phase
	if public {
		coordinator.lastPublicObservation = observation
	}
	coordinator.transitions = append(coordinator.transitions, fmt.Sprintf("%s: %s", phase, observation))
}

func (coordinator *lifecycleCoordinator) phaseFailure(phase lifecyclePhase, detail string) error {
	if coordinator == nil {
		return fmt.Errorf("worker CLI lifecycle phase %s failed: %s", phase, detail)
	}
	coordinator.mu.Lock()
	lastObservation := coordinator.lastPublicObservation
	coordinator.phase = phase
	coordinator.transitions = append(coordinator.transitions, fmt.Sprintf("%s: FAILED: %s", phase, detail))
	topology := "root.BuildProcess -> Process.Execute -> controlled ProviderCommandRunner/API server -> Process.Close"
	elapsed := time.Since(coordinator.started)
	coordinator.mu.Unlock()
	coordinator.releaseAll()
	return fmt.Errorf(
		"worker CLI lifecycle phase %s failed after %s: %s; last public observation=%q; topology=%s",
		phase,
		elapsed,
		detail,
		lastObservation,
		topology,
	)
}

func (coordinator *lifecycleCoordinator) releaseAll() {
	if coordinator == nil {
		return
	}
	coordinator.mu.Lock()
	gates := append([]*lifecycleGate(nil), coordinator.gates...)
	coordinator.mu.Unlock()
	for _, gate := range gates {
		gate.release()
	}
}

func (coordinator *lifecycleCoordinator) close() {
	if coordinator == nil {
		return
	}
	coordinator.closeOnce.Do(func() {
		coordinator.releaseAll()
		started := time.Now()
		closeContext, cancel := context.WithTimeout(context.Background(), lifecycleProcessCloseTimeout)
		err := coordinator.process.Close(closeContext)
		cancel()
		coordinator.recordPhase(
			lifecyclePhaseProcessClose,
			fmt.Sprintf("Process.Close completed after %s", time.Since(started)),
			false,
		)
		if err != nil {
			coordinator.t.Errorf("close lifecycle process: %v; %s", err, coordinator.diagnostics())
			return
		}
		coordinator.t.Logf("worker CLI lifecycle: %s", coordinator.diagnostics())
	})
}

func (coordinator *lifecycleCoordinator) diagnostics() string {
	if coordinator == nil {
		return "coordinator unavailable"
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return fmt.Sprintf(
		"phase=%s elapsed=%s last public observation=%q topology=root.BuildProcess -> Process.Execute -> controlled ProviderCommandRunner/API server -> Process.Close transitions=%q",
		coordinator.phase,
		time.Since(coordinator.started),
		coordinator.lastPublicObservation,
		coordinator.transitions,
	)
}

// ObserveHostedServerAttached waits for the two public observations that must
// precede release of the gated provider: a live Factory Session and terminal
// Work. The ticker is only a bounded retry cadence for eventually-consistent
// public HTTP projections; readiness and completion are signal-driven, and a
// deadline turns a missing phase into diagnostics instead of a hang.
func (coordinator *lifecycleCoordinator) ObserveHostedServerAttached(
	baseURL, wantWorkText string,
	releaseWorker func(),
	done <-chan struct{},
) (factoryapi.FactorySession, string, bool, error) {
	coordinator.t.Helper()
	deadline := time.NewTimer(lifecycleObservationTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(lifecyclePollInterval)
	defer ticker.Stop()

	var (
		sessionRead    bool
		workVisible    bool
		sessionDuring  factoryapi.FactorySession
		terminalWorkID string
		lastSessionErr string
		lastWorkErr    string
	)
	for {
		if !sessionRead {
			if session, ok, diagnostic := tryReadDefaultFactorySession(baseURL); ok {
				sessionDuring = session
				sessionRead = true
				coordinator.publicObservation(lifecyclePhaseActive, fmt.Sprintf("Factory Session %q readable", session.Id))
				if releaseWorker != nil {
					releaseWorker()
				}
			} else if diagnostic != "" {
				lastSessionErr = diagnostic
			}
		}
		if !workVisible {
			if workID, ok, diagnostic := tryReadTerminalWorkPrimaryText(baseURL, wantWorkText); ok {
				terminalWorkID = workID
				workVisible = true
				coordinator.publicObservation(lifecyclePhaseTerminal, fmt.Sprintf("terminal Work %q readable", workID))
			} else if diagnostic != "" {
				lastWorkErr = diagnostic
			}
		}
		if sessionRead && workVisible {
			return sessionDuring, terminalWorkID, true, nil
		}

		select {
		case <-done:
			return factoryapi.FactorySession{}, terminalWorkID, workVisible, coordinator.phaseFailure(
				lifecyclePhaseTerminal,
				fmt.Sprintf("command completed before Factory Session and terminal Work were both observable; session=%s; work=%s", lastSessionErr, lastWorkErr),
			)
		case <-deadline.C:
			return factoryapi.FactorySession{}, terminalWorkID, workVisible, coordinator.phaseFailure(
				lifecyclePhaseTerminal,
				fmt.Sprintf("observation deadline expired at %s; session=%s; work=%s", baseURL, lastSessionErr, lastWorkErr),
			)
		case <-ticker.C:
		}
	}
}
