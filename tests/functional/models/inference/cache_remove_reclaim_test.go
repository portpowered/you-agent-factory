package inference_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformlocking "github.com/portpowered/infinite-you/pkg/platform/locking"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Removal has no Factory Session selector. The active-cache and HTTP parity
// observation windows are isolated first. Stop joins the hosted ~default
// session before local commands reuse that same identity and root process.
// Local removal cannot select a session, so this small cohort is sequential;
// the explicit-session inference cohort remains parallel.
func TestModelsCacheReclamationSharedProcess(t *testing.T) {
	t.Parallel()
	delegate, err := platformlocking.New(platformlocking.LocalFileSystem{})
	if err != nil {
		t.Fatal(err)
	}
	coordination := &removalMatrixCoordination{delegate: delegate, attempted: make(chan string, 1)}
	witness := newActiveASRRemovalWitness(t, coordination)
	t.Run("active lease refuses removal then release permits retry", func(t *testing.T) { runModelsActiveRemoval(t, witness) })
	t.Run("CLI and HTTP parity", func(t *testing.T) { runModelsRemovalParity(t, witness) })
	for _, test := range []struct {
		name string
		run  func(*testing.T)
	}{
		{"default revision only and repeat", func(t *testing.T) { runModelsDefaultRemoval(t, witness.server) }},
		{"shared backend retained once", func(t *testing.T) { runModelsSharedBackendRemoval(t, witness.server) }},
		{"unsafe reference preserves cache", func(t *testing.T) { runModelsUnsafeRemoval(t, witness.server) }},
		{"candidate lock failure preserves cache", func(t *testing.T) { runModelsLockFailureRemoval(t, witness.server) }},
		{"publication is observed after lock release", func(t *testing.T) { runModelsPublicationRemoval(t, witness.server, coordination) }},
	} {
		t.Run(test.name, test.run)
	}
}

func runModelsRemovalParity(t *testing.T, witness activeASRRemovalWitness) {
	t.Helper()

	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	localHome := functionalTempDir(t)
	localCache := filepath.Join(localHome, "model-cache")
	localFixture := writeDefaultASRRemovalFixture(t, localCache)
	localEnvironment := isolatedModelEnvironment(localHome, localCache)
	localEvidence := measureASRReclamationEvidence(t, localFixture)

	serverCache := filepath.Dir(filepath.Dir(witness.fixture.revisionPath))
	serverHome := filepath.Dir(serverCache)
	serverFixture := writeDefaultASRRemovalFixture(t, serverCache)
	serverEnvironment := isolatedModelEnvironment(serverHome, serverCache)
	serverEvidence := measureASRReclamationEvidence(t, serverFixture)
	if serverEvidence.modelBytes != localEvidence.modelBytes || serverEvidence.backendBytes != localEvidence.backendBytes {
		t.Fatalf("equivalent fixture snapshot bytes differ: local=%#v server=%#v", localEvidence, serverEvidence)
	}
	server := witness.server
	serverURL := server.URL()

	serverInputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", serverURL, "--json", "models", "remove", models.BuiltInModelNameASR,
		"--reclaim-unused-cache",
	})
	serverInputs.Input.Env = serverEnvironment
	serverInputs.Input.WorkingDirectory = factoryDir
	if err := server.Execute(t, serverInputs.Input); err != nil {
		t.Fatalf("Process.Execute(configured-server opt-in ASR remove) error = %v\nstdout=%s\nstderr=%s",
			err, serverInputs.Stdout(), serverInputs.Stderr())
	}
	var serverResponse factoryapi.ModelRemoveResponse
	if err := json.Unmarshal([]byte(serverInputs.Stdout()), &serverResponse); err != nil {
		t.Fatalf("decode configured-server opt-in remove response: %v\nstdout=%s", err, serverInputs.Stdout())
	}
	assertASRReclaimResponse(t, serverResponse, serverFixture, serverEvidence)
	assertASRReclaimEffects(t, serverFixture)
	serverRepeat := support.FakeInputs(t.Context(), []string{
		"you", "--server", serverURL, "--json", "models", "remove", models.BuiltInModelNameASR,
		"--reclaim-unused-cache",
	})
	serverRepeat.Input.Env = serverEnvironment
	serverRepeat.Input.WorkingDirectory = factoryDir
	serverRepeatErr := server.Execute(t, serverRepeat.Input)
	assertASRReclaimRepeatNotFound(t, "configured-server", serverRepeatErr, serverRepeat.Stdout(), serverRepeat.Stderr())
	server.Stop(t)
	localInputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "remove", models.BuiltInModelNameASR, "--reclaim-unused-cache",
	})
	localInputs.Input.Env = localEnvironment
	localInputs.Input.WorkingDirectory = factoryDir
	if err := witness.server.Execute(t, localInputs.Input); err != nil {
		t.Fatalf("Process.Execute(local opt-in ASR remove) error = %v\nstdout=%s\nstderr=%s",
			err, localInputs.Stdout(), localInputs.Stderr())
	}
	var localResponse factoryapi.ModelRemoveResponse
	if err := json.Unmarshal([]byte(localInputs.Stdout()), &localResponse); err != nil {
		t.Fatalf("decode local opt-in remove response: %v\nstdout=%s", err, localInputs.Stdout())
	}
	assertASRReclaimResponse(t, localResponse, localFixture, localEvidence)
	assertASRReclaimEffects(t, localFixture)

	localRepeat := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "remove", models.BuiltInModelNameASR, "--reclaim-unused-cache",
	})
	localRepeat.Input.Env = localEnvironment
	localRepeat.Input.WorkingDirectory = factoryDir
	localRepeatErr := witness.server.Execute(t, localRepeat.Input)
	assertASRReclaimRepeatNotFound(t, "local", localRepeatErr, localRepeat.Stdout(), localRepeat.Stderr())

	if localResponse.ModelName != serverResponse.ModelName || localResponse.Revision != serverResponse.Revision ||
		localResponse.Outcome != serverResponse.Outcome || localResponse.BytesRemoved != serverResponse.BytesRemoved ||
		int64Value(localResponse.ReclaimedCacheBytes) != int64Value(serverResponse.ReclaimedCacheBytes) ||
		int64Value(localResponse.RetainedSharedCacheBytes) != int64Value(serverResponse.RetainedSharedCacheBytes) {
		t.Fatalf("normalized local/configured-server responses differ: local=%#v server=%#v", localResponse, serverResponse)
	}

}

func runModelsSharedBackendRemoval(t *testing.T, server *support.FunctionalAPIServer) {
	t.Helper()

	home := functionalTempDir(t)
	cacheRoot := filepath.Join(home, "model-cache")
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	fixture := writeDefaultASRRemovalFixture(t, cacheRoot)
	managed, backend := readASRBackendReference(t, cacheRoot)
	writeASRPeerBackendReference(t, cacheRoot, "OTHER_MODEL_A", backend)
	writeASRPeerBackendReference(t, cacheRoot, "OTHER_MODEL_B", backend)
	modelBytes := story003RegularFileBytes(t, filepath.Dir(fixture.modelCASPath))
	backendBytes := story003RegularFileBytes(t, filepath.Dir(fixture.backendPath))

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "remove", models.BuiltInModelNameASR, "--reclaim-unused-cache",
	})
	inputs.Input.Env = isolatedModelEnvironment(home, cacheRoot)
	inputs.Input.WorkingDirectory = factoryDir
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("Process.Execute(shared opt-in remove) error = %v\nstdout=%s\nstderr=%s",
			err, inputs.Stdout(), inputs.Stderr())
	}
	var response factoryapi.ModelRemoveResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &response); err != nil {
		t.Fatalf("decode shared remove response: %v\nstdout=%s", err, inputs.Stdout())
	}
	if response.BytesRemoved != fixture.revisionBytes || int64Value(response.ReclaimedCacheBytes) != modelBytes ||
		int64Value(response.RetainedSharedCacheBytes) != backendBytes || response.ReclaimedCacheBytes == nil ||
		response.RetainedSharedCacheBytes == nil {
		t.Fatalf("shared remove response = %#v, want model reclaimed:%d backend retained once:%d managed:%#v",
			response, modelBytes, backendBytes, managed)
	}
	if _, err := os.Stat(fixture.revisionPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed ASR revision stat error = %v, want not-exist", err)
	}
	if _, err := os.Stat(filepath.Dir(fixture.modelCASPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unshared model snapshot stat error = %v, want not-exist", err)
	}
	if _, err := os.Stat(filepath.Dir(fixture.backendPath)); err != nil {
		t.Fatalf("multiply referenced backend snapshot stat error = %v, want retained", err)
	}
}

func runModelsActiveRemoval(t *testing.T, witness activeASRRemovalWitness) {
	t.Helper()

	defer witness.releaseBackend()

	invocation := support.FakeInputs(t.Context(), []string{
		"you", "--json", "--server", witness.server.URL(), "models", "invoke", models.BuiltInModelNameASR,
		"--operation", "ASR", "--input", "audio=@" + witness.inputPath,
	})
	invocation.Input.Env = witness.environment
	invocation.Input.WorkingDirectory = witness.factoryDir
	invocationDone := make(chan error, 1)
	go func() { invocationDone <- witness.server.Execute(t, invocation.Input) }()
	select {
	case <-witness.backendStarted:
	//nolint:testsleep // The test waits for an observed event; this deadline only bounds failure or cleanup.
	case <-time.After(5 * time.Second):
		select {
		case invokeErr := <-invocationDone:
			t.Fatalf("ASR invocation ended before controlled backend: %v stdout=%q stderr=%q", invokeErr, invocation.Stdout(), invocation.Stderr())
		default:
			t.Fatal("ASR invocation did not reach the controlled backend")
		}
	}
	if !witness.hostLauncher.Active() {
		t.Fatal("ASR managed host is not active while the backend call is blocked")
	}

	removeErr, removeStdout, removeStderr := executeASRRemoval(t, witness, "--reclaim-unused-cache")
	assertASRRemovalInUse(t, removeErr, removeStdout, removeStderr)
	assertASRRemovalFixtureRetained(t, witness.fixture)

	witness.releaseBackend()
	select {
	case invokeErr := <-invocationDone:
		assertActiveASRInvocationFinished(t, witness, invokeErr, invocation.Stdout())
	//nolint:testsleep // The test waits for an observed event; this deadline only bounds failure or cleanup.
	case <-time.After(30 * time.Second):
		t.Fatal("controlled ASR invocation did not finish after backend release")
	}
	removeErr, removeStdout, removeStderr = executeASRRemoval(t, witness, "--reclaim-unused-cache")
	if removeErr != nil {
		t.Fatalf("opt-in ASR removal after invocation release: %v stdout=%q stderr=%q", removeErr, removeStdout, removeStderr)
	}
	var removed factoryapi.ModelRemoveResponse
	if err := json.Unmarshal([]byte(removeStdout), &removed); err != nil {
		t.Fatalf("decode ASR removal after release: %v stdout=%q", err, removeStdout)
	}
	assertASRReclaimResponse(t, removed, witness.fixture, witness.evidence)
	assertASRReclaimEffects(t, witness.fixture)
	if witness.hostLauncher.Active() || witness.hostLauncher.StopCalls() != 1 {
		t.Fatalf("ASR host after released-cache removal = active:%t stops:%d, want inactive and exactly one stop", witness.hostLauncher.Active(), witness.hostLauncher.StopCalls())
	}
	t.Logf("active ASR removal proof: in-flight invocation returned MODEL_CACHE_IN_USE without mutation; after release, opt-in removal stopped the host once and reclaimed %d bytes", int64Value(removed.ReclaimedCacheBytes))
}

type activeASRRemovalWitness struct {
	server         *support.FunctionalAPIServer
	factoryDir     string
	environment    []string
	inputPath      string
	fixture        defaultASRRemovalFixture
	evidence       asrReclamationEvidence
	hostLauncher   *recordingModelHostLauncher
	backendStarted <-chan struct{}
	releaseBackend func()
}

func newActiveASRRemovalWitness(t *testing.T, coordination *removalMatrixCoordination) activeASRRemovalWitness {
	t.Helper()
	home := functionalTempDir(t)
	cacheRoot := filepath.Join(home, "model-cache")
	fixture := writeDefaultASRRemovalFixture(t, cacheRoot)
	evidence := measureASRReclamationEvidence(t, fixture)
	_, inputPath, _, _, _ := loadASRStoryFixture(t)
	modelServer := functionalNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	started := make(chan struct{})
	released := make(chan struct{})
	var startOnce, releaseOnce sync.Once
	backend := func(ctx context.Context, _ models.ASRBackendRequest) (models.ASRBackendResponse, error) {
		startOnce.Do(func() { close(started) })
		select {
		case <-released:
			return models.ASRBackendResponse{
				Text:     "controlled lease transcript",
				Segments: []models.ASRBackendSegment{{ID: 0, Start: 0, End: 1, Text: "controlled lease transcript"}},
			}, nil
		case <-ctx.Done():
			return models.ASRBackendResponse{}, ctx.Err()
		}
	}
	assetFiles := functionalModelAssetFileSystem{home: home}
	selection := pinnedASRBackendSelection()
	backendBody := []byte("controlled ASR backend snapshot")
	selection.Bytes = int64(len(backendBody))
	selection.SHA256 = fmt.Sprintf("%x", sha256.Sum256(backendBody))
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL, exclusive: true}
	edges := activeASRRemovalEdges(assetFiles, modelServer.Client(), hostLauncher, backend, selection)
	edges.ModelAssetStagingCoordinationFactory = func() (serviceedges.AssetStagingCoordination, error) { return coordination, nil }
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir, Env: isolatedModelEnvironment(home, cacheRoot),
		WaitForServiceModeRuntime: true, ServerReadyTimeout: 60 * time.Second,
		Edges: edges, BeforeStart: bootstrapFixedLeafProfile,
	})
	return activeASRRemovalWitness{
		server: server, factoryDir: functionalTempDir(t),
		environment: functionalHomeEnvironment(home), inputPath: inputPath,
		fixture: fixture, evidence: evidence, hostLauncher: hostLauncher,
		backendStarted: started, releaseBackend: func() { releaseOnce.Do(func() { close(released) }) },
	}
}

func activeASRRemovalEdges(
	assetFiles functionalModelAssetFileSystem,
	hostHTTP asrStoryHTTPClient,
	hostLauncher *recordingModelHostLauncher,
	backend serviceedges.ModelASRBackend,
	selection serviceedges.ModelBackendArtifactSelection,
) serviceedges.Edges {
	return serviceedges.Edges{
		ModelAssetHTTPClient:           &rejectingModelAssetHTTP{},
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
		ModelHostProcessLauncher:       hostLauncher,
		ModelHostProtocolNegotiator:    &joinedProtocolNegotiator{},
		ModelHostCompatibilityChecker:  &joinedCompatibilityChecker{},
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelResolveBackendArtifact: func(ctx context.Context, request serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
			if err := ctx.Err(); err != nil {
				return serviceedges.ModelBackendArtifactSelection{}, err
			}
			if request.Backend != "localai-whisper" {
				return serviceedges.ModelBackendArtifactSelection{}, fmt.Errorf("unexpected ASR backend %q", request.Backend)
			}
			return selection, nil
		},
		ModelASRBackend:     backend,
		ModelHostHTTPClient: hostHTTP,
	}
}

func executeASRRemoval(t *testing.T, witness activeASRRemovalWitness, flags ...string) (error, string, string) {
	t.Helper()
	args := []string{"you", "--server", witness.server.URL(), "--json", "models", "remove", models.BuiltInModelNameASR}
	args = append(args, flags...)
	inputs := support.FakeInputs(context.Background(), args)
	inputs.Input.Env = witness.environment
	inputs.Input.WorkingDirectory = witness.factoryDir
	err := witness.server.Execute(t, inputs.Input)
	return err, inputs.Stdout(), inputs.Stderr()
}

func assertASRRemovalInUse(t *testing.T, err error, stdout, stderr string) {
	t.Helper()
	if err == nil || !errors.Is(err, modelscli.ErrModelCacheInUse) {
		t.Fatalf("opt-in ASR removal during invocation = %v stdout=%q stderr=%q, want typed in-use conflict", err, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("in-use ASR removal stdout = %q, want empty", stdout)
	}
	diagnostic := decodeFirstDiagnostic(t, stderr)
	if diagnostic.Code != factoryapi.ErrorResponseCode("MODEL_CACHE_IN_USE") || diagnostic.Family != factoryapi.ErrorFamilyConflict {
		t.Fatalf("in-use ASR removal diagnostic = %#v, want MODEL_CACHE_IN_USE/CONFLICT", diagnostic)
	}
}

func assertASRRemovalFixtureRetained(t *testing.T, fixture defaultASRRemovalFixture) {
	t.Helper()
	for _, path := range []string{fixture.revisionPath, filepath.Dir(fixture.modelCASPath), filepath.Dir(fixture.backendPath)} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("cache path %q after in-use refusal: %v, want retained", path, err)
		}
	}
	if body, err := os.ReadFile(fixture.siblingPath); err != nil || string(body) != "another managed revision" {
		t.Fatalf("ASR sibling revision changed during in-use refusal: body=%q error=%v", body, err)
	}
}

func assertActiveASRInvocationFinished(t *testing.T, witness activeASRRemovalWitness, err error, stdout string) {
	t.Helper()
	if err != nil {
		t.Fatalf("controlled ASR invocation after backend release: %v stdout=%q", err, stdout)
	}
	var response factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		t.Fatalf("decode released ASR invocation: %v stdout=%q", err, stdout)
	}
	if response.Failure != nil || len(response.Outputs) < 1 || response.Outputs[0].Content == nil || *response.Outputs[0].Content != "controlled lease transcript" {
		t.Fatalf("released ASR invocation response = %#v, want controlled transcript", response)
	}
	if witness.hostLauncher.Calls() != 1 {
		t.Fatalf("ASR managed host starts = %d, want one during active invocation", witness.hostLauncher.Calls())
	}
}

func runModelsUnsafeRemoval(t *testing.T, server *support.FunctionalAPIServer) {
	t.Helper()

	home := functionalTempDir(t)
	cacheRoot := filepath.Join(home, "model-cache")
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	fixture := writeDefaultASRRemovalFixture(t, cacheRoot)
	_, backend := readASRBackendReference(t, cacheRoot)
	unsafe := make(map[string]any)
	backendBytes, err := json.Marshal(backend)
	if err != nil {
		t.Fatalf("marshal backend reference: %v", err)
	}
	if err := json.Unmarshal(backendBytes, &unsafe); err != nil {
		t.Fatalf("clone backend reference: %v", err)
	}
	// The malformed reference escapes this owned cache into a scenario-owned
	// sibling, so neither its resolution nor its sentinel depends on test cwd.
	outside := filepath.Join(home, "outside", "backend")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("outside cache sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	relativeOutside, err := filepath.Rel(cacheRoot, outside)
	if err != nil {
		t.Fatal(err)
	}
	unsafe["cachePath"] = relativeOutside
	writeASRPeerBackendReference(t, cacheRoot, "UNSAFE_MODEL", unsafe)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "remove", models.BuiltInModelNameASR, "--reclaim-unused-cache",
	})
	inputs.Input.Env = isolatedModelEnvironment(home, cacheRoot)
	inputs.Input.WorkingDirectory = factoryDir
	removeErr := server.Execute(t, inputs.Input)
	if removeErr == nil || !errors.Is(removeErr, modelscli.ErrModelCacheReferenceUncertain) {
		t.Fatalf("unsafe-reference remove = %v, want ErrModelCacheReferenceUncertain; stdout=%q stderr=%q",
			removeErr, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stdout() != "" {
		t.Fatalf("unsafe-reference remove stdout = %q, want no success response", inputs.Stdout())
	}
	diagnostic := decodeFirstDiagnostic(t, inputs.Stderr())
	if diagnostic.Code != factoryapi.ErrorResponseCode("MODEL_CACHE_REFERENCE_UNCERTAIN") ||
		diagnostic.Family != factoryapi.ErrorFamilyConflict {
		t.Fatalf("unsafe-reference diagnostic = %#v, want typed conflict", diagnostic)
	}
	for _, path := range []string{fixture.revisionPath, filepath.Dir(fixture.modelCASPath), filepath.Dir(fixture.backendPath)} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("fail-closed path %q stat error = %v, want retained", path, err)
		}
	}
	if contents, err := os.ReadFile(outside); err != nil || string(contents) != "outside cache sentinel" {
		t.Fatalf("unsafe reference changed outside-cache data: contents=%q error=%v", contents, err)
	}

}

func runModelsLockFailureRemoval(t *testing.T, server *support.FunctionalAPIServer) {
	t.Helper()

	home := functionalTempDir(t)
	cacheRoot := filepath.Join(home, "candidate-lock-failure")
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	fixture := writeDefaultASRRemovalFixture(t, cacheRoot)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "remove", models.BuiltInModelNameASR, "--reclaim-unused-cache",
	})
	inputs.Input.Env = isolatedModelEnvironment(home, cacheRoot)
	inputs.Input.WorkingDirectory = factoryDir
	removeErr := server.Execute(t, inputs.Input)
	if removeErr == nil || !errors.Is(removeErr, modelscli.ErrModelCacheReferenceUncertain) {
		t.Fatalf("candidate-lock remove error = %v, want ErrModelCacheReferenceUncertain; stdout=%q stderr=%q",
			removeErr, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stdout() != "" {
		t.Fatalf("candidate-lock remove stdout = %q, want no success response", inputs.Stdout())
	}
	diagnostic := decodeFirstDiagnostic(t, inputs.Stderr())
	if diagnostic.Code != factoryapi.ErrorResponseCode("MODEL_CACHE_REFERENCE_UNCERTAIN") ||
		diagnostic.Family != factoryapi.ErrorFamilyConflict {
		t.Fatalf("candidate-lock diagnostic = %#v, want typed conflict", diagnostic)
	}
	for _, path := range []string{fixture.revisionPath, filepath.Dir(fixture.modelCASPath), filepath.Dir(fixture.backendPath)} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("candidate-lock failure path %q stat error = %v, want retained", path, err)
		}
	}
}

func runModelsPublicationRemoval(t *testing.T, server *support.FunctionalAPIServer, coordination *removalMatrixCoordination) {
	t.Helper()

	home := functionalTempDir(t)
	cacheRoot := filepath.Join(home, "reference-publication")
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	fixture := writeDefaultASRRemovalFixture(t, cacheRoot)
	_, backend := readASRBackendReference(t, cacheRoot)
	lockPath := filepath.Join(cacheRoot, ".you-asset-locks", "references.lock")
	owner, err := coordination.delegate.Lock(t.Context(), lockPath)
	if err != nil {
		t.Fatalf("acquire reference publication owner: %v", err)
	}
	defer owner.Close()

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "remove", models.BuiltInModelNameASR, "--reclaim-unused-cache",
	})
	inputs.Input.Env = isolatedModelEnvironment(home, cacheRoot)
	inputs.Input.WorkingDirectory = factoryDir
	removeDone := make(chan error, 1)
	joined := make(chan struct{})
	go func() { defer close(joined); removeDone <- server.Execute(t, inputs.Input) }()
	t.Cleanup(func() { _ = owner.Close(); <-joined })
	select {
	case <-coordination.attempted:
	//nolint:testsleep // The test waits for an observed event; this deadline only bounds failure or cleanup.
	case <-time.After(5 * time.Second):
		t.Fatal("opt-in removal did not attempt reference coordination")
	}
	writeASRPeerBackendReference(t, cacheRoot, "CONCURRENT_MODEL", backend)
	if err := owner.Close(); err != nil {
		t.Fatalf("release reference publication owner: %v", err)
	}
	select {
	case err := <-removeDone:
		if err != nil {
			t.Fatalf("Process.Execute(coordinated remove) error = %v\nstdout=%s\nstderr=%s", err, inputs.Stdout(), inputs.Stderr())
		}
	//nolint:testsleep // The test waits for an observed event; this deadline only bounds failure or cleanup.
	case <-time.After(30 * time.Second):
		t.Fatal("coordinated remove did not finish")
	}
	var response factoryapi.ModelRemoveResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &response); err != nil {
		t.Fatalf("decode coordinated remove response: %v\nstdout=%s", err, inputs.Stdout())
	}
	if int64Value(response.RetainedSharedCacheBytes) != story003RegularFileBytes(t, filepath.Dir(fixture.backendPath)) {
		t.Fatalf("coordinated remove did not observe the published backend reference: %#v", response)
	}
}

// The edge shape is fixed once; only the public cache pathname selects a fault.
// Reference publication owns its gate and all other cache roots delegate unchanged.
type removalMatrixCoordination struct {
	delegate  serviceedges.AssetStagingCoordination
	attempted chan string
	once      sync.Once
}

func (c *removalMatrixCoordination) Lock(ctx context.Context, path string) (io.Closer, error) {
	normalized := filepath.ToSlash(path)
	if strings.Contains(normalized, "/candidate-lock-failure/") && filepath.Base(path) != "references.lock" {
		return nil, errors.New("candidate cache lock unavailable")
	}
	if strings.Contains(normalized, "/reference-publication/") && filepath.Base(path) == "references.lock" {
		c.once.Do(func() { c.attempted <- path })
	}
	return c.delegate.Lock(ctx, path)
}

func readASRBackendReference(t *testing.T, cacheRoot string) (map[string]any, map[string]any) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(cacheRoot, "ASR", ".managed-cache.json"))
	if err != nil {
		t.Fatalf("read ASR managed metadata: %v", err)
	}
	var managed map[string]any
	if err := json.Unmarshal(body, &managed); err != nil {
		t.Fatalf("decode ASR managed metadata: %v", err)
	}
	backend, ok := managed["backend"].(map[string]any)
	if !ok {
		t.Fatalf("ASR backend reference = %#v, want object", managed["backend"])
	}
	return managed, backend
}

func writeASRPeerBackendReference(t *testing.T, cacheRoot, modelName string, backend any) {
	t.Helper()
	modelRoot := filepath.Join(cacheRoot, modelName)
	revisionPath := filepath.Join(modelRoot, "peer-revision")
	writeDefaultASRFile(t, filepath.Join(revisionPath, "peer.bin"), []byte("peer"))
	writeCacheSelectionMetadata(t, filepath.Join(modelRoot, ".managed-cache.json"), map[string]any{
		"modelName": modelName,
		"revision":  "peer-revision",
		"files": []map[string]any{{
			"path": "peer.bin", "bytes": 4, "sha256": fmt.Sprintf("%x", sha256.Sum256([]byte("peer"))),
		}},
		"backend": backend,
	})
}

type asrReclamationEvidence struct {
	modelBytes   int64
	backendBytes int64
}

func measureASRReclamationEvidence(t *testing.T, fixture defaultASRRemovalFixture) asrReclamationEvidence {
	t.Helper()
	evidence := asrReclamationEvidence{
		modelBytes:   story003RegularFileBytes(t, filepath.Dir(fixture.modelCASPath)),
		backendBytes: story003RegularFileBytes(t, filepath.Dir(fixture.backendPath)),
	}
	if evidence.modelBytes <= 0 || evidence.backendBytes <= 0 {
		t.Fatalf("ASR fixture cache measurements must be positive: %#v", evidence)
	}
	return evidence
}

func assertASRReclaimResponse(
	t *testing.T,
	response factoryapi.ModelRemoveResponse,
	fixture defaultASRRemovalFixture,
	evidence asrReclamationEvidence,
) {
	t.Helper()
	if response.ModelName != "ASR" || response.Revision != fixture.revision ||
		response.CachePath != fixture.revisionPath || response.BytesRemoved != fixture.revisionBytes ||
		response.Outcome != factoryapi.REMOVED || int64Value(response.ReclaimedCacheBytes) != evidence.modelBytes+evidence.backendBytes ||
		int64Value(response.RetainedSharedCacheBytes) != 0 {
		t.Fatalf("ASR opt-in remove response = %#v, want revision bytes %d and separate reclaimed bytes %d + %d",
			response, fixture.revisionBytes, evidence.modelBytes, evidence.backendBytes)
	}
	if response.ReclaimedCacheBytes == nil || response.RetainedSharedCacheBytes == nil {
		t.Fatalf("opt-in ASR response omitted cache byte fields: %#v", response)
	}
}

func assertASRReclaimEffects(t *testing.T, fixture defaultASRRemovalFixture) {
	t.Helper()
	for _, path := range []string{
		fixture.revisionPath,
		filepath.Dir(fixture.modelCASPath),
		filepath.Dir(fixture.backendPath),
	} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("reclaimed ASR path %q stat error = %v, want not-exist", path, err)
		}
	}
	if body, err := os.ReadFile(fixture.siblingPath); err != nil || string(body) != "another managed revision" {
		t.Fatalf("ASR sibling revision changed: body=%q error=%v", body, err)
	}
}

func assertASRReclaimRepeatNotFound(t *testing.T, label string, err error, stdout, stderr string) {
	t.Helper()
	if err == nil || !errors.Is(err, modelscli.ErrModelCacheNotFound) {
		t.Fatalf("%s repeated opt-in remove error = %v, want typed cache-not-found", label, err)
	}
	if stdout != "" {
		t.Fatalf("%s repeated opt-in remove stdout = %q, want empty", label, stdout)
	}
	diagnostic := decodeFirstDiagnostic(t, stderr)
	if diagnostic.Code != factoryapi.ErrorResponseCodeMODELCACHENOTFOUND || diagnostic.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("%s repeated opt-in remove diagnostic = %#v, want MODEL_CACHE_NOT_FOUND/NOT_FOUND", label, diagnostic)
	}
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
