package support

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/tests/functional/internal/restclient"
)

const rootRunFunctionalHostStartupTimeout = 10 * time.Second
const rootRunFunctionalHostCleanupTimeout = 5 * time.Second
const rootRunFunctionalHostReadinessAttemptTimeout = 250 * time.Millisecond

// RootRunFunctionalHostConfig contains only explicit process inputs and the
// approved deterministic edges used by production-shaped functional tests.
type RootRunFunctionalHostConfig struct {
	FactoryRoot     string
	SystemRoot      string
	FunctionalEdges wire.FunctionalEdges
	StartupTimeout  time.Duration
	// DisableMockWorkers preserves an injected worker command edge. The default
	// remains mock workers so ordinary root-run functional scenarios stay local
	// and deterministic.
	DisableMockWorkers bool
	// ListenAddress requests a specific loopback host and port. Empty reserves
	// an ephemeral listener before root.Run starts.
	ListenAddress string
}

// RootRunProcessOutcome classifies why the customer-boundary process returned.
type RootRunProcessOutcome string

const (
	RootRunProcessStopped   RootRunProcessOutcome = "stopped"
	RootRunProcessCompleted RootRunProcessOutcome = "completed"
	RootRunProcessFailed    RootRunProcessOutcome = "failed"
)

// RootRunProcessResult is the immutable customer-boundary outcome of root.Run.
type RootRunProcessResult struct {
	ExitCode    int
	Diagnostics string
	Outcome     RootRunProcessOutcome
}

// RootRunFunctionalHost is a customer-boundary driver around root.Run. It does
// not retain the application graph, service, runtime, or event store.
type RootRunFunctionalHost struct {
	endpoint string
	rest     *restclient.Adapter
	cancel   context.CancelFunc
	done     chan struct{}

	resultMu      sync.RWMutex
	result        RootRunProcessResult
	lastReadiness string
	finished      bool
}

// StartRootRunFunctionalHost starts the production root on an isolated local
// listener and proves readiness through the generated GET /status operation.
func StartRootRunFunctionalHost(
	ctx context.Context,
	cfg RootRunFunctionalHostConfig,
) (*RootRunFunctionalHost, error) {
	if ctx == nil {
		return nil, fmt.Errorf("start root.Run functional host: context is required")
	}
	if cfg.FactoryRoot == "" || cfg.SystemRoot == "" {
		return nil, fmt.Errorf("start root.Run functional host: factory and system roots are required")
	}
	endpoint, listener, err := prepareRootRunHostListener(cfg.ListenAddress)
	if err != nil {
		return nil, err
	}

	hostCtx, cancel := context.WithCancel(ctx)
	httpClient := &http.Client{}
	adapter, err := restclient.New(endpoint, httpClient)
	if err != nil {
		cancel()
		_ = listener.Close()
		return nil, err
	}
	host := &RootRunFunctionalHost{
		endpoint: endpoint,
		rest:     adapter,
		cancel:   cancel,
		done:     make(chan struct{}),
	}

	edges := cfg.FunctionalEdges
	edges.APIServerListener = listener
	var diagnostics bytes.Buffer
	go host.run(hostCtx, cfg, edges, &diagnostics)

	timeout := cfg.StartupTimeout
	if timeout <= 0 {
		timeout = rootRunFunctionalHostStartupTimeout
	}
	readyCtx, readyCancel := context.WithTimeout(ctx, timeout)
	defer readyCancel()
	readinessStarted := time.Now()
	if err := host.waitUntilReady(readyCtx); err != nil {
		cancel()
		if listener != nil {
			_ = listener.Close()
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), rootRunFunctionalHostCleanupTimeout)
		defer cleanupCancel()
		return nil, host.startupFailure(err, time.Since(readinessStarted), cleanupCtx)
	}
	return host, nil
}

func prepareRootRunHostListener(requestedAddress string) (string, net.Listener, error) {
	address := strings.TrimSpace(requestedAddress)
	if address != "" {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return "", nil, fmt.Errorf("start root.Run functional host listener address %q: %w", address, err)
		}
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			return "", nil, fmt.Errorf("start root.Run functional host listener address %q: loopback host is required", address)
		}
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber <= 0 || portNumber > 65535 {
			return "", nil, fmt.Errorf("start root.Run functional host listener address %q: valid non-zero TCP port is required", address)
		}
		return "http://" + net.JoinHostPort(host, port), nil, nil
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("start root.Run functional host listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	return "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), listener, nil
}

func (host *RootRunFunctionalHost) run(
	ctx context.Context,
	cfg RootRunFunctionalHostConfig,
	edges wire.FunctionalEdges,
	diagnostics *bytes.Buffer,
) {
	args := []string{
		"you", "--server", host.endpoint, "run", "--dir", cfg.FactoryRoot,
		"--continuously", "--quiet", "--verbose", "--no-record",
	}
	if !cfg.DisableMockWorkers {
		args = append(args, "--with-mock-workers")
	}
	exitCode := root.Run(root.Input{
		Args:    args,
		Env:     rootRunHostEnvironment(cfg.SystemRoot),
		Stderr:  diagnostics,
		Context: ctx,
	}, root.Dependencies{FunctionalEdges: edges})
	if exitCode != root.ExitSuccess {
		captureRequestedListenerFailure(diagnostics, cfg.ListenAddress)
	}
	host.resultMu.Lock()
	host.result = RootRunProcessResult{
		ExitCode:    exitCode,
		Diagnostics: diagnostics.String(),
		Outcome:     rootRunProcessOutcome(exitCode, ctx.Err()),
	}
	host.finished = true
	host.resultMu.Unlock()
	close(host.done)
}

func captureRequestedListenerFailure(diagnostics *bytes.Buffer, requestedAddress string) {
	address := strings.TrimSpace(requestedAddress)
	if address == "" {
		return
	}
	_, port, splitErr := net.SplitHostPort(address)
	if splitErr != nil {
		return
	}
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		_, _ = fmt.Fprintf(diagnostics, "requested listener address %s remains unavailable: %v", address, err)
		return
	}
	_ = listener.Close()
}

func rootRunProcessOutcome(exitCode int, processErr error) RootRunProcessOutcome {
	if exitCode != root.ExitSuccess {
		return RootRunProcessFailed
	}
	if processErr != nil {
		return RootRunProcessStopped
	}
	return RootRunProcessCompleted
}

func (host *RootRunFunctionalHost) waitUntilReady(ctx context.Context) error {
	var lastOutcome error
	retry := time.NewTicker(10 * time.Millisecond)
	defer retry.Stop()
	for {
		attemptCtx, cancelAttempt := context.WithTimeout(ctx, rootRunFunctionalHostReadinessAttemptTimeout)
		response, err := host.rest.GetStatus(attemptCtx)
		cancelAttempt()
		if err == nil && response.StatusCode() == http.StatusOK && response.JSON200 != nil {
			host.recordReadiness("generated GET /status succeeded with HTTP 200")
			return nil
		}
		if err != nil {
			lastOutcome = err
		} else {
			lastOutcome = fmt.Errorf("GET /status returned HTTP %d", response.StatusCode())
		}
		host.recordReadiness(lastOutcome.Error())
		select {
		case <-host.done:
			return fmt.Errorf("process completed before generated REST readiness (last outcome: %v)", lastOutcome)
		case <-ctx.Done():
			return fmt.Errorf("generated REST readiness: %w (last outcome: %v)", ctx.Err(), lastOutcome)
		case <-retry.C:
		}
	}
}

func (host *RootRunFunctionalHost) recordReadiness(outcome string) {
	host.resultMu.Lock()
	host.lastReadiness = outcome
	host.resultMu.Unlock()
}

func rootRunHostEnvironment(systemRoot string) []string {
	environment := []string{"HOME=" + systemRoot, "USERPROFILE=" + systemRoot}
	if runtime.GOOS == "windows" {
		volume := systemRoot[:0]
		if len(systemRoot) >= 2 && systemRoot[1] == ':' {
			volume = systemRoot[:2]
		}
		environment = append(environment, "HOMEDRIVE="+volume, "HOMEPATH="+systemRoot[len(volume):])
	}
	return environment
}

// Endpoint returns the resolved customer API endpoint.
func (host *RootRunFunctionalHost) Endpoint() string { return host.endpoint }

// REST returns the generated REST adapter for this endpoint.
func (host *RootRunFunctionalHost) REST() *restclient.Adapter { return host.rest }

// OpenFactoryEvents opens the canonical Factory Session event SSE stream.
func (host *RootRunFunctionalHost) OpenFactoryEvents(
	ctx context.Context,
	sessionID generatedclient.SessionID,
	params *generatedclient.GetEventsBySessionIdParams,
) (*http.Response, error) {
	return host.rest.OpenFactoryEventsBySessionID(ctx, sessionID, params)
}

// Done closes when the root.Run process returns.
func (host *RootRunFunctionalHost) Done() <-chan struct{} { return host.done }

// Result returns the immutable process result after completion.
func (host *RootRunFunctionalHost) Result() (RootRunProcessResult, bool) {
	host.resultMu.RLock()
	defer host.resultMu.RUnlock()
	return host.result, host.finished
}

func (host *RootRunFunctionalHost) waitForResult(ctx context.Context) (RootRunProcessResult, error) {
	select {
	case <-host.done:
		result, _ := host.Result()
		return result, nil
	case <-ctx.Done():
		return RootRunProcessResult{}, ctx.Err()
	}
}

func (host *RootRunFunctionalHost) startupFailure(
	readinessErr error,
	elapsed time.Duration,
	cleanupCtx context.Context,
) error {
	result, cleanupErr := host.waitForResult(cleanupCtx)
	host.resultMu.RLock()
	lastReadiness := host.lastReadiness
	finished := host.finished
	if finished {
		result = host.result
	}
	host.resultMu.RUnlock()

	processOutcome := "pending"
	if finished {
		processOutcome = fmt.Sprintf(
			"outcome=%s exit=%d diagnostics=%q",
			result.Outcome,
			result.ExitCode,
			result.Diagnostics,
		)
	}
	if cleanupErr != nil {
		processOutcome += fmt.Sprintf(" cleanup join=%v", cleanupErr)
	}
	return fmt.Errorf(
		"start root.Run functional host at %s failed during generated REST readiness after %s: %w (last generated-client outcome=%q; terminal process=%s)",
		host.endpoint,
		elapsed.Round(time.Millisecond),
		readinessErr,
		lastReadiness,
		processOutcome,
	)
}

func (host *RootRunFunctionalHost) shutdownDiagnostic(cause error) (RootRunProcessResult, error) {
	host.resultMu.RLock()
	defer host.resultMu.RUnlock()
	processOutcome := "pending"
	if host.finished {
		processOutcome = fmt.Sprintf(
			"outcome=%s exit=%d diagnostics=%q",
			host.result.Outcome,
			host.result.ExitCode,
			host.result.Diagnostics,
		)
	}
	return host.result, fmt.Errorf(
		"shut down root.Run functional host at %s: %w (last readiness=%q; process=%s)",
		host.endpoint,
		cause,
		host.lastReadiness,
		processOutcome,
	)
}

// Shutdown requests process cancellation and joins it within ctx's bound.
func (host *RootRunFunctionalHost) Shutdown(ctx context.Context) (RootRunProcessResult, error) {
	if ctx == nil {
		return RootRunProcessResult{}, fmt.Errorf("shut down root.Run functional host at %s: context is required", host.endpoint)
	}
	host.cancel()
	if err := ctx.Err(); err != nil {
		return host.shutdownDiagnostic(err)
	}
	result, err := host.waitForResult(ctx)
	if err != nil {
		return host.shutdownDiagnostic(err)
	}
	return result, nil
}
