package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type invalidGenericCLIRequestCase struct {
	name        string
	arguments   func(string) []string
	wantMessage string
}

// TestModelsGenericCLIInvalidRequestsAreTypedAndEffectFree proves the public
// Process.Execute boundary classifies the selected invalid request forms as
// BAD_REQUEST before asset estimation, backend lifecycle, invocation, or
// mapped-output publication. Each case runs in normal and JSON command modes.
func TestModelsGenericCLIInvalidRequestsAreTypedAndEffectFree(t *testing.T) {
	t.Parallel()

	cases := []invalidGenericCLIRequestCase{
		{
			name: "unsupported operation",
			arguments: func(_ string) []string {
				return []string{"--operation", "ASR", "--input", "prompt=invalid operation", "--output", "result.txt"}
			},
			wantMessage: "unknown operation",
		},
		{
			name: "unknown input slot",
			arguments: func(_ string) []string {
				return []string{"--operation", "OMNI", "--input", "missing=value"}
			},
			wantMessage: "unknown input slot",
		},
		{
			name: "missing required input slot",
			arguments: func(_ string) []string {
				return []string{"--operation", "OMNI", "--input", "style=brief"}
			},
			wantMessage: "required input slot is missing",
		},
		{
			name: "duplicate input slot",
			arguments: func(_ string) []string {
				return []string{"--operation", "OMNI", "--input", "prompt=one", "--input", "prompt=two"}
			},
			wantMessage: "accepts at most one value",
		},
		{
			name: "malformed parameter",
			arguments: func(_ string) []string {
				return []string{"--operation", "OMNI", "--text", "invalid parameter", "--parameter", `{"name":"temperature","value":}`}
			},
			wantMessage: "parse --parameter 1",
		},
		{
			name: "unsupported parameter",
			arguments: func(_ string) []string {
				return []string{"--operation", "OMNI", "--text", "unsupported parameter", "--parameter", `{"name":"temperature","value":0.2}`}
			},
			wantMessage: "operation does not accept named parameters",
		},
		{
			name: "invalid output mapping",
			arguments: func(_ string) []string {
				return []string{"--operation", "OMNI", "--text", "invalid output", "--output-map", "other=result.txt"}
			},
			wantMessage: "unknown slot",
		},
		{
			name: "repeated unqualified output",
			arguments: func(outputDir string) []string {
				return []string{"--operation", "OMNI", "--text", "repeated output", "--output", filepath.Join(outputDir, "first.txt"), "--output", filepath.Join(outputDir, "second.txt")}
			},
			wantMessage: "after the first unqualified path",
		},
		{
			name: "mixed output forms",
			arguments: func(outputDir string) []string {
				return []string{"--operation", "OMNI", "--text", "mixed output", "--output", filepath.Join(outputDir, "answer.txt"), "--output-map", "text=" + filepath.Join(outputDir, "named.txt")}
			},
			wantMessage: "cannot be combined",
		},
	}
	fixture := newSharedInvalidGenericCLIProcess(t)

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			home := functionalTempDir(t)
			directory := functionalScaffoldFactory(t, invalidGenericCLIModelFactoryConfig(fixture.modelServerURL))
			outputDir := functionalTempDir(t)
			environment := functionalHomeEnvironment(home)
			assertInvalidGenericCLIRequest(t, fixture, testCase, directory, environment, outputDir)
		})
	}
}

func assertInvalidGenericCLIRequest(
	t *testing.T,
	fixture invalidGenericCLIProcess,
	testCase invalidGenericCLIRequestCase,
	directory string,
	environment []string,
	outputDir string,
) {
	t.Helper()
	before := fixture.effectSnapshot()
	for _, jsonMode := range []bool{false, true} {
		args := []string{"you"}
		if jsonMode {
			args = append(args, "--json")
		}
		args = append(args, "models", "invoke", "llm")
		args = append(args, testCase.arguments(outputDir)...)

		var stdout, stderr bytes.Buffer
		inputs := support.FakeInputs(context.Background(), args)
		inputs.Input.Env = environment
		inputs.Input.WorkingDirectory = directory
		inputs.Input.Stdout = &stdout
		inputs.Input.Stderr = &stderr
		err := fixture.execute(func() error { return fixture.process.Execute(inputs.Input) })
		if err == nil {
			t.Fatalf("Process.Execute(%s) returned nil, want invalid-request failure", map[bool]string{false: "human", true: "json"}[jsonMode])
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
	}
	fixture.assertNoEffectsSince(t, before)
}

type invalidGenericCLIProcess struct {
	process        support.ApplicationProcess
	modelServerURL string
	assetHome      string
	directory      string
	environment    []string
	executeMu      *sync.Mutex
	assetNetwork   *rejectingModelAssetHTTP
	launcher       *recordingModelHostLauncher
	protocol       *joinedProtocolNegotiator
	compatibility  *joinedCompatibilityChecker
	output         *genericCLIOutputFailureEffects
}

type invalidGenericCLIEffectSnapshot struct {
	assetNetwork  int
	launcher      int
	protocol      int
	compatibility int
	output        [4]int
}

func newInvalidGenericCLIProcess(t *testing.T, configFactory func(string) map[string]any) invalidGenericCLIProcess {
	t.Helper()
	fixture := buildInvalidGenericCLIProcess(t)
	fixture.directory = functionalScaffoldFactory(t, configFactory(fixture.modelServerURL))
	fixture.environment = functionalHomeEnvironment(fixture.assetHome)
	return fixture
}

func newSharedInvalidGenericCLIProcess(t *testing.T) invalidGenericCLIProcess {
	t.Helper()
	return buildInvalidGenericCLIProcess(t)
}

func buildInvalidGenericCLIProcess(t *testing.T) invalidGenericCLIProcess {
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
	fixture := invalidGenericCLIProcess{
		process: process, modelServerURL: modelServer.URL,
		assetHome:    home,
		executeMu:    &sync.Mutex{},
		assetNetwork: assetNetwork, launcher: launcher, protocol: protocol,
		compatibility: compatibility, output: output,
	}
	return fixture
}

func (fixture invalidGenericCLIProcess) execute(run func() error) error {
	fixture.executeMu.Lock()
	defer fixture.executeMu.Unlock()
	return run()
}

func (fixture invalidGenericCLIProcess) effectSnapshot() invalidGenericCLIEffectSnapshot {
	return invalidGenericCLIEffectSnapshot{
		assetNetwork:  fixture.assetNetwork.Calls(),
		launcher:      fixture.launcher.Calls(),
		protocol:      fixture.protocol.Calls(),
		compatibility: fixture.compatibility.Calls(),
		output:        fixture.output.Calls(),
	}
}

func (fixture invalidGenericCLIProcess) assertNoEffectsSince(t *testing.T, before invalidGenericCLIEffectSnapshot) {
	t.Helper()
	after := fixture.effectSnapshot()
	if after != before {
		t.Fatalf("invalid-request effects changed from %#v to %#v; want no per-scenario effects", before, after)
	}
}

func (fixture invalidGenericCLIProcess) assertNoEffects(t *testing.T) {
	t.Helper()
	fixture.assertNoEffectsSince(t, invalidGenericCLIEffectSnapshot{})
}

func (fixture invalidGenericCLIProcess) close(t *testing.T) {
	t.Helper()
	closeRootProcess(t, fixture.process, "close invalid generic CLI root process")
}

func invalidGenericCLIModelFactoryConfig(endpoint string) map[string]any {
	config := singleOutputModelFactoryConfig(endpoint)
	workers := config["workers"].([]map[string]any)
	operations := workers[0]["operations"].([]map[string]any)
	inputs := operations[0]["inputs"].([]map[string]any)
	operations[0]["inputs"] = append(inputs, map[string]any{
		"name": "style", "contentTypes": []string{"TEXT"}, "required": false,
	})
	workers[0]["operations"] = operations
	return config
}
