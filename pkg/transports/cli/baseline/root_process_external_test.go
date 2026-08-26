package baseline_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
)

func TestBuildProcessComposesTypedSessionAndModelsAdaptersAcrossExecutions(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/factory-sessions":
			_, _ = io.WriteString(w, `{"sessions":[]}`)
		case "/models":
			_, _ = io.WriteString(w, `{"results":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	_, portText, found := strings.Cut(endpoint.Host, ":")
	if !found {
		t.Fatalf("test server host %q has no port", endpoint.Host)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse test server port: %v", err)
	}

	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	home := t.TempDir()
	execute := func(args ...string) string {
		t.Helper()
		var stdout bytes.Buffer
		err := process.Execute(root.Input{
			Args:             append([]string{"you"}, args...),
			Env:              append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
			WorkingDirectory: home, Stdout: &stdout, Stderr: io.Discard,
		})
		if err != nil {
			t.Fatalf("Process.Execute(%v): %v", args, err)
		}
		return stdout.String()
	}

	firstSession := execute("session", "list", "--scope", "live", "--port", strconv.Itoa(port), "--json")
	secondSession := execute("session", "list", "--scope", "live", "--server", server.URL, "--json")
	firstModels := execute("--server", server.URL, "models", "list", "--json")
	secondModels := execute("models", "list", "--server", server.URL, "--json")

	if firstSession != "{\"sessions\":[]}\n" || secondSession != "{\"sessions\":[]}\n" {
		t.Fatalf("session list outputs = (%q, %q), want stable JSON", firstSession, secondSession)
	}
	if firstModels != "{\"results\":[]}\n" || secondModels != "{\"results\":[]}\n" {
		t.Fatalf("models list outputs = (%q, %q), want stable JSON", firstModels, secondModels)
	}
	wantRequests := []string{"/factory-sessions?scope=live", "/factory-sessions?scope=live", "/models", "/models"}
	if strings.Join(requests, ",") != strings.Join(wantRequests, ",") {
		t.Fatalf("typed adapter requests = %v, want %v", requests, wantRequests)
	}
}

func TestWorkListMapsStableInputsIntoFreshTypedRequestsAcrossExecutions(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[]}`)
	}))
	t.Cleanup(server.Close)

	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	home := t.TempDir()
	execute := func(args ...string) {
		t.Helper()
		err := process.Execute(root.Input{
			Args:             append([]string{"you"}, args...),
			Env:              append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
			WorkingDirectory: home, Stdout: io.Discard, Stderr: io.Discard,
		})
		if err != nil {
			t.Fatalf("Process.Execute(%v): %v", args, err)
		}
	}

	execute("work", "list", "--server", server.URL, "--name", "first", "--max-results", "7")
	execute("work", "list", "--server", server.URL)

	if len(requests) != 2 {
		t.Fatalf("work list request count = %d, want 2 (%v)", len(requests), requests)
	}
	first, err := url.ParseRequestURI(requests[0])
	if err != nil {
		t.Fatalf("parse first request: %v", err)
	}
	second, err := url.ParseRequestURI(requests[1])
	if err != nil {
		t.Fatalf("parse second request: %v", err)
	}
	if got := first.Query().Get("name"); got != "first" {
		t.Fatalf("first request name = %q, want first", got)
	}
	if got := first.Query().Get("maxResults"); got != "7" {
		t.Fatalf("first request maxResults = %q, want 7", got)
	}
	if second.RawQuery != "" {
		t.Fatalf("second request query = %q, want omitted defaults", second.RawQuery)
	}
}

func productionCLIObservation(t testing.TB, arguments ...string) (cliobservation.Result, error) {
	t.Helper()
	var result cliobservation.Result
	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{CLIObserver: cliobservation.Capture(&result)})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	home := t.TempDir()
	err = process.Execute(root.Input{
		Args: append([]string{"you"}, arguments...), Env: append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		WorkingDirectory: home, Stdout: io.Discard, Stderr: io.Discard,
	})
	return result, err
}

func executeProductionCLI(t testing.TB, arguments ...string) (string, error) {
	t.Helper()
	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	home := t.TempDir()
	var stdout bytes.Buffer
	err = process.Execute(root.Input{
		Args: append([]string{"you"}, arguments...), Env: append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		WorkingDirectory: home, Stdout: &stdout, Stderr: io.Discard,
	})
	return stdout.String(), err
}
