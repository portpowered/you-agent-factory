package root_composition_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Rejected inputs must leave the owned session empty and never reach a provider.
// This is the missing-name HTTP cell and malformed-JSON CLI parity witness;
// pure validation permutations stay in Work's component tests.
func assertInvalidAdmissionBeforeDispatch(t *testing.T, process support.Process, scenario *flushCase, baseURL string) {
	t.Helper()
	input := support.FakeInputs(t.Context(), []string{"you", "--server", baseURL, "submit", "batch",
		"--session", scenario.sessionID, `{"requestId":"invalid-json",`})
	input.Input.Env, input.Input.WorkingDirectory = recoveryActivationHomeEnvironment(scenario.home), scenario.dir
	input.Input.Stdin = strings.NewReader("")
	err := process.Execute(input.Input)
	if err == nil || !strings.Contains(err.Error(), "parse inline JSON") || !strings.Contains(err.Error(), "unexpected end") {
		t.Fatalf("malformed CLI batch error = %v, stderr=%s", err, input.Stderr())
	}
	if strings.Contains(input.Stdout(), "work count:") {
		t.Fatalf("rejected CLI batch acknowledged success: %s", input.Stdout())
	}
	endpoint := support.SessionWorkURL(baseURL, scenario.sessionID, "/work-requests/missing-name")
	// Keep the session marker in the rejected payload so an erroneous dispatch
	// would be observable by this scenario's provider-command counter.
	body := []byte(fmt.Sprintf(`{"requestId":"missing-name","type":"FACTORY_REQUEST_BATCH","works":[{"workTypeName":"task","payload":{"title":%q}}]}`, scenario.sessionID))
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var diagnostic factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&diagnostic); err != nil {
		t.Fatalf("HTTP error envelope: %v", err)
	}
	if response.StatusCode != http.StatusBadRequest || diagnostic.Code != factoryapi.ErrorResponseCodeBADREQUEST ||
		diagnostic.Family != factoryapi.ErrorFamilyBadRequest || diagnostic.Message == "" {
		t.Fatalf("missing-name HTTP error = %d %#v, want BAD_REQUEST family/code", response.StatusCode, diagnostic)
	}
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, scenario.sessionID, "/work"))
	if len(listed.Results) != 0 || scenario.dispatches.Load() != 0 {
		t.Fatalf("invalid admission produced Work/dispatch: %#v / %d", listed, scenario.dispatches.Load())
	}
}
