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

func TestSessionListMapsStableInputsIntoFreshTypedRequestsAcrossExecutions(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"sessions":[]}`)
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

	first := execute("session", "list", "--port", strconv.Itoa(port), "--json")
	second := execute("session", "list", "--server", server.URL, "--json")

	if first != "{\"sessions\":[]}\n" || second != "{\"sessions\":[]}\n" {
		t.Fatalf("session list outputs = (%q, %q), want stable JSON", first, second)
	}
	if len(requests) != 2 || requests[0] != "/factory-sessions" || requests[1] != "/factory-sessions" {
		t.Fatalf("session list requests = %v, want two collection requests", requests)
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
