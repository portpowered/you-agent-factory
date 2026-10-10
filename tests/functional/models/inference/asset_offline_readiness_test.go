package inference_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

// Standalone Models binds ~default. Only the causally ordered download,
// failure and offline recovery cohort serializes; other journeys run parallel.
func TestModelAssetOfflineReadiness(t *testing.T) {
	t.Parallel()
	modelBody, backendBody := []byte("asset readiness weights"), []byte("asset readiness backend")
	selection := pullToReadyBackendSelection(backendBody)
	client := newPullToReadyAssetClient(modelBody, backendBody, selection.Location)
	healthy, failed := t.TempDir(), t.TempDir()
	for _, home := range []string{healthy, failed} {
		writeGenericModelSourceOverride(t, home, pullToReadyModelName, pullToReadySource, "localai-whisper")
	}
	host := story004HostServer(t)
	launcher := &recordingModelHostLauncher{endpoint: host.URL}
	t.Cleanup(func() {
		if launcher.Active() {
			t.Error("managed asset host remained active after process cleanup")
		}
	})
	edges := pullToReadyEdges(client, healthy, selection)
	var installations atomic.Int32
	edges.ModelAssetRenamePath = func(old, target string) error {
		installations.Add(1)
		return os.Rename(old, target)
	}
	edges.ModelHostProcessLauncher = launcher
	edges.ModelHostHTTPClient = host.Client()
	edges.ModelHostProtocolNegotiator = &joinedProtocolNegotiator{}
	edges.ModelHostCompatibilityChecker = &joinedCompatibilityChecker{}
	edges.ModelAssetHostPlatform = models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"}
	edges.ModelASRBackend = func(_ context.Context, request models.ASRBackendRequest) (models.ASRBackendResponse, error) {
		if !bytes.Equal(request.Audio, localai.AudioBytes()) {
			return models.ASRBackendResponse{}, errors.New("wrong selected audio")
		}
		return models.ASRBackendResponse{Text: "asset ready", Segments: []models.ASRBackendSegment{{ID: 0, Start: 0, End: 1, Text: "asset ready"}}}, nil
	}
	process := buildPullToReadyProcess(t, edges)
	directory := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	audio := filepath.Join(t.TempDir(), "input.wav")
	if err := os.WriteFile(audio, localai.AudioBytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Run("B09-S pull and invoke", func(t *testing.T) {
		pull := executePullToReadyCommand(t, process, healthy, "models", "pull", pullToReadyModelName)
		assertPullToReadySuccess(t, pull, modelBody, filepath.Join(healthy, "managed-cache"))
		assertAssetReadinessInvocation(t, process, directory, healthy, audio, false)
	})
	t.Run("B09-F backend download rejection preserves verified peer", func(t *testing.T) {
		runAssetBackendFailure(t, process, client, directory, healthy, failed, audio)
	})
	t.Run("B09-H verified offline cache hit", func(t *testing.T) {
		calls, installed := client.Calls(), installations.Load()
		assertAssetReadinessInvocation(t, process, directory, healthy, audio, true)
		if client.Calls() != calls || installations.Load() != installed {
			t.Fatal("offline cache hit attempted asset HTTP or installation")
		}
	})
	t.Run("B09-O uncached offline backend", func(t *testing.T) {
		calls, starts := client.Calls(), launcher.Calls()
		inputs := assetReadinessInputs(t, directory, failed, audio, true)
		err := process.Execute(inputs.Input)
		var offline *models.AssetOfflineError
		if !errors.As(err, &offline) || !strings.Contains(strings.Join(offline.Missing, ","), selection.Name) || inputs.Stdout() != "" {
			t.Fatalf("offline miss = %v; stdout=%q", err, inputs.Stdout())
		}
		if client.Calls() != calls || launcher.Calls() != starts {
			t.Fatal("offline miss started remote or host effects")
		}
	})
}

func assetReadinessInputs(t *testing.T, directory, home, audio string, offline bool) *support.CapturedInputs {
	t.Helper()
	args := []string{"you", "--json", "models", "invoke", pullToReadyModelName, "--operation", "ASR", "--input", "audio=@" + audio}
	if offline {
		args = append(args, "--offline")
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = isolatedModelEnvironment(home, filepath.Join(home, "managed-cache"))
	inputs.Input.WorkingDirectory = directory
	return inputs
}

func assertAssetReadinessInvocation(t *testing.T, process rootProcess, directory, home, audio string, offline bool) {
	t.Helper()
	inputs := assetReadinessInputs(t, directory, home, audio, offline)
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("cached invocation: %v; stderr=%s", err, inputs.Stderr())
	}
	var result factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Outputs) < 1 || (result.Outputs[0].Content == nil || *result.Outputs[0].Content != "asset ready") {
		t.Fatalf("cached ASR outputs = %#v", result.Outputs)
	}
}

func runAssetBackendFailure(t *testing.T, process rootProcess, client *pullToReadyAssetClient, directory, healthy, failed, audio string) {
	t.Helper()
	client.failBackend.Store(true)
	defer client.failBackend.Store(false)
	pull, pullErr := executePullToReadyCommandResult(t, process, failed, "models", "pull", pullToReadyModelName)
	var diagnostic interface{ CLIErrorCode() string }
	if !errors.As(pullErr, &diagnostic) || diagnostic.CLIErrorCode() != "CLI_MODEL_PULL_FAILED" {
		t.Fatalf("pull failure = %v", pullErr)
	}
	var result factoryapi.ModelPullResponse
	decodePullToReadyJSON(t, pull.raw, &result)
	if result.Outcome != factoryapi.ModelPullOutcomeFAILED || result.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED || result.ManagedRuntimePull.ReadinessState != factoryapi.ManagedRuntimeReadinessStateFAILED || strings.Contains(pull.raw, "private backend response") {
		t.Fatalf("backend failure outcome = %s", result.ManagedRuntimePull.PullOutcome)
	}
	inspect, err := executePullToReadyCommandResult(t, process, failed, "models", "inspect", pullToReadyModelName)
	if err != nil {
		t.Fatalf("failed asset inspect: %v", err)
	}
	assertPullToReadyMissingInspect(t, inspect)
	assertAssetReadinessInvocation(t, process, directory, healthy, audio, true)
}
