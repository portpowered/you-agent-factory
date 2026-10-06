package acceptance

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The restart scenario owns sequential host reconstruction because persisted
// reads after a joined shutdown are the customer invariant under test.
func readCapturedRestartTranscript(t *testing.T, host invokeContinueStartedProcess, home, dir string) factoryapi.WorkerSessionTranscriptResponse {
	t.Helper()

	request := support.FakeInputs(t.Context(), []string{"you", "--server", host.baseURL, "--json", "worker-sessions", "read", "--worker-session-id", "restart-source"})
	request.Input.Env, request.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	if err := host.process.Execute(request.Input); err != nil {
		t.Fatalf("captured CLI read: %v stderr=%s", err, request.Stderr())
	}
	var want factoryapi.WorkerSessionTranscriptResponse
	if err := json.Unmarshal([]byte(request.Stdout()), &want); err != nil {
		t.Fatal(err)
	}
	if want.WorkerSessionId != "restart-source" || want.AttemptId != "restart-source-attempt" || want.ProviderSession.Id != "opaque-restart-thread" ||
		len(want.Entries) != 1 || want.Entries[0].Text == nil || *want.Entries[0].Text != "initial COMPLETE" {
		t.Fatalf("captured transcript lost public content/correlation: %+v", want)
	}
	httpRequest, err := http.NewRequestWithContext(t.Context(), http.MethodGet, host.baseURL+"/worker-sessions/restart-source/transcript", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var got factoryapi.WorkerSessionTranscriptResponse
	if response.StatusCode != http.StatusOK {
		t.Fatalf("captured HTTP status: %d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CLI/HTTP transcript differs: %+v / %+v", want, got)
	}
	return want
}

func assertCapturedRestartScopeDenial(t *testing.T, host invokeContinueStartedProcess) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, host.baseURL+"/factory-sessions/foreign/worker-sessions/restart-source/transcript", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var failure factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNotFound || failure.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("foreign capture scope: %d %+v", response.StatusCode, failure)
	}
}
