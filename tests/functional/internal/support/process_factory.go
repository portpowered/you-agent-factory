package support

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/providers/inference"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// MockInferenceProvider returns a typed provider override for functional tests
// without requiring destination packages to import service implementation paths.
func MockInferenceProvider(contents ...string) workerprovider.Provider {
	responses := make([]workerexecution.InferenceResponse, len(contents))
	for index, content := range contents {
		responses[index] = workerexecution.InferenceResponse{Content: content}
	}
	return testutil.NewMockProvider(responses...)
}

// BlockingInferenceProvider blocks the first inference call until release is
// closed or the context is canceled, then completes subsequent calls immediately.
func BlockingInferenceProvider(release <-chan struct{}) workerprovider.Provider {
	return &blockingInferenceProvider{release: release}
}

type blockingInferenceProvider struct {
	release <-chan struct{}
	mu      sync.Mutex
	calls   int
}

func (p *blockingInferenceProvider) Infer(
	ctx context.Context,
	_ workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	if call == 1 {
		select {
		case <-p.release:
		case <-ctx.Done():
			return workerexecution.InferenceResponse{}, ctx.Err()
		}
	}
	return workerexecution.InferenceResponse{Content: "completed"}, nil
}

// RunFactoryToCompletion executes the customer daemon command through the
// canonical root-built process and returns its public default-session
// projection after every visible work token becomes terminal.
func RunFactoryToCompletion(
	t testing.TB,
	dir string,
	provider workerprovider.Provider,
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
	session, _, _, _ := runFactoryToCompletion(t, dir, overrides, timeout, false)
	return session
}

// RunFactoryToCompletionWithEdgesAndWork also returns the customer-visible
// Work listing captured before the daemon stops.
func RunFactoryToCompletionWithEdgesAndWork(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse) {
	session, work, _, _ := runFactoryToCompletion(t, dir, overrides, timeout, false)
	return session, work
}

// RunFactoryToCompletionWithEdgesAndObservations also returns the public Work
// listing and retained Factory Event history captured before the daemon stops.
func RunFactoryToCompletionWithEdgesAndObservations(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	session, work, events, _ := runFactoryToCompletion(t, dir, overrides, timeout, false)
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
	session, work, events, _ := runFactoryToCompletionWithHome(t, dir, overrides, timeout, false, configure)
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
	return runFactoryToCompletion(t, dir, overrides, timeout, true)
}

func runFactoryToCompletion(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
	captureResponseEvents bool,
) (
	factoryapi.FactorySession,
	factoryapi.ListWorkResponse,
	[]factoryapi.FactoryEvent,
	[]factoryapi.FactoryResponseEvent,
) {
	return runFactoryToCompletionWithHome(t, dir, overrides, timeout, captureResponseEvents, nil)
}

func runFactoryToCompletionWithHome(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
	captureResponseEvents bool,
	configure func(string),
) (
	factoryapi.FactorySession,
	factoryapi.ListWorkResponse,
	[]factoryapi.FactoryEvent,
	[]factoryapi.FactoryResponseEvent,
) {
	t.Helper()

	server := NewProcessAPIServer()
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
	baseURL := server.WaitForURL(t)
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
		liveSession := GetDefaultSession(t, baseURL)
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
	WaitForTerminalStatus(t, baseURL, timeout)

	session := GetDefaultSession(t, baseURL)
	work := ListDefaultSessionWork(t, baseURL)
	events := GetFactoryEventsAt(t, baseURL)
	var responseEvents []factoryapi.FactoryResponseEvent
	if responseCaptureCancel != nil {
		// Work completion and response-stream publication use separate observers.
		// Wait for the stream's terminal event instead of relying on a scheduler
		// sleep, with a short ceiling for providers that expose partial streams.
		waitForTerminalResponseEvent(responseActivity, 500*time.Millisecond)
		responseCaptureCancel()
		capture := <-responseCaptureDone
		if capture.err != nil {
			t.Fatalf("capture factory response events: %v", capture.err)
		}
		responseEvents = capture.events
	}
	daemon.Stop(t)
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
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(ctx.Err(), context.Canceled) {
		return nil, fmt.Errorf("read response-event stream: %w", err)
	}
	return events, nil
}

func waitForTerminalResponseEvent(
	activity <-chan factoryapi.FactoryResponseEvent,
	ceiling time.Duration,
) {
	timer := time.NewTimer(ceiling)
	defer timer.Stop()
	for {
		select {
		case event := <-activity:
			if isTerminalResponseEvent(event) {
				return
			}
		case <-timer.C:
			return
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
