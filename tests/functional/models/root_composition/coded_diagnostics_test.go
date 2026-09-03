package root_composition_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	modelservice "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const codedDiagnosticModelName = "OMNIVOICE_Q4_K_M"
const codedDiagnosticUnknownModelName = "missing-model"

// TestModelsLocalRemoveMissingCacheRendersCodedDiagnostic proves local removal renders a coded diagnostic for a missing cache entry.
func TestModelsLocalRemoveMissingCacheRendersCodedDiagnostic(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		flags []string
		debug bool
	}{
		{name: "normal"},
		{name: "json", flags: []string{"--json"}},
		{name: "debug", flags: []string{"--debug"}, debug: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			inputs, err := executeLocalMissingCache(t, test.flags)
			if err == nil {
				t.Fatal("Process.Execute(models remove) error = nil, want missing-cache failure")
			}
			if !errors.Is(err, modelscli.ErrModelCacheNotFound) {
				t.Fatalf("Process.Execute(models remove) error = %v, want ErrModelCacheNotFound", err)
			}
			if !errors.Is(err, modelservice.ErrModelCacheNotFound) {
				t.Fatalf("Process.Execute(models remove) error = %v, want underlying Models cache-not-found cause", err)
			}
			if inputs.Stdout() != "" {
				t.Fatalf("models remove failure stdout = %q, want empty", inputs.Stdout())
			}

			response := decodeFirstDiagnostic(t, inputs.Stderr())
			wantMessage := "model cache is not installed; run you models pull " + codedDiagnosticModelName + " first"
			if response.Code != factoryapi.ErrorResponseCodeMODELCACHENOTFOUND ||
				response.Family != factoryapi.ErrorFamilyNotFound || response.Message != wantMessage {
				t.Fatalf("models remove diagnostic = %#v, want code/family/message %q/%q/%q", response, factoryapi.ErrorResponseCodeMODELCACHENOTFOUND, factoryapi.ErrorFamilyNotFound, wantMessage)
			}
			for _, fallback := range []string{"CLI_COMMAND_FAILED", "INTERNAL_SERVER_ERROR", "command failed"} {
				if strings.Contains(inputs.Stderr(), fallback) {
					t.Fatalf("models remove diagnostic contains fallback %q: %q", fallback, inputs.Stderr())
				}
			}
			if test.debug && !strings.Contains(inputs.Stderr(), "managed model cache not found") {
				t.Fatalf("debug models remove diagnostic = %q, want underlying cache-not-found cause", inputs.Stderr())
			}
		})
	}
}

// TestModelsLocalRemoveMissingCacheMatchesHTTPDiagnostic proves local and HTTP removal expose equivalent missing-cache diagnostics.
func TestModelsLocalRemoveMissingCacheMatchesHTTPDiagnostic(t *testing.T) {
	t.Parallel()
	const message = "model cache is not installed; run you models pull " + codedDiagnosticModelName + " first"

	server := functionalNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete || request.URL.Path != "/models/"+codedDiagnosticModelName {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(writer).Encode(factoryapi.ErrorResponse{
			Code: factoryapi.ErrorResponseCodeMODELCACHENOTFOUND, Family: factoryapi.ErrorFamilyNotFound, Message: message,
		})
	}))
	t.Cleanup(server.Close)

	localInputs, localErr := executeLocalMissingCache(t, nil)
	if localErr == nil {
		t.Fatal("local Process.Execute(models remove) error = nil, want missing-cache failure")
	}
	localResponse := decodeFirstDiagnostic(t, localInputs.Stderr())

	process := functionalSharedDefaultProcess(t)
	remoteInputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", server.URL, "models", "remove", codedDiagnosticModelName,
	})
	remoteInputs.Input.Env = functionalSharedDefaultEnvironment()
	remoteInputs.Input.WorkingDirectory = functionalTempDir(t)
	remoteErr := process.Execute(remoteInputs.Input)
	if remoteErr == nil || !errors.Is(remoteErr, modelscli.ErrModelCacheNotFound) {
		t.Fatalf("remote Process.Execute(models remove) error = %v, want ErrModelCacheNotFound", remoteErr)
	}
	remoteResponse := decodeFirstDiagnostic(t, remoteInputs.Stderr())
	if localResponse.Code != remoteResponse.Code || localResponse.Family != remoteResponse.Family || localResponse.Message != remoteResponse.Message {
		t.Fatalf("local diagnostic = %#v, remote diagnostic = %#v, want parity", localResponse, remoteResponse)
	}
	if localResponse.Code != factoryapi.ErrorResponseCodeMODELCACHENOTFOUND || localResponse.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("local diagnostic = %#v, want MODEL_CACHE_NOT_FOUND/NOT_FOUND", localResponse)
	}
}

// TestModelsLocalInspectUnknownRendersCodedDiagnostic proves local inspection renders a coded diagnostic for an unknown model.
func TestModelsLocalInspectUnknownRendersCodedDiagnostic(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		flags []string
		debug bool
	}{
		{name: "normal"},
		{name: "json", flags: []string{"--json"}},
		{name: "debug", flags: []string{"--debug"}, debug: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			inputs, err := executeLocalUnknownModel(t, test.flags)
			if err == nil {
				t.Fatal("Process.Execute(models inspect) error = nil, want not-found failure")
			}
			if inputs.Stdout() != "" {
				t.Fatalf("models inspect failure stdout = %q, want empty", inputs.Stdout())
			}

			response := decodeFirstDiagnostic(t, inputs.Stderr())
			wantMessage := "model not found: " + codedDiagnosticUnknownModelName
			if response.Code != factoryapi.ErrorResponseCodeNOTFOUND ||
				response.Family != factoryapi.ErrorFamilyNotFound || response.Message != wantMessage {
				t.Fatalf("models inspect diagnostic = %#v, want code/family/message %q/%q/%q; stdout=%q stderr=%q", response, factoryapi.ErrorResponseCodeNOTFOUND, factoryapi.ErrorFamilyNotFound, wantMessage, inputs.Stdout(), inputs.Stderr())
			}
			for _, fallback := range []string{"CLI_COMMAND_FAILED", "INTERNAL_SERVER_ERROR", "command failed"} {
				if strings.Contains(inputs.Stderr(), fallback) {
					t.Fatalf("models inspect diagnostic contains fallback %q: %q", fallback, inputs.Stderr())
				}
			}
			if !errors.Is(err, modelscli.ErrModelNotFound) {
				t.Fatalf("Process.Execute(models inspect) error = %v, want ErrModelNotFound; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
			}
			if !errors.Is(err, modelservice.ErrNotFound) {
				t.Fatalf("Process.Execute(models inspect) error = %v, want underlying Models not-found cause; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
			}
			if test.debug && !strings.Contains(inputs.Stderr(), "debug: cause[1]=model not found: "+codedDiagnosticUnknownModelName) {
				t.Fatalf("debug models inspect diagnostic = %q, want underlying not-found cause", inputs.Stderr())
			}
		})
	}
}

// TestModelsLocalInspectUnknownMatchesHTTPDiagnostic proves local and HTTP inspection expose equivalent unknown-model diagnostics.
func TestModelsLocalInspectUnknownMatchesHTTPDiagnostic(t *testing.T) {
	t.Parallel()
	server := functionalNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/models/"+codedDiagnosticUnknownModelName {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(writer).Encode(factoryapi.ErrorResponse{
			Code:    factoryapi.ErrorResponseCodeNOTFOUND,
			Family:  factoryapi.ErrorFamilyNotFound,
			Message: "model not found",
		})
	}))
	t.Cleanup(server.Close)

	localInputs, localErr := executeLocalUnknownModel(t, nil)
	if localErr == nil {
		t.Fatal("local Process.Execute(models inspect) error = nil, want not-found failure")
	}
	localResponse := decodeFirstDiagnostic(t, localInputs.Stderr())

	process := functionalSharedDefaultProcess(t)
	remoteInputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", server.URL, "models", "inspect", codedDiagnosticUnknownModelName,
	})
	remoteInputs.Input.Env = functionalSharedDefaultEnvironment()
	remoteInputs.Input.WorkingDirectory = functionalTempDir(t)
	remoteErr := process.Execute(remoteInputs.Input)
	if remoteErr == nil || !errors.Is(remoteErr, modelscli.ErrModelNotFound) {
		t.Fatalf("remote Process.Execute(models inspect) error = %v, want ErrModelNotFound", remoteErr)
	}
	remoteResponse := decodeFirstDiagnostic(t, remoteInputs.Stderr())
	if localResponse.Code != remoteResponse.Code || localResponse.Family != remoteResponse.Family {
		t.Fatalf("local diagnostic = %#v, remote diagnostic = %#v, want code/family parity", localResponse, remoteResponse)
	}
	if localResponse.Code != factoryapi.ErrorResponseCodeNOTFOUND || localResponse.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("local diagnostic = %#v, want NOT_FOUND/NOT_FOUND", localResponse)
	}
	if !strings.Contains(localResponse.Message, codedDiagnosticUnknownModelName) {
		t.Fatalf("local diagnostic message = %q, want requested model name", localResponse.Message)
	}
}

func executeLocalMissingCache(t *testing.T, flags []string) (*support.CapturedInputs, error) {
	t.Helper()
	factoryDir := functionalScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	inputsArgs := append([]string{"you"}, flags...)
	inputsArgs = append(inputsArgs, "models", "remove", codedDiagnosticModelName)
	inputs := support.FakeInputs(t.Context(), inputsArgs)
	inputs.Input.Env = functionalSharedDefaultEnvironment()
	inputs.Input.WorkingDirectory = factoryDir

	process := functionalSharedDefaultProcess(t)
	return inputs, process.Execute(inputs.Input)
}

func executeLocalUnknownModel(t *testing.T, flags []string) (*support.CapturedInputs, error) {
	t.Helper()
	factoryDir := functionalScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	inputsArgs := append([]string{"you"}, flags...)
	inputsArgs = append(inputsArgs, "models", "inspect", codedDiagnosticUnknownModelName)
	inputs := support.FakeInputs(t.Context(), inputsArgs)
	inputs.Input.Env = functionalSharedDefaultEnvironment()
	inputs.Input.WorkingDirectory = factoryDir

	process := functionalSharedDefaultProcess(t)
	return inputs, process.Execute(inputs.Input)
}

func decodeFirstDiagnostic(t *testing.T, stderr string) factoryapi.ErrorResponse {
	t.Helper()
	firstLine := strings.SplitN(strings.TrimSpace(stderr), "\n", 2)[0]
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(firstLine), &response); err != nil {
		t.Fatalf("decode rendered diagnostic: %v\nstderr=%q", err, stderr)
	}
	return response
}
