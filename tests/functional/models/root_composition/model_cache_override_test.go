package root_composition_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const cacheSelectionPrompt = "cache selection payload"

// TestModelsModelCacheOverrideDirectInvokeUsesInvocationLocalCacheSelectionThroughRootProcess
// proves the full Process.Execute value path for the selected cache and the
// home-derived default. The model and backend are deterministic cached
// fixtures; no real model, backend, or network dependency is used.
func TestModelsModelCacheOverrideDirectInvokeUsesInvocationLocalCacheSelectionThroughRootProcess(t *testing.T) {
	t.Parallel()

	t.Run("override", func(t *testing.T) {
		t.Parallel()
		scenario := newCacheSelectionScenario(t, true, cacheSelectionProtocolClient{})
		response := executeCacheSelectionInvoke(t, scenario.process, scenario.factoryDir, scenario.environment)
		assertCacheSelectionSuccess(t, response)
		assertCacheSelectionEffects(t, scenario, scenario.cacheRoot)
	})

	t.Run("isolated-home-default", func(t *testing.T) {
		t.Parallel()
		scenario := newCacheSelectionScenario(t, false, cacheSelectionProtocolClient{})
		response := executeCacheSelectionInvoke(t, scenario.process, scenario.factoryDir, scenario.environment)
		assertCacheSelectionSuccess(t, response)
		assertCacheSelectionEffects(t, scenario, filepath.Join(scenario.home, ".agent-factory", "models"))
	})
}

// TestModelsModelCacheOverrideExplicitServerKeepsClientCacheOwnershipAndNormalizesFailures
// proves success and controlled backend failure parity across a direct local
// invocation and an explicit loopback Models server. The client process has a
// host launcher that fails if the remote command accidentally opens a local
// presentation scope.
func TestModelsModelCacheOverrideExplicitServerKeepsClientCacheOwnershipAndNormalizesFailures(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		protocol   cacheSelectionProtocolClient
		wantFailed bool
	}{
		{name: "success", protocol: cacheSelectionProtocolClient{}, wantFailed: false},
		{name: "backend-error", protocol: cacheSelectionProtocolClient{failure: errors.New("controlled backend protocol failure")}, wantFailed: true},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runCacheSelectionServerParity(t, test.protocol, test.wantFailed)
		})
	}
}

type cacheSelectionScenario struct {
	process     support.ApplicationProcess
	factoryDir  string
	home        string
	cacheRoot   string
	useOverride bool
	ambientRoot string
	environment []string
	trace       *cacheSelectionTrace
	launcher    *recordingModelHostLauncher
	network     *rejectingModelAssetHTTP
}

func newCacheSelectionScenario(
	t *testing.T,
	useOverride bool,
	protocol cacheSelectionProtocolClient,
) cacheSelectionScenario {
	t.Helper()
	home := functionalTempDir(t)
	cacheRoot := filepath.Join(functionalTempDir(t), "selected", "cache-root")
	ambientRoot := filepath.Join(functionalTempDir(t), "ambient-cache")
	modelRoot := filepath.Join(home, ".agent-factory", "models")
	if !useOverride {
		cacheRoot = modelRoot
	}
	writeCacheSelectionFixtures(t, cacheRoot)
	if err := os.MkdirAll(ambientRoot, 0o755); err != nil {
		t.Fatalf("create ambient cache root: %v", err)
	}
	ambientSentinel := filepath.Join(ambientRoot, "must-remain-untouched")
	if err := os.WriteFile(ambientSentinel, []byte("ambient"), 0o644); err != nil {
		t.Fatalf("write ambient cache sentinel: %v", err)
	}

	modelServer := functionalNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	trace := &cacheSelectionTrace{}
	files := cacheSelectionFileSystem{home: home, trace: trace}
	launcher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	network := &rejectingModelAssetHTTP{}
	factoryDir := functionalScaffoldFactory(t, singleOutputModelFactoryConfig(modelServer.URL))
	edges := cacheSelectionEdges(files, network, launcher, protocol, modelServer)
	process := functionalBuildProcess(t, edges)
	environment := isolatedModelEnvironment(home, "")
	if useOverride {
		environment = isolatedModelEnvironment(home, cacheRoot)
	}
	return cacheSelectionScenario{
		process: process, factoryDir: factoryDir, home: home, cacheRoot: cacheRoot,
		useOverride: useOverride,
		ambientRoot: ambientRoot, environment: environment, trace: trace,
		launcher: launcher, network: network,
	}
}

func runCacheSelectionServerParity(
	t *testing.T,
	protocol cacheSelectionProtocolClient,
	wantFailed bool,
) {
	t.Helper()
	local := newCacheSelectionScenario(t, true, protocol)
	localResponse, localErr := executeCacheSelectionInvokeResult(
		t, local.process, local.factoryDir, local.environment, "",
	)

	serverHome := functionalTempDir(t)
	serverCache := filepath.Join(functionalTempDir(t), "server", "cache-root")
	writeCacheSelectionFixtures(t, serverCache)
	modelServer := functionalNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)
	serverTrace := &cacheSelectionTrace{}
	serverFiles := cacheSelectionFileSystem{home: serverHome, trace: serverTrace}
	serverNetwork := &rejectingModelAssetHTTP{}
	serverLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	serverEdges := cacheSelectionEdges(serverFiles, serverNetwork, serverLauncher, protocol, modelServer)
	factoryDir := functionalScaffoldFactory(t, singleOutputModelFactoryConfig(modelServer.URL))
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       isolatedModelEnvironment(serverHome, serverCache),
		Edges:                     serverEdges,
	})

	clientHome := functionalTempDir(t)
	clientCache := filepath.Join(functionalTempDir(t), "client", "cache-root")
	clientLauncher := &cacheSelectionHostLauncherFailure{}
	clientTrace := &cacheSelectionTrace{}
	clientFiles := cacheSelectionFileSystem{home: clientHome, trace: clientTrace}
	clientProcess := functionalBuildProcess(t, cacheSelectionEdges(
		clientFiles, &rejectingModelAssetHTTP{}, clientLauncher, protocol, modelServer,
	))
	clientEnvironment := isolatedModelEnvironment(clientHome, clientCache)
	remoteResponse, remoteErr := executeCacheSelectionInvokeResult(
		t, clientProcess, factoryDir, clientEnvironment, server.URL(),
	)

	if wantFailed {
		assertCacheSelectionFailureParity(t, localErr, remoteErr, localResponse, remoteResponse)
	} else {
		if localErr != nil || remoteErr != nil {
			t.Fatalf("local/remote success errors = %v / %v", localErr, remoteErr)
		}
		assertCacheSelectionSuccess(t, localResponse)
		assertCacheSelectionSuccess(t, remoteResponse)
		if cacheSelectionObservation(localResponse) != cacheSelectionObservation(remoteResponse) {
			t.Fatalf("local/remote normalized observations differ: %#v / %#v", cacheSelectionObservation(localResponse), cacheSelectionObservation(remoteResponse))
		}
	}
	if clientLauncher.called {
		t.Fatal("explicit --server invocation opened the client local model host")
	}
	assertCacheRootAbsent(t, clientCache)
	assertCacheRootAbsent(t, filepath.Join(clientHome, ".agent-factory", "models"))
	assertNoTraceUnder(t, clientTrace, clientCache)
	assertNoTraceUnder(t, clientTrace, filepath.Join(clientHome, ".agent-factory", "models"))
	assertCacheRootEffects(t, serverTrace, serverCache)
	if serverNetwork.Calls() != 0 {
		t.Fatalf("server model-asset network calls = %d, want zero from controlled cache", serverNetwork.Calls())
	}
}

func cacheSelectionEdges(
	files cacheSelectionFileSystem,
	network *rejectingModelAssetHTTP,
	launcher interface {
		Start(context.Context, serviceedges.HostProcessStartSpec) (interface {
			HealthEndpoint() string
			Wait() error
			Stop(context.Context) error
		}, error)
	},
	protocol cacheSelectionProtocolClient,
	modelServer *httptest.Server,
) serviceedges.Edges {
	return serviceedges.Edges{
		ModelAssetHTTPClient: network,
		ModelResolveBackendArtifact: func(context.Context, serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
			return genericLlamaBackendSelection(), nil
		},
		ModelAssetMakeDirectories:      files.MkdirAll,
		ModelAssetInspectPath:          files.Stat,
		ModelAssetResolveHomeDirectory: files.UserHomeDir,
		ModelAssetResolveEnvironment:   func(string) string { return "" },
		ModelAssetWriteFile:            files.WriteFile,
		ModelAssetRenamePath:           files.Rename,
		ModelAssetRemovePath:           files.Remove,
		ModelAssetReadFile:             files.ReadFile,
		ModelAssetReadDirectory:        files.ReadDir,
		ModelAssetCreateFile:           files.Create,
		ModelAssetOpenFile:             files.Open,
		ModelHostProcessLauncher:       launcher,
		ModelHostProtocolNegotiator:    &joinedProtocolNegotiator{},
		ModelHostCompatibilityChecker:  &joinedCompatibilityChecker{},
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelHostHTTPClient:            modelServer.Client(),
		ModelRuntimeHTTPClient:         modelServer.Client(),
		ModelRuntimeInspectFile:        files.Stat,
		ModelInvocationProtocolClient:  protocol,
	}
}

func executeCacheSelectionInvoke(
	t *testing.T,
	process support.Process,
	factoryDir string,
	environment []string,
) factoryapi.GenericModelInvocationResponse {
	t.Helper()
	response, err := executeCacheSelectionInvokeResult(t, process, factoryDir, environment, "")
	if err != nil {
		t.Fatalf("Process.Execute(models invoke) error = %v", err)
	}
	return response
}

func executeCacheSelectionInvokeResult(
	t *testing.T,
	process support.Process,
	factoryDir string,
	environment []string,
	serverURL string,
) (factoryapi.GenericModelInvocationResponse, error) {
	t.Helper()
	args := []string{"you", "--json"}
	if strings.TrimSpace(serverURL) != "" {
		args = append(args, "--server", strings.TrimSuffix(serverURL, "/"))
	}
	input, err := json.Marshal(map[string]string{
		"name": "prompt", "modality": "TEXT", "contentType": "text/plain",
		"mediaType": "text/plain", "content": cacheSelectionPrompt,
	})
	if err != nil {
		t.Fatalf("marshal cache-selection input: %v", err)
	}
	args = append(args, "models", "invoke", models.BuiltInModelNameLLM, "--operation", models.OperationOMNI, "--input", string(input))
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append([]string(nil), environment...)
	inputs.Input.WorkingDirectory = factoryDir
	err = process.Execute(inputs.Input)
	var response factoryapi.GenericModelInvocationResponse
	if strings.TrimSpace(inputs.Stdout()) != "" {
		if decodeErr := json.Unmarshal([]byte(inputs.Stdout()), &response); decodeErr != nil && err == nil {
			t.Fatalf("decode cache-selection response: %v\nstdout=%s", decodeErr, inputs.Stdout())
		}
	}
	return response, err
}

func assertCacheSelectionSuccess(t *testing.T, response factoryapi.GenericModelInvocationResponse) {
	t.Helper()
	output, ok := cacheSelectionTextOutput(response)
	if response.Failure != nil || !ok {
		t.Fatalf("cache-selection response = %#v, want successful text output", response)
	}
	if output.Name != "text" || output.Content == nil || *output.Content != cacheSelectionPrompt {
		t.Fatalf("cache-selection output = %#v, want text %q", output, cacheSelectionPrompt)
	}
	if output.ContentType == nil || *output.ContentType != "text/plain" || output.MediaType == nil || *output.MediaType != "text/plain" {
		t.Fatalf("cache-selection output metadata = %#v, want text/plain", output)
	}
}

func cacheSelectionTextOutput(response factoryapi.GenericModelInvocationResponse) (factoryapi.ModelInvocationOutput, bool) {
	for _, output := range response.Outputs {
		if output.Name == "text" {
			return output, true
		}
	}
	return factoryapi.ModelInvocationOutput{}, false
}

type cacheSelectionNormalizedObservation struct {
	Name        string
	Modality    factoryapi.ModelInvocationContentType
	ContentType string
	MediaType   string
	Content     string
}

func cacheSelectionObservation(response factoryapi.GenericModelInvocationResponse) cacheSelectionNormalizedObservation {
	output, ok := cacheSelectionTextOutput(response)
	if !ok {
		return cacheSelectionNormalizedObservation{}
	}
	var contentType, mediaType, content string
	if output.ContentType != nil {
		contentType = *output.ContentType
	}
	if output.MediaType != nil {
		mediaType = *output.MediaType
	}
	if output.Content != nil {
		content = *output.Content
	}
	return cacheSelectionNormalizedObservation{
		Name: output.Name, Modality: output.Modality, ContentType: contentType,
		MediaType: mediaType, Content: content,
	}
}

func assertCacheSelectionEffects(t *testing.T, scenario cacheSelectionScenario, cacheRoot string) {
	t.Helper()
	if scenario.network.Calls() != 0 {
		t.Fatalf("model-asset network calls = %d, want zero from controlled cache", scenario.network.Calls())
	}
	assertCacheRootEffects(t, scenario.trace, cacheRoot)
	defaultRoot := filepath.Join(scenario.home, ".agent-factory", "models")
	if scenario.useOverride {
		assertCacheRootAbsent(t, defaultRoot)
	}
	if data, err := os.ReadFile(filepath.Join(scenario.ambientRoot, "must-remain-untouched")); err != nil || string(data) != "ambient" {
		t.Fatalf("ambient cache sentinel = %q, %v; want unchanged", data, err)
	}
	if scenario.launcher.Calls() == 0 {
		t.Fatal("controlled local invocation did not start the model host")
	}
	if scenario.useOverride {
		assertNoTraceUnder(t, scenario.trace, defaultRoot)
	}
}

func assertCacheRootEffects(t *testing.T, trace *cacheSelectionTrace, root string) {
	t.Helper()
	operations := trace.snapshot()
	if len(operations) == 0 {
		t.Fatal("model cache filesystem trace is empty")
	}
	for _, operation := range operations {
		if operation.kind == "read" || operation.kind == "stat" || operation.kind == "readdir" || operation.kind == "open" {
			continue
		}
		if !pathWithinRoot(operation.path, root) || (operation.other != "" && !pathWithinRoot(operation.other, root)) {
			t.Fatalf("cache filesystem mutation escaped selected root %q: %#v", root, operation)
		}
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("selected cache root %q: %v", root, err)
	}
}

func assertNoTraceUnder(t *testing.T, trace *cacheSelectionTrace, root string) {
	t.Helper()
	for _, operation := range trace.snapshot() {
		if pathWithinRoot(operation.path, root) || (operation.other != "" && pathWithinRoot(operation.other, root)) {
			t.Fatalf("filesystem trace touched forbidden cache root %q: %#v", root, operation)
		}
	}
}

func assertCacheRootAbsent(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cache root %q exists with error %v, want absent", root, err)
	}
}

func assertCacheSelectionFailureParity(
	t *testing.T,
	localErr, remoteErr error,
	localResponse, remoteResponse factoryapi.GenericModelInvocationResponse,
) {
	t.Helper()
	if localErr == nil || remoteErr == nil {
		t.Fatalf("local/remote failure errors = %v / %v, want both failures", localErr, remoteErr)
	}
	if len(localResponse.Outputs) != 0 || len(remoteResponse.Outputs) != 0 {
		t.Fatalf("failure responses published output: %#v / %#v", localResponse.Outputs, remoteResponse.Outputs)
	}
	localFailure := cacheSelectionFailure(t, localErr)
	remoteFailure := cacheSelectionFailure(t, remoteErr)
	if localFailure.Class != models.InvocationFailureClassBackendProtocol {
		t.Fatalf("local failure class = %s, want BACKEND_PROTOCOL", localFailure.Class)
	}
	if localFailure.Code != "MODEL_BACKEND_FAILURE" || remoteFailure.Code != "MODEL_BACKEND_FAILURE" {
		t.Fatalf("local/remote failure codes = %s / %s, want MODEL_BACKEND_FAILURE", localFailure.Code, remoteFailure.Code)
	}
	if localFailure.Family != factoryapi.ErrorFamilyInternalServerError || remoteFailure.Family != factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("local/remote failure families = %s / %s, want INTERNAL_SERVER_ERROR", localFailure.Family, remoteFailure.Family)
	}
}

type cacheSelectionFailureDetails struct {
	Class  models.InvocationFailureClass
	Code   string
	Family factoryapi.ErrorFamily
}

func cacheSelectionFailure(t *testing.T, err error) cacheSelectionFailureDetails {
	t.Helper()
	var failure *models.InvocationFailure
	_ = errors.As(err, &failure)
	var diagnostic interface {
		CLIErrorCode() string
		CLIErrorFamily() factoryapi.ErrorFamily
	}
	if !errors.As(err, &diagnostic) || diagnostic == nil {
		t.Fatalf("failure = %T %v, want CLI diagnostic", err, err)
	}
	details := cacheSelectionFailureDetails{Code: diagnostic.CLIErrorCode(), Family: diagnostic.CLIErrorFamily()}
	if failure != nil {
		details.Class = failure.Class
	}
	return details
}

func isolatedModelEnvironment(home, cacheRoot string) []string {
	keys := map[string]struct{}{
		"HOME": {}, "USERPROFILE": {}, "INFINITE_YOU_OMNIVOICE_CACHE_DIR": {},
		"HF_HOME": {}, "HUGGINGFACE_HUB_CACHE": {},
	}
	environment := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		key := strings.ToUpper(strings.TrimSpace(strings.SplitN(entry, "=", 2)[0]))
		if _, ok := keys[key]; ok {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment, "HOME="+home, "USERPROFILE="+home)
	if strings.TrimSpace(cacheRoot) != "" {
		environment = append(environment, runcli.ModelCacheDirEnvironment+"="+cacheRoot)
	}
	return environment
}

func pathWithinRoot(path, root string) bool {
	pathAbs, pathErr := filepath.Abs(filepath.Clean(path))
	rootAbs, rootErr := filepath.Abs(filepath.Clean(root))
	if pathErr != nil || rootErr != nil {
		return false
	}
	relative, err := filepath.Rel(rootAbs, pathAbs)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func writeCacheSelectionFixtures(t *testing.T, root string) {
	t.Helper()
	definition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameLLM)
	if !ok {
		t.Fatal("built-in LLM definition is unavailable")
	}
	writeCacheSelectionModelFixture(t, root, definition.Source)
	writeCacheSelectionBackendFixture(t, root, definition.Backend, genericLlamaBackendSelection(), []byte("localai-llamacpp/linux-amd64"))
}

func writeCacheSelectionModelFixture(t *testing.T, root, source string) {
	t.Helper()
	name := filepath.Base(strings.Split(strings.TrimSuffix(strings.TrimSpace(source), "@"), "@")[0])
	body := []byte("cache selection model fixture")
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	identity := fmt.Sprintf("model|%s|%s:%d:%s", source, name, len(body), digest)
	identityHash := fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
	snapshot := filepath.Join(root, ".you-content-addressed", "model", identityHash)
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatalf("create model fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, name), body, 0o644); err != nil {
		t.Fatalf("write model fixture: %v", err)
	}
	metadata := map[string]any{
		"kind": "model", "identity": identity, "source": source, "sourceKey": source,
		"artifacts": []map[string]any{{"Name": name, "Bytes": len(body), "SHA256": digest}},
	}
	writeCacheSelectionMetadata(t, filepath.Join(snapshot, ".you-assets.json"), metadata)
}

func writeCacheSelectionBackendFixture(
	t *testing.T,
	root, backend string,
	selection serviceedges.ModelBackendArtifactSelection,
	body []byte,
) {
	t.Helper()
	urlHash := fmt.Sprintf("%x", sha256.Sum256([]byte(selection.Location)))
	source := "backend://" + backend + "/release://" + urlHash
	identity := fmt.Sprintf("backend|%s|%s:%d:%s", source, selection.Name, selection.Bytes, selection.SHA256)
	identityHash := fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
	snapshot := filepath.Join(root, "backend-artifacts", ".you-content-addressed", "backend", identityHash)
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatalf("create backend fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, selection.Name), body, 0o644); err != nil {
		t.Fatalf("write backend fixture: %v", err)
	}
	metadata := map[string]any{
		"kind": "backend", "identity": identity, "source": source, "sourceKey": source,
		"artifacts": []map[string]any{{"Name": selection.Name, "Bytes": selection.Bytes, "SHA256": selection.SHA256}},
	}
	writeCacheSelectionMetadata(t, filepath.Join(snapshot, ".you-assets.json"), metadata)
}

func writeCacheSelectionMetadata(t *testing.T, path string, metadata map[string]any) {
	t.Helper()
	body, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal cache fixture metadata: %v", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write cache fixture metadata: %v", err)
	}
}

type cacheSelectionProtocolClient struct {
	failure error
}

func (client cacheSelectionProtocolClient) Predict(
	ctx context.Context,
	request models.InvocationProtocolRequest,
) (models.InvocationProtocolResponse, error) {
	if err := ctx.Err(); err != nil {
		return models.InvocationProtocolResponse{}, err
	}
	if client.failure != nil {
		return models.InvocationProtocolResponse{}, client.failure
	}
	value := request.Prompt
	if value == "" && len(request.Inputs) > 0 {
		value = request.Inputs[0].Content
	}
	return models.InvocationProtocolResponse{Text: value, Usage: value}, nil
}

type cacheSelectionTrace struct {
	mu         sync.Mutex
	operations []cacheSelectionOperation
}

type cacheSelectionOperation struct {
	kind  string
	path  string
	other string
}

func (trace *cacheSelectionTrace) record(kind, path, other string) {
	if trace == nil {
		return
	}
	trace.mu.Lock()
	trace.operations = append(trace.operations, cacheSelectionOperation{kind: kind, path: path, other: other})
	trace.mu.Unlock()
}

func (trace *cacheSelectionTrace) snapshot() []cacheSelectionOperation {
	if trace == nil {
		return nil
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	return append([]cacheSelectionOperation(nil), trace.operations...)
}

type cacheSelectionFileSystem struct {
	home  string
	trace *cacheSelectionTrace
}

func (filesystem cacheSelectionFileSystem) MkdirAll(path string, mode os.FileMode) error {
	filesystem.trace.record("mkdir", path, "")
	return os.MkdirAll(path, mode)
}
func (filesystem cacheSelectionFileSystem) Stat(path string) (os.FileInfo, error) {
	filesystem.trace.record("stat", path, "")
	return os.Stat(path)
}
func (filesystem cacheSelectionFileSystem) UserHomeDir() (string, error) { return filesystem.home, nil }
func (filesystem cacheSelectionFileSystem) WriteFile(path string, data []byte, mode os.FileMode) error {
	filesystem.trace.record("write", path, "")
	return os.WriteFile(path, data, mode)
}
func (filesystem cacheSelectionFileSystem) Rename(oldPath, newPath string) error {
	filesystem.trace.record("rename", oldPath, newPath)
	return os.Rename(oldPath, newPath)
}
func (filesystem cacheSelectionFileSystem) Remove(path string) error {
	filesystem.trace.record("remove", path, "")
	return os.Remove(path)
}
func (filesystem cacheSelectionFileSystem) ReadFile(path string) ([]byte, error) {
	filesystem.trace.record("read", path, "")
	return os.ReadFile(path)
}
func (filesystem cacheSelectionFileSystem) ReadDir(path string) ([]os.DirEntry, error) {
	filesystem.trace.record("readdir", path, "")
	return os.ReadDir(path)
}
func (filesystem cacheSelectionFileSystem) Create(path string) (io.WriteCloser, error) {
	filesystem.trace.record("create", path, "")
	return os.Create(path)
}
func (filesystem cacheSelectionFileSystem) Open(path string) (io.ReadCloser, error) {
	filesystem.trace.record("open", path, "")
	return os.Open(path)
}

type cacheSelectionHostLauncherFailure struct {
	called bool
}

func (launcher *cacheSelectionHostLauncherFailure) Start(
	context.Context,
	serviceedges.HostProcessStartSpec,
) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	launcher.called = true
	return nil, errors.New("client local model host must not start for explicit server")
}
