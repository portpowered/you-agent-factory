package runtime_api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	"github.com/portpowered/infinite-you/pkg/service"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	"github.com/portpowered/infinite-you/tests/functional/internal/restclient"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const generatedRESTClientSmokeTimeout = 10 * time.Second

const generatedRESTClientDeadline = 2 * time.Second

// TestGeneratedRESTClientSmoke_ConfiguresCallerOwnedDependencies is a pre-DI
// transport/client proof. Production-shaped graph equivalence belongs to the
// functional graph coverage introduced after Wire DI.
func TestGeneratedRESTClientSmoke_ConfiguresCallerOwnedDependencies(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	host := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	var requests atomic.Int32
	httpClient := &http.Client{Transport: countingRoundTripper{
		count: &requests,
		base:  http.DefaultTransport,
	}}
	adapter, err := restclient.New(host.URL(), httpClient)
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

// TestGeneratedRESTClientSmoke_RoundTripsTypedSuccessAndAPIFailure is a
// pre-DI transport/client proof against the current functional HTTP host. It
// does not claim equivalence with the future Wire-composed production graph.
func TestGeneratedRESTClientSmoke_RoundTripsTypedSuccessAndAPIFailure(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.Codex, "gpt-5-codex"))
	host := startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		configureGeneratedRESTRunner(t, cfg)
	})

	traceID := submitGeneratedWork(t, host.URL(), factoryapi.SubmitWorkRequest{
		Name:         stringPtr("generated-rest-client-success"),
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "generated REST client success"},
	})
	waitForGeneratedWorkComplete(t, host.URL(), traceID, generatedRESTClientSmokeTimeout)

	responseStarted := make(chan struct{}, 1)
	httpClient := &http.Client{Transport: responseStartedRoundTripper{
		started: responseStarted,
		base:    http.DefaultTransport,
	}}
	adapter, err := restclient.New(host.URL(), httpClient)
	if err != nil {
		t.Fatalf("construct generated REST adapter: %v", err)
	}

	type callResult struct {
		response *generatedclient.GetFactoryResponseEventsBySessionIdClientResponse
		err      error
	}
	result := make(chan callResult, 1)
	go func() {
		response, callErr := adapter.GetFactoryResponseEventsBySessionID(context.Background(), "~default", nil)
		result <- callResult{response: response, err: callErr}
	}()

	select {
	case <-responseStarted:
	case <-time.After(generatedRESTClientSmokeTimeout):
		t.Fatal("timed out waiting for generated REST success response to start")
	}
	closeDefaultFactorySession(t, host.URL())

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
}

// TestGeneratedRESTClientSmoke_BoundsCancellationAndDeadline is a pre-DI
// transport/client proof against the current functional HTTP host. It verifies
// caller-owned context bounds without claiming production-graph equivalence.
func TestGeneratedRESTClientSmoke_BoundsCancellationAndDeadline(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.Codex, "gpt-5-codex"))
	host := startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		configureGeneratedRESTRunner(t, cfg)
	})

	traceID := submitGeneratedWork(t, host.URL(), factoryapi.SubmitWorkRequest{
		Name:         stringPtr("generated-rest-client-context-bounds"),
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "generated REST client context bounds"},
	})
	waitForGeneratedWorkComplete(t, host.URL(), traceID, generatedRESTClientSmokeTimeout)

	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		started, result := startOutstandingGeneratedRESTCall(t, host.URL(), ctx)
		waitForGeneratedRESTResponseStart(t, started, result)

		cancel()
		assertGeneratedRESTContextError(t, result, context.Canceled)
	})

	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), generatedRESTClientDeadline)
		defer cancel()
		started, result := startOutstandingGeneratedRESTCall(t, host.URL(), ctx)
		waitForGeneratedRESTResponseStart(t, started, result)

		assertGeneratedRESTContextError(t, result, context.DeadlineExceeded)
	})
}

func configureGeneratedRESTRunner(t *testing.T, cfg *service.FactoryServiceConfig) {
	t.Helper()
	components, err := workerapplication.New(cfg.Logger, workerapplication.Edges{
		ProviderCommandRunner: generatedRESTStreamingRunner{},
	})
	if err != nil {
		t.Fatalf("construct worker application: %v", err)
	}
	cfg.WorkerApplication = components
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

func closeDefaultFactorySession(t *testing.T, baseURL string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodDelete, strings.TrimSuffix(baseURL, "/")+"/factory-sessions/~default", nil)
	if err != nil {
		t.Fatalf("build close default Factory Session request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("close default Factory Session: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("close default Factory Session status = %d, want 204", response.StatusCode)
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
