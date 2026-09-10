package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestModelsGenericCLIInvalidRequestsAreTypedAndEffectFree proves the public
// Process.Execute boundary classifies the selected invalid request forms as
// BAD_REQUEST before asset estimation, backend lifecycle, invocation, or
// mapped-output publication. Each case runs in normal and JSON command modes.
func TestModelsGenericCLIInvalidRequestsAreTypedAndEffectFree(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		configFactory func(string) map[string]any
		arguments     []string
		wantMessage   string
	}{
		{
			name:          "unsupported operation",
			configFactory: singleOutputModelFactoryConfig,
			arguments:     []string{"--operation", "ASR", "--input", "prompt=invalid operation", "--output", "result.txt"},
			wantMessage:   "unknown operation",
		},
		{
			name:          "unknown input slot",
			configFactory: singleOutputModelFactoryConfig,
			arguments:     []string{"--operation", "OMNI", "--input", "missing=value"},
			wantMessage:   "unknown input slot",
		},
		{
			name:          "missing required input slot",
			configFactory: missingRequiredGenericCLIModelFactoryConfig,
			arguments:     []string{"--operation", "OMNI", "--input", "style=brief"},
			wantMessage:   "required input slot is missing",
		},
		{
			name:          "duplicate input slot",
			configFactory: singleOutputModelFactoryConfig,
			arguments:     []string{"--operation", "OMNI", "--input", "prompt=one", "--input", "prompt=two"},
			wantMessage:   "accepts at most one value",
		},
		{
			name:          "malformed parameter",
			configFactory: singleOutputModelFactoryConfig,
			arguments:     []string{"--operation", "OMNI", "--text", "invalid parameter", "--parameter", `{"name":"temperature","value":}`},
			wantMessage:   "parse --parameter 1",
		},
		{
			name:          "invalid output mapping",
			configFactory: singleOutputModelFactoryConfig,
			arguments:     []string{"--operation", "OMNI", "--text", "invalid output", "--output-map", "other=result.txt"},
			wantMessage:   "unknown slot",
		},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fixture := newInvalidGenericCLIProcess(t, testCase.configFactory)
			for _, jsonMode := range []bool{false, true} {
				jsonMode := jsonMode
				t.Run(map[bool]string{false: "human", true: "json"}[jsonMode], func(t *testing.T) {
					args := []string{"you"}
					if jsonMode {
						args = append(args, "--json")
					}
					args = append(args, "models", "invoke", "llm")
					args = append(args, testCase.arguments...)

					var stdout, stderr bytes.Buffer
					inputs := support.FakeInputs(context.Background(), args)
					inputs.Input.Env = fixture.environment
					inputs.Input.WorkingDirectory = fixture.directory
					inputs.Input.Stdout = &stdout
					inputs.Input.Stderr = &stderr
					err := fixture.process.Execute(inputs.Input)
					if err == nil {
						t.Fatal("Process.Execute returned nil, want invalid-request failure")
					}
					if stdout.Len() != 0 {
						t.Fatalf("invalid request stdout = %q, want empty", stdout.String())
					}
					var diagnostic factoryapi.ErrorResponse
					if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(stderr.String())), &diagnostic); decodeErr != nil {
						t.Fatalf("decode invalid-request diagnostic: %v; stderr=%q; error=%v", decodeErr, stderr.String(), err)
					}
					if diagnostic.Code != factoryapi.ErrorResponseCode("BAD_REQUEST") || diagnostic.Family != factoryapi.ErrorFamilyBadRequest {
						t.Fatalf("invalid-request diagnostic = %#v, want BAD_REQUEST/BAD_REQUEST", diagnostic)
					}
					if !strings.Contains(diagnostic.Message, testCase.wantMessage) {
						t.Fatalf("invalid-request message = %q, want %q", diagnostic.Message, testCase.wantMessage)
					}
					if strings.Contains(diagnostic.Message, "CLI_COMMAND_FAILED") || strings.Contains(diagnostic.Message, "INTERNAL_SERVER_ERROR") {
						t.Fatalf("invalid-request diagnostic used generic fallback: %#v", diagnostic)
					}
					fixture.assertNoEffects(t)
				})
			}
			fixture.close(t)
		})
	}
}

type invalidGenericCLIProcess struct {
	process       support.ApplicationProcess
	directory     string
	environment   []string
	assetNetwork  *rejectingModelAssetHTTP
	launcher      *recordingModelHostLauncher
	protocol      *joinedProtocolNegotiator
	compatibility *joinedCompatibilityChecker
	output        *genericCLIOutputFailureEffects
}

func newInvalidGenericCLIProcess(
	t *testing.T,
	configFactory func(string) map[string]any,
) invalidGenericCLIProcess {
	t.Helper()
	modelServer := functionalNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)
	home := functionalTempDir(t)
	assetFiles := functionalModelAssetFileSystem{home: home}
	assetNetwork := &rejectingModelAssetHTTP{}
	launcher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	output := &genericCLIOutputFailureEffects{}
	selection := genericLlamaBackendSelection()
	directory := functionalScaffoldFactory(t, configFactory(modelServer.URL))
	process := functionalBuildProcess(t, serviceedges.Edges{
		ModelAssetHTTPClient:           assetNetwork,
		ModelAssetMakeDirectories:      assetFiles.MkdirAll,
		ModelAssetInspectPath:          assetFiles.Stat,
		ModelAssetResolveHomeDirectory: assetFiles.UserHomeDir,
		ModelAssetResolveEnvironment:   func(string) string { return "" },
		ModelAssetWriteFile:            assetFiles.WriteFile,
		ModelAssetRenamePath:           assetFiles.Rename,
		ModelAssetRemovePath:           assetFiles.Remove,
		ModelAssetReadFile:             assetFiles.ReadFile,
		ModelAssetReadDirectory:        assetFiles.ReadDir,
		ModelAssetCreateFile:           assetFiles.Create,
		ModelAssetOpenFile:             assetFiles.Open,
		ModelHostProcessLauncher:       launcher,
		ModelHostProtocolNegotiator:    protocol,
		ModelHostCompatibilityChecker:  compatibility,
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelResolveBackendArtifact: func(
			context.Context,
			serviceedges.ModelBackendArtifactSelectionRequest,
		) (serviceedges.ModelBackendArtifactSelection, error) {
			return selection, nil
		},
		ModelHostHTTPClient:           modelServer.Client(),
		ModelRuntimeHTTPClient:        modelServer.Client(),
		ModelInvocationProtocolClient: genericCLIProtocolClient{},
		ModelCLIOutputCreateTempFile:  output.CreateTemp,
		ModelCLIOutputInspectPath:     output.Inspect,
		ModelCLIOutputRemovePath:      output.Remove,
		ModelCLIOutputRenamePath:      output.Rename,
	})
	return invalidGenericCLIProcess{
		process: process, directory: directory, environment: functionalHomeEnvironment(home),
		assetNetwork: assetNetwork, launcher: launcher, protocol: protocol,
		compatibility: compatibility, output: output,
	}
}

func (fixture invalidGenericCLIProcess) assertNoEffects(t *testing.T) {
	t.Helper()
	if fixture.assetNetwork.Calls() != 0 || fixture.launcher.Calls() != 0 || fixture.protocol.Calls() != 0 || fixture.compatibility.Calls() != 0 {
		t.Fatalf("invalid-request effects = network:%d host:%d protocol:%d compatibility:%d; want all zero", fixture.assetNetwork.Calls(), fixture.launcher.Calls(), fixture.protocol.Calls(), fixture.compatibility.Calls())
	}
	if got := fixture.output.Calls(); got != [4]int{} {
		t.Fatalf("invalid-request output effects = %#v, want all zero", got)
	}
}

func (fixture invalidGenericCLIProcess) close(t *testing.T) {
	t.Helper()
	closeRootProcess(t, fixture.process, "close invalid generic CLI root process")
}

func missingRequiredGenericCLIModelFactoryConfig(endpoint string) map[string]any {
	config := singleOutputModelFactoryConfig(endpoint)
	workers := config["workers"].([]map[string]any)
	operations := workers[0]["operations"].([]map[string]any)
	inputs := operations[0]["inputs"].([]map[string]any)
	operations[0]["inputs"] = append(inputs, map[string]any{
		"name": "style", "contentTypes": []string{"TEXT"},
	})
	return config
}
