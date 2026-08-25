package diagnostics_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const serverStopFunctionalObservationTimeout = 2 * time.Second

func TestServerStopStopsComposedLoopbackServerAndRendersSuccess(t *testing.T) {
	for _, test := range []struct {
		name       string
		jsonOutput bool
	}{{name: "human"}, {name: "json", jsonOutput: true}} {
		t.Run(test.name, func(t *testing.T) {
			server := startObservedLoopbackServer(t)
			inputs, err := executeServerStop(t, server.process, context.Background(), server.url, test.jsonOutput)
			if err != nil {
				t.Fatalf("Process.Execute(server stop) error = %v\nstdout=%q\nstderr=%q", err, inputs.Stdout(), inputs.Stderr())
			}
			if inputs.Stderr() != "" {
				t.Fatalf("successful server stop stderr = %q, want empty", inputs.Stderr())
			}

			calls := server.shutdownRequests.Calls()
			if len(calls) != 1 {
				t.Fatalf("shutdown HTTP calls = %#v, want exactly one typed request", calls)
			}
			if calls[0].method != http.MethodPost || calls[0].path != "/shutdown" {
				t.Fatalf("shutdown HTTP call = %#v, want POST /shutdown", calls[0])
			}
			assertLoopbackListenerClosed(t, server.url)
			waitForProcessDone(t, server.daemon)

			if test.jsonOutput {
				var result struct {
					Server string `json:"server"`
					Status string `json:"status"`
				}
				if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &result); err != nil {
					t.Fatalf("decode JSON server-stop success: %v\nstdout=%q", err, inputs.Stdout())
				}
				if result.Server != server.url || result.Status != "stopped" {
					t.Fatalf("JSON server-stop result = %#v, want server=%q status=stopped", result, server.url)
				}
				return
			}
			if got, want := inputs.Stdout(), fmt.Sprintf("Server stopped: %s\n", server.url); got != want {
				t.Fatalf("human server-stop stdout = %q, want %q", got, want)
			}
		})
	}
}

func TestServerStopComposedFailureDiagnostics(t *testing.T) {
	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)

	t.Run("non-loopback target is rejected before request", func(t *testing.T) {
		inputs, err := executeServerStop(t, process, context.Background(), "http://203.0.113.10:7437", true)
		assertServerStopDiagnostic(t, inputs, err, "SERVER_STOP_INVALID_TARGET", factoryapi.ErrorFamilyBadRequest, "not a local bind target")
	})

	t.Run("unreachable loopback endpoint", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		endpoint := server.URL
		server.Close()

		inputs, err := executeServerStop(t, process, context.Background(), endpoint, true)
		assertServerStopDiagnostic(t, inputs, err, "SERVER_STOP_UNREACHABLE", factoryapi.ErrorFamilyInternalServerError, "cannot reach Factory API")
	})

	t.Run("server-declared rejection", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(writer).Encode(factoryapi.ErrorResponse{
				Code:    factoryapi.ErrorResponseCode("SHUTDOWN_CONTROL_REJECTED"),
				Family:  factoryapi.ErrorFamilyBadRequest,
				Message: "shutdown control requires a loopback peer",
			})
		}))
		t.Cleanup(server.Close)

		inputs, err := executeServerStop(t, process, context.Background(), server.URL, true)
		assertServerStopDiagnostic(t, inputs, err, "SHUTDOWN_CONTROL_REJECTED", factoryapi.ErrorFamilyBadRequest, "requires a loopback peer")
	})

	t.Run("listener remains open until observation deadline", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Method != http.MethodPost || request.URL.Path != "/shutdown" {
				http.NotFound(writer, request)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(writer).Encode(factoryapi.ShutdownAcceptedResponse{
				Status:  factoryapi.Accepted,
				Message: "graceful shutdown accepted",
			})
		}))
		t.Cleanup(server.Close)

		// The live HTTP boundary must exercise the production observer's
		// observation-timeout classification when the accepted server stays
		// open. This caller deadline is the bounded negative-case contract
		// observation; an injected observer or manual cancellation would test a
		// different error path and would not prove the composed behavior.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		inputs, err := executeServerStop(t, process, ctx, server.URL, true)
		assertServerStopDiagnostic(t, inputs, err, "SERVER_STOP_OBSERVATION_TIMEOUT", factoryapi.ErrorFamilyInternalServerError, "did not stop")
		if inputs.Stdout() != "" {
			t.Fatalf("listener-open server-stop stdout = %q, want empty", inputs.Stdout())
		}
	})
}

func TestServerStopCanceledRequestPreservesProcessCancellationContract(t *testing.T) {
	requestStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		<-request.Context().Done()
	}))
	t.Cleanup(server.Close)

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inputs := support.FakeInputs(ctx, []string{"you", "--json", "--server", server.URL, "server", "stop"})

	result := make(chan error, 1)
	go func() { result <- process.Execute(inputs.Input) }()
	// requestStarted is the deterministic signal that the composed command
	// reached the HTTP boundary. The timer is only a hang guard for a
	// regression that prevents request dispatch; an injected edge would skip
	// the live transport boundary this cancellation scenario is meant to prove.
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("canceled server-stop request did not reach the HTTP boundary")
	}
	cancel()

	err := <-result
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled server-stop error = %v, want context.Canceled", err)
	}
	if inputs.Stdout() != "" {
		t.Fatalf("canceled server-stop stdout = %q, want empty", inputs.Stdout())
	}
	if got, want := inputs.Stderr(), "Error: context canceled\n"; got != want {
		t.Fatalf("canceled server-stop stderr = %q, want existing process-control diagnostic %q", got, want)
	}
}

func startObservedLoopbackServer(t *testing.T) *composedLoopbackServer {
	t.Helper()

	api := support.NewProcessAPIServer()
	requests := &shutdownRequestProbe{}
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			handler := request.Handler
			request.Handler = http.HandlerFunc(func(writer http.ResponseWriter, incoming *http.Request) {
				if incoming.Method == http.MethodPost && incoming.URL.Path == "/shutdown" {
					requests.Record(incoming)
				}
				handler.ServeHTTP(writer, incoming)
			})
			return api.Start(ctx, request)
		},
	})
	support.CleanupProcess(t, process)

	factoryDir := support.ScaffoldSingleStepFactory(t, "server-stop-functional")
	homeDir := t.TempDir()
	daemonInputs := support.FakeInputs(context.Background(), []string{
		"you", "run",
		"--dir", factoryDir,
		"--continuously",
		"--with-server",
		"--quiet",
		"--no-record",
	})
	daemonInputs.Input.Env = serverStopEnvironment(homeDir)
	daemonInputs.Input.WorkingDirectory = factoryDir
	daemon := support.StartProcessCommand(t, process, daemonInputs.Input)

	return &composedLoopbackServer{
		process:          process,
		daemon:           daemon,
		url:              api.WaitForURL(t),
		shutdownRequests: requests,
	}
}

type composedLoopbackServer struct {
	process          support.Process
	daemon           *support.ProcessCommand
	url              string
	shutdownRequests *shutdownRequestProbe
}

type shutdownRequest struct {
	method string
	path   string
}

type shutdownRequestProbe struct {
	mu    sync.Mutex
	calls []shutdownRequest
}

func (probe *shutdownRequestProbe) Record(request *http.Request) {
	if probe == nil || request == nil {
		return
	}
	probe.mu.Lock()
	defer probe.mu.Unlock()
	probe.calls = append(probe.calls, shutdownRequest{method: request.Method, path: request.URL.Path})
}

func (probe *shutdownRequestProbe) Calls() []shutdownRequest {
	if probe == nil {
		return nil
	}
	probe.mu.Lock()
	defer probe.mu.Unlock()
	return append([]shutdownRequest(nil), probe.calls...)
}

func executeServerStop(
	t testing.TB,
	process support.Process,
	ctx context.Context,
	endpoint string,
	jsonOutput bool,
) (*support.CapturedInputs, error) {
	t.Helper()
	args := []string{"you"}
	if jsonOutput {
		args = append(args, "--json")
	}
	args = append(args, "--server", endpoint, "server", "stop")
	inputs := support.FakeInputs(ctx, args)
	homeDir := t.TempDir()
	inputs.Input.Env = serverStopEnvironment(homeDir)
	inputs.Input.WorkingDirectory = homeDir
	return inputs, process.Execute(inputs.Input)
}

func serverStopEnvironment(homeDir string) []string {
	return append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func assertServerStopDiagnostic(
	t testing.TB,
	inputs *support.CapturedInputs,
	err error,
	wantCode string,
	wantFamily factoryapi.ErrorFamily,
	wantMessage string,
) {
	t.Helper()
	if err == nil {
		t.Fatalf("server-stop error = nil, want %s", wantCode)
	}
	if inputs.Stdout() != "" {
		t.Fatalf("server-stop failure stdout = %q, want empty", inputs.Stdout())
	}
	var response factoryapi.ErrorResponse
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stderr())), &response); decodeErr != nil {
		t.Fatalf("decode server-stop diagnostic: %v\nstderr=%q\nerror=%v", decodeErr, inputs.Stderr(), err)
	}
	if response.Code != factoryapi.ErrorResponseCode(wantCode) || response.Family != wantFamily || !strings.Contains(response.Message, wantMessage) {
		t.Fatalf("server-stop diagnostic = %#v, want code=%s family=%s message containing %q", response, wantCode, wantFamily, wantMessage)
	}
}

func assertLoopbackListenerClosed(t *testing.T, endpoint string) {
	t.Helper()
	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatalf("parse stopped server URL: %v", err)
	}
	address := net.JoinHostPort(parsed.Hostname(), parsed.Port())
	// Process.Execute returns success only after the production listener
	// observer has seen this loopback endpoint stop accepting connections. A
	// direct dial is therefore the deterministic postcondition check; the
	// listener is numeric loopback, so a separate dial timeout would add no
	// useful synchronization or contract coverage.
	connection, err := net.Dial("tcp", address)
	if err == nil {
		_ = connection.Close()
		t.Fatalf("stopped listener at %s still accepted a TCP connection", address)
	}
}

func waitForProcessDone(t *testing.T, command *support.ProcessCommand) {
	t.Helper()
	// Done is the deterministic signal that the composed daemon's
	// Process.Execute invocation returned. The timer is only a bounded hang
	// guard for a lifecycle regression; replacing it with an edge would remove
	// the process-boundary behavior this functional scenario verifies.
	select {
	case <-command.Done():
		if err := command.Err(); err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("composed server process error after server stop = %v", err)
		}
	case <-time.After(serverStopFunctionalObservationTimeout):
		t.Fatalf("composed server process did not finish within %s after listener shutdown", serverStopFunctionalObservationTimeout)
	}
}
