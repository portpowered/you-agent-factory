package server_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/pprof/profile"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAPIServerDiagnosticsUseProductionLoopbackStarter proves the diagnostics
// contract through the customer process with the production HTTP starter. The
// existing API-server helper intentionally owns an httptest transport for most
// HTTP functional tests; this cell reaches the real bind, Serve, and shutdown
// lifecycle so those host-network branches remain in the functional profile.
func TestAPIServerDiagnosticsUseProductionLoopbackStarter(t *testing.T) {
	factory := scaffoldC06IsolatedFactory(t, startupShutdownTestFactoryConfig())

	for _, test := range []struct {
		name  string
		pprof bool
	}{
		{name: "disabled by default"},
		{name: "enabled by opt-in", pprof: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			lifecycleID := "c06-production-diagnostics-disabled"
			if test.pprof {
				lifecycleID = "c06-production-diagnostics-enabled"
			}
			server := startProductionLoopbackServerWithID(
				t,
				factory.factoryDir,
				test.pprof,
				lifecycleID,
				"diagnostics mode changes production listener startup configuration",
			)
			assertLiveStatusAndRuntimeDiagnostics(t, server.url)
			if test.pprof {
				assertLivePprofHeap(t, server.url)
				assertLivePprofDiagnostics(t, server.url)
				server.close(t)
				return
			}
			assertPprofUnavailable(t, server.url)
			server.close(t)
		})
	}
}

// TestAPIServerGracefulShutdownThroughProductionLoopbackLifecycle proves the
// delivered server-stop path drains a public long-lived response, returns its
// serve lifecycle, and leaves the real loopback listener unavailable.
func TestAPIServerGracefulShutdownThroughProductionLoopbackLifecycle(t *testing.T) {
	// The stop path mutates the hosted process and must observe a real listener,
	// active stream termination, and process join in its own lifecycle.
	factory := scaffoldC06IsolatedFactory(t, startupShutdownTestFactoryConfig())
	server := startProductionLoopbackServerWithID(
		t,
		factory.factoryDir,
		false,
		"c06-production-graceful-shutdown",
		"graceful stop mutates the process and drains its real listener",
	)
	session := getFactorySession(t, server.url, factorysessions.DefaultSessionID)
	stream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(server.url, session.Id),
	)

	stopProcess := c06BuildIsolatedProcess(
		t,
		"c06-production-stop-caller",
		"server stop uses an independent root-built caller process",
		c06IsolationExpectation{},
		serviceedges.Edges{},
	)
	stopContext, cancelStop := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancelStop()
	stopInputs := support.FakeInputs(stopContext, []string{
		"you", "--server", server.url, "server", "stop",
	})
	stopInputs.Input.WorkingDirectory = factory.factoryDir
	home := t.TempDir()
	stopInputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)

	if err := stopProcess.Execute(stopInputs.Input); err != nil {
		t.Fatalf(
			"server stop through root process: %v\nstdout=%q\nstderr=%q",
			err,
			stopInputs.Stdout(),
			stopInputs.Stderr(),
		)
	}
	stopProcess.markJoined()
	stopProcess.close(t)
	if !strings.Contains(stopInputs.Stdout(), "Server stopped:") {
		t.Fatalf("server stop stdout = %q, want successful stop confirmation", stopInputs.Stdout())
	}

	select {
	case <-server.command.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the production serve lifecycle to return")
	}
	if err := server.command.Err(); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("production serve lifecycle error after server stop = %v", err)
	}

	stream.WaitClosed(10 * time.Second)
	observer := platformhttpserver.NewListenerStopObserver(
		(&net.Dialer{}).DialContext,
		platformhttpserver.DefaultListenerStopObservationInterval,
	)
	if err := observer.Wait(t.Context(), server.address, 5*time.Second); err != nil {
		t.Fatalf("listener stop observation after server stop = %v, want success", err)
	}
	server.close(t)
}

// TestListenerStopObserverReportsBoundedOpenListenerOutcomes proves the
// observer distinguishes an open listener's deadline from caller cancellation.
// This is an isolated listener-only witness because the open OS listener and
// accept goroutine are the behavior under test, not an application process.
func TestListenerStopObserverReportsBoundedOpenListenerOutcomes(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for listener observer: %v", err)
	}
	const lifecycleID = "c06-listener-stop-observer"
	if err := c06IsolatedLifecycle.register(
		lifecycleID,
		"the listener observer owns an open OS listener and accept goroutine",
		c06IsolationExpectation{listener: true, portRelease: true},
	); err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	c06IsolatedLifecycle.listenerBound(lifecycleID)
	listenerAddress := listener.Addr().String()
	acceptDone := make(chan struct{})
	var acceptedMu sync.Mutex
	acceptedConnections := make([]net.Conn, 0, 128)
	go func() {
		defer close(acceptDone)
		for {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			acceptedMu.Lock()
			acceptedConnections = append(acceptedConnections, connection)
			acceptedMu.Unlock()
		}
	}()
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Errorf("close listener observer fixture: %v", err)
		}
		acceptedMu.Lock()
		for _, connection := range acceptedConnections {
			_ = connection.Close()
		}
		acceptedMu.Unlock()
		<-acceptDone
		c06IsolatedLifecycle.listenerClosed(lifecycleID)
		if err := rebindC06Listener(listenerAddress); err != nil {
			t.Errorf("listener observer port cleanup: %v", err)
			return
		}
		c06IsolatedLifecycle.portReleased(lifecycleID)
	})

	dialContext := func(ctx context.Context, network string, address string) (net.Conn, error) {
		// A real open listener can transiently reject a probe while the accept
		// goroutine is being scheduled, especially on Windows. Retry until the
		// context ends so that transient dial errors cannot be mistaken for a
		// listener that has actually stopped.
		for {
			connection, dialErr := (&net.Dialer{}).DialContext(ctx, network, address)
			if dialErr == nil {
				return connection, nil
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			retryTimer := time.NewTimer(time.Millisecond)
			select {
			case <-ctx.Done():
				if !retryTimer.Stop() {
					<-retryTimer.C
				}
				return nil, ctx.Err()
			case <-retryTimer.C:
			}
		}
	}
	observer := platformhttpserver.NewListenerStopObserver(
		dialContext,
		1*time.Millisecond,
	)
	address := listenerAddress
	if err := observer.Wait(context.Background(), address, 100*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("open-listener observation error = %v, want context deadline exceeded", err)
	}

	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	if err := observer.Wait(canceledContext, address, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled open-listener observation error = %v, want context canceled", err)
	}
}

type productionLoopbackServer struct {
	url         string
	address     string
	command     *support.ProcessCommand
	process     *c06IsolatedProcess
	lifecycleID string
	closeOnce   sync.Once
}

func startProductionLoopbackServerWithID(
	t *testing.T,
	dir string,
	pprofEnabled bool,
	lifecycleID string,
	reason string,
) *productionLoopbackServer {
	t.Helper()

	bound := make(chan platformhttpserver.Binding, 1)
	starter, err := platformhttpserver.NewStarter(
		net.Listen,
		nil,
		func() []string { return []string{"you", "run", "--with-server"} },
	)
	if err != nil {
		t.Fatalf("NewStarter() error = %v", err)
	}

	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(t, &edges, support.NewStaticSuccessCommandRunner("live diagnostics"), nil)
	edges.APIServerStarter = func(ctx context.Context, request platformhttpserver.StartRequest) error {
		c06IsolatedLifecycle.processHosted(lifecycleID)
		originalOnBound := request.OnBound
		request.OnBound = func(binding platformhttpserver.Binding) {
			c06IsolatedLifecycle.listenerBound(lifecycleID)
			select {
			case bound <- binding:
			default:
			}
			if originalOnBound != nil {
				originalOnBound(binding)
			}
		}
		return starter(ctx, request)
	}

	process := c06BuildIsolatedProcess(t, lifecycleID, reason, c06IsolationExpectation{
		listener:    true,
		portRelease: true,
	}, edges)
	requestedURL := reserveConfiguredLoopbackURL(t)
	requestedEndpoint, err := url.Parse(requestedURL)
	if err != nil {
		t.Fatalf("parse reserved loopback URL %q: %v", requestedURL, err)
	}
	requestedPort, err := strconv.Atoi(requestedEndpoint.Port())
	if err != nil {
		t.Fatalf("parse reserved loopback port from %q: %v", requestedURL, err)
	}
	args := []string{
		"you", "run", "--dir", dir, "--continuously", "--with-server",
		"--listen", net.JoinHostPort("127.0.0.1", strconv.Itoa(requestedPort)),
		"--quiet", "--no-record",
	}
	if pprofEnabled {
		args = append(args, "--pprof")
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = dir
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	command := support.StartProcessCommand(t, process, inputs.Input)
	server := &productionLoopbackServer{
		command:     command,
		process:     process,
		lifecycleID: lifecycleID,
	}
	// The cleanup is registered after StartProcessCommand so it stops the
	// invocation, closes the root process, and then verifies the listener.
	t.Cleanup(func() { server.close(t) })

	var binding platformhttpserver.Binding
	select {
	case binding = <-bound:
	case <-time.After(15 * time.Second):
		command.AcceptError()
		t.Fatal("timed out waiting for production HTTP starter binding")
	}
	if binding.Host != "127.0.0.1" || binding.Port <= 0 {
		command.AcceptError()
		t.Fatalf("production HTTP binding = %+v, want a loopback endpoint", binding)
	}

	serverURL := "http://" + net.JoinHostPort(binding.Host, strconv.Itoa(binding.Port))
	support.WaitForStatus(t, serverURL, 15*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.RuntimeStatus != ""
	})
	server.url = serverURL
	server.address = net.JoinHostPort(binding.Host, strconv.Itoa(binding.Port))
	return server
}

func (server *productionLoopbackServer) close(t testing.TB) {
	if server == nil {
		return
	}
	server.closeOnce.Do(func() {
		if server.command != nil {
			server.command.Stop(t)
			server.process.markJoined()
		}
		server.process.close(t)
		if server.address == "" {
			return
		}
		// This bounded observer is the lifecycle witness: deterministic request
		// completion cannot replace checking that the OS listener has stopped.
		observer := platformhttpserver.NewListenerStopObserver(
			(&net.Dialer{}).DialContext,
			platformhttpserver.DefaultListenerStopObservationInterval,
		)
		if err := observer.Wait(context.Background(), server.address, 5*time.Second); err != nil {
			t.Errorf("production listener %s stop observation: %v", server.address, err)
			return
		}
		c06IsolatedLifecycle.listenerClosed(server.lifecycleID)
		if err := assertC06ListenerReleased(server.url); err != nil {
			t.Errorf("production listener %s cleanup: %v", server.address, err)
			return
		}
		c06IsolatedLifecycle.portReleased(server.lifecycleID)
	})
}

func assertLiveStatusAndRuntimeDiagnostics(t *testing.T, serverURL string) {
	t.Helper()
	status := support.GetJSON[factoryapi.StatusResponse](t, serverURL+"/status")
	if status.FactoryState == "" || status.RuntimeStatus == "" {
		t.Fatalf("live /status = %+v, want factory and runtime state", status)
	}

	snapshot := support.GetJSON[platformhttpserver.RuntimeSnapshot](t, serverURL+"/debug/runtime")
	if snapshot.HeapAllocBytes == 0 || snapshot.HeapInuseBytes < snapshot.HeapAllocBytes ||
		snapshot.SysBytes < snapshot.HeapInuseBytes || snapshot.Goroutines <= 0 {
		t.Fatalf("live runtime snapshot = %+v, want plausible values", snapshot)
	}
	if snapshot.ProcessCommitAvailable && snapshot.ProcessCommitBytes == 0 {
		t.Fatalf("live runtime snapshot = %+v, want positive commit bytes when available", snapshot)
	}
}

func assertPprofUnavailable(t *testing.T, serverURL string) {
	t.Helper()
	response, err := http.Get(serverURL + "/debug/pprof/heap")
	if err != nil {
		t.Fatalf("GET disabled pprof heap: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("disabled pprof heap status = %d, want %d", response.StatusCode, http.StatusNotFound)
	}
}

func assertLivePprofHeap(t *testing.T, serverURL string) {
	t.Helper()
	response, err := http.Get(serverURL + "/debug/pprof/heap")
	if err != nil {
		t.Fatalf("GET enabled pprof heap: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read enabled pprof heap: %v", err)
	}
	if response.StatusCode != http.StatusOK || len(body) == 0 {
		t.Fatalf("enabled pprof heap = (%d, body length %d), want a non-empty HTTP 200 response", response.StatusCode, len(body))
	}
	parsed, err := profile.Parse(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("parse enabled live pprof heap: %v", err)
	}
	if parsed == nil || len(parsed.SampleType) == 0 || len(parsed.Sample) == 0 {
		t.Fatalf("enabled live pprof heap = %+v, want sample types and samples", parsed)
	}
}

func assertLivePprofDiagnostics(t *testing.T, serverURL string) {
	t.Helper()

	for _, test := range []struct {
		path      string
		wantBody  string
		wantState int
	}{
		{path: "/debug/pprof/cmdline", wantBody: "you\x00run", wantState: http.StatusOK},
		{path: "/debug/pprof/symbol", wantBody: "num_symbols: 1", wantState: http.StatusOK},
		{path: "/debug/pprof/heap?seconds=0", wantBody: `invalid value for "seconds"`, wantState: http.StatusBadRequest},
	} {
		response, err := http.Get(serverURL + test.path)
		if err != nil {
			t.Fatalf("GET enabled pprof %s: %v", test.path, err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read enabled pprof %s: %v", test.path, err)
		}
		if response.StatusCode != test.wantState || !strings.Contains(string(body), test.wantBody) {
			t.Fatalf("enabled pprof %s = (%d, %q), want status %d containing %q", test.path, response.StatusCode, body, test.wantState, test.wantBody)
		}
	}

	trace, err := http.Get(serverURL + "/debug/pprof/trace?seconds=0.01")
	if err != nil {
		t.Fatalf("GET enabled pprof trace: %v", err)
	}
	traceBody, err := io.ReadAll(trace.Body)
	_ = trace.Body.Close()
	if err != nil {
		t.Fatalf("read enabled pprof trace: %v", err)
	}
	if trace.StatusCode != http.StatusOK || len(traceBody) == 0 {
		t.Fatalf("enabled pprof trace = (%d, body length %d), want non-empty HTTP 200 response", trace.StatusCode, len(traceBody))
	}

	delta, err := http.Get(serverURL + "/debug/pprof/goroutine?seconds=1")
	if err != nil {
		t.Fatalf("GET enabled pprof goroutine delta: %v", err)
	}
	deltaBody, err := io.ReadAll(delta.Body)
	_ = delta.Body.Close()
	if err != nil {
		t.Fatalf("read enabled pprof goroutine delta: %v", err)
	}
	if delta.StatusCode != http.StatusOK || len(deltaBody) == 0 {
		t.Fatalf("enabled pprof goroutine delta = (%d, body length %d), want non-empty HTTP 200 response", delta.StatusCode, len(deltaBody))
	}
}
