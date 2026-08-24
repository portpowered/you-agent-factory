package root_composition_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelservice "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const codedDiagnosticModelName = "OMNIVOICE_Q4_K_M"
const codedDiagnosticUnknownModelName = "missing-model"

func TestModelsLocalInvokeJSONWithoutOutputIsValidationOnly(t *testing.T) {
	t.Parallel()

	factoryDir := support.ScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	homeDir := t.TempDir()

	for _, test := range []struct {
		name  string
		debug bool
	}{
		{name: "normal"},
		{name: "debug", debug: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := []string{
				"you", "models", "invoke", "asr",
				"--operation", "ASR", "--text", "x", "--json",
			}
			if test.debug {
				args = append(args, "--debug")
			}
			inputs := support.FakeInputs(t.Context(), args)
			inputs.Input.Env = functionalHomeEnvironment(homeDir)
			inputs.Input.WorkingDirectory = factoryDir

			err := process.Execute(inputs.Input)
			if err != nil {
				t.Fatalf("Process.Execute(models invoke asr) error = %v, want validation-only success", err)
			}
			for _, signal := range []string{`"mode":"VALIDATION_ONLY"`, `"validationOnly":true`, `"inferenceExecuted":false`} {
				if !strings.Contains(inputs.Stdout(), signal) {
					t.Fatalf("models invoke validation stdout = %q, missing %q", inputs.Stdout(), signal)
				}
			}
		})
	}

	pullInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "pull", "missing-pull-model", "--json",
	})
	pullInputs.Input.Env = functionalHomeEnvironment(homeDir)
	pullInputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(pullInputs.Input); err == nil {
		t.Fatal("Process.Execute(models pull missing-pull-model) error = nil, want not-found failure")
	}
	pullResponse := decodeFirstDiagnostic(t, pullInputs.Stderr())
	if pullResponse.Code != factoryapi.ErrorResponseCodeNOTFOUND || pullResponse.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("models pull missing-model diagnostic = %#v, want NOT_FOUND/NOT_FOUND", pullResponse)
	}
}

func TestModelsLocalRemoveMissingCacheRendersCodedDiagnostic(t *testing.T) {
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

func TestModelsLocalRemoveMissingCacheMatchesHTTPDiagnostic(t *testing.T) {
	const message = "model cache is not installed; run you models pull " + codedDiagnosticModelName + " first"

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	remoteInputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", server.URL, "models", "remove", codedDiagnosticModelName,
	})
	remoteInputs.Input.WorkingDirectory = t.TempDir()
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

func TestModelsLocalInspectUnknownRendersCodedDiagnostic(t *testing.T) {
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

func TestModelsLocalInspectUnknownMatchesHTTPDiagnostic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	remoteInputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", server.URL, "models", "inspect", codedDiagnosticUnknownModelName,
	})
	remoteInputs.Input.WorkingDirectory = t.TempDir()
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
	factoryDir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	cacheDirectory := t.TempDir()
	inputsArgs := append([]string{"you"}, flags...)
	inputsArgs = append(inputsArgs, "models", "remove", codedDiagnosticModelName)
	inputs := support.FakeInputs(t.Context(), inputsArgs)
	inputs.Input.Env = append(
		functionalHomeEnvironment(t.TempDir()),
		runcli.ModelCacheDirEnvironment+"="+cacheDirectory,
	)
	inputs.Input.WorkingDirectory = factoryDir

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	return inputs, process.Execute(inputs.Input)
}

func executeLocalUnknownModel(t *testing.T, flags []string) (*support.CapturedInputs, error) {
	t.Helper()
	factoryDir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	inputsArgs := append([]string{"you"}, flags...)
	inputsArgs = append(inputsArgs, "models", "inspect", codedDiagnosticUnknownModelName)
	inputs := support.FakeInputs(t.Context(), inputsArgs)
	inputs.Input.Env = functionalHomeEnvironment(t.TempDir())
	inputs.Input.WorkingDirectory = factoryDir

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
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
