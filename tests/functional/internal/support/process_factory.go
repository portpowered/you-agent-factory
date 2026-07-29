package support

import (
	"context"
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
	WaitForTerminalStatus(t, baseURL, timeout)

	session := GetDefaultSession(t, baseURL)
	work := ListDefaultSessionWork(t, baseURL)
	events := GetFactoryEventsAt(t, baseURL)
	var responseEvents []factoryapi.FactoryResponseEvent
	if captureResponseEvents {
		responseEvents = GetFactoryResponseEventsAt(t, baseURL, session.Id)
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
