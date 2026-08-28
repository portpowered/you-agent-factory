package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/restclient"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

const generatedClientTestTimeout = 10 * time.Second

const generatedClientDeadline = 2 * time.Second

// TestGeneratedClientStatusAndSessionRoundTrip proves status and Factory Session
// round-trips through the published generated HTTP client succeed with typed success
// decoding against the live functional server, use a caller-owned HTTP dependency for
// public requests, and honor caller-owned cancellation and deadline context bounds.
func TestGeneratedClientStatusAndSessionRoundTrip(t *testing.T) {
	dir := scaffoldC06HTTPFactory(t, startupShutdownTestFactoryConfig())
	server := c06SharedHTTPServer(t).newScenario(t, "generated-client-round-trip", dir)
	server.registerRunner(t, []string{dir}, generatedClientStreamingRunner{})
	sessionID := server.openSession(t, dir)

	var requests atomic.Int32
	httpClient := &http.Client{Transport: generatedClientCountingRoundTripper{
		count: &requests,
		base:  server.trackingTransport(http.DefaultTransport),
	}}
	adapter, err := restclient.New(server.URL(), httpClient)
	if err != nil {
		t.Fatalf("construct generated REST adapter: %v", err)
	}

	statusResponse, err := adapter.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("request status through generated REST adapter: %v", err)
	}
	if requests.Load() < 1 {
		t.Fatalf("caller-owned HTTP client request count = %d, want at least 1", requests.Load())
	}
	if statusResponse.StatusCode() != http.StatusOK || statusResponse.JSON200 == nil {
		t.Fatalf("generated status response = %#v, want typed 200", statusResponse)
	}
	if statusResponse.JSON200.FactoryState == "" || statusResponse.JSON200.RuntimeStatus == "" {
		t.Fatalf("generated status = %#v, want populated factoryState and runtimeStatus", statusResponse.JSON200)
	}

	generatedClient, err := generatedclient.NewClientWithResponses(
		server.URL(),
		generatedclient.WithHTTPClient(httpClient),
	)
	if err != nil {
		t.Fatalf("construct published generated HTTP client: %v", err)
	}

	missingCursor := generatedclient.AfterEventId("generated-client-missing-cursor")
	sessionEvents, err := generatedClient.GetEventsBySessionIdWithResponse(
		context.Background(),
		sessionID,
		&generatedclient.GetEventsBySessionIdParams{AfterEventId: &missingCursor},
		setGeneratedClientAcceptJSON,
	)
	if err != nil {
		t.Fatalf("request session events through generated HTTP client: %v", err)
	}
	if requests.Load() < 2 {
		t.Fatalf("caller-owned HTTP client request count = %d, want at least 2", requests.Load())
	}
	if sessionEvents.StatusCode() != http.StatusOK || sessionEvents.JSON200 == nil {
		t.Fatalf("generated session events response = %#v, want typed 200 reconnect probe", sessionEvents)
	}
	if sessionEvents.JSON200.Outcome == "" {
		t.Fatalf("generated session events outcome = %#v, want documented reconnect outcome", sessionEvents.JSON200)
	}

	traceID := submitGeneratedClientWork(t, server.URL(), sessionID, factoryapi.SubmitWorkRequest{
		Name:         stringPtr("generated-client-status-session"),
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "generated client status and session"},
	})
	waitForGeneratedClientWorkComplete(t, server.URL(), sessionID, traceID, generatedClientTestTimeout)

	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		started, result := startOutstandingGeneratedClientCall(t, server, sessionID, ctx)
		waitForGeneratedClientResponseStart(t, started, result)

		cancel()
		assertGeneratedClientContextError(t, result, context.Canceled)
	})

	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), generatedClientDeadline)
		defer cancel()
		started, result := startOutstandingGeneratedClientCall(t, server, sessionID, ctx)
		waitForGeneratedClientResponseStart(t, started, result)

		assertGeneratedClientContextError(t, result, context.DeadlineExceeded)
	})
}

// TestGeneratedClientDecodesRepresentativeStructuredError proves representative
// structured API failures decode through the published generated HTTP client into
// typed failure results with documented HTTP status and error family/code, not
// opaque transport errors or unstructured response bodies.
func TestGeneratedClientDecodesRepresentativeStructuredError(t *testing.T) {
	dir := scaffoldC06HTTPFactory(t, startupShutdownTestFactoryConfig())
	server := c06SharedHTTPServer(t).newScenario(t, "generated-client-errors", dir)
	httpClient := &http.Client{Transport: server.trackingTransport(http.DefaultTransport)}

	generatedClient, err := generatedclient.NewClientWithResponses(
		server.URL(),
		generatedclient.WithHTTPClient(httpClient),
	)
	if err != nil {
		t.Fatalf("construct published generated HTTP client: %v", err)
	}

	const missingSessionID = "missing-session"
	failure, err := generatedClient.GetFactoryResponseEventsBySessionIdWithResponse(
		context.Background(),
		missingSessionID,
		nil,
	)
	if err != nil {
		t.Fatalf("request missing-session response events through generated HTTP client: %v", err)
	}
	if failure.StatusCode() != http.StatusNotFound || failure.JSON404 == nil {
		t.Fatalf("generated failure response = %#v, want typed 404 API failure", failure)
	}
	if failure.JSON404.Family != generatedclient.ErrorFamilyNotFound {
		t.Fatalf("generated API error family = %q, want %q", failure.JSON404.Family, generatedclient.ErrorFamilyNotFound)
	}
	if failure.JSON404.Code != generatedclient.ErrorResponseCodeRESPONSEEVENTSESSIONNOTFOUND {
		t.Fatalf("generated API error code = %q, want %q", failure.JSON404.Code, generatedclient.ErrorResponseCodeRESPONSEEVENTSESSIONNOTFOUND)
	}
	if strings.TrimSpace(failure.JSON404.Message) == "" {
		t.Fatalf("generated API error message = %#v, want documented structured message", failure.JSON404.Message)
	}

	sessionID := server.openSession(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled, cancelErr := generatedClient.GetFactoryResponseEventsBySessionIdWithResponse(
		ctx,
		sessionID,
		nil,
	)
	if cancelled != nil {
		t.Fatalf("cancelled generated client response = %#v, want no typed API response", cancelled)
	}
	if !errors.Is(cancelErr, context.Canceled) {
		t.Fatalf("cancelled generated client error = %v, want context.Canceled", cancelErr)
	}
}

// TestGeneratedClientAndServerSchemaStayAligned proves the published generated HTTP
// client and live functional server stay aligned with the published OpenAPI contract,
// demonstrating typed success decoding for status and session-visible observations
// after a representative work submission against the live runtime.
func TestGeneratedClientAndServerSchemaStayAligned(t *testing.T) {
	dir := scaffoldC06HTTPFactory(t, startupShutdownTestFactoryConfig())
	server := c06SharedHTTPServer(t).newScenario(t, "generated-client-schema", dir)
	server.registerRunner(t, []string{dir}, generatedClientStreamingRunner{})
	sessionID := server.openSession(t, dir)
	httpClient := &http.Client{Transport: server.trackingTransport(http.DefaultTransport)}

	generatedClient, err := generatedclient.NewClientWithResponses(
		server.URL(),
		generatedclient.WithHTTPClient(httpClient),
	)
	if err != nil {
		t.Fatalf("construct published generated HTTP client: %v", err)
	}

	traceID := submitGeneratedClientWork(t, server.URL(), sessionID, factoryapi.SubmitWorkRequest{
		Name:         stringPtr("generated-client-schema-alignment"),
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "generated client schema alignment"},
	})
	if traceID == "" {
		t.Fatal("POST /work returned an empty trace_id")
	}
	waitForGeneratedClientWorkComplete(t, server.URL(), sessionID, traceID, generatedClientTestTimeout)

	status := getGeneratedClientJSON[factoryapi.StatusResponse](t, c06SessionWorkURL(server.URL(), sessionID, "/status"))
	if status.FactoryState == "" || status.RuntimeStatus == "" {
		t.Fatalf("generated status = %#v, want populated factoryState and runtimeStatus", status)
	}
	if status.TotalTokens < 1 {
		t.Fatalf("generated status total_tokens = %d, want at least 1 after completed work", status.TotalTokens)
	}
	if status.Categories.Terminal < 1 {
		t.Fatalf("generated status terminal count = %d, want at least 1 after completed work", status.Categories.Terminal)
	}

	staleCursor := generatedclient.AfterEventId("generated-client-schema-stale-cursor")
	sessionEvents, err := generatedClient.GetEventsBySessionIdWithResponse(
		context.Background(),
		sessionID,
		&generatedclient.GetEventsBySessionIdParams{AfterEventId: &staleCursor},
		setGeneratedClientAcceptJSON,
	)
	if err != nil {
		t.Fatalf("request session events reconnect probe through generated HTTP client: %v", err)
	}
	if sessionEvents.StatusCode() != http.StatusOK || sessionEvents.JSON200 == nil {
		t.Fatalf("generated session events response = %#v, want typed 200 reconnect probe", sessionEvents)
	}
	if sessionEvents.JSON200.Outcome != generatedclient.FactorySessionEventStreamRecoveryOutcomeCURSORSTALE {
		t.Fatalf(
			"generated session events outcome = %q, want %q",
			sessionEvents.JSON200.Outcome,
			generatedclient.FactorySessionEventStreamRecoveryOutcomeCURSORSTALE,
		)
	}
	if sessionEvents.JSON200.FactorySessionId != sessionID {
		t.Fatalf(
			"generated session events factorySessionId = %q, want %q",
			sessionEvents.JSON200.FactorySessionId,
			sessionID,
		)
	}
	if !sessionEvents.JSON200.Retry.OmitAfterEventId {
		t.Fatalf("generated session events retry = %#v, want omitAfterEventId true for stale cursor", sessionEvents.JSON200.Retry)
	}

	functionalevidence.Covers(
		t,
		"rest/getEventsBySessionId",
		"rest/getStatusBySessionId",
		"rest/listWorkBySessionId",
		"rest/submitWorkBySessionId",
	)
}

func setGeneratedClientAcceptJSON(_ context.Context, req *http.Request) error {
	req.Header.Set("Accept", "application/json")
	return nil
}

type generatedClientCallResult struct {
	response *generatedclient.GetFactoryResponseEventsBySessionIdClientResponse
	err      error
}

func startOutstandingGeneratedClientCall(
	t *testing.T,
	scenario *c06SharedHTTPScenario,
	sessionID string,
	ctx context.Context,
) (<-chan struct{}, <-chan generatedClientCallResult) {
	t.Helper()
	started := make(chan struct{}, 1)
	adapter, err := restclient.New(scenario.URL(), &http.Client{Transport: generatedClientResponseStartedRoundTripper{
		started: started,
		base:    scenario.trackingTransport(http.DefaultTransport),
	}})
	if err != nil {
		t.Fatalf("construct generated REST adapter: %v", err)
	}

	result := make(chan generatedClientCallResult, 1)
	go func() {
		response, callErr := adapter.GetFactoryResponseEventsBySessionID(ctx, sessionID, nil)
		result <- generatedClientCallResult{response: response, err: callErr}
	}()
	return started, result
}

func waitForGeneratedClientResponseStart(
	t *testing.T,
	started <-chan struct{},
	result <-chan generatedClientCallResult,
) {
	t.Helper()
	select {
	case <-started:
	case completed := <-result:
		t.Fatalf("generated client call completed before it was outstanding: response=%#v error=%v", completed.response, completed.err)
	case <-time.After(generatedClientTestTimeout):
		t.Fatal("timed out waiting for generated client response to start")
	}
}

func assertGeneratedClientContextError(
	t *testing.T,
	result <-chan generatedClientCallResult,
	want error,
) {
	t.Helper()
	select {
	case completed := <-result:
		if completed.response != nil {
			t.Fatalf("generated client response = %#v, want no typed API response", completed.response)
		}
		if !errors.Is(completed.err, want) {
			t.Fatalf("generated client error = %v, want errors.Is(error, %v)", completed.err, want)
		}
	case <-time.After(generatedClientTestTimeout):
		t.Fatalf("generated client call did not terminate within %s", generatedClientTestTimeout)
	}
}

func submitGeneratedClientWork(
	t *testing.T,
	baseURL, sessionID string,
	req factoryapi.SubmitWorkRequest,
) string {
	t.Helper()
	if req.Name == nil || *req.Name == "" {
		req.Name = stringPtr("generated-client-submit")
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal generated submit request: %v", err)
	}
	endpoint := c06SessionWorkURL(baseURL, sessionID, "/work")
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /work status = %d, want 201", resp.StatusCode)
	}
	var out factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode generated submit response: %v", err)
	}
	return out.TraceId
}

func waitForGeneratedClientWorkComplete(
	t *testing.T,
	baseURL, sessionID, traceID string,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := getGeneratedClientJSON[factoryapi.ListWorkResponse](t, c06SessionWorkURL(baseURL, sessionID, "/work"))
		for _, item := range work.Results {
			if stringPointerValue(item.TraceId) == traceID && generatedClientWorkPlaceID(item) == "task:complete" {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	work := getGeneratedClientJSON[factoryapi.ListWorkResponse](t, c06SessionWorkURL(baseURL, sessionID, "/work"))
	t.Fatalf("timed out waiting for trace %q at task:complete; last work response: %#v", traceID, work)
}

func c06SessionWorkURL(baseURL, sessionID, path string) string {
	return strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + path
}

func getGeneratedClientJSON[T any](t *testing.T, endpoint string) T {
	t.Helper()
	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s status = %d, want 200: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s as %T: %v", endpoint, out, err)
	}
	return out
}

func generatedClientWorkPlaceID(work factoryapi.Work) string {
	if work.State == nil {
		return stringPointerValue(work.WorkTypeName) + ":"
	}
	return stringPointerValue(work.WorkTypeName) + ":" + work.State.Name
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

type generatedClientCountingRoundTripper struct {
	count *atomic.Int32
	base  http.RoundTripper
}

func (t generatedClientCountingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	t.count.Add(1)
	return t.base.RoundTrip(request)
}

type generatedClientResponseStartedRoundTripper struct {
	started chan<- struct{}
	base    http.RoundTripper
}

func (t generatedClientResponseStartedRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err == nil {
		select {
		case t.started <- struct{}{}:
		default:
		}
	}
	return response, err
}

type generatedClientStreamingRunner struct{}

func (generatedClientStreamingRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	output := strings.Join([]string{
		`{"type":"thread.started","thread_id":"generated-client-thread"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"generated-client-message","type":"agent_message","text":"generated client response COMPLETE"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":4,"output_tokens":3}}`,
	}, "\n") + "\n"
	return platformprocess.CommandResult{Stdout: []byte(output)}, nil
}

func (generatedClientStreamingRunner) SupportsResponseStreaming() bool { return true }

var _ platformprocess.CommandRunner = generatedClientStreamingRunner{}
