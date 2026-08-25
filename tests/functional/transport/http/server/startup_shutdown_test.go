package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/pprof/profile"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAPIServerPprofIsOptInThroughThePublicRunPath proves the diagnostics
// routes are absent by default and that --pprof exposes a live heap profile on
// the same loopback API server built through root.BuildProcess.
func TestAPIServerPprofIsOptInThroughThePublicRunPath(t *testing.T) {
	dir := support.ScaffoldFactory(t, startupShutdownTestFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(t, &edges, support.NewStaticSuccessCommandRunner("pprof diagnostics"), nil)
	metricsDir := t.TempDir()
	defaultServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
		Args:                      []string{"--runtime-metrics-dir", metricsDir},
	})
	for _, path := range []string{
		"/debug/pprof/", "/debug/pprof/heap", "/debug/pprof/profile",
		"/debug/pprof/trace", "/debug/pprof/goroutine", "/debug/pprof/cmdline",
		"/debug/pprof/symbol", "/debug/vars",
	} {
		response, err := http.Get(defaultServer.URL() + path)
		if err != nil {
			t.Fatalf("default GET %s: %v", path, err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read default GET %s: %v", path, err)
		}
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("default GET %s status = %d, want %d", path, response.StatusCode, http.StatusNotFound)
		}
		if path != "/debug/pprof/heap" {
			continue
		}
		var notFound factoryapi.ErrorResponse
		if err := json.Unmarshal(body, &notFound); err != nil {
			t.Fatalf("decode default GET %s error response: %v; body=%q", path, err, body)
		}
		if notFound.Code != factoryapi.ErrorResponseCode("NOT_FOUND") ||
			notFound.Family != factoryapi.ErrorFamily("NOT_FOUND") {
			t.Fatalf("default GET %s error response = %+v, want NOT_FOUND JSON", path, notFound)
		}
	}
	runtimeSnapshot := support.GetJSON[platformhttpserver.RuntimeSnapshot](t, defaultServer.URL()+"/debug/runtime")
	if runtimeSnapshot.HeapAllocBytes == 0 || runtimeSnapshot.HeapInuseBytes < runtimeSnapshot.HeapAllocBytes ||
		runtimeSnapshot.SysBytes < runtimeSnapshot.HeapInuseBytes || runtimeSnapshot.Goroutines <= 0 {
		t.Fatalf("default runtime snapshot = %+v, want plausible live runtime values", runtimeSnapshot)
	}
	defaultServer.Stop(t)
	assertRuntimeMemoryMetrics(t, metricsDir)

	enabledServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
		Args:                      []string{"--pprof"},
	})
	assertEnabledPprofServer(t, enabledServer)
	enabledServer.Stop(t)
}

// TestAPIServerDiagnosticsUseProductionLoopbackStarter proves the diagnostics
// contract through the customer process with the production HTTP starter. The
// existing API-server helper intentionally owns an httptest transport for most
// HTTP functional tests; this cell reaches the real bind, Serve, and shutdown
// lifecycle so those host-network branches remain in the functional profile.
func TestAPIServerDiagnosticsUseProductionLoopbackStarter(t *testing.T) {
	dir := support.ScaffoldFactory(t, startupShutdownTestFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))

	for _, test := range []struct {
		name  string
		pprof bool
	}{
		{name: "disabled by default"},
		{name: "enabled by opt-in", pprof: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			serverURL := startProductionLoopbackAPIServer(t, dir, test.pprof)
			assertLiveStatusAndRuntimeDiagnostics(t, serverURL)
			if test.pprof {
				assertLivePprofHeap(t, serverURL)
				assertLivePprofDiagnostics(t, serverURL)
				return
			}
			assertPprofUnavailable(t, serverURL)
		})
	}
}

// TestAPIServerGracefulShutdownThroughProductionLoopbackLifecycle proves the
// delivered server-stop path drains a public long-lived response, returns its
// serve lifecycle, and leaves the real loopback listener unavailable.
func TestAPIServerGracefulShutdownThroughProductionLoopbackLifecycle(t *testing.T) {
	dir := support.ScaffoldFactory(t, startupShutdownTestFactoryConfig())
	server := startProductionLoopbackServer(t, dir, false)
	session := getFactorySession(t, server.url, factorysessions.DefaultSessionID)
	stream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(server.url, session.Id),
	)

	stopProcess := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, stopProcess)
	stopContext, cancelStop := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancelStop()
	stopInputs := support.FakeInputs(stopContext, []string{
		"you", "--server", server.url, "server", "stop",
	})
	stopInputs.Input.WorkingDirectory = dir
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
}

// TestListenerStopObserverReportsBoundedOpenListenerOutcomes proves the
// observer distinguishes an open listener's deadline from caller cancellation.
func TestListenerStopObserverReportsBoundedOpenListenerOutcomes(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for listener observer: %v", err)
	}
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
	defer func() {
		_ = listener.Close()
		acceptedMu.Lock()
		for _, connection := range acceptedConnections {
			_ = connection.Close()
		}
		acceptedMu.Unlock()
		<-acceptDone
	}()

	dialContext := func(ctx context.Context, network string, address string) (net.Conn, error) {
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
	address := listener.Addr().String()
	if err := observer.Wait(context.Background(), address, 100*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("open-listener observation error = %v, want context deadline exceeded", err)
	}

	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	if err := observer.Wait(canceledContext, address, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled open-listener observation error = %v, want context canceled", err)
	}
}

func startProductionLoopbackAPIServer(
	t *testing.T,
	dir string,
	pprofEnabled bool,
) string {
	t.Helper()
	return startProductionLoopbackServer(t, dir, pprofEnabled).url
}

type productionLoopbackServer struct {
	url     string
	address string
	command *support.ProcessCommand
}

func startProductionLoopbackServer(
	t *testing.T,
	dir string,
	pprofEnabled bool,
) productionLoopbackServer {
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
		originalOnBound := request.OnBound
		request.OnBound = func(binding platformhttpserver.Binding) {
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

	process := support.BuildProcess(t, edges)
	support.CleanupProcess(t, process)
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
	return productionLoopbackServer{
		url:     serverURL,
		address: net.JoinHostPort(binding.Host, strconv.Itoa(binding.Port)),
		command: command,
	}
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

func assertEnabledPprofServer(t *testing.T, enabledServer *support.FunctionalAPIServer) {
	t.Helper()
	index, err := http.Get(enabledServer.URL() + "/debug/pprof/")
	if err != nil {
		t.Fatalf("enabled GET pprof index: %v", err)
	}
	indexBody, err := io.ReadAll(index.Body)
	_ = index.Body.Close()
	if err != nil {
		t.Fatalf("read enabled pprof index: %v", err)
	}
	if index.StatusCode != http.StatusOK || !strings.Contains(string(indexBody), "heap") {
		t.Fatalf("enabled pprof index = (%d, %q), want HTTP 200 with heap profile", index.StatusCode, indexBody)
	}

	heap, err := http.Get(enabledServer.URL() + "/debug/pprof/heap")
	if err != nil {
		t.Fatalf("enabled GET pprof heap: %v", err)
	}
	heapBody, err := io.ReadAll(heap.Body)
	_ = heap.Body.Close()
	if err != nil {
		t.Fatalf("read enabled pprof heap: %v", err)
	}
	if heap.StatusCode != http.StatusOK || len(heapBody) == 0 {
		t.Fatalf("enabled pprof heap = (%d, body length %d), want non-empty HTTP 200 response", heap.StatusCode, len(heapBody))
	}
	heapProfile, err := profile.Parse(bytes.NewReader(heapBody))
	if err != nil {
		t.Fatalf("parse enabled live heap profile: %v", err)
	}
	if len(heapProfile.SampleType) == 0 || len(heapProfile.Sample) == 0 {
		t.Fatalf("enabled live heap profile = %+v, want sample types and samples", heapProfile)
	}

	for _, path := range []string{"/debug/pprof/allocs", "/debug/pprof/goroutine"} {
		response, err := http.Get(enabledServer.URL() + path)
		if err != nil {
			t.Fatalf("enabled GET %s: %v", path, err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read enabled GET %s: %v", path, err)
		}
		if response.StatusCode != http.StatusOK || len(body) == 0 {
			t.Fatalf("enabled GET %s = (%d, body length %d), want non-empty HTTP 200 response", path, response.StatusCode, len(body))
		}
	}
	cpu, err := http.Get(enabledServer.URL() + "/debug/pprof/profile?seconds=1")
	if err != nil {
		t.Fatalf("enabled GET CPU profile: %v", err)
	}
	cpuBody, err := io.ReadAll(cpu.Body)
	_ = cpu.Body.Close()
	if err != nil {
		t.Fatalf("read enabled CPU profile: %v", err)
	}
	if cpu.StatusCode != http.StatusOK || len(cpuBody) == 0 {
		t.Fatalf("enabled CPU profile = (%d, body length %d), want non-empty HTTP 200 response", cpu.StatusCode, len(cpuBody))
	}
	cpuProfile, err := profile.Parse(bytes.NewReader(cpuBody))
	if err != nil {
		t.Fatalf("parse enabled live CPU profile: %v", err)
	}
	if len(cpuProfile.SampleType) == 0 {
		t.Fatalf("enabled live CPU profile = %+v, want sample types", cpuProfile)
	}
}

func assertRuntimeMemoryMetrics(t *testing.T, root string) {
	t.Helper()

	reader, err := platformmetrics.NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("construct runtime metrics reader: %v", err)
	}
	records, err := reader.Read(t.Context(), root)
	if err != nil {
		t.Fatalf("read runtime metrics: %v", err)
	}
	values := make(map[string]float64, 7)
	units := make(map[string]string, 7)
	memoryNames := make([]string, 0, 7)
	for _, record := range records {
		name, _ := record["metric_name"].(string)
		if !isRuntimeMemoryMetric(name) {
			continue
		}
		memoryNames = append(memoryNames, name)
		value, ok := record["value"].(float64)
		if !ok {
			t.Fatalf("runtime memory metric %q value = %#v, want JSON number", name, record["value"])
		}
		values[name] = value
		units[name], _ = record["unit"].(string)
		if metricType, _ := record["metric_type"].(string); metricType != "sample" {
			t.Fatalf("runtime memory metric %q type = %q, want sample", name, metricType)
		}
	}
	assertCompleteRuntimeMemoryObservations(t, memoryNames)
	heapAlloc, allocOK := values["runtime.memory.heap_alloc"]
	heapInuse, inuseOK := values["runtime.memory.heap_inuse"]
	sys, sysOK := values["runtime.memory.sys"]
	numGC, gcOK := values["runtime.memory.num_gc"]
	goroutines, goroutinesOK := values["runtime.memory.goroutines"]
	processCommit, commitOK := values["runtime.memory.process_commit"]
	processCommitAvailable, commitAvailableOK := values["runtime.memory.process_commit_available"]
	if !allocOK || !inuseOK || !sysOK || !gcOK || !goroutinesOK || !commitOK || !commitAvailableOK {
		t.Fatalf("runtime memory records = %#v, want the complete runtime snapshot metric set", values)
	}
	if units["runtime.memory.heap_alloc"] != "bytes" ||
		units["runtime.memory.heap_inuse"] != "bytes" ||
		units["runtime.memory.sys"] != "bytes" ||
		units["runtime.memory.num_gc"] != "count" ||
		units["runtime.memory.goroutines"] != "count" ||
		units["runtime.memory.process_commit"] != "bytes" ||
		units["runtime.memory.process_commit_available"] != "boolean" {
		t.Fatalf("runtime memory units = %#v, want bytes/count/boolean fields", units)
	}
	if heapAlloc <= 0 || heapInuse < heapAlloc || sys < heapInuse || numGC < 0 || goroutines <= 0 {
		t.Fatalf("runtime memory values = heap_alloc:%v heap_inuse:%v sys:%v num_gc:%v goroutines:%v, want plausible values", heapAlloc, heapInuse, sys, numGC, goroutines)
	}
	if processCommitAvailable != 0 && processCommitAvailable != 1 {
		t.Fatalf("runtime memory process commit availability = %v, want 0 or 1", processCommitAvailable)
	}
	if processCommitAvailable == 1 && processCommit <= 0 {
		t.Fatalf("runtime memory process commit = %v, want positive when available", processCommit)
	}
}

func assertCompleteRuntimeMemoryObservations(t *testing.T, names []string) {
	t.Helper()
	want := runtimeMemoryMetricNames()
	if len(names) == 0 || len(names)%len(want) != 0 {
		t.Fatalf("runtime memory observation count = %d, want a positive multiple of %d; names = %#v", len(names), len(want), names)
	}
	for start := 0; start < len(names); start += len(want) {
		seen := make(map[string]struct{}, len(want))
		for _, name := range names[start : start+len(want)] {
			if _, duplicate := seen[name]; duplicate {
				t.Fatalf("runtime memory observation contains duplicate %q: names = %#v", name, names[start:start+len(want)])
			}
			seen[name] = struct{}{}
		}
		for _, name := range want {
			if _, ok := seen[name]; !ok {
				t.Fatalf("runtime memory observation is incomplete: names = %#v", names[start:start+len(want)])
			}
		}
	}
}

func runtimeMemoryMetricNames() []string {
	return []string{
		"runtime.memory.heap_alloc",
		"runtime.memory.heap_inuse",
		"runtime.memory.sys",
		"runtime.memory.num_gc",
		"runtime.memory.goroutines",
		"runtime.memory.process_commit",
		"runtime.memory.process_commit_available",
	}
}

func isRuntimeMemoryMetric(name string) bool {
	switch name {
	case "runtime.memory.heap_alloc",
		"runtime.memory.heap_inuse",
		"runtime.memory.sys",
		"runtime.memory.num_gc",
		"runtime.memory.goroutines",
		"runtime.memory.process_commit",
		"runtime.memory.process_commit_available":
		return true
	default:
		return false
	}
}

// TestAPIServerStartsOnConfiguredListenerAndServesStatus proves the public API
// server becomes reachable on its configured loopback listener and serves a
// non-empty readiness status observation after start.
func TestAPIServerStartsOnConfiguredListenerAndServesStatus(t *testing.T) {
	configuredURL := reserveConfiguredLoopbackURL(t)
	dir := support.ScaffoldFactory(t, startupShutdownTestFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--server", configuredURL},
	})
	defer server.Stop(t)

	listenerURL := server.URL()
	if listenerURL == "" {
		t.Fatal("started API server returned empty listener URL")
	}
	if !strings.HasPrefix(listenerURL, "http://127.0.0.1:") &&
		!strings.HasPrefix(listenerURL, "http://localhost:") {
		t.Fatalf("listener URL = %q, want loopback HTTP URL", listenerURL)
	}

	response, err := http.Get(listenerURL + "/status")
	if err != nil {
		t.Fatalf("GET %s/status: %v", listenerURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s/status status = %d, want %d", listenerURL, response.StatusCode, http.StatusOK)
	}

	status := support.GetJSON[factoryapi.StatusResponse](t, listenerURL+"/status")
	if status.FactoryState == "" {
		t.Fatal("GET /status returned empty factoryState")
	}
	if status.RuntimeStatus == "" {
		t.Fatal("GET /status returned empty runtimeStatus")
	}
}

// TestAPIServerShutdownClosesListenerAndActiveStreams proves shutdown through the
// public API server lifecycle closes the listener and terminates active public streams.
func TestAPIServerShutdownClosesListenerAndActiveStreams(t *testing.T) {
	dir := scaffoldConcurrentRequestsFactory(t)
	blocking := newBlockingInvocationRunner()
	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(t, &edges, blocking, nil)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})

	listenerURL := server.URL()
	parsed, err := url.Parse(listenerURL)
	if err != nil || parsed.Host == "" {
		t.Fatalf("parse listener URL %q: %v", listenerURL, err)
	}

	session := getFactorySession(t, listenerURL, factorysessions.DefaultSessionID)
	activeRequest := startActiveBlockingInvocation(t, listenerURL, session.Id, blocking)

	statusResp, err := http.Get(listenerURL + "/status")
	if err != nil {
		t.Fatalf("GET %s/status before shutdown: %v", listenerURL, err)
	}
	_ = statusResp.Body.Close()
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf(
			"GET %s/status before shutdown status = %d, want %d",
			listenerURL,
			statusResp.StatusCode,
			http.StatusOK,
		)
	}

	stopDone := stopFunctionalAPIServerAsync(t, server)
	waitForListenerClosed(t, parsed.Host, listenerURL, 10*time.Second)
	assertActiveRequestTerminatedAfterShutdown(t, activeRequest)

	select {
	case <-stopDone:
	case <-time.After(10 * time.Second):
		activeRequest.cancel()
		t.Fatal("timed out waiting for API server shutdown to complete")
	}
}

// TestAPIServerBindFailureUnwindsStartedLifecycleRoles proves bind failure through
// the public API server lifecycle reports a documented failure and leaves no
// leaked listeners or readiness side effects.
func TestAPIServerBindFailureUnwindsStartedLifecycleRoles(t *testing.T) {
	dir := support.ScaffoldFactory(t, startupShutdownTestFactoryConfig())
	requestedURL := "http://127.0.0.1:65534"

	var attempts []string
	starter, err := platformhttpserver.NewStarter(func(_ string, address string) (net.Listener, error) {
		attempts = append(attempts, address)
		return nil, errors.New("address unavailable")
	}, nil, nil)
	if err != nil {
		t.Fatalf("NewStarter() error = %v", err)
	}

	var browserCalls, readinessCalls atomic.Int32
	edges := serviceedges.Edges{
		APIServerStarter: starter,
		BrowserOpener: func(context.Context, string) error {
			browserCalls.Add(1)
			return nil
		},
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
			readinessCalls.Add(1)
		},
	}
	process := support.BuildProcess(t, edges)

	home := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", requestedURL,
		"run", "--dir", dir,
		"--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)

	if err := process.Execute(inputs.Input); err == nil {
		t.Fatalf(
			"Process.Execute(run with bind failure) error = nil; stdout=%q stderr=%q",
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	for _, forbidden := range []string{"Factory initiated:", "Dashboard URL:"} {
		if strings.Contains(inputs.Stdout(), forbidden) {
			t.Fatalf("run stdout exposed readiness %q before bind failure:\n%s", forbidden, inputs.Stdout())
		}
	}

	stderr := inputs.Stderr()
	const legacyBindWarning = "warning: --server is deprecated for local listener binding; use --listen <host:port> instead\n"
	if !strings.HasPrefix(stderr, legacyBindWarning) {
		t.Fatalf("run stderr is missing the legacy listener migration warning:\n%s", stderr)
	}
	stderr = strings.TrimPrefix(stderr, legacyBindWarning)
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(stderr), &response); err != nil {
		t.Fatalf("run stderr is not exactly one ErrorResponse after the migration warning: %v\n%s", err, inputs.Stderr())
	}
	if response.Code != factoryapi.ErrorResponseCode("SERVER_BIND_FAILED") {
		t.Fatalf("ErrorResponse = %#v, want SERVER_BIND_FAILED", response)
	}
	if got, want := strings.Join(attempts, ","), "127.0.0.1:65534,127.0.0.1:65535"; got != want {
		t.Fatalf("listener attempts = %q, want %q", got, want)
	}
	if browserCalls.Load() != 0 || readinessCalls.Load() != 0 {
		t.Fatalf(
			"post-failure effects = browser:%d readiness:%d, want none",
			browserCalls.Load(),
			readinessCalls.Load(),
		)
	}

	parsed, err := url.Parse(requestedURL)
	if err != nil {
		t.Fatalf("parse requested listener URL %q: %v", requestedURL, err)
	}
	rebound, err := net.Listen("tcp4", parsed.Host)
	if err != nil {
		t.Fatalf("requested listener address %s remained unavailable after bind failure: %v", parsed.Host, err)
	}
	_ = rebound.Close()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	statusResponse, err := client.Get(requestedURL + "/status")
	if err == nil {
		_ = statusResponse.Body.Close()
		t.Fatalf("GET %s/status succeeded after bind failure", requestedURL)
	}
}

func stopFunctionalAPIServerAsync(t *testing.T, server *support.FunctionalAPIServer) <-chan struct{} {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Stop(t)
	}()
	return done
}

type activeBlockingInvocation struct {
	cancel context.CancelFunc
	done   chan error
}

func startActiveBlockingInvocation(
	t *testing.T,
	baseURL string,
	sessionID string,
	blocking *blockingInvocationRunner,
) *activeBlockingInvocation {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	request := &activeBlockingInvocation{
		cancel: cancel,
		done:   make(chan error, 1),
	}
	go func() {
		request.done <- postBlockingInvocation(ctx, baseURL, sessionID, blocking)
	}()

	select {
	case <-blocking.started:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("timed out waiting for active blocking invocation")
	}

	return request
}

func assertActiveRequestTerminatedAfterShutdown(t *testing.T, request *activeBlockingInvocation) {
	t.Helper()

	select {
	case err := <-request.done:
		if err == nil {
			t.Fatal("active blocking invocation completed without error after shutdown")
		}
		if !errors.Is(err, context.Canceled) && !isClosedConnectionError(err) {
			t.Fatalf("active blocking invocation after shutdown error = %v, want closed stream", err)
		}
	case <-time.After(5 * time.Second):
		request.cancel()
		t.Fatal("active blocking invocation remained open after shutdown")
	}
}

func waitForListenerClosed(t *testing.T, listenerHost, listenerURL string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		client := &http.Client{Timeout: 500 * time.Millisecond}
		response, err := client.Get(listenerURL + "/status")
		if err == nil {
			_ = response.Body.Close()
		} else {
			rebound, listenErr := net.Listen("tcp4", listenerHost)
			if listenErr == nil {
				_ = rebound.Close()
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("listener %s remained reachable after %s", listenerHost, timeout)
}

func isClosedConnectionError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "connection reset") ||
		strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "use of closed network connection") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "eof")
}

func reserveConfiguredLoopbackURL(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve configured loopback port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release configured loopback port: %v", err)
	}
	return "http://127.0.0.1:" + strconv.Itoa(port)
}

func startupShutdownTestFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}
