package inference_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

// Standalone Models commands own invocation-local profiles and ~default, so
// this causal cancel/repair/retry cohort is serial within its reusable graph.
// Independent immutable gallery graphs run in parallel.
func TestModelsGalleryBootstrapCancellation(t *testing.T) {
	requireGalleryPlatform(t)
	t.Parallel()
	home, fixture := newGalleryLifecycleFixture(t, "cancellation")
	fixture.failed.Store(true)
	fixture.downloadStarted = make(chan struct{})
	process := buildPullToReadyProcess(t, galleryLifecycleInferenceEdges(t, fixture, home))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	inputs := support.FakeInputs(ctx, []string{"you", "--json", "models", "pull", pullToReadyModelName})
	inputs.Input.Env = isolatedModelEnvironment(home, filepath.Join(home, "managed-cache"))
	inputs.Input.WorkingDirectory = t.TempDir()
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input) }()
	select {
	case <-fixture.downloadStarted:
	case err := <-done:
		t.Fatalf("pull returned before download gate: %v", err)
	case <-t.Context().Done():
		t.Fatal("download did not start")
	}
	cancel()
	err := <-done
	var diagnostic interface{ CLIErrorCode() string }
	if !errors.Is(err, context.Canceled) || !errors.As(err, &diagnostic) || diagnostic.CLIErrorCode() != "CLI_MODEL_PULL_FAILED" || inputs.Stdout() != "" {
		t.Fatalf("canceled download = %T %v; stdout=%q", err, err, inputs.Stdout())
	}
	galleryAssertNoUnverifiedInstallation(t, fixture)
	peer := seedGalleryOfflinePeer(t, fixture)
	requests, commands := fixture.requests.Load(), fixture.commands.Load()
	assertGalleryLifecycleInference(t, process, peer)
	if fixture.requests.Load() != requests || fixture.commands.Load() != commands {
		t.Fatal("canceled download disrupted the cached offline peer")
	}
	fixture.failed.Store(false)
	assertGalleryLifecyclePull(t, process, home, fixture)
	assertGalleryLifecycleInference(t, process, home)
}

func galleryLifecycleInferenceEdges(t *testing.T, fixture *galleryPullFixture, home string) serviceedges.Edges {
	return galleryInferenceEdges(t, galleryPullEdges(fixture, home, "", ""))
}

func galleryInferenceEdges(t *testing.T, edges serviceedges.Edges) serviceedges.Edges {
	t.Helper()
	host := story004HostServer(t)
	edges.ModelHostProcessLauncher = &recordingModelHostLauncher{endpoint: host.URL}
	edges.ModelHostHTTPClient = host.Client()
	edges.ModelHostProtocolNegotiator = &joinedProtocolNegotiator{}
	edges.ModelHostCompatibilityChecker = &joinedCompatibilityChecker{}
	edges.ModelASRBackend = func(_ context.Context, request models.ASRBackendRequest) (models.ASRBackendResponse, error) {
		if !bytes.Equal(request.Audio, localai.AudioBytes()) {
			return models.ASRBackendResponse{}, errors.New("incorrect gallery audio")
		}
		return models.ASRBackendResponse{Text: "asset ready", Segments: []models.ASRBackendSegment{{ID: 0, Start: 0, End: 1, Text: "asset ready"}}}, nil
	}
	return edges
}

func assertGalleryLifecycleInference(t *testing.T, process rootProcess, home string) {
	t.Helper()
	directory := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	audio := filepath.Join(t.TempDir(), "audio.wav")
	if err := os.WriteFile(audio, localai.AudioBytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	assertAssetReadinessInvocation(t, process, directory, home, audio, true)
}

// Publication is atomic: an unsuccessful replacement leaves the customer's
// existing executable intact, even when it differs from the new release hash.
func TestModelsGalleryBootstrapPublicationPreservesExistingExecutable(t *testing.T) {
	requireGalleryPlatform(t)
	t.Parallel()
	home, fixture := newGalleryLifecycleFixture(t, "publication")
	previous := []byte("previous customer executable")
	if err := os.MkdirAll(filepath.Dir(fixture.wantedCommand), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.wantedCommand, previous, 0o700); err != nil {
		t.Fatal(err)
	}
	fixture.failed.Store(true)
	process := buildPullToReadyProcess(t, galleryLifecycleInferenceEdges(t, fixture, home))
	galleryAssertPullFailure(t, process, home)
	contents, err := os.ReadFile(fixture.wantedCommand)
	if err != nil || !bytes.Equal(contents, previous) {
		t.Fatalf("publication failure changed existing executable: %q, %v", contents, err)
	}
	staged, err := filepath.Glob(filepath.Join(filepath.Dir(fixture.wantedCommand), ".local-ai-*"))
	if err != nil || len(staged) != 0 || fixture.commands.Load() != 0 {
		t.Fatalf("failed publication left staging or ran installer: %v, %v, commands=%d", staged, err, fixture.commands.Load())
	}
	fixture.failed.Store(false)
	assertGalleryLifecyclePull(t, process, home, fixture)
	contents, err = os.ReadFile(fixture.wantedCommand)
	if err != nil || !bytes.Equal(contents, galleryBinary) {
		t.Fatalf("repaired publication = %q, %v", contents, err)
	}
	assertGalleryLifecycleInference(t, process, home)
}

// Reconstruction is the customer persistence boundary: the new process has
// no memoized executable, so reuse must validate the on-disk release checksum.
func TestModelsGalleryBootstrapVerifiedCacheAfterReconstruction(t *testing.T) {
	requireGalleryPlatform(t)
	t.Parallel()
	home, fixture := newGalleryLifecycleFixture(t, "checksum")
	first := buildPullToReadyProcess(t, galleryLifecycleInferenceEdges(t, fixture, home))
	assertGalleryLifecyclePull(t, first, home, fixture)
	closePullToReadyProcess(t, first)
	before := fixture.downloads.Load()
	fixture.failed.Store(true) // A transfer would now return corrupt bytes.
	second := buildPullToReadyProcess(t, galleryLifecycleInferenceEdges(t, fixture, home))
	assertPullToReadyAlreadyPresent(t, executePullToReadyCommand(t, second, home, "models", "pull", pullToReadyModelName), fixture.model.model, filepath.Join(home, "managed-cache"))
	if fixture.downloads.Load() != before {
		t.Fatal("verified persisted executable was downloaded again")
	}
	assertGalleryLifecycleInference(t, second, home)
}

func newGalleryLifecycleFixture(t *testing.T, fault string) (string, *galleryPullFixture) {
	t.Helper()
	home, cache := t.TempDir(), t.TempDir()
	writeGenericModelSourceOverride(t, home, pullToReadyModelName, pullToReadySource, "localai-whisper")
	fixture := &galleryPullFixture{
		model: newPullToReadyAssetClient([]byte("gallery lifecycle weights"), nil, ""),
		cache: cache, fault: fault,
		wantedCommand: filepath.Join(cache, "you", "localai", "v1", galleryBinaryName),
	}
	return home, fixture
}

func assertGalleryLifecyclePull(t *testing.T, process rootProcess, home string, fixture *galleryPullFixture) {
	t.Helper()
	cache := filepath.Join(home, "managed-cache")
	assertPullToReadySuccess(t, executePullToReadyCommand(t, process, home, "models", "pull", pullToReadyModelName), fixture.model.model, cache)
	assertPullToReadyInspect(t, executePullToReadyCommand(t, process, home, "models", "inspect", pullToReadyModelName), cache, fixture.model.model)
}

// Offline Models invocation is a standalone CLI contract. Hosted Session Work
// isolation is independently exercised by the retained controlled-host story.
func TestModelsGalleryBootstrapInstallationOffline(t *testing.T) {
	requireGalleryPlatform(t)
	t.Parallel()
	home, fixture := newGalleryLifecycleFixture(t, "")
	missing := t.TempDir()
	writeGenericModelSourceOverride(t, missing, pullToReadyModelName, pullToReadySource, "localai-whisper")
	host := story004HostServer(t)
	launcher := &recordingModelHostLauncher{endpoint: host.URL}
	edges := galleryPullEdges(fixture, home, "", "")
	edges.ModelHostProcessLauncher = launcher
	edges.ModelHostHTTPClient = host.Client()
	edges.ModelHostProtocolNegotiator = &joinedProtocolNegotiator{}
	edges.ModelHostCompatibilityChecker = &joinedCompatibilityChecker{}
	edges.ModelASRBackend = func(_ context.Context, request models.ASRBackendRequest) (models.ASRBackendResponse, error) {
		if !bytes.Equal(request.Audio, localai.AudioBytes()) {
			return models.ASRBackendResponse{}, errors.New("incorrect gallery audio")
		}
		return models.ASRBackendResponse{Text: "asset ready", Segments: []models.ASRBackendSegment{{ID: 0, Start: 0, End: 1, Text: "asset ready"}}}, nil
	}
	process := buildPullToReadyProcess(t, edges)
	directory := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	audio := filepath.Join(t.TempDir(), "audio.wav")
	if err := os.WriteFile(audio, localai.AudioBytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	assertGalleryLifecyclePull(t, process, home, fixture)
	assertGalleryLifecyclePull(t, process, missing, fixture)
	assertAssetReadinessInvocation(t, process, directory, home, audio, false)
	beforeRequests, beforeInstallations := fixture.requests.Load(), fixture.commands.Load()
	assertAssetReadinessInvocation(t, process, directory, home, audio, true)
	if fixture.requests.Load() != beforeRequests || fixture.commands.Load() != beforeInstallations {
		t.Fatal("installed offline gallery attempted HTTP or installation")
	}
	// Remove only the gallery installation through a scenario-owned effect;
	// verified weights and the selected executable remain cached.
	script := filepath.Join(fixture.cache, "you", "localai-backends", "cpu-whisper", "run.sh")
	if err := os.Remove(script); err != nil {
		t.Fatal(err)
	}
	beforeRequests, beforeCommands, beforeHosts := fixture.requests.Load(), fixture.commands.Load(), launcher.Calls()
	inputs := assetReadinessInputs(t, directory, missing, audio, true)
	err := process.Execute(inputs.Input)
	if !errors.Is(err, models.ErrAssetOffline) || inputs.Stdout() != "" {
		t.Fatalf("missing offline gallery = %T %v, stdout=%q", err, err, inputs.Stdout())
	}
	if fixture.requests.Load() != beforeRequests || fixture.commands.Load() != beforeCommands || launcher.Calls() != beforeHosts {
		t.Fatal("offline missing gallery performed download, installation or host launch")
	}
	assertPullToReadyAlreadyPresent(t, executePullToReadyCommand(t, process, missing, "models", "pull", pullToReadyModelName), fixture.model.model, filepath.Join(missing, "managed-cache"))
	assertAssetReadinessInvocation(t, process, directory, missing, audio, true)
	// The healthy profile still returns the exact inference after peer repair.
	assertAssetReadinessInvocation(t, process, directory, home, audio, true)
}
