package support

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// MockInferenceProvider returns a typed provider override for functional tests
// without requiring destination packages to import service implementation paths.
func MockInferenceProvider(contents ...string) providers.Service {
	responses := make([]providers.ExecuteResult, len(contents))
	for index, content := range contents {
		responses[index] = providers.ExecuteResult{Content: content}
	}
	return testutil.NewNativeMockProvider(responses...)
}

type inferenceProvider interface {
	Infer(context.Context, workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error)
}

// ProviderServiceFromInference bridges an Infer-shaped test provider into the
// Providers-root edge used by root.BuildProcess. The adapter keeps the
// compatibility fake unchanged while exposing the same Execute-shaped
// contract as native Providers test doubles.
func ProviderServiceFromInference(provider inferenceProvider) providers.Service {
	if provider == nil {
		return nil
	}
	adapter := &testutil.ProviderServiceAdapter{}
	adapter.InferFunc = provider.Infer
	return adapter
}

// BlockingInferenceProvider blocks the first provider call until release is
// closed or the context is canceled, then completes subsequent calls immediately.
func BlockingInferenceProvider(release <-chan struct{}) providers.Service {
	provider := &blockingProvider{release: release}
	provider.NativeProvider.ExecuteFunc = provider.Execute
	return provider
}

type blockingProvider struct {
	testutil.NativeProvider
	release <-chan struct{}
	mu      sync.Mutex
	calls   int
}

func (p *blockingProvider) Execute(
	ctx context.Context,
	_ providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	if call == 1 {
		select {
		case <-p.release:
		case <-ctx.Done():
			return providers.ExecuteResult{}, ctx.Err()
		}
	}
	return providers.ExecuteResult{Content: "completed"}, nil
}

// RunFactoryToCompletion executes the customer daemon command through the
// canonical root-built process and returns its public default-session
// projection after every visible work token becomes terminal.
func RunFactoryToCompletion(
	t testing.TB,
	dir string,
	provider providers.Service,
	timeout time.Duration,
) factoryapi.FactorySession {
	return RunFactoryToCompletionWithEdges(t, dir, serviceedges.Edges{
		ProviderOverride: provider,
	}, timeout)
}

// RunFactoryToCompletionWithEdges executes the same customer daemon while
// allowing a test to replace external process boundaries such as command
// execution.
func RunFactoryToCompletionWithEdges(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
) factoryapi.FactorySession {
	session, _, _, _ := runFactoryToCompletion(t, dir, overrides, timeout, false, 0)
	return session
}

// RunFactoryToCompletionWithEdgesAndWork also returns the customer-visible
// Work listing captured before the daemon stops.
func RunFactoryToCompletionWithEdgesAndWork(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
	expectedWorkCounts ...int,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse) {
	expectedWorkCount := 0
	if len(expectedWorkCounts) > 1 {
		t.Fatal("expected at most one Work admission count")
	}
	if len(expectedWorkCounts) == 1 {
		expectedWorkCount = expectedWorkCounts[0]
	}
	session, work, _, _ := runFactoryToCompletion(t, dir, overrides, timeout, false, expectedWorkCount)
	return session, work
}

// RunFactoryToCompletionWithEdgesAndObservations also returns the public Work
// listing and retained Factory Event history captured before the daemon stops.
func RunFactoryToCompletionWithEdgesAndObservations(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
	expectedWorkCounts ...int,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	expectedWorkCount := 0
	if len(expectedWorkCounts) > 1 {
		t.Fatal("expected at most one Work admission count")
	}
	if len(expectedWorkCounts) == 1 {
		expectedWorkCount = expectedWorkCounts[0]
	}
	session, work, events, _ := runFactoryToCompletion(t, dir, overrides, timeout, false, expectedWorkCount)
	return session, work, events
}

// RunFactoryToCompletionWithEdgesAndObservationsStableBeforeClose is retained
// only as the excluded ACP provider lane's compatibility handoff. Its
// historical stable-window behavior has been removed: completion is
// correlated to canonical event observation before the pre-shutdown hook.
func RunFactoryToCompletionWithEdgesAndObservationsStableBeforeClose(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
	beforeClose func(),
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	t.Helper()
	session, work, events, _ := runFactoryToCompletionWithHome(
		t,
		dir,
		overrides,
		timeout,
		false,
		0,
		nil,
		nil,
		beforeClose,
	)
	return session, work, events
}

// RunFactoryToCompletionWithConfiguredHome exposes the invocation-local
// operator home before process start so functional tests can author settings
// through the same filesystem contract consumed by the CLI.
func RunFactoryToCompletionWithConfiguredHome(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
	configure func(string),
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	session, work, events, _ := runFactoryToCompletionWithHome(
		t,
		dir,
		overrides,
		timeout,
		false,
		0,
		configure,
		nil,
		nil,
	)
	return session, work, events
}

// RunFactoryToCompletionWithEdgesAndResponseEvents also reads the public
// ephemeral response-event stream before stopping the root-built process.
func RunFactoryToCompletionWithEdgesAndResponseEvents(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
) (
	factoryapi.FactorySession,
	factoryapi.ListWorkResponse,
	[]factoryapi.FactoryEvent,
	[]factoryapi.FactoryResponseEvent,
) {
	return runFactoryToCompletion(t, dir, overrides, timeout, true, 0)
}

// RunFactoryToCompletionWithEdgesAndResponseEventsAndWorkerSessionEvents also
// drains every public Worker Session history correlated with the completed
// Work before the root-built process stops. The callback stays at the public
// HTTP boundary so provider-neutral sessions can be inspected by Worker ID.
func RunFactoryToCompletionWithEdgesAndResponseEventsAndWorkerSessionEvents(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
) (
	factoryapi.FactorySession,
	factoryapi.ListWorkResponse,
	[]factoryapi.FactoryEvent,
	[]factoryapi.FactoryResponseEvent,
	[]factoryapi.WorkerSessionEvent,
) {
	var workerEvents []factoryapi.WorkerSessionEvent
	session, work, events, responseEvents := runFactoryToCompletionWithHome(
		t,
		dir,
		overrides,
		timeout,
		true,
		0,
		nil,
		func(baseURL string, listed factoryapi.ListWorkResponse) {
			seen := make(map[string]struct{})
			for _, item := range listed.Results {
				workID := StringPointerValue(item.WorkId)
				if workID == "" {
					continue
				}
				observations := ListDefaultSessionWorkerSessions(t, baseURL, workID)
				for _, observation := range observations.Sessions {
					if observation.WorkerSessionId == "" {
						continue
					}
					if _, ok := seen[observation.WorkerSessionId]; ok {
						continue
					}
					seen[observation.WorkerSessionId] = struct{}{}
					workerEvents = append(workerEvents, GetWorkerSessionEventsByIDAt(t, baseURL, observation.WorkerSessionId)...)
				}
			}
		},
		nil,
	)
	return session, work, events, responseEvents, workerEvents
}

func runFactoryToCompletion(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
	captureResponseEvents bool,
	expectedWorkCount int,
) (
	factoryapi.FactorySession,
	factoryapi.ListWorkResponse,
	[]factoryapi.FactoryEvent,
	[]factoryapi.FactoryResponseEvent,
) {
	return runFactoryToCompletionWithHome(
		t,
		dir,
		overrides,
		timeout,
		captureResponseEvents,
		expectedWorkCount,
		nil,
		nil,
		nil,
	)
}

// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func runFactoryToCompletionWithHome(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
	captureResponseEvents bool,
	expectedWorkCount int,
	configure func(string),
	captureWorkerSessionEvents func(string, factoryapi.ListWorkResponse),
	beforeClose func(),
) (
	factoryapi.FactorySession,
	factoryapi.ListWorkResponse,
	[]factoryapi.FactoryEvent,
	[]factoryapi.FactoryResponseEvent,
) {
	t.Helper()

	server := NewProcessAPIServer()
	shutdownGate := make(chan struct{})
	server.HoldShutdownUntilSignaled(shutdownGate)
	overrides.APIServerStarter = server.Start
	process := BuildProcess(t, overrides)
	inputs := FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	homeDir := t.TempDir()
	if configure != nil {
		configure(homeDir)
	}
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = dir
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		if stderr := strings.TrimSpace(inputs.Stderr()); stderr != "" {
			t.Logf("daemon stderr:\n%s", stderr)
		}
		if stdout := strings.TrimSpace(inputs.Stdout()); stdout != "" {
			t.Logf("daemon stdout:\n%s", stdout)
		}
	})
	daemon := StartProcessCommand(t, process, inputs.Input)
	shutdownReleased := false
	releaseShutdown := func() {
		if shutdownReleased {
			return
		}
		close(shutdownGate)
		shutdownReleased = true
	}
	t.Cleanup(releaseShutdown)
	baseURL := server.WaitForURL(t)
	liveSession := GetDefaultSession(t, baseURL)
	terminalObservation := OpenSessionTerminalFactoryEventObservation(t, baseURL, liveSession.Id)
	var (
		responseCaptureCancel context.CancelFunc
		responseCaptureDone   <-chan responseEventCaptureResult
		responseActivity      <-chan factoryapi.FactoryResponseEvent
	)
	if captureResponseEvents {
		// Subscribe while the continuously hosted session is live. Waiting until
		// terminal work is observed leaves the session itself active, so a fresh
		// SSE request correctly remains open and the old retained-history helper
		// could spend its entire timeout waiting for the HTTP response.
		captureContext, cancelCapture := context.WithCancel(context.Background())
		responseCaptureCancel = cancelCapture
		captureDone := make(chan responseEventCaptureResult, 1)
		captureStarted := make(chan error, 1)
		activity := make(chan factoryapi.FactoryResponseEvent, 256)
		responseCaptureDone = captureDone
		responseActivity = activity
		go func() {
			events, err := captureFactoryResponseEvents(
				captureContext,
				SessionResponseEventsURL(baseURL, liveSession.Id),
				captureStarted,
				activity,
			)
			captureDone <- responseEventCaptureResult{events: events, err: err}
		}()
		if err := <-captureStarted; err != nil {
			t.Fatalf("start factory response-event capture: %v", err)
		}
	}
	if expectedWorkCount > 0 {
		WaitForSessionWorkCountTerminalFromFactoryEvents(t, baseURL, liveSession.Id, expectedWorkCount, timeout)
	} else {
		WaitForSessionWorkTerminalFromFactoryEvents(t, baseURL, liveSession.Id, timeout)
	}

	var responseStreamComplete bool
	if beforeClose != nil {
		beforeClose()
	}
	// Request shutdown after Work completion. Runtime shutdown records
	// RUN_RESPONSE before the transport is allowed to close, so the already-open
	// terminal observer receives the canonical completion event without a
	// status-based success wait. The API server holds its listener open while
	// that event and all dependent observations are captured; releasing the gate
	// then lets the process finish.
	daemon.cancel()
	terminalObservation.Wait(timeout)
	if responseCaptureCancel != nil {
		// Work completion and response-stream publication use separate observers.
		// Wait for the stream's terminal event instead of relying on a scheduler
		// sleep. The caller's deadline is a failure guard for a delayed response
		// stream, not permission to return a partial event snapshot.
		responseStreamComplete = waitForTerminalResponseEvent(responseActivity, timeout)
	}
	session := GetDefaultSession(t, baseURL)
	work := ListDefaultSessionWork(t, baseURL)
	events := GetFactoryEventsAt(t, baseURL)
	if captureWorkerSessionEvents != nil {
		// Worker Session replay is the provider-owned lifecycle boundary. Its
		// terminal replay summary is authoritative for source observations that
		// can be published after the Work projection or response RUN event.
		captureWorkerSessionEvents(baseURL, work)
	}
	var responseEvents []factoryapi.FactoryResponseEvent
	if responseCaptureCancel != nil {
		// The terminal response event has been observed before canceling this
		// independent capture request, so its snapshot is complete without
		// relying on transport shutdown to flush it.
		responseCaptureCancel()
		capture := <-responseCaptureDone
		if capture.err != nil {
			t.Fatalf("capture factory response events: %v", capture.err)
		}
		responseEvents = capture.events
		if !responseStreamComplete {
			t.Fatalf(
				"timed out waiting for terminal response event after %s; captured %d events",
				timeout,
				len(responseEvents),
			)
		}
	}
	releaseShutdown()
	select {
	case <-daemon.Done():
	case <-time.After(processCommandStopTimeout):
		t.Fatalf("timed out waiting for Process.Execute() shutdown")
	}
	if err := daemon.Err(); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Process.Execute() after shutdown error = %v", err)
	}
	closeCtx, cancelClose := context.WithTimeout(context.Background(), processCommandStopTimeout)
	defer cancelClose()
	if closer, ok := process.(interface{ Close(context.Context) error }); ok {
		if err := closer.Close(closeCtx); err != nil {
			t.Fatalf("close application process: %v", err)
		}
	}
	return session, work, events, responseEvents
}

type responseEventCaptureResult struct {
	events []factoryapi.FactoryResponseEvent
	err    error
}

func captureFactoryResponseEvents(
	ctx context.Context,
	endpoint string,
	started chan<- error,
	activity chan<- factoryapi.FactoryResponseEvent,
) ([]factoryapi.FactoryResponseEvent, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		started <- err
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return nil, nil
		}
		return nil, fmt.Errorf("GET response-event stream: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		started <- fmt.Errorf("status = %d", response.StatusCode)
		return nil, fmt.Errorf("GET response-event stream status = %d", response.StatusCode)
	}
	started <- nil

	var events []factoryapi.FactoryResponseEvent
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.FactoryResponseEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			return nil, fmt.Errorf("decode response event: %w", err)
		}
		events = append(events, event)
		select {
		case activity <- event:
		default:
		}
	}
	if err := scanner.Err(); err != nil &&
		!errors.Is(err, context.Canceled) &&
		!errors.Is(ctx.Err(), context.Canceled) &&
		!errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("read response-event stream: %w", err)
	}
	return events, nil
}

func waitForTerminalResponseEvent(
	activity <-chan factoryapi.FactoryResponseEvent,
	ceiling time.Duration,
) bool {
	if ceiling <= 0 {
		return false
	}
	timer := time.NewTimer(ceiling)
	defer timer.Stop()
	for {
		select {
		case event, ok := <-activity:
			if !ok {
				return false
			}
			if isTerminalResponseEvent(event) {
				return true
			}
		case <-timer.C:
			return false
		}
	}
}

func isTerminalResponseEvent(event factoryapi.FactoryResponseEvent) bool {
	if event.Kind == factoryapi.FactoryResponseEventKindError {
		return event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
			event.Phase == factoryapi.FactoryResponseEventPhaseCanceled
	}
	return event.Kind == factoryapi.FactoryResponseEventKindRun &&
		(event.Phase == factoryapi.FactoryResponseEventPhaseCompleted ||
			event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
			event.Phase == factoryapi.FactoryResponseEventPhaseCanceled)
}
