package root_composition_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestModelsInvokeCleanDirectoryUsesStandaloneModelsRoot proves the private
// root-composition fallback with the public Models CLI boundary. The working
// directory has no Factory layout; all model assets, host readiness and raw
// inference are controlled through edges.Edges.
func TestModelsInvokeCleanDirectoryUsesStandaloneModelsRoot(t *testing.T) {
	t.Parallel()
	const generated = "clean standalone invocation response"

	modelServer := functionalNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	home := functionalTempDir(t)
	workingDirectory := functionalTempDir(t)
	definition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameLLM)
	if !ok {
		t.Fatal("built-in catalog did not publish the LLM model definition")
	}
	writeGenericBuiltinModelCache(t, home, definition.Source)
	selection := genericLlamaBackendSelection()
	writeGenericBackendCache(t, home, definition.Backend, selection, []byte("localai-llamacpp/linux-amd64"))

	assetTrace := &functionalModelAssetTrace{}
	assetFiles := functionalModelAssetFileSystem{home: home, trace: assetTrace}
	rejectingNetwork := &rejectingModelAssetHTTP{}
	host := &recordingModelHostLauncher{endpoint: modelServer.URL}
	hostProtocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	protocol := &omniTextProtocolFixture{response: generated}
	factoryOpening := &cleanDirectoryFactoryOpeningProbe{}
	process := functionalBuildProcess(t, serviceedges.Edges{
		FactorySessionExecutionOpeningFileSystem: factoryOpening,
		FactorySessionResolveHomeDirectory: func() (string, error) {
			return home, nil
		},
		ModelAssetHTTPClient:           rejectingNetwork,
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
		ModelHostProcessLauncher:       host,
		ModelHostProtocolNegotiator:    hostProtocol,
		ModelHostCompatibilityChecker:  compatibility,
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelResolveBackendArtifact: func(_ context.Context, request serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
			if request.Backend != definition.Backend {
				return serviceedges.ModelBackendArtifactSelection{}, fmt.Errorf("unexpected clean-directory backend %q", request.Backend)
			}
			return selection, nil
		},
		ModelHostHTTPClient:           modelServer.Client(),
		ModelRuntimeHTTPClient:        modelServer.Client(),
		ModelInvocationProtocolClient: protocol,
	})
	defer closeRootProcess(t, process, "close clean-directory Models root process")

	var stdout, stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", models.BuiltInModelNameLLM,
		"--input", "prompt=clean standalone invocation",
	})
	inputs.Input.Env = cleanModelsEnvironment(home)
	inputs.Input.WorkingDirectory = workingDirectory
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(clean-directory Models invoke) error = %v stdout=%q stderr=%q assets=%v network=%d hostCalls=%d hostActive=%t protocol=%d", err, stdout.String(), stderr.String(), assetTrace.snapshot(), rejectingNetwork.Calls(), host.Calls(), host.Active(), protocol.Calls())
	}

	if stdout.String() != generated {
		t.Fatalf("clean-directory stdout = %q, want exact controlled response %q", stdout.String(), generated)
	}
	if rejectingNetwork.Calls() != 0 {
		t.Fatalf("clean-directory asset network calls = %d, want zero", rejectingNetwork.Calls())
	}
	if got := factoryOpening.Calls(); got != 0 {
		t.Fatalf("Factory execution-opening calls = %d, want zero for standalone invocation", got)
	}
	request := protocol.Request()
	if request.Operation != models.OperationOMNI || request.Prompt != "clean standalone invocation" || len(request.Inputs) != 1 ||
		request.Inputs[0].Slot != "prompt" || request.Inputs[0].Modality != models.ModalityText ||
		request.Inputs[0].MediaType != "text/plain" || request.Inputs[0].Content != request.Prompt {
		t.Fatalf("clean-directory protocol request = %#v, want normalized OMNI prompt", request)
	}
	if got := host.Calls(); got != 1 || host.Active() {
		t.Fatalf("clean-directory host lifecycle = calls:%d active:%t, want one exactly-released controlled host", got, host.Active())
	}
	if got := protocol.Calls(); got != 1 {
		t.Fatalf("clean-directory protocol calls = %d, want one controlled invocation", got)
	}
	t.Logf("clean-directory runtime proof: root=BuildProcess command=you models invoke %s --input prompt=<controlled> output=semantic text response=%q factoryExecutionOpens=%d assetNetworkCalls=%d hostStarts=%d activeHosts=%t protocolInvokes=%d", models.BuiltInModelNameLLM, stdout.String(), factoryOpening.Calls(), rejectingNetwork.Calls(), host.Calls(), host.Active(), protocol.Calls())
}

func cleanModelsEnvironment(home string) []string {
	environment := functionalHomeEnvironment(home)
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, runcli.ModelCacheDirEnvironment) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func (launcher *recordingModelHostLauncher) Active() bool {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.active
}

type cleanDirectoryFactoryOpeningProbe struct {
	mu    sync.Mutex
	calls int
}

func (probe *cleanDirectoryFactoryOpeningProbe) Getwd() (string, error) {
	probe.record()
	return "", errors.New("clean-directory test must not open Factory runtime")
}

func (probe *cleanDirectoryFactoryOpeningProbe) Stat(string) (fs.FileInfo, error) {
	probe.record()
	return nil, errors.New("clean-directory test must not inspect Factory runtime")
}

func (probe *cleanDirectoryFactoryOpeningProbe) record() {
	probe.mu.Lock()
	probe.calls++
	probe.mu.Unlock()
}

func (probe *cleanDirectoryFactoryOpeningProbe) Calls() int {
	probe.mu.Lock()
	defer probe.mu.Unlock()
	return probe.calls
}
