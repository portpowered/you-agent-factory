package tts

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

// managedFactoryTTSFixture reuses the immutable process and model edge for
// the Models live/replay rows. Each live/replay pair still receives a fresh
// customer home, Factory copy, cache copy, recording, and backend state so
// replay and artifact lineage remain scenario-local.
type managedFactoryTTSFixture struct {
	process        support.ApplicationProcess
	backend        *packagedTTSModelsBackend
	factorySeedDir string
	configSeed     []byte
	cacheSeedDir   string

	launcher *packagedTTSModelHostLauncher

	mu              sync.Mutex
	processBuilds   int
	factoryInstalls int
	factoryCopies   int
	cacheSeeds      int
	cacheCopies     int
	recordings      int
}

func newManagedFactoryTTSFixture(t *testing.T) *managedFactoryTTSFixture {
	t.Helper()

	backend := newPackagedTTSModelsBackend([]byte(packagedTTSFakeAudioFixture))
	privateFixture := localai.Start(t, localai.Options{TTSFailureText: "models-backed factory tts failure"})
	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	launcher := &packagedTTSModelHostLauncher{endpoint: privateFixture.Endpoint()}
	edges := serviceedges.Edges{
		ModelAssetHostPlatform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelResolveBackendArtifact: func(
			context.Context,
			serviceedges.ModelBackendArtifactSelectionRequest,
		) (serviceedges.ModelBackendArtifactSelection, error) {
			return packagedTTSPinnedBackendSelection(), nil
		},
		ModelHostProcessLauncher:      launcher,
		ModelHostProtocolNegotiator:   packagedTTSHostProtocolNegotiator{},
		ModelHostCompatibilityChecker: packagedTTSHostCompatibilityChecker{},
		ModelHostHTTPClient:           modelServer.Client(),
		ModelRuntimeHTTPClient:        modelServer.Client(),
		ModelInvocationBackend:        backend.Invoke,
	}
	process := support.BuildProcess(t, edges)

	seedHomeDir := t.TempDir()
	seedEnvironment := append(os.Environ(),
		"HOME="+seedHomeDir,
		"USERPROFILE="+seedHomeDir,
	)
	seedFactoryDir := support.InstallPackagedFactoryWithProcess(
		t,
		process,
		seedEnvironment,
		t.TempDir(),
		factorydefinitions.PackagedTTSFactoryName,
	)
	configSeed, err := os.ReadFile(filepath.Join(seedHomeDir, ".you-agent-factory", "config.json"))
	if err != nil {
		t.Fatalf("read managed TTS Factory config seed: %v", err)
	}
	cacheSeedDir := t.TempDir()
	writePackagedTTSReadyModelCache(t, cacheSeedDir)

	fixture := &managedFactoryTTSFixture{
		process:         process,
		backend:         backend,
		factorySeedDir:  seedFactoryDir,
		configSeed:      configSeed,
		cacheSeedDir:    cacheSeedDir,
		launcher:        launcher,
		processBuilds:   1,
		factoryInstalls: 1,
		cacheSeeds:      1,
	}
	// Register the observation before process cleanup so the latter runs first
	// and the host lifecycle is checked after all process-owned resources stop.
	t.Cleanup(func() {
		starts, stops := launcher.StartCount(), launcher.StopCount()
		if starts != stops {
			t.Errorf("managed TTS model host lifecycle = starts %d, stops %d; want balanced cleanup", starts, stops)
		}
		t.Logf("managed TTS reuse counts: roots=%d installs=%d factoryCopies=%d cacheSeeds=%d cacheCopies=%d liveRecordings=%d modelHostStarts=%d modelHostStops=%d",
			fixture.processBuilds, fixture.factoryInstalls, fixture.factoryCopies, fixture.cacheSeeds,
			fixture.cacheCopies, fixture.recordings, starts, stops)
	})
	t.Cleanup(modelServer.Close)
	support.CleanupProcess(t, process)
	return fixture
}

func (fixture *managedFactoryTTSFixture) assertReuseCounts(t *testing.T) {
	t.Helper()
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.processBuilds != 1 || fixture.factoryInstalls != 1 || fixture.cacheSeeds != 1 {
		t.Fatalf("managed TTS immutable setup counts = roots:%d installs:%d cacheSeeds:%d; want one each",
			fixture.processBuilds, fixture.factoryInstalls, fixture.cacheSeeds)
	}
	if fixture.factoryCopies != 2 || fixture.cacheCopies != 2 || fixture.recordings != 2 {
		t.Fatalf("managed TTS scenario setup counts = factoryCopies:%d cacheCopies:%d recordings:%d; want two each",
			fixture.factoryCopies, fixture.cacheCopies, fixture.recordings)
	}
}

func (fixture *managedFactoryTTSFixture) newRecordingPaths(
	t *testing.T,
) (homeDir, factoryDir, cacheDir string) {
	t.Helper()
	homeDir = t.TempDir()
	factoryDir = support.CopyFactoryAsNamed(
		t,
		fixture.factorySeedDir,
		homeDir,
		factorydefinitions.PackagedTTSFactoryName,
	)
	fixture.copySystemConfig(t, homeDir)
	cacheDir = t.TempDir()
	copyManagedTTSDirectory(t, fixture.cacheSeedDir, cacheDir)
	fixture.mu.Lock()
	fixture.factoryCopies++
	fixture.cacheCopies++
	fixture.mu.Unlock()
	return homeDir, factoryDir, cacheDir
}

func (fixture *managedFactoryTTSFixture) copySystemConfig(t *testing.T, homeDir string) {
	t.Helper()
	var document map[string]json.RawMessage
	if err := json.Unmarshal(fixture.configSeed, &document); err != nil {
		t.Fatalf("decode managed TTS Factory config seed: %v", err)
	}
	backendScopeID, err := json.Marshal("local-" + uuid.NewString())
	if err != nil {
		t.Fatalf("encode managed TTS backend scope: %v", err)
	}
	document["backendScopeID"] = backendScopeID
	config, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode managed TTS Factory config: %v", err)
	}
	configPath := filepath.Join(homeDir, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create managed TTS Factory config directory: %v", err)
	}
	if err := os.WriteFile(configPath, config, 0o600); err != nil {
		t.Fatalf("write managed TTS Factory config: %v", err)
	}
}

func copyManagedTTSDirectory(t testing.TB, sourceDir, targetDir string) {
	t.Helper()
	err := filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(targetDir, 0o755)
		}
		targetPath := filepath.Join(targetDir, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(targetPath, info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copy managed TTS cache %q to %q: %v", sourceDir, targetDir, err)
	}
}

func managedTTSModelEdges(
	t *testing.T,
	backend *packagedTTSModelsBackend,
) (serviceedges.Edges, func()) {
	t.Helper()
	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	privateFixture := localai.Start(t)
	edges := serviceedges.Edges{
		ModelAssetHostPlatform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelResolveBackendArtifact: func(
			context.Context,
			serviceedges.ModelBackendArtifactSelectionRequest,
		) (serviceedges.ModelBackendArtifactSelection, error) {
			return packagedTTSPinnedBackendSelection(), nil
		},
		ModelHostProcessLauncher:      &packagedTTSModelHostLauncher{endpoint: privateFixture.Endpoint()},
		ModelHostProtocolNegotiator:   packagedTTSHostProtocolNegotiator{},
		ModelHostCompatibilityChecker: packagedTTSHostCompatibilityChecker{},
		ModelHostHTTPClient:           modelServer.Client(),
		ModelRuntimeHTTPClient:        modelServer.Client(),
		ModelInvocationBackend:        backend.Invoke,
	}
	return edges, modelServer.Close
}

func assertFactoryTTSWorkEquivalent(
	t *testing.T,
	live, replay factoryapi.Work,
	label string,
) {
	t.Helper()
	if live.WorkTypeName == nil || replay.WorkTypeName == nil ||
		*live.WorkTypeName != *replay.WorkTypeName ||
		live.State == nil || replay.State == nil ||
		live.State.Name != replay.State.Name ||
		live.Content == nil || replay.Content == nil ||
		len(*live.Content) != len(*replay.Content) {
		t.Fatalf("%s shape = live:%#v replay:%#v", label, live, replay)
	}
	for index := range *live.Content {
		liveJSON, err := json.Marshal((*live.Content)[index])
		if err != nil {
			t.Fatalf("marshal live Work content[%d]: %v", index, err)
		}
		replayJSON, err := json.Marshal((*replay.Content)[index])
		if err != nil {
			t.Fatalf("marshal replay Work content[%d]: %v", index, err)
		}
		if string(liveJSON) != string(replayJSON) {
			t.Fatalf("%s content[%d] differs\nlive=%s\nreplay=%s", label, index, liveJSON, replayJSON)
		}
	}
}

func assertFactoryTTSEventProjectionEquivalent(
	t *testing.T,
	live, replay []factoryapi.FactoryEvent,
	label string,
) {
	t.Helper()
	if len(live) != len(replay) {
		t.Fatalf("%s event count = live:%d replay:%d\nlive=%#v\nreplay=%#v", label, len(live), len(replay), live, replay)
	}
	for index := range live {
		if live[index].Type != replay[index].Type {
			t.Fatalf("%s event[%d] type = live:%q replay:%q", label, index, live[index].Type, replay[index].Type)
		}
		liveWorkIDs := []string(nil)
		if live[index].Context.WorkIds != nil {
			liveWorkIDs = *live[index].Context.WorkIds
		}
		replayWorkIDs := []string(nil)
		if replay[index].Context.WorkIds != nil {
			replayWorkIDs = *replay[index].Context.WorkIds
		}
		if strings.Join(liveWorkIDs, ",") != strings.Join(replayWorkIDs, ",") {
			t.Fatalf("%s event[%d] work ids = live:%#v replay:%#v", label, index, live[index].Context.WorkIds, replay[index].Context.WorkIds)
		}
	}
}
