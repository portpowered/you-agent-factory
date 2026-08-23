package httpserver

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandlerWithPprofIsOptInAndUsesStandardRoutes(t *testing.T) {
	const notFoundBody = "application route"
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(writer, notFoundBody)
	})

	disabled := httptest.NewServer(HandlerWithPprof(handler, false))
	defer disabled.Close()
	for _, path := range []string{
		"/debug/pprof/", "/debug/pprof/heap", "/debug/pprof/profile",
		"/debug/pprof/trace", "/debug/pprof/goroutine", "/debug/pprof/cmdline",
		"/debug/pprof/symbol",
	} {
		response := getPprofTestResponse(t, disabled.URL+path)
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("disabled GET %s status = %d, want %d", path, response.StatusCode, http.StatusNotFound)
		}
	}

	enabled := httptest.NewServer(HandlerWithPprof(handler, true))
	defer enabled.Close()
	index := getPprofTestResponse(t, enabled.URL+"/debug/pprof/")
	if index.StatusCode != http.StatusOK || !strings.Contains(index.Body, "heap") {
		t.Fatalf("enabled pprof index = (%d, %q), want HTTP 200 with heap profile", index.StatusCode, index.Body)
	}

	for _, path := range []string{
		"/debug/pprof/heap", "/debug/pprof/goroutine", "/debug/pprof/cmdline",
		"/debug/pprof/symbol",
	} {
		response := getPprofTestResponse(t, enabled.URL+path)
		if response.StatusCode != http.StatusOK || response.Body == "" {
			t.Fatalf("enabled GET %s = (%d, %q), want non-empty HTTP 200 response", path, response.StatusCode, response.Body)
		}
	}

	for _, path := range []string{"/debug/pprof/profile?seconds=1", "/debug/pprof/trace?seconds=1"} {
		response := getPprofTestResponse(t, enabled.URL+path)
		if response.StatusCode != http.StatusOK || response.Body == "" {
			t.Fatalf("enabled GET %s = (%d, body length %d), want non-empty HTTP 200 response", path, response.StatusCode, len(response.Body))
		}
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
	if response.StatusCode != http.StatusOK || response.Body == "" {
		t.Fatalf("starter pprof heap = (%d, %q), want non-empty HTTP 200 response", response.StatusCode, response.Body)
	}
	cancel()
	if err := <-exit; err != nil {
		t.Fatalf("starter cancellation: %v", err)
	}
}

func getPprofTestResponse(t *testing.T, url string) struct {
	StatusCode int
	Body       string
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
		Body       string
	}{StatusCode: response.StatusCode, Body: string(body)}
}
