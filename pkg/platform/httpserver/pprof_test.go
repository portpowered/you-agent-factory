package httpserver

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/pprof/profile"
)

func TestHandlerWithPprofIsOptInAndUsesStandardRoutes(t *testing.T) {
	const notFoundBody = "application route"
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(writer, notFoundBody)
	})

	disabled := httptest.NewServer(HandlerWithPprof(handler, false))
	defer disabled.Close()
	assertDisabledPprofRoutes(t, disabled)

	enabled := httptest.NewServer(HandlerWithPprof(handler, true))
	defer enabled.Close()
	assertEnabledPprofRoutes(t, enabled)
}

func assertDisabledPprofRoutes(t *testing.T, server *httptest.Server) {
	t.Helper()
	for _, path := range []string{
		"/debug/pprof/", "/debug/pprof/heap", "/debug/pprof/allocs", "/debug/pprof/profile",
		"/debug/pprof/trace", "/debug/pprof/goroutine", "/debug/pprof/cmdline",
		"/debug/pprof/symbol",
	} {
		response := getPprofTestResponse(t, server.URL+path)
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("disabled GET %s status = %d, want %d", path, response.StatusCode, http.StatusNotFound)
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
	if index.StatusCode != http.StatusOK || !strings.Contains(string(index.Body), "heap") {
		t.Fatalf("enabled pprof index = (%d, %q), want HTTP 200 with heap profile", index.StatusCode, index.Body)
	}

	for _, path := range []string{
		"/debug/pprof/allocs", "/debug/pprof/goroutine", "/debug/pprof/cmdline",
		"/debug/pprof/symbol",
	} {
		response := getPprofTestResponse(t, server.URL+path)
		if response.StatusCode != http.StatusOK || len(response.Body) == 0 {
			t.Fatalf("enabled GET %s = (%d, %q), want non-empty HTTP 200 response", path, response.StatusCode, response.Body)
		}
	}
}

func assertEnabledPprofProfiles(t *testing.T, server *httptest.Server) {
	t.Helper()
	heap := getPprofTestResponse(t, server.URL+"/debug/pprof/heap")
	if heap.StatusCode != http.StatusOK || len(heap.Body) == 0 {
		t.Fatalf("enabled pprof heap = (%d, body length %d), want non-empty HTTP 200 response", heap.StatusCode, len(heap.Body))
	}
	heapProfile, err := profile.Parse(bytes.NewReader(heap.Body))
	if err != nil {
		t.Fatalf("parse enabled pprof heap profile: %v", err)
	}
	assertParsedPprofProfile(t, "heap", heapProfile)

	for _, test := range []struct {
		path string
		name string
	}{
		{path: "/debug/pprof/goroutine", name: "goroutine"},
		{path: "/debug/pprof/allocs", name: "allocs"},
		{path: "/debug/pprof/profile?seconds=1", name: "CPU"},
	} {
		response := getPprofTestResponse(t, server.URL+test.path)
		if response.StatusCode != http.StatusOK || len(response.Body) == 0 {
			t.Fatalf("enabled GET %s = (%d, body length %d), want non-empty HTTP 200 response", test.path, response.StatusCode, len(response.Body))
		}
		parsed, err := profile.Parse(bytes.NewReader(response.Body))
		if err != nil {
			t.Fatalf("parse enabled %s profile: %v", test.name, err)
		}
		if test.name == "CPU" {
			if len(parsed.SampleType) == 0 {
				t.Fatalf("%s profile has no sample types", test.name)
			}
		} else {
			assertParsedPprofProfile(t, test.name, parsed)
		}
	}
}

func assertEnabledPprofTrace(t *testing.T, server *httptest.Server) {
	t.Helper()
	for _, path := range []string{"/debug/pprof/trace?seconds=1"} {
		response := getPprofTestResponse(t, server.URL+path)
		if response.StatusCode != http.StatusOK || len(response.Body) == 0 {
			t.Fatalf("enabled GET %s = (%d, body length %d), want non-empty HTTP 200 response", path, response.StatusCode, len(response.Body))
		}
	}
}

func TestHandlerWithPprofDoesNotPolluteDefaultServeMux(t *testing.T) {
	server := httptest.NewServer(http.DefaultServeMux)
	defer server.Close()

	response := getPprofTestResponse(t, server.URL+"/debug/pprof/heap")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("default mux pprof status = %d, want %d", response.StatusCode, http.StatusNotFound)
	}
}

func TestStarterWithListenerServesPprofOnlyWhenRequested(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	starter := StarterWithListener(listener)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	exit := make(chan error, 1)
	bound := make(chan struct{})
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
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for starter binding")
	}

	response := getPprofTestResponse(t, "http://"+listener.Addr().String()+"/debug/pprof/heap")
	if response.StatusCode != http.StatusOK || len(response.Body) == 0 {
		t.Fatalf("starter pprof heap = (%d, %q), want non-empty HTTP 200 response", response.StatusCode, response.Body)
	}
	cancel()
	if err := <-exit; err != nil {
		t.Fatalf("starter cancellation: %v", err)
	}
	client := &http.Client{Timeout: time.Second}
	shutdownResponse, shutdownErr := client.Get("http://" + listener.Addr().String() + "/debug/pprof/heap")
	if shutdownErr == nil {
		_ = shutdownResponse.Body.Close()
		t.Fatal("pprof-enabled starter still served a profile after cancellation")
	}
}

func getPprofTestResponse(t *testing.T, url string) struct {
	StatusCode int
	Body       []byte
} {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read GET %s: %v", url, err)
	}
	return struct {
		StatusCode int
		Body       []byte
	}{StatusCode: response.StatusCode, Body: body}
}

func assertParsedPprofProfile(t *testing.T, name string, parsed *profile.Profile) {
	t.Helper()
	if parsed == nil {
		t.Fatalf("%s profile is nil", name)
	}
	if len(parsed.SampleType) == 0 {
		t.Fatalf("%s profile has no sample types", name)
	}
	if len(parsed.Sample) == 0 {
		t.Fatalf("%s profile has no samples", name)
	}
}
