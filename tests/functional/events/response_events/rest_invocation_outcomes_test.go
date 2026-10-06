package response_events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Every invocation and observation crosses REST. A timed-out caller leaves its
// Work running, and success or failure in one explicit session preserves peers.
func TestRESTInvocationOutcomesPreservePeerSessions(t *testing.T) {
	t.Parallel()
	gates := map[string]*isolatedCommandGate{}
	for _, prompt := range []string{"rest-success", "rest-failure", "rest-timeout"} {
		for _, marker := range []string{prompt, prompt + "-peer"} {
			gates[marker] = &isolatedCommandGate{entered: make(chan context.Context, 4), release: make(chan struct{}), returned: make(chan error, 4), output: marker + " COMPLETE"}
		}
	}
	gates["rest-failure"].failure = errors.New("private-provider-failure")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                support.ScaffoldFactory(t, concurrentIsolationFactoryConfig()),
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: &fourScopeCodexRunner{gates: gates}},
	})
	assertRESTInvocationMissingSession(t, server.URL())
	for _, test := range []struct {
		name, status string
		timeout      int64
	}{
		{name: "rest-success", status: "COMPLETED"},
		{name: "rest-failure", status: "FAILED"},
		{name: "rest-timeout", status: "TIMED_OUT", timeout: 2000},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runRESTInvocationOutcome(t, server.URL(), gates[test.name], gates[test.name+"-peer"], test.name, test.status, test.timeout)
		})
	}
}

func runRESTInvocationOutcome(t *testing.T, baseURL string, gate, peerGate *isolatedCommandGate, prompt, status string, timeout int64) {
	t.Helper()
	selected := openRESTOutcomeSession(t, baseURL, prompt)
	peerID := openRESTOutcomeSession(t, baseURL, prompt+"-peer")
	peer := make(chan concurrentIsolationInvocation, 1)
	go func() {
		value, err := postConcurrentIsolationInvocation(t.Context(), baseURL, peerID, "peer invocation")
		peer <- concurrentIsolationInvocation{response: value, err: err}
	}()
	peerCtx := awaitRESTOutcomeProvider(t, peerGate)
	done := make(chan concurrentIsolationInvocation, 1)
	go func() {
		value, err := postRESTOutcomeInvocation(t.Context(), baseURL, selected, timeout)
		done <- concurrentIsolationInvocation{response: value, err: err}
	}()
	providerCtx := awaitRESTOutcomeProvider(t, gate)
	if status != "TIMED_OUT" {
		close(gate.release)
	}
	result := awaitConcurrentIsolationInvocation(t, done)
	assertRESTInvocationOutcome(t, result, gate.output, status)
	assertRESTOutcomeWorkCorrelation(t, baseURL, selected, result.response)
	if status == "TIMED_OUT" {
		if providerCtx.Err() != nil {
			t.Fatalf("timed-out wait canceled ongoing Work: %v", providerCtx.Err())
		}
		stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(baseURL, selected))
		defer stream.Close()
		close(gate.release)
		awaitRESTContinuedWork(t, stream, gate.output)
	}
	if peerCtx.Err() != nil {
		t.Fatalf("selected %s canceled peer: %v", status, peerCtx.Err())
	}
	close(peerGate.release)
	assertConcurrentIsolationInvocationCompleted(t, awaitConcurrentIsolationInvocation(t, peer), peerGate.output)
	next, err := postConcurrentIsolationInvocation(t.Context(), baseURL, peerID, "next peer invocation")
	assertConcurrentIsolationInvocationCompleted(t, concurrentIsolationInvocation{response: next, err: err}, peerGate.output)
}

func openRESTOutcomeSession(t *testing.T, baseURL, marker string) string {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, baseURL, scaffoldConcurrentIsolationFactory(t, marker))
	id := opened.Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, id) })
	warmup, err := postConcurrentIsolationInvocation(t.Context(), baseURL, id, "warmup")
	if err != nil {
		t.Fatal(err)
	}
	assertConcurrentIsolationInvocationCompleted(t, concurrentIsolationInvocation{response: warmup}, "warmup "+marker)
	return id
}

func awaitRESTOutcomeProvider(t *testing.T, gate *isolatedCommandGate) context.Context {
	t.Helper()
	select {
	case ctx := <-gate.entered:
		return ctx
	case <-time.After(concurrentIsolationTimeout):
		t.Fatal("provider did not enter the owned invocation")
	}
	return nil
}

func postRESTOutcomeInvocation(ctx context.Context, baseURL, id string, timeout int64) (factoryapi.InvocationResponse, error) {
	body := fmt.Sprintf(`{"sourceKind":"text","content":[{"type":"text","text":"owned invocation"}],"timeoutMillis":%d}`, timeout)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/factory-sessions/"+url.PathEscape(id)+"/invocations", strings.NewReader(body))
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	defer response.Body.Close()
	var value factoryapi.InvocationResponse
	err = json.NewDecoder(response.Body).Decode(&value)
	if response.StatusCode != http.StatusOK {
		return value, fmt.Errorf("invocation HTTP status=%d result=%#v", response.StatusCode, value)
	}
	return value, err
}

func assertRESTInvocationOutcome(t *testing.T, result concurrentIsolationInvocation, output, status string) {
	t.Helper()
	value := result.response
	if result.err != nil || string(value.Status) != status || value.RequestId == "" || value.TraceId == "" {
		t.Fatalf("invocation=%#v error=%v want=%s", value, result.err, status)
	}
	if status == "COMPLETED" {
		assertConcurrentIsolationInvocationCompleted(t, result, output)
		return
	}
	codes := map[string]string{"TIMED_OUT": "INVOCATION_TIMED_OUT", "FAILED": "INVOCATION_RUNTIME_FAILURE"}
	if value.WorkId == nil || *value.WorkId == "" || value.ErrorCode == nil || string(*value.ErrorCode) != codes[status] {
		t.Fatalf("terminal Work identity/code=%#v", value)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private-provider-failure") {
		t.Fatalf("public response leaked provider failure: %s", encoded)
	}
}

func assertRESTOutcomeWorkCorrelation(t *testing.T, baseURL, id string, result factoryapi.InvocationResponse) {
	t.Helper()
	for _, event := range support.GetFactoryEventsForSessionAt(t, baseURL, id) {
		if event.Context.RequestId == nil || *event.Context.RequestId != result.RequestId || event.Context.WorkIds == nil {
			continue
		}
		for _, workID := range *event.Context.WorkIds {
			if workID != "" && (result.WorkId == nil || workID == *result.WorkId) {
				return
			}
		}
	}
	t.Fatalf("Factory history omitted invocation request/Work correlation: %#v", result)
}

func awaitRESTContinuedWork(t *testing.T, stream *support.FactoryEventStream, output string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), concurrentIsolationTimeout)
	defer cancel()
	for {
		event := stream.NextEventContext(ctx)
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		encoded, err := json.Marshal(event.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), output) {
			return
		}
	}
}

func assertRESTInvocationMissingSession(t *testing.T, baseURL string) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/factory-sessions/b26c8a2c-4b12-43f7-9a11-12cb69f20271/invocations", strings.NewReader(`{"content":[{"type":"text","text":"missing selector"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusNotFound || !strings.Contains(string(body), "NOT_FOUND") {
		t.Fatalf("missing selector HTTP=%d body=%s error=%v", response.StatusCode, body, err)
	}
}
