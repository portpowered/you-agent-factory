package mock

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// M7 shares the immutable fault-group host. Correlated direct sources own two
// explicit Factory Sessions and distinct policy routes, gates and usage.
func checkMockInterruptScopeIsolation(t *testing.T, baseURL, dir string, gate, siblingGate *support.MockWorkerGate) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	t.Cleanup(siblingGate.Release)
	owned := support.OpenFactorySessionAt(t, baseURL, dir).Session.Id
	foreign := support.OpenFactorySessionAt(t, baseURL, dir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, owned) })
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, foreign) })
	for name, scope := range map[string]string{"scope-isolation": owned, "isolation-sibling": foreign} {
		execution := mockInterruptExecution(dir, name)
		execution["factorySessionId"] = scope
		execution["userMessage"] = "synthetic-private-mock-token"
		interruptPost(t, ctx, baseURL+"/worker-sessions", map[string]any{
			"requestId": name + "-start", "workerSessionId": name + "-source", "execution": execution,
		}, http.StatusAccepted)
	}
	gate.WaitForArrival(t, 10*time.Second)
	siblingGate.WaitForArrival(t, 10*time.Second)
	if err := os.Remove(gate.Config(30 * time.Second).ArrivedFile); err != nil {
		t.Fatal(err)
	}
	request := map[string]any{"factorySessionId": foreign, "requestId": "scope-isolation-interrupt",
		"successorWorkerSessionId": "scope-isolation-successor", "replacementMessage": "replacement", "resumeMode": "recorded"}
	endpoint := baseURL + "/worker-sessions/scope-isolation-source/interrupt"
	body := interruptPost(t, ctx, endpoint, request, http.StatusNotFound)
	var refused factoryapi.WorkerSessionInterruptError
	if err := json.Unmarshal(body, &refused); err != nil || refused.Code != "NOT_FOUND" || refused.Phase != "VALIDATION" {
		t.Fatalf("foreign scope refusal = %+v, %v", refused, err)
	}
	assertMockIsolationState(t, baseURL, "scope-isolation-source", "RUNNING", 17)
	assertMockIsolationState(t, baseURL, "isolation-sibling-source", "RUNNING", 29)
	request["factorySessionId"] = owned
	interruptPost(t, ctx, endpoint, request, http.StatusAccepted)
	gate.WaitForArrival(t, 10*time.Second)
	assertMockIsolationState(t, baseURL, "scope-isolation-source", "CANCELED", 17)
	assertMockIsolationState(t, baseURL, "scope-isolation-successor", "RUNNING", 17)
	assertMockIsolationState(t, baseURL, "isolation-sibling-source", "RUNNING", 29)
	// A new control against the stopped predecessor cannot target its successor.
	request["requestId"], request["successorWorkerSessionId"] = "stale-source-interrupt", "stale-source-successor"
	interruptPost(t, ctx, endpoint, request, http.StatusConflict)
	assertMockIsolationState(t, baseURL, "scope-isolation-successor", "RUNNING", 17)
	gate.Release()
	siblingGate.Release()
	assertMockIsolationState(t, baseURL, "scope-isolation-successor", "COMPLETED", 17)
	assertMockIsolationState(t, baseURL, "isolation-sibling-source", "COMPLETED", 29)
}

func assertMockIsolationState(t *testing.T, baseURL, id, state string, tokens int) {
	t.Helper()
	observation := support.GetJSON[factoryapi.WorkerSessionObservation](t, baseURL+"/worker-sessions/"+id)
	if state != "RUNNING" {
		// The gate observes execution, not asynchronous capture finalization.
		// Observe terminal state and declared usage through the public API.
		var err error
		observation, err = support.WaitForObservation(10*time.Second, func() (factoryapi.WorkerSessionObservation, error) {
			return support.GetJSON[factoryapi.WorkerSessionObservation](t, baseURL+"/worker-sessions/"+id), nil
		}, func(observation factoryapi.WorkerSessionObservation) bool {
			return mockInterruptStateUsageReady(observation, state, int64(tokens))
		})
		if err != nil {
			t.Fatalf("isolated mock state/usage = %+v, %v", observation, err)
		}
	}
	if string(observation.State) != state || state != "RUNNING" && (observation.TokenUsage == nil || observation.TokenUsage.InputTokens == nil || *observation.TokenUsage.InputTokens != tokens) {
		t.Fatalf("isolated mock state/usage = %+v, want %s/%d", observation, state, tokens)
	}
	logs := support.GetJSON[map[string]any](t, baseURL+"/worker-sessions/"+id+"/logs")
	encoded, err := json.Marshal([]any{observation, logs})
	if err != nil || strings.Contains(string(encoded), "synthetic-private-mock-token") {
		t.Fatal("private source prompt leaked through public observations or logs")
	}
}
