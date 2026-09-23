package root_composition_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const zeroConfigFirstUsePrompt = "Explain how a SHA-256 checksum differs from encryption and which provides confidentiality."

const zeroConfigFirstUseResponse = "A SHA-256 checksum detects changes to data but does not hide it. Encryption uses a key to protect data and provides confidentiality."

func TestModelsInvokeCleanDirectorySeparatesFactoryDebugAndProtocolFailure(t *testing.T) {
	t.Parallel()
	const (
		protocolFaultInvocation = 4
		cells                   = 5
	)
	scenario := newZeroConfigFirstUseScenario(t, protocolFaultInvocation)

	// Keep these cells sequential so every factor uses the same cache and the recovery call follows its fault.
	for _, cell := range []zeroConfigFirstUseCell{
		{name: "F-01 blank directory debug off"},
		{name: "F-02 minimal Factory debug off", factory: true},
		{name: "F-03 blank directory debug on", debug: true},
		{name: "F-04 blank directory protocol failure", failure: true},
		{name: "F-04 recovery in blank directory"},
	} {
		t.Run(cell.name, func(t *testing.T) { runZeroConfigFirstUseCell(t, scenario, cell) })
	}
	if scenario.protocol.Calls() != cells {
		t.Fatalf("controlled protocol calls = %d, want one per matrix/recovery cell (%d)", scenario.protocol.Calls(), cells)
	}
}

type zeroConfigFirstUseCell struct {
	name    string
	factory bool
	debug   bool
	failure bool
}

type zeroConfigFirstUseScenario struct {
	process          support.ApplicationProcess
	home             string
	blankDirectory   string
	factoryDirectory string
	network          *rejectingModelAssetHTTP
	launcher         *recordingModelHostLauncher
	protocol         *zeroConfigFirstUseProtocolFixture
}

func newZeroConfigFirstUseScenario(t *testing.T, failureAtCall int) zeroConfigFirstUseScenario {
	t.Helper()
	modelServer := functionalNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	definition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameLLM)
	if !ok {
		t.Fatal("built-in catalog did not publish the LLM model definition")
	}
	home := functionalTempDir(t)
	selection := genericLlamaBackendSelection()
	writeGenericBuiltinModelCache(t, home, definition.Source)
	writeGenericBackendCache(t, home, definition.Backend, selection, []byte("localai-llamacpp/linux-amd64"))

	network := &rejectingModelAssetHTTP{}
	assetFiles := functionalModelAssetFileSystem{home: home, trace: &functionalModelAssetTrace{}}
	launcher := &recordingModelHostLauncher{endpoint: modelServer.URL, exclusive: true}
	protocol := &zeroConfigFirstUseProtocolFixture{
		response: zeroConfigFirstUseResponse, failure: errors.New("controlled backend protocol failure"), failureAtCall: failureAtCall,
	}
	process := functionalBuildProcess(t, serviceedges.Edges{
		FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
		ModelAssetHTTPClient:               network,
		ModelAssetMakeDirectories:          assetFiles.MkdirAll,
		ModelAssetInspectPath:              assetFiles.Stat,
		ModelAssetResolveHomeDirectory:     assetFiles.UserHomeDir,
		ModelAssetResolveEnvironment:       func(string) string { return "" },
		ModelAssetWriteFile:                assetFiles.WriteFile,
		ModelAssetRenamePath:               assetFiles.Rename,
		ModelAssetRemovePath:               assetFiles.Remove,
		ModelAssetReadFile:                 assetFiles.ReadFile,
		ModelAssetReadDirectory:            assetFiles.ReadDir,
		ModelAssetCreateFile:               assetFiles.Create,
		ModelAssetOpenFile:                 assetFiles.Open,
		ModelHostProcessLauncher:           launcher,
		ModelHostProtocolNegotiator:        &joinedProtocolNegotiator{},
		ModelHostCompatibilityChecker:      &joinedCompatibilityChecker{},
		ModelAssetHostPlatform:             models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelResolveBackendArtifact: func(_ context.Context, request serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
			if request.Backend != definition.Backend {
				return serviceedges.ModelBackendArtifactSelection{}, fmt.Errorf("unexpected backend %q", request.Backend)
			}
			return selection, nil
		},
		ModelHostHTTPClient:           modelServer.Client(),
		ModelRuntimeHTTPClient:        modelServer.Client(),
		ModelInvocationProtocolClient: protocol,
	})
	return zeroConfigFirstUseScenario{
		process: process, home: home, blankDirectory: functionalTempDir(t),
		factoryDirectory: functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig()),
		network:          network, launcher: launcher, protocol: protocol,
	}
}

func runZeroConfigFirstUseCell(t *testing.T, scenario zeroConfigFirstUseScenario, cell zeroConfigFirstUseCell) {
	t.Helper()
	args := []string{"you", "models", "invoke", models.BuiltInModelNameLLM, "--offline", "--operation", models.OperationOMNI, "--input", "prompt=" + zeroConfigFirstUsePrompt}
	if cell.debug {
		args = append([]string{"you", "--debug"}, args[1:]...)
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = cleanModelsEnvironment(scenario.home)
	inputs.Input.WorkingDirectory = scenario.blankDirectory
	if cell.factory {
		inputs.Input.WorkingDirectory = scenario.factoryDirectory
	}
	var stdout, stderr bytes.Buffer
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr

	protocolBefore := scenario.protocol.Calls()
	stopsBefore := scenario.launcher.StopCalls()
	err := scenario.process.Execute(inputs.Input)
	assertZeroConfigFirstUseStage(t, scenario, protocolBefore, stopsBefore)
	assertZeroConfigFirstUseRequest(t, scenario.protocol.Request())
	diagnosticCode := assertZeroConfigFirstUseOutcome(t, cell, err, stdout.String(), stderr.String())
	t.Logf("cell=%q factory=%t debug=%t stage=protocol-result output=%q stderr=%q typedError=%s hostRelease=stopped", cell.name, cell.factory, cell.debug, stdout.String(), stderr.String(), diagnosticCode)
}

func assertZeroConfigFirstUseStage(t *testing.T, scenario zeroConfigFirstUseScenario, protocolBefore, stopsBefore int) {
	t.Helper()
	if scenario.protocol.Calls() != protocolBefore+1 {
		t.Fatalf("backend protocol attempts = %d→%d, want one result for this CLI invocation", protocolBefore, scenario.protocol.Calls())
	}
	if scenario.network.Calls() != 0 {
		t.Fatalf("offline asset network calls = %d, want zero", scenario.network.Calls())
	}
	if scenario.launcher.Active() || scenario.launcher.StopCalls() <= stopsBefore {
		t.Fatalf("host release = stops %d→%d active %t; want no active host after invocation", stopsBefore, scenario.launcher.StopCalls(), scenario.launcher.Active())
	}
}

func assertZeroConfigFirstUseRequest(t *testing.T, request models.InvocationProtocolRequest) {
	t.Helper()
	if request.Operation != models.OperationOMNI || request.Prompt != zeroConfigFirstUsePrompt || len(request.Inputs) != 1 ||
		request.Inputs[0].Slot != "prompt" || request.Inputs[0].Content != zeroConfigFirstUsePrompt {
		t.Fatalf("controlled backend request = %#v, want the unchanged OMNI prompt", request)
	}
}

func assertZeroConfigFirstUseOutcome(t *testing.T, cell zeroConfigFirstUseCell, err error, stdout, stderr string) string {
	t.Helper()
	if cell.failure {
		return assertZeroConfigFirstUseFailure(t, err, stdout, stderr)
	}
	if err != nil {
		t.Fatalf("Process.Execute(%s) = %v, stdout=%q stderr=%q", cell.name, err, stdout, stderr)
	}
	if stdout != zeroConfigFirstUseResponse || !strings.Contains(stdout, "checksum") || !strings.Contains(stdout, "confidentiality") {
		t.Fatalf("Process.Execute(%s) stdout = %q, want the controlled meaningful text %q", cell.name, stdout, zeroConfigFirstUseResponse)
	}
	return "none"
}

func assertZeroConfigFirstUseFailure(t *testing.T, err error, stdout, stderr string) string {
	t.Helper()
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendProtocol || failure.Operation != models.OperationOMNI {
		t.Fatalf("Process.Execute(protocol fault) = %v, want typed OMNI backend-protocol failure", err)
	}
	if stdout != "" {
		t.Fatalf("protocol failure stdout = %q, want empty output", stdout)
	}
	diagnostic := decodeFirstDiagnostic(t, stderr)
	if diagnostic.Code != "MODEL_BACKEND_FAILURE" || diagnostic.Family != factoryapi.ErrorFamilyInternalServerError || strings.TrimSpace(diagnostic.Message) == "" {
		t.Fatalf("protocol failure diagnostic = %#v, want typed actionable backend failure", diagnostic)
	}
	return string(diagnostic.Code)
}

type zeroConfigFirstUseProtocolFixture struct {
	mu            sync.Mutex
	request       models.InvocationProtocolRequest
	response      string
	failure       error
	failureAtCall int
	calls         int
}

func (fixture *zeroConfigFirstUseProtocolFixture) Predict(
	ctx context.Context,
	request models.InvocationProtocolRequest,
) (models.InvocationProtocolResponse, error) {
	if err := ctx.Err(); err != nil {
		return models.InvocationProtocolResponse{}, err
	}
	fixture.mu.Lock()
	fixture.calls++
	fixture.request = request
	failure := fixture.failure
	shouldFail := fixture.failureAtCall == fixture.calls
	response := fixture.response
	fixture.mu.Unlock()
	if shouldFail {
		return models.InvocationProtocolResponse{}, failure
	}
	return models.InvocationProtocolResponse{Text: response}, nil
}

func (fixture *zeroConfigFirstUseProtocolFixture) Request() models.InvocationProtocolRequest {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	request := fixture.request
	request.Inputs = append([]models.InvocationProtocolInput(nil), request.Inputs...)
	return request
}

func (fixture *zeroConfigFirstUseProtocolFixture) Calls() int {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.calls
}
