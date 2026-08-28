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
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/pprof/profile"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAPIServerPprofIsOptInThroughThePublicRunPath proves the diagnostics
// routes are absent by default and that --pprof exposes a live heap profile on
// the same loopback API server built through root.BuildProcess.
//
// This case is isolated because the diagnostics flag changes process startup
// configuration; the default and opt-in servers cannot share one invocation.
func TestAPIServerPprofIsOptInThroughThePublicRunPath(t *testing.T) {
	factory := scaffoldC06IsolatedFactory(t, startupShutdownTestFactoryConfig())
	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(t, &edges, support.NewStaticSuccessCommandRunner("pprof diagnostics"), nil)
	metricsDir := t.TempDir()
	defaultServer := startC06IsolatedHTTPServer(
		t,
		"c06-pprof-disabled",
		"diagnostics mode changes process startup configuration",
		factory.factoryDir,
		[]string{"--runtime-metrics-dir", metricsDir},
		edges,
	)
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
	assertRuntimeDiagnosticsRejectsInvalidMethod(t, defaultServer.URL())
	assertApplicationStatusRemainsAvailable(t, defaultServer.URL())
	defaultServer.stop(t)
	assertRuntimeMemoryMetrics(t, metricsDir)

	enabledServer := startC06IsolatedHTTPServer(
		t,
		"c06-pprof-enabled",
		"diagnostics opt-in requires a separate process startup mode",
		factory.factoryDir,
		[]string{"--pprof"},
		edges,
	)
	assertEnabledPprofServer(t, enabledServer.URL())
	enabledServer.stop(t)
}

func assertEnabledPprofServer(t *testing.T, baseURL string) {
	t.Helper()
	assertPprofIndex(t, baseURL)
	assertPprofHeap(t, baseURL)
	assertPprofNamedProfiles(t, baseURL)
	assertPprofCPU(t, baseURL)
	assertPprofHeapDelta(t, baseURL)
	assertPprofTrace(t, baseURL)
	assertPprofTextEndpoints(t, baseURL)
	assertPprofUnknownProfile(t, baseURL)
	assertPprofInvalidQuery(t, baseURL)
}

func getPprofResponse(t *testing.T, baseURL, path string) (*http.Response, []byte) {
	t.Helper()
	response, err := http.Get(baseURL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read GET %s: %v", path, err)
	}
	return response, body
}

func assertPprofIndex(t *testing.T, baseURL string) {
	t.Helper()
	response, body := getPprofResponse(t, baseURL, "/debug/pprof/")
	if response.StatusCode != http.StatusOK ||
		response.Header.Get("Content-Type") != "text/html; charset=utf-8" ||
		response.Header.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(string(body), "heap") {
		t.Fatalf("pprof index = (%d, %q), want HTTP 200 with heap profile", response.StatusCode, body)
	}
}

func assertPprofHeap(t *testing.T, baseURL string) {
	t.Helper()
	response, body := getPprofResponse(t, baseURL, "/debug/pprof/heap")
	if response.StatusCode != http.StatusOK ||
		response.Header.Get("Content-Type") != "application/octet-stream" ||
		response.Header.Get("Content-Disposition") != `attachment; filename="heap"` ||
		response.Header.Get("X-Content-Type-Options") != "nosniff" || len(body) == 0 {
		t.Fatalf("pprof heap = (%d, body length %d), want non-empty HTTP 200 response", response.StatusCode, len(body))
	}
	heapProfile, err := profile.Parse(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("parse enabled live heap profile: %v", err)
	}
	if len(heapProfile.SampleType) == 0 || len(heapProfile.Sample) == 0 {
		t.Fatalf("enabled live heap profile = %+v, want sample types and samples", heapProfile)
	}
}

func assertPprofNamedProfiles(t *testing.T, baseURL string) {
	t.Helper()
	for _, path := range []string{"/debug/pprof/allocs", "/debug/pprof/goroutine"} {
		response, body := getPprofResponse(t, baseURL, path)
		if response.StatusCode != http.StatusOK || len(body) == 0 {
			t.Fatalf("GET %s = (%d, body length %d), want non-empty HTTP 200 response", path, response.StatusCode, len(body))
		}
	}
}

func assertPprofCPU(t *testing.T, baseURL string) {
	t.Helper()
	response, body := getPprofResponse(t, baseURL, "/debug/pprof/profile?seconds=1")
	if response.StatusCode != http.StatusOK || len(body) == 0 {
		t.Fatalf("CPU profile = (%d, body length %d), want non-empty HTTP 200 response", response.StatusCode, len(body))
	}
	cpuProfile, err := profile.Parse(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("parse enabled live CPU profile: %v", err)
	}
	if len(cpuProfile.SampleType) == 0 {
		t.Fatalf("enabled live CPU profile = %+v, want sample types", cpuProfile)
	}
}

func assertPprofHeapDelta(t *testing.T, baseURL string) {
	t.Helper()
	response, body := getPprofResponse(t, baseURL, "/debug/pprof/heap?seconds=1")
	if response.StatusCode != http.StatusOK ||
		response.Header.Get("Content-Type") != "application/octet-stream" ||
		!strings.Contains(response.Header.Get("Content-Disposition"), `heap-delta`) || len(body) == 0 {
		t.Fatalf("heap delta profile = (%d, headers=%v, body length=%d), want non-empty HTTP 200 profile", response.StatusCode, response.Header, len(body))
	}
	if parsed, err := profile.Parse(bytes.NewReader(body)); err != nil || parsed == nil || len(parsed.SampleType) == 0 {
		t.Fatalf("parse heap delta profile = (%v, %+v), want a valid profile with sample types", err, parsed)
	}
}

func assertPprofTrace(t *testing.T, baseURL string) {
	t.Helper()
	response, body := getPprofResponse(t, baseURL, "/debug/pprof/trace?seconds=0.01")
	if response.StatusCode != http.StatusOK ||
		response.Header.Get("Content-Type") != "application/octet-stream" ||
		response.Header.Get("Content-Disposition") != `attachment; filename="trace"` || len(body) == 0 {
		t.Fatalf("trace profile = (%d, headers=%v, body length=%d), want non-empty HTTP 200 profile", response.StatusCode, response.Header, len(body))
	}
}

func assertPprofTextEndpoints(t *testing.T, baseURL string) {
	t.Helper()
	for _, path := range []string{"/debug/pprof/cmdline", "/debug/pprof/symbol"} {
		response, body := getPprofResponse(t, baseURL, path)
		if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Fatalf("GET %s = (%d, headers=%v, body=%q), want text HTTP 200 response", path, response.StatusCode, response.Header, body)
		}
		if path == "/debug/pprof/symbol" && !strings.Contains(string(body), "num_symbols: 1") {
			t.Fatalf("GET %s body = %q, want symbol count", path, body)
		}
	}
}

func assertPprofUnknownProfile(t *testing.T, baseURL string) {
	t.Helper()
	response, body := getPprofResponse(t, baseURL, "/debug/pprof/not-a-profile")
	if response.StatusCode != http.StatusNotFound || response.Header.Get("X-Go-Pprof") != "1" ||
		!strings.Contains(string(body), "Unknown profile") {
		t.Fatalf("unknown pprof profile = (%d, headers=%v, body=%q), want HTTP 404 diagnostic", response.StatusCode, response.Header, body)
	}
}

func assertPprofInvalidQuery(t *testing.T, baseURL string) {
	t.Helper()
	response, body := getPprofResponse(t, baseURL, "/debug/pprof/heap?seconds=not-a-duration")
	if response.StatusCode != http.StatusBadRequest ||
		response.Header.Get("Content-Type") != "text/plain; charset=utf-8" ||
		response.Header.Get("X-Go-Pprof") != "1" ||
		!strings.Contains(string(body), `invalid value for "seconds"`) {
		t.Fatalf(
			"invalid heap profile query = (%d, headers=%v, body=%q), want actionable HTTP 400 pprof error",
			response.StatusCode,
			response.Header,
			body,
		)
	}
}

func assertRuntimeDiagnosticsRejectsInvalidMethod(t *testing.T, baseURL string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, baseURL+"/debug/runtime", nil)
	if err != nil {
		t.Fatalf("construct invalid runtime diagnostics request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST /debug/runtime: %v", err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read POST /debug/runtime response: %v", err)
	}
	if response.StatusCode != http.StatusMethodNotAllowed ||
		response.Header.Get("Allow") != http.MethodGet || len(body) != 0 {
		t.Fatalf(
			"POST /debug/runtime = (%d, headers=%v, body=%q), want empty HTTP 405 with Allow: GET",
			response.StatusCode,
			response.Header,
			body,
		)
	}
}

func assertApplicationStatusRemainsAvailable(t *testing.T, baseURL string) {
	t.Helper()
	response, err := http.Get(baseURL + "/status")
	if err != nil {
		t.Fatalf("GET /status after invalid diagnostics: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /status after invalid diagnostics status = %d, want %d", response.StatusCode, http.StatusOK)
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
	// The configured-listener argument is a startup/configuration witness; keep
	// it isolated from the package fixture so its listener selection is observed
	// on a process with no prior server lifecycle.
	configuredURL := reserveConfiguredLoopbackURL(t)
	factory := scaffoldC06IsolatedFactory(t, startupShutdownTestFactoryConfig())
	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(t, &edges, support.NewStaticSuccessCommandRunner("configured listener"), nil)
	server := startC06IsolatedHTTPServer(
		t,
		"c06-configured-listener",
		"configured listener selection is a process-startup property",
		factory.factoryDir,
		[]string{"--server", configuredURL},
		edges,
	)

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
	server.stop(t)
}

// TestAPIServerUsesPlatformStarterThroughRootProcess proves the customer run
// path can use the real loopback listener and Serve lifecycle through the
// replaceable API-server edge, rather than only an httptest transport.
func TestAPIServerUsesPlatformStarterThroughRootProcess(t *testing.T) {
	// The real platform starter owns an OS listener and its Serve/join path;
	// retain this process-isolated witness instead of sharing an httptest host.
	factory := scaffoldC06IsolatedFactory(t, startupShutdownTestFactoryConfig())
	server := startProductionLoopbackServerWithID(
		t,
		factory.factoryDir,
		true,
		"c06-platform-starter",
		"the production starter owns a real OS listener and Serve lifecycle",
	)
	assertLiveStatusAndRuntimeDiagnostics(t, server.url)
	assertLivePprofHeap(t, server.url)
	server.close(t)
}

// TestAPIServerShutdownClosesListenerAndActiveStreams proves shutdown through the
// public API server lifecycle closes the listener and terminates active public streams.
func TestAPIServerShutdownClosesListenerAndActiveStreams(t *testing.T) {
	// Shutdown is destructive and must terminate active streams and its listener;
	// it therefore cannot run on the package-owned shared process.
	factory := scaffoldC06IsolatedFactory(t, concurrentRequestsTestConfig())
	dir := factory.factoryDir
	blocking := newBlockingInvocationRunner()
	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(t, &edges, blocking, nil)
	server := startC06IsolatedHTTPServer(
		t,
		"c06-active-stream-shutdown",
		"shutdown closes active streams and terminates the owning HTTP process",
		dir,
		nil,
		edges,
	)

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

	server.stop(t)
	select {
	case <-server.Done():
	default:
		t.Fatal("Process.Execute remained active after API server Stop returned")
	}
	assertActiveRequestTerminatedAfterShutdown(t, activeRequest)
	assertListenerRefused(t, parsed.Host, listenerURL)
}

// TestAPIServerBindFailureUnwindsStartedLifecycleRoles proves bind failure through
// the public API server lifecycle reports a documented failure and leaves no
// leaked listeners or readiness side effects.
func TestAPIServerBindFailureUnwindsStartedLifecycleRoles(t *testing.T) {
	// Bind failure is isolated because it exercises the starter's rejected-port
	// fallback and process unwind rather than a serving listener.
	factory := scaffoldC06IsolatedFactory(t, startupShutdownTestFactoryConfig())
	dir := factory.factoryDir
	requestedURL := "http://127.0.0.1:65534"

	var attempts []string
	const lifecycleID = "c06-bind-failure"
	starter, err := platformhttpserver.NewStarter(func(_ string, address string) (net.Listener, error) {
		c06IsolatedLifecycle.rejectedBind(lifecycleID)
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
	edges.APIServerStarter = func(ctx context.Context, request platformhttpserver.StartRequest) error {
		c06IsolatedLifecycle.processHosted(lifecycleID)
		return starter(ctx, request)
	}
	process := c06BuildIsolatedProcess(t, lifecycleID, "bind failure exercises rejected ports and process unwind", c06IsolationExpectation{
		portRelease:   true,
		rejectedBinds: 2,
	}, edges)

	home := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", requestedURL,
		"run", "--dir", dir,
		"--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)

	executeErr := process.Execute(inputs.Input)
	process.markJoined()
	process.close(t)
	if executeErr == nil {
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
	if err := rebindC06Listener(parsed.Host); err != nil {
		t.Fatalf("requested listener address %s remained unavailable after bind failure: %v", parsed.Host, err)
	}
	c06IsolatedLifecycle.portReleased(lifecycleID)

	client := &http.Client{Timeout: 500 * time.Millisecond}
	statusResponse, err := client.Get(requestedURL + "/status")
	if err == nil {
		_ = statusResponse.Body.Close()
		t.Fatalf("GET %s/status succeeded after bind failure", requestedURL)
	}
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

func assertListenerRefused(t *testing.T, listenerHost, listenerURL string) {
	t.Helper()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	response, err := client.Get(listenerURL + "/status")
	if err == nil {
		_ = response.Body.Close()
		t.Fatalf("GET %s/status succeeded after Process.Execute joined", listenerURL)
	}
	rebound, listenErr := net.Listen("tcp4", listenerHost)
	if listenErr != nil {
		t.Fatalf("listener %s could not be rebound after Process.Execute joined: %v", listenerHost, listenErr)
	}
	_ = rebound.Close()
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
