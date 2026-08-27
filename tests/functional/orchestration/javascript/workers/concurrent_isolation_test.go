package workers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const concurrentJavaScriptWaitTimeout = 15 * time.Second

type concurrentJavaScriptResult struct {
	response factoryapi.FactorySessionSyncExecutionResponse
	err      error
}

func runJavaScriptConcurrentIsolation(t *testing.T, fixture *javascriptSharedProcessFixture) {
	successPrompt := "shared concurrent success"
	successGate := make(chan struct{})
	successRunner := support.NewGatedSuccessCommandRunner("concurrent success output", successGate)
	if err := fixture.router.register(successPrompt, successRunner); err != nil {
		t.Fatalf("register concurrent success route: %v", err)
	}
	t.Cleanup(func() {
		select {
		case <-successGate:
		default:
			close(successGate)
		}
		if err := fixture.router.unregister(successPrompt); err != nil {
			t.Errorf("unregister concurrent success route: %v", err)
		}
	})

	successWorkflow := strings.ReplaceAll(liveProviderChildWorkflow, "use the live provider command edge", successPrompt)
	failureWorkflow := invalidPermissionsOverrideWorkflowWithValue("true", "shared concurrent failure")
	requestBase := fixture.requestSequence.Load() + 1
	successRequestID := fmt.Sprintf("shared-javascript-concurrent-success-%d", requestBase)
	failureRequestID := fmt.Sprintf("shared-javascript-concurrent-failure-%d", requestBase+1)
	beforeCalls := fixture.router.callCount()

	successCh := make(chan concurrentJavaScriptResult, 1)
	go func() {
		response, err := postOverridesWorkflow(context.Background(), fixture.baseURL, successRequestID, successWorkflow)
		successCh <- concurrentJavaScriptResult{response: response, err: err}
	}()

	// The success command is intentionally held at the controlled edge while
	// the failure request runs. This bounded context is required because the
	// producers are real HTTP/Process.Execute calls and the gated edge must
	// prove ordering; channel receives below are the deterministic completion
	// barriers, so a sleep or timeout-padded polling helper cannot substitute.
	waitContext, cancelWait := context.WithTimeout(context.Background(), concurrentJavaScriptWaitTimeout)
	defer cancelWait()
	if err := fixture.router.waitForCall(waitContext, beforeCalls+1); err != nil {
		t.Fatalf("wait for concurrent success provider request: %v", err)
	}

	failureCh := make(chan concurrentJavaScriptResult, 1)
	go func() {
		response, err := postOverridesWorkflow(context.Background(), fixture.baseURL, failureRequestID, failureWorkflow)
		failureCh <- concurrentJavaScriptResult{response: response, err: err}
	}()

	failure, err := awaitConcurrentJavaScriptResult(waitContext, failureCh, "failure")
	if err != nil {
		t.Fatal(err)
	}
	if failure.err != nil {
		t.Fatalf("concurrent failure workflow: %v", failure.err)
	}
	if failure.response.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("concurrent failure status = %q, want FAILED", failure.response.Status)
	}

	close(successGate)
	success, err := awaitConcurrentJavaScriptResult(waitContext, successCh, "success")
	if err != nil {
		t.Fatal(err)
	}
	assertConcurrentJavaScriptIsolation(t, fixture, successPrompt, success, failure)
}

func assertConcurrentJavaScriptIsolation(
	t *testing.T,
	fixture *javascriptSharedProcessFixture,
	successPrompt string,
	success, failure concurrentJavaScriptResult,
) {
	t.Helper()
	if success.err != nil {
		t.Fatalf("concurrent success workflow: %v", success.err)
	}
	if success.response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("concurrent success status = %q, want SUCCEEDED", success.response.Status)
	}
	if success.response.SessionId == failure.response.SessionId {
		t.Fatalf("concurrent workflows reused Factory Session ID %q", success.response.SessionId)
	}
	fixture.trackSession(t, failure.response.SessionId)
	fixture.trackSession(t, success.response.SessionId)
	assertSucceededPrimaryContains(t, success.response, "concurrent success output")
	assertUnavailableFactoryResult(t, failure.response.Result)

	failureSession := readOverridesDurableSession(t, fixture.baseURL, failure.response.SessionId)
	if failureSession.FailureDetail == nil || !strings.Contains(strings.ToLower(failureSession.FailureDetail.Message), "permissions") {
		t.Fatalf("concurrent failure detail = %#v, want permissions diagnostic", failureSession.FailureDetail)
	}
	successEvents := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, success.response.SessionId)
	failureEvents := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, failure.response.SessionId)
	assertJavaScriptEventsDoNotContain(t, successEvents, "shared concurrent failure")
	assertJavaScriptEventsDoNotContain(t, failureEvents, "concurrent success output")

	assertConcurrentDispatches(t, fixture, success.response.SessionId, failure.response.SessionId)
	requests := fixture.router.requestRecords()
	if len(requests) == 0 || !bytes.Contains(requests[len(requests)-1].Stdin, []byte(successPrompt)) || bytes.Contains(requests[len(requests)-1].Stdin, []byte("shared concurrent failure")) {
		t.Fatalf("concurrent command requests = %#v, want only success request content", requests)
	}
}

func assertConcurrentDispatches(t *testing.T, fixture *javascriptSharedProcessFixture, successSessionID, failureSessionID string) {
	t.Helper()
	baseURL := strings.TrimSuffix(fixture.baseURL, "/")
	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		baseURL+"/factory-sessions/"+successSessionID+"/dispatches",
	)
	if len(dispatches.Dispatches) != 1 || dispatches.Dispatches[0].Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("concurrent success dispatches = %#v, want one completed dispatch", dispatches.Dispatches)
	}
	failureDispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		baseURL+"/factory-sessions/"+failureSessionID+"/dispatches",
	)
	if len(failureDispatches.Dispatches) != 0 {
		t.Fatalf("concurrent failure dispatches = %#v, want no provider dispatch", failureDispatches.Dispatches)
	}
}

func awaitConcurrentJavaScriptResult(
	ctx context.Context,
	results <-chan concurrentJavaScriptResult,
	name string,
) (concurrentJavaScriptResult, error) {
	select {
	case result := <-results:
		return result, nil
	case <-ctx.Done():
		return concurrentJavaScriptResult{}, fmt.Errorf("timed out waiting for concurrent %s workflow: %w", name, ctx.Err())
	}
}

func assertJavaScriptEventsDoNotContain(t *testing.T, events []factoryapi.FactoryEvent, forbidden string) {
	t.Helper()
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal Factory Events: %v", err)
	}
	if strings.Contains(string(encoded), forbidden) {
		t.Fatalf("Factory Events contain foreign content %q: %s", forbidden, encoded)
	}
}
