package root_composition_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestModelsPublicRemoveMissingCachePreservesJSONDiagnostic proves public removal preserves the structured missing-cache diagnostic.
func TestModelsPublicRemoveMissingCachePreservesJSONDiagnostic(t *testing.T) {
	const safeMessage = "model cache is not installed; run you models pull not-cached-model first"

	server := characterizationNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete || request.URL.Path != "/models/not-cached-model" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(writer).Encode(factoryapi.ErrorResponse{
			Code:    factoryapi.ErrorResponseCodeMODELCACHENOTFOUND,
			Family:  factoryapi.ErrorFamilyNotFound,
			Message: safeMessage,
		})
	}))
	t.Cleanup(server.Close)

	process := characterizationBuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "--server", server.URL, "models", "remove", "not-cached-model",
	})
	inputs.Input.WorkingDirectory = characterizationTempDir(t)
	err := process.Execute(inputs.Input)
	if err == nil {
		t.Fatal("Process.Execute(json models remove) error = nil, want missing-cache failure")
	}
	if !errors.Is(err, modelscli.ErrModelCacheNotFound) {
		t.Fatalf("Process.Execute(json models remove) error = %v, want ErrModelCacheNotFound", err)
	}
	if inputs.Stdout() != "" {
		t.Fatalf("json models remove failure stdout = %q, want empty", inputs.Stdout())
	}

	var response factoryapi.ErrorResponse
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stderr())), &response); decodeErr != nil {
		t.Fatalf("decode rendered json models remove diagnostic: %v\nstderr=%q", decodeErr, inputs.Stderr())
	}
	if response.Code != factoryapi.ErrorResponseCodeMODELCACHENOTFOUND ||
		response.Family != factoryapi.ErrorFamilyNotFound || response.Message != safeMessage {
		t.Fatalf("rendered json models remove diagnostic = %#v, want server response", response)
	}
	for _, fallback := range []string{"CLI_COMMAND_FAILED", "INTERNAL_SERVER_ERROR"} {
		if strings.Contains(inputs.Stderr(), fallback) {
			t.Fatalf("rendered json models remove diagnostic contains fallback %q: %q", fallback, inputs.Stderr())
		}
	}
}
