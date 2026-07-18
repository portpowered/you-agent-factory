package runtime_api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/restclient"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const generatedRESTClientSmokeTimeout = 10 * time.Second

const generatedRESTClientDeadline = 2 * time.Second

// TestGeneratedRESTClientSmoke_ConfiguresCallerOwnedDependencies proves the
// generated adapter preserves its caller-owned transport against the composed
// production root.
func TestGeneratedRESTClientSmoke_ConfiguresCallerOwnedDependencies(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	host := startGeneratedRESTClientRootHost(t, dir, nil)

	var requests atomic.Int32
	httpClient := &http.Client{Transport: countingRoundTripper{
		count: &requests,
		base:  http.DefaultTransport,
	}}
	adapter, err := restclient.New(host.Endpoint(), httpClient)
	if err != nil {
		t.Fatalf("construct generated REST adapter: %v", err)
	}

	response, err := adapter.GetFactoryResponseEventsBySessionID(context.Background(), "missing-session", nil)
	if err != nil {
		t.Fatalf("request response events through generated REST adapter: %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("caller-owned HTTP client request count = %d, want 1", requests.Load())
	}
	if response.StatusCode() != http.StatusNotFound || response.JSON404 == nil {
		t.Fatalf("generated response = %#v, want typed 404 from functional HTTP host", response)
	}
	if response.JSON404.Code != generatedclient.ErrorResponseCodeRESPONSEEVENTSESSIONNOTFOUND {
		t.Fatalf("generated error code = %q, want %q", response.JSON404.Code, generatedclient.ErrorResponseCodeRESPONSEEVENTSESSIONNOTFOUND)
	}
}

// TestGeneratedRESTClientSmoke_RoundTripsTypedSuccessAndAPIFailure proves the
// generated adapter's typed success and failure contract against the composed
// production root.
func TestGeneratedRESTClientSmoke_RoundTripsTypedSuccessAndAPIFailure(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.Codex, "gpt-5-codex"))
	host := startGeneratedRESTClientRootHost(t, dir, generatedRESTStreamingRunner{})
	sessionID := openGeneratedRESTClientFactorySession(t, host.Endpoint(), dir)

	traceID := submitGeneratedRESTClientWork(t, host.Endpoint(), sessionID, factoryapi.SubmitWorkRequest{
		Name:         "generated-rest-client-success",
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "generated REST client success"},
	})
	waitForGeneratedRESTClientWorkComplete(t, host.Endpoint(), sessionID, traceID)

	adapter, err := restclient.New(host.Endpoint(), http.DefaultClient)
	if err != nil {
		t.Fatalf("construct generated REST adapter: %v", err)
	}
	failure, err := adapter.GetFactoryResponseEventsBySessionID(context.Background(), "missing-session", nil)
	if err != nil {
		t.Fatalf("request generated REST API failure: %v", err)
	}
	if failure.StatusCode() != http.StatusNotFound || failure.JSON404 == nil {
		t.Fatalf("generated failure response = %#v, want typed 404", failure)
	}
	if failure.JSON404.Family != generatedclient.ErrorFamilyNotFound || failure.JSON404.Code != generatedclient.ErrorResponseCodeRESPONSEEVENTSESSIONNOTFOUND {
		t.Fatalf("generated API error = %#v, want NOT_FOUND/RESPONSE_EVENT_SESSION_NOT_FOUND", failure.JSON404)
	}

	responseStarted := make(chan struct{}, 1)
	httpClient := &http.Client{Transport: responseStartedRoundTripper{
		started: responseStarted,
		base:    http.DefaultTransport,
	}}
	adapter, err = restclient.New(host.Endpoint(), httpClient)
	if err != nil {
		t.Fatalf("construct generated REST streaming adapter: %v", err)
	}

	type callResult struct {
		response *generatedclient.GetFactoryResponseEventsBySessionIdClientResponse
		err      error
	}
	result := make(chan callResult, 1)
	go func() {
		response, callErr := adapter.GetFactoryResponseEventsBySessionID(context.Background(), sessionID, nil)
		result <- callResult{response: response, err: callErr}
	}()

	select {
	case <-responseStarted:
	case <-time.After(generatedRESTClientSmokeTimeout):
		t.Fatal("timed out waiting for generated REST success response to start")
	}
	closeGeneratedRESTClientFactorySession(t, host.Endpoint(), sessionID)

	var success callResult
	select {
	case success = <-result:
	case <-time.After(generatedRESTClientSmokeTimeout):
		t.Fatal("timed out waiting for generated REST success response to finish")
	}
	if success.err != nil {
		t.Fatalf("request generated REST success response: %v", success.err)
	}
	if success.response.StatusCode() != http.StatusOK {
		t.Fatalf("generated success status = %d, want 200", success.response.StatusCode())
	}
	if contentType := success.response.HTTPResponse.Header.Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("generated success Content-Type = %q, want text/event-stream", contentType)
	}
	wantKind := fmt.Sprintf(`"kind":"%s"`, generatedclient.FactoryResponseEventKindMessage)
	if body := string(success.response.Body); !strings.Contains(body, wantKind) || !strings.Contains(body, "generated client response COMPLETE") {
		t.Fatalf("generated success body = %q, want typed MESSAGE event payload", body)
	}

}

// TestGeneratedRESTClientSmoke_BoundsCancellationAndDeadline proves the
// generated adapter respects caller-owned context bounds against the composed
// production root.
func TestGeneratedRESTClientSmoke_BoundsCancellationAndDeadline(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.Codex, "gpt-5-codex"))
	host := startGeneratedRESTClientRootHost(t, dir, generatedRESTStreamingRunner{})

	traceID := submitGeneratedWork(t, host.Endpoint(), factoryapi.SubmitWorkRequest{
		Name:         "generated-rest-client-context-bounds",
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "generated REST client context bounds"},
	})
	waitForGeneratedWorkComplete(t, host.Endpoint(), traceID, generatedRESTClientSmokeTimeout)

	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		started, result := startOutstandingGeneratedRESTCall(t, host.Endpoint(), ctx)
		waitForGeneratedRESTResponseStart(t, started, result)

		cancel()
		assertGeneratedRESTContextError(t, result, context.Canceled)
	})

	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), generatedRESTClientDeadline)
		defer cancel()
		started, result := startOutstandingGeneratedRESTCall(t, host.Endpoint(), ctx)
		waitForGeneratedRESTResponseStart(t, started, result)

		assertGeneratedRESTContextError(t, result, context.DeadlineExceeded)
	})
}

func startGeneratedRESTClientRootHost(
	t *testing.T,
	factoryRoot string,
	runner workers.CommandRunner,
) *support.RootRunFunctionalHost {
	t.Helper()

	config := support.RootRunFunctionalHostConfig{
		FactoryRoot: factoryRoot,
		SystemRoot:  t.TempDir(),
	}
	if runner != nil {
		config.DisableMockWorkers = true
		config.FunctionalEdges = wire.FunctionalEdges{ProviderCommandRunner: runner}
	}
	host, err := support.StartRootRunFunctionalHost(context.Background(), config)
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}
	t.Cleanup(func() {
		shutdownGeneratedRESTClientRootHost(t, host)
	})
	return host
}

func shutdownGeneratedRESTClientRootHost(t *testing.T, host *support.RootRunFunctionalHost) {
	t.Helper()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := host.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}
}

func openGeneratedRESTClientFactorySession(
	t *testing.T,
	baseURL string,
	factoryRoot string,
) string {
	t.Helper()
	body, err := json.Marshal(factoryapi.OpenFactorySessionRequest{FolderPath: factoryRoot})
	if err != nil {
		t.Fatalf("marshal open Factory Session request: %v", err)
	}
	response, err := http.Post(strings.TrimSuffix(baseURL, "/")+"/factory-sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("open Factory Session: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("open Factory Session status = %d, want 200", response.StatusCode)
	}
	var opened factoryapi.OpenFactorySessionResponse
	if err := json.NewDecoder(response.Body).Decode(&opened); err != nil {
		t.Fatalf("decode open Factory Session response: %v", err)
	}
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("open Factory Session response = %#v, want session ID", opened)
	}
	return opened.Session.Id
}

func submitGeneratedRESTClientWork(
	t *testing.T,
	baseURL string,
	sessionID string,
	request factoryapi.SubmitWorkRequest,
) string {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal generated REST client work: %v", err)
	}
	response, err := http.Post(generatedRESTClientSessionURL(baseURL, sessionID, "/work"), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("submit generated REST client work: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("submit generated REST client work status = %d, want 201", response.StatusCode)
	}
	var submitted factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&submitted); err != nil {
		t.Fatalf("decode generated REST client work response: %v", err)
	}
	return submitted.TraceId
}

func waitForGeneratedRESTClientWorkComplete(t *testing.T, baseURL, sessionID, traceID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), generatedRESTClientSmokeTimeout)
	defer cancel()
	httpClient := &http.Client{Timeout: generatedRESTClientDeadline}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, generatedRESTClientSessionURL(baseURL, sessionID, "/work"), nil)
		if err != nil {
			t.Fatalf("build generated REST client work read: %v", err)
		}
		response, err := httpClient.Do(request)
		if err != nil {
			t.Fatalf("read generated REST client work: %v", err)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			t.Fatalf("read generated REST client work status = %d, want 200", response.StatusCode)
		}
		var work factoryapi.ListWorkResponse
		if err := json.NewDecoder(response.Body).Decode(&work); err != nil {
			response.Body.Close()
			t.Fatalf("decode generated REST client work: %v", err)
		}
		response.Body.Close()
		for _, item := range work.Results {
			if stringPointerValue(item.TraceId) == traceID && generatedWorkStateName(item.State) == "complete" {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for generated REST client work %q: %v", traceID, ctx.Err())
		case <-ticker.C:
		}
	}
}

func closeGeneratedRESTClientFactorySession(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodDelete, generatedRESTClientSessionURL(baseURL, sessionID, ""), nil)
	if err != nil {
		t.Fatalf("build close Factory Session request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("close Factory Session: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("close Factory Session status = %d, want 204", response.StatusCode)
	}
}

func generatedRESTClientSessionURL(baseURL, sessionID, suffix string) string {
	return strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + suffix
}

type generatedRESTCallResult struct {
	response *generatedclient.GetFactoryResponseEventsBySessionIdClientResponse
	err      error
}

func startOutstandingGeneratedRESTCall(
	t *testing.T,
	baseURL string,
	ctx context.Context,
) (<-chan struct{}, <-chan generatedRESTCallResult) {
	t.Helper()
	started := make(chan struct{}, 1)
	adapter, err := restclient.New(baseURL, &http.Client{Transport: responseStartedRoundTripper{
		started: started,
		base:    http.DefaultTransport,
	}})
	if err != nil {
		t.Fatalf("construct generated REST adapter: %v", err)
	}

	result := make(chan generatedRESTCallResult, 1)
	go func() {
		response, callErr := adapter.GetFactoryResponseEventsBySessionID(ctx, "~default", nil)
		result <- generatedRESTCallResult{response: response, err: callErr}
	}()
	return started, result
}

func waitForGeneratedRESTResponseStart(
	t *testing.T,
	started <-chan struct{},
	result <-chan generatedRESTCallResult,
) {
	t.Helper()
	select {
	case <-started:
	case completed := <-result:
		t.Fatalf("generated REST call completed before it was outstanding: response=%#v error=%v", completed.response, completed.err)
	case <-time.After(generatedRESTClientSmokeTimeout):
		t.Fatal("timed out waiting for generated REST response to start")
	}
}

func assertGeneratedRESTContextError(
	t *testing.T,
	result <-chan generatedRESTCallResult,
	want error,
) {
	t.Helper()
	select {
	case completed := <-result:
		if completed.response != nil {
			t.Fatalf("generated REST response = %#v, want no typed API response", completed.response)
		}
		if !errors.Is(completed.err, want) {
			t.Fatalf("generated REST error = %v, want errors.Is(error, %v)", completed.err, want)
		}
	case <-time.After(generatedRESTClientSmokeTimeout):
		t.Fatalf("generated REST call did not terminate within %s", generatedRESTClientSmokeTimeout)
	}
}

type countingRoundTripper struct {
	count *atomic.Int32
	base  http.RoundTripper
}

func (t countingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	t.count.Add(1)
	return t.base.RoundTrip(request)
}

type responseStartedRoundTripper struct {
	started chan<- struct{}
	base    http.RoundTripper
}

func (t responseStartedRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err == nil {
		select {
		case t.started <- struct{}{}:
		default:
		}
	}
	return response, err
}

type generatedRESTStreamingRunner struct{}

func (generatedRESTStreamingRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	output := strings.Join([]string{
		`{"type":"thread.started","thread_id":"generated-rest-client-thread"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"generated-rest-client-message","type":"agent_message","text":"generated client response COMPLETE"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":4,"output_tokens":3}}`,
	}, "\n") + "\n"
	return workers.CommandResult{Stdout: []byte(output)}, nil
}

func (generatedRESTStreamingRunner) SupportsResponseStreaming() bool { return true }

var _ workers.CommandRunner = generatedRESTStreamingRunner{}
