package httpserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/pprof/profile"
)

const (
	pprofTestRequestTimeout = 10 * time.Second
	pprofRealWaitMinimum    = 900 * time.Millisecond
	pprofRealWaitMaximum    = 5 * time.Second
)

func TestHandlerWithPprofIsOptInAndUsesStandardRoutes(t *testing.T) {
	const notFoundBody = "application route"
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(writer, notFoundBody)
	})
	commandLineReader := CommandLineReader(func() []string { return []string{"you", "test"} })

	disabled := httptest.NewServer(HandlerWithPprof(handler, false, commandLineReader))
	defer closePprofTestServer(disabled)
	assertDisabledPprofRoutes(t, disabled, notFoundBody)

	waitDurations := make([]time.Duration, 0, 3)
	controlledWaiter := func(ctx context.Context, duration time.Duration) error {
		if ctx == nil {
			return errors.New("controlled pprof waiter: context is required")
		}
		waitDurations = append(waitDurations, duration)
		return nil
	}
	enabled := httptest.NewServer(handlerWithPprof(handler, true, commandLineReader, controlledWaiter))
	defer closePprofTestServer(enabled)
	assertEnabledPprofRoutes(t, enabled)
	if want := []time.Duration{time.Second, time.Second, time.Second}; !reflect.DeepEqual(waitDurations, want) {
		t.Fatalf("controlled pprof wait durations = %v, want %v", waitDurations, want)
	}

	assertRealPprofRoute(t, handler, commandLineReader)
}

func assertDisabledPprofRoutes(t *testing.T, server *httptest.Server, wantBody string) {
	t.Helper()
	for _, path := range []string{
		"/debug/pprof/", "/debug/pprof/allocs", "/debug/pprof/block", "/debug/pprof/goroutine",
		"/debug/pprof/heap", "/debug/pprof/mutex", "/debug/pprof/threadcreate",
		"/debug/pprof/profile?seconds=1", "/debug/pprof/trace?seconds=1",
		"/debug/pprof/cmdline", "/debug/pprof/symbol",
	} {
		response := getPprofTestResponse(t, server.URL+path)
		if response.StatusCode != http.StatusNotFound || string(response.Body) != wantBody {
			t.Fatalf("disabled GET %s = (%d, %q), want application 404 body %q", path, response.StatusCode, response.Body, wantBody)
		}
	}
}

func assertEnabledPprofRoutes(t *testing.T, server *httptest.Server) {
	t.Helper()
	assertEnabledPprofIndexAndRoutes(t, server)
	assertEnabledPprofProfiles(t, server)
	assertEnabledPprofTrace(t, server)
}

func assertEnabledPprofIndexAndRoutes(t *testing.T, server *httptest.Server) {
	t.Helper()
	index := getPprofTestResponse(t, server.URL+"/debug/pprof/")
	if index.StatusCode != http.StatusOK {
		t.Fatalf("enabled pprof index status = %d, want %d", index.StatusCode, http.StatusOK)
	}
	assertPprofResponseHeaders(t, index, "text/html; charset=utf-8", "")
	indexBody := string(index.Body)
	for _, name := range []string{
		"allocs", "block", "cmdline", "goroutine", "heap", "mutex", "profile", "symbol", "threadcreate", "trace",
	} {
		link := fmt.Sprintf("<a href='%s?debug=1'>%s</a>", name, name)
		if !strings.Contains(indexBody, link) || !strings.Contains(indexBody, name+":") {
			t.Fatalf("enabled pprof index is missing route/description for %q: %q", name, indexBody)
		}
	}

	commandLine := getPprofTestResponse(t, server.URL+"/debug/pprof/cmdline")
	if commandLine.StatusCode != http.StatusOK || string(commandLine.Body) != "you\x00test" {
		t.Fatalf("enabled cmdline = (%d, %q), want injected command line", commandLine.StatusCode, commandLine.Body)
	}
	assertPprofResponseHeaders(t, commandLine, "text/plain; charset=utf-8", "")

	pc := reflect.ValueOf(pprofSymbol).Pointer()
	symbol := getPprofTestResponse(t, fmt.Sprintf("%s/debug/pprof/symbol?0x%x+0", server.URL, pc))
	if symbol.StatusCode != http.StatusOK || !strings.Contains(string(symbol.Body), "num_symbols: 1") {
		t.Fatalf("enabled symbol = (%d, %q), want symbol response", symbol.StatusCode, symbol.Body)
	}
	assertPprofResponseHeaders(t, symbol, "text/plain; charset=utf-8", "")
	if function := runtime.FuncForPC(pc); function != nil && !strings.Contains(string(symbol.Body), function.Name()) {
		t.Fatalf("enabled symbol body = %q, want function %q", symbol.Body, function.Name())
	}
}

func assertEnabledPprofProfiles(t *testing.T, server *httptest.Server) {
	t.Helper()
	assertEnabledPprofHeap(t, server)
	assertEnabledPprofNamedProfiles(t, server)
	assertEnabledPprofCPU(t, server)
	assertEnabledPprofHeapDelta(t, server)
}

func assertEnabledPprofHeap(t *testing.T, server *httptest.Server) {
	t.Helper()
	heap := getPprofTestResponse(t, server.URL+"/debug/pprof/heap")
	if heap.StatusCode != http.StatusOK || len(heap.Body) == 0 {
		t.Fatalf("enabled pprof heap = (%d, body length %d), want non-empty HTTP 200 response", heap.StatusCode, len(heap.Body))
	}
	assertPprofResponseHeaders(t, heap, "application/octet-stream", `attachment; filename="heap"`)
	heapProfile, err := profile.Parse(bytes.NewReader(heap.Body))
	if err != nil {
		t.Fatalf("parse enabled pprof heap profile: %v", err)
	}
	assertParsedPprofProfile(t, "heap", heapProfile)
}

func assertEnabledPprofNamedProfiles(t *testing.T, server *httptest.Server) {
	t.Helper()
	for _, test := range []struct {
		path          string
		name          string
		requireSample bool
	}{
		{path: "/debug/pprof/goroutine", name: "goroutine", requireSample: true},
		{path: "/debug/pprof/allocs", name: "allocs", requireSample: true},
		{path: "/debug/pprof/block", name: "block"},
		{path: "/debug/pprof/mutex", name: "mutex"},
		{path: "/debug/pprof/threadcreate", name: "threadcreate"},
	} {
		response := getPprofTestResponse(t, server.URL+test.path)
		if response.StatusCode != http.StatusOK || len(response.Body) == 0 {
			t.Fatalf("enabled GET %s = (%d, body length %d), want non-empty HTTP 200 response", test.path, response.StatusCode, len(response.Body))
		}
		assertPprofResponseHeaders(t, response, "application/octet-stream", fmt.Sprintf(`attachment; filename="%s"`, test.name))
		parsed, err := profile.Parse(bytes.NewReader(response.Body))
		if err != nil {
			t.Fatalf("parse enabled %s profile: %v", test.name, err)
		}
		assertPprofProfileSampleTypes(t, test.name, parsed)
		if test.requireSample && len(parsed.Sample) == 0 {
			t.Fatalf("%s profile has no samples", test.name)
		}
	}
}

func assertEnabledPprofCPU(t *testing.T, server *httptest.Server) {
	t.Helper()
	response := getPprofTestResponse(t, server.URL+"/debug/pprof/profile?seconds=1")
	if response.StatusCode != http.StatusOK || len(response.Body) == 0 {
		t.Fatalf("enabled CPU profile = (%d, body length %d), want non-empty HTTP 200 response", response.StatusCode, len(response.Body))
	}
	assertPprofResponseHeaders(t, response, "application/octet-stream", `attachment; filename="profile"`)
	parsed, err := profile.Parse(bytes.NewReader(response.Body))
	if err != nil {
		t.Fatalf("parse enabled CPU profile: %v", err)
	}
	assertPprofProfileSampleTypes(t, "CPU", parsed)
}

func assertEnabledPprofHeapDelta(t *testing.T, server *httptest.Server) {
	t.Helper()
	delta := getPprofTestResponse(t, server.URL+"/debug/pprof/heap?seconds=1")
	if delta.StatusCode != http.StatusOK || len(delta.Body) == 0 {
		t.Fatalf("enabled heap delta = (%d, body length %d), want non-empty HTTP 200 response", delta.StatusCode, len(delta.Body))
	}
	assertPprofResponseHeaders(t, delta, "application/octet-stream", `attachment; filename="heap-delta"`)
	parsedDelta, err := profile.Parse(bytes.NewReader(delta.Body))
	if err != nil {
		t.Fatalf("parse enabled heap delta profile: %v", err)
	}
	assertPprofProfileSampleTypes(t, "heap delta", parsedDelta)
	if parsedDelta.DurationNanos != int64(time.Second) {
		t.Fatalf("heap delta duration = %d ns, want %d ns", parsedDelta.DurationNanos, int64(time.Second))
	}
}

func assertEnabledPprofTrace(t *testing.T, server *httptest.Server) {
	t.Helper()
	response := getPprofTestResponse(t, server.URL+"/debug/pprof/trace?seconds=1")
	if response.StatusCode != http.StatusOK || len(response.Body) == 0 {
		t.Fatalf("enabled trace = (%d, body length %d), want non-empty HTTP 200 response", response.StatusCode, len(response.Body))
	}
	assertPprofResponseHeaders(t, response, "application/octet-stream", `attachment; filename="trace"`)
	if !bytes.HasPrefix(response.Body, []byte("go ")) {
		t.Fatalf("enabled trace prefix = %q, want Go trace framing", response.Body[:min(3, len(response.Body))])
	}
}

func assertRealPprofRoute(t *testing.T, handler http.Handler, commandLineReader CommandLineReader) {
	t.Helper()
	server := httptest.NewServer(HandlerWithPprof(handler, true, commandLineReader))
	defer closePprofTestServer(server)

	started := time.Now()
	response := getPprofTestResponse(t, server.URL+"/debug/pprof/profile?seconds=1")
	elapsed := time.Since(started)
	if elapsed < pprofRealWaitMinimum || elapsed > pprofRealWaitMaximum {
		t.Fatalf("real pprof CPU request elapsed = %s, want between %s and %s", elapsed, pprofRealWaitMinimum, pprofRealWaitMaximum)
	}
	if response.StatusCode != http.StatusOK || len(response.Body) == 0 {
		t.Fatalf("real pprof CPU response = (%d, body length %d), want non-empty HTTP 200 response", response.StatusCode, len(response.Body))
	}
	assertPprofResponseHeaders(t, response, "application/octet-stream", `attachment; filename="profile"`)
	parsed, err := profile.Parse(bytes.NewReader(response.Body))
	if err != nil {
		t.Fatalf("parse real pprof CPU profile: %v", err)
	}
	assertPprofProfileSampleTypes(t, "real CPU", parsed)
}

func TestHandlerWithPprofDoesNotPolluteDefaultServeMux(t *testing.T) {
	server := httptest.NewServer(http.DefaultServeMux)
	defer closePprofTestServer(server)

	response := getPprofTestResponse(t, server.URL+"/debug/pprof/heap")
	if response.StatusCode != http.StatusNotFound || string(response.Body) != "404 page not found\n" {
		t.Fatalf("default mux pprof = (%d, %q), want isolated standard 404", response.StatusCode, response.Body)
	}
	if response.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("default mux pprof Content-Type = %q, want text/plain; charset=utf-8", response.ContentType)
	}
}

func TestStarterWithListenerServesPprofOnlyWhenRequested(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	commandLineReader := CommandLineReader(func() []string { return []string{"you", "test"} })
	starter := StarterWithListener(listener, nil, commandLineReader)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	exit := make(chan error, 1)
	bound := make(chan struct{})
	finished := false
	defer func() {
		cancel()
		if finished {
			return
		}
		select {
		case err := <-exit:
			if err != nil {
				t.Errorf("starter cleanup: %v", err)
			}
		case <-time.After(pprofTestRequestTimeout):
			t.Errorf("starter cleanup timed out")
		}
	}()
	go func() {
		exit <- starter(ctx, StartRequest{
			Handler: http.NotFoundHandler(),
			Pprof:   true,
			OnBound: func(Binding) { close(bound) },
		})
	}()
	select {
	case <-bound:
	case err := <-exit:
		t.Fatalf("starter exited before binding: %v", err)
	case <-time.After(pprofTestRequestTimeout):
		t.Fatal("timed out waiting for starter binding")
	}

	response := getPprofTestResponse(t, "http://"+listener.Addr().String()+"/debug/pprof/heap")
	if response.StatusCode != http.StatusOK || len(response.Body) == 0 {
		t.Fatalf("starter pprof heap = (%d, %q), want non-empty HTTP 200 response", response.StatusCode, response.Body)
	}
	assertPprofResponseHeaders(t, response, "application/octet-stream", `attachment; filename="heap"`)
	parsed, err := profile.Parse(bytes.NewReader(response.Body))
	if err != nil {
		t.Fatalf("parse starter pprof heap: %v", err)
	}
	assertParsedPprofProfile(t, "starter heap", parsed)

	cancel()
	if err := <-exit; err != nil {
		t.Fatalf("starter cancellation: %v", err)
	}
	finished = true
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	shutdownResponse, shutdownErr := client.Get("http://" + listener.Addr().String() + "/debug/pprof/heap")
	if shutdownResponse != nil {
		_ = shutdownResponse.Body.Close()
	}
	if shutdownErr == nil {
		t.Fatal("pprof-enabled starter still served a profile after cancellation")
	}
}

type pprofTestResponse struct {
	StatusCode         int
	Body               []byte
	ContentType        string
	ContentDisposition string
	ContentTypeOptions string
}

func closePprofTestServer(server *httptest.Server) {
	server.CloseClientConnections()
	server.Close()
}

func getPprofTestResponse(t *testing.T, url string) pprofTestResponse {
	t.Helper()
	client := &http.Client{Timeout: pprofTestRequestTimeout}
	defer client.CloseIdleConnections()
	response, err := client.Get(url)
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
		}
		t.Fatalf("GET %s: %v", url, err)
	}
	body, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil {
		t.Fatalf("read GET %s: %v", url, readErr)
	}
	if closeErr != nil {
		t.Fatalf("close GET %s response body: %v", url, closeErr)
	}
	return pprofTestResponse{
		StatusCode:         response.StatusCode,
		Body:               body,
		ContentType:        response.Header.Get("Content-Type"),
		ContentDisposition: response.Header.Get("Content-Disposition"),
		ContentTypeOptions: response.Header.Get("X-Content-Type-Options"),
	}
}

func assertPprofResponseHeaders(t *testing.T, response pprofTestResponse, wantContentType, wantDisposition string) {
	t.Helper()
	if response.ContentType != wantContentType {
		t.Fatalf("pprof Content-Type = %q, want %q", response.ContentType, wantContentType)
	}
	if response.ContentDisposition != wantDisposition {
		t.Fatalf("pprof Content-Disposition = %q, want %q", response.ContentDisposition, wantDisposition)
	}
	if response.ContentTypeOptions != "nosniff" {
		t.Fatalf("pprof X-Content-Type-Options = %q, want nosniff", response.ContentTypeOptions)
	}
}

func assertPprofProfileSampleTypes(t *testing.T, name string, parsed *profile.Profile) {
	t.Helper()
	if parsed == nil {
		t.Fatalf("%s profile is nil", name)
	}
	if len(parsed.SampleType) == 0 {
		t.Fatalf("%s profile has no sample types", name)
	}
}

func assertParsedPprofProfile(t *testing.T, name string, parsed *profile.Profile) {
	t.Helper()
	assertPprofProfileSampleTypes(t, name, parsed)
	if len(parsed.Sample) == 0 {
		t.Fatalf("%s profile has no samples", name)
	}
}
