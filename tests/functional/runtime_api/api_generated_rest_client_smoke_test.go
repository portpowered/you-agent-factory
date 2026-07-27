package runtime_api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
	host := startFunctionalServer(t, dir, true)

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
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	host := startFunctionalServerWithArgs(t, dir, false, nil, withWorkerCommands(generatedRESTStreamingRunner{}, nil))

	opened := postJSON[factoryapi.OpenFactorySessionResponse](
		t,
		host.URL()+"/factory-sessions",
		factoryapi.OpenFactorySessionRequest{FolderPath: dir},
		"open a dedicated Factory Session for generated REST streaming",
	)
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("opened Factory Session = %#v, want a public session ID", opened)
	}
	streamSessionID := opened.Session.Id

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
		response, callErr := adapter.GetFactoryResponseEventsBySessionID(context.Background(), streamSessionID, nil)
		result <- callResult{response: response, err: callErr}
	}()

	traceID := submitGeneratedWorkToSession(t, host.URL(), streamSessionID, factoryapi.SubmitWorkRequest{
		Name:         stringPtr("generated-rest-client-success"),
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "generated REST client success"},
	})
	waitForGeneratedWorkCompleteInSession(t, host.URL(), streamSessionID, traceID, generatedRESTClientSmokeTimeout)
	select {
	case <-responseStarted:
	case <-time.After(generatedRESTClientSmokeTimeout):
		t.Fatal("timed out waiting for generated REST success response to start")
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

	closeFactorySession(t, host.URL(), streamSessionID)

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
	wantKind := fmt.Sprintf(`"kind":"%s"`, generatedclient.FactoryResponseEventKindRun)
	if body := string(success.response.Body); !strings.Contains(body, wantKind) || !strings.Contains(body, `"status":"completed"`) {
		t.Fatalf("generated success body = %q, want typed completed RUN event payload", body)
	}

}

// TestGeneratedRESTClientSmoke_BoundsCancellationAndDeadline is a pre-DI
// transport/client proof against the current functional HTTP host. It verifies
// caller-owned context bounds without claiming production-graph equivalence.
func TestGeneratedRESTClientSmoke_BoundsCancellationAndDeadline(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	host := startFunctionalServerWithArgs(t, dir, false, nil, withWorkerCommands(generatedRESTStreamingRunner{}, nil))

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

func submitGeneratedWorkToSession(t *testing.T, baseURL, sessionID string, req factoryapi.SubmitWorkRequest) string {
	t.Helper()
	if req.Name == nil || *req.Name == "" {
		req.Name = stringPtr("generated-api-submit")
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal generated submit request: %v", err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID + "/work"
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST %s status = %d, want 201", endpoint, resp.StatusCode)
	}
	var out factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode generated submit response: %v", err)
	}
	return out.TraceId
}

func waitForGeneratedWorkCompleteInSession(t *testing.T, baseURL, sessionID, traceID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID + "/work"
	for time.Now().Before(deadline) {
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, endpoint)
		for _, item := range work.Results {
			if stringPointerValue(item.TraceId) == traceID && generatedWorkPlaceID(item) == "task:complete" {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, endpoint)
	t.Fatalf("timed out waiting for trace %q at task:complete in session %q; last work response: %#v", traceID, sessionID, work)
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

func (generatedRESTStreamingRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	output := strings.Join([]string{
		`{"type":"thread.started","thread_id":"generated-rest-client-thread"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"generated-rest-client-message","type":"agent_message","text":"generated client response COMPLETE"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":4,"output_tokens":3}}`,
	}, "\n") + "\n"
	return platformprocess.CommandResult{Stdout: []byte(output)}, nil
}

func (generatedRESTStreamingRunner) SupportsResponseStreaming() bool { return true }

var _ platformprocess.CommandRunner = generatedRESTStreamingRunner{}
