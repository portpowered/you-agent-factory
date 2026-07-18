package support

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/tests/functional/internal/restclient"
)

const rootRunFunctionalHostStartupTimeout = 10 * time.Second
const rootRunFunctionalHostReadinessAttemptTimeout = 250 * time.Millisecond

// RootRunFunctionalHostConfig contains only explicit process inputs and the
// approved deterministic edges used by production-shaped functional tests.
type RootRunFunctionalHostConfig struct {
	FactoryRoot     string
	SystemRoot      string
	FunctionalEdges wire.FunctionalEdges
	StartupTimeout  time.Duration
}

// RootRunProcessResult is the immutable customer-boundary outcome of root.Run.
type RootRunProcessResult struct {
	ExitCode    int
	Diagnostics string
}

// RootRunFunctionalHost is a customer-boundary driver around root.Run. It does
// not retain the application graph, service, runtime, or event store.
type RootRunFunctionalHost struct {
	endpoint string
	rest     *restclient.Adapter
	cancel   context.CancelFunc
	done     chan struct{}

	resultMu sync.RWMutex
	result   RootRunProcessResult
	finished bool
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
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start root.Run functional host listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	endpoint := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

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
	if err := host.waitUntilReady(readyCtx); err != nil {
		cancel()
		_ = listener.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), timeout)
		defer cleanupCancel()
		result, cleanupErr := host.waitForResult(cleanupCtx)
		if cleanupErr != nil {
			return nil, fmt.Errorf("start root.Run functional host at %s: %w; cleanup: %v", endpoint, err, cleanupErr)
		}
		return nil, fmt.Errorf("start root.Run functional host at %s: %w; process exit=%d diagnostics=%q", endpoint, err, result.ExitCode, result.Diagnostics)
	}
	return host, nil
}

func (host *RootRunFunctionalHost) run(
	ctx context.Context,
	cfg RootRunFunctionalHostConfig,
	edges wire.FunctionalEdges,
	diagnostics *bytes.Buffer,
) {
	exitCode := root.Run(root.Input{
		Args: []string{
			"you", "--server", host.endpoint, "run", "--dir", cfg.FactoryRoot,
			"--continuously", "--quiet", "--verbose", "--no-record", "--with-mock-workers",
		},
		Env:     rootRunHostEnvironment(cfg.SystemRoot),
		Stderr:  diagnostics,
		Context: ctx,
	}, root.Dependencies{FunctionalEdges: edges})
	host.resultMu.Lock()
	host.result = RootRunProcessResult{ExitCode: exitCode, Diagnostics: diagnostics.String()}
	host.finished = true
	host.resultMu.Unlock()
	close(host.done)
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
			return nil
		}
		if err != nil {
			lastOutcome = err
		} else {
			lastOutcome = fmt.Errorf("GET /status returned HTTP %d", response.StatusCode())
		}
		select {
		case <-host.done:
			return fmt.Errorf("process completed before generated REST readiness (last outcome: %v)", lastOutcome)
		case <-ctx.Done():
			return fmt.Errorf("generated REST readiness: %w (last outcome: %v)", ctx.Err(), lastOutcome)
		case <-retry.C:
		}
	}
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

// Shutdown requests process cancellation and joins it within ctx's bound.
func (host *RootRunFunctionalHost) Shutdown(ctx context.Context) (RootRunProcessResult, error) {
	if ctx == nil {
		return RootRunProcessResult{}, fmt.Errorf("shut down root.Run functional host at %s: context is required", host.endpoint)
	}
	host.cancel()
	result, err := host.waitForResult(ctx)
	if err != nil {
		return RootRunProcessResult{}, fmt.Errorf("shut down root.Run functional host at %s: %w", host.endpoint, err)
	}
	return result, nil
}
