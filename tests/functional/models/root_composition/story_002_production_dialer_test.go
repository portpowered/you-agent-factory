package root_composition_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

// TestModelsEmptyEdgeRootBuildProcessUsesProductionGRPCDialer proves that an
// empty production edge set reaches the pinned private TTS RPC through the
// policy-free network dialer composed by root.BuildProcess.
func TestModelsEmptyEdgeRootBuildProcessUsesProductionGRPCDialer(t *testing.T) {
	t.Parallel()

	fixture := functionalStartLocalAI(t)
	scenario := setupProductionDialerTTSScenario(t, fixture.Endpoint(), nil)
	audioPath, audio := executeProductionDialerTTS(t, scenario, "production default")
	assertSemanticTTSAudio(t, audio, "empty-edge root output")

	calls := fixture.Calls()
	if len(calls) != 1 || calls[0].Method != "TTS" || calls[0].Text != "production default" {
		t.Fatalf("production fixture calls = %#v, want one private unary TTS call", calls)
	}
	if calls[0].Destination == "" {
		t.Fatal("production fixture TTS call has empty staged destination")
	}
	if got := scenario.generic.Calls(); got != 0 {
		t.Fatalf("generic TTS fallback calls = %d, want zero", got)
	}
	assertProductionDialerRelease(t, scenario, audioPath)
	t.Logf("G-DIAL default evidence root=BuildProcess edges=empty method=/backend.Backend/TTS mediaType=audio/wav bytes=%d sha256=%s fixtureCalls=%d", len(audio), ttsDigest(audio), len(calls))
}

// TestModelsExplicitTTSDialerOverridesProductionListener proves the root
// replacement precedence promised by edges.Merge: an explicit controlled
// dialer handles TTS while a live production listener remains untouched.
func TestModelsExplicitTTSDialerOverridesProductionListener(t *testing.T) {
	t.Parallel()

	productionFixture := functionalStartLocalAI(t)
	explicitDialer := newTTSPrivateProtocolFixture(localai.AudioBytes())
	scenario := setupProductionDialerTTSScenario(t, productionFixture.Endpoint(), explicitDialer)
	audioPath, audio := executeProductionDialerTTS(t, scenario, "explicit override")
	assertSemanticTTSAudio(t, audio, "explicit override output")

	if calls := productionFixture.Calls(); len(calls) != 0 {
		t.Fatalf("production listener calls = %#v, want zero when explicit dialer is supplied", calls)
	}
	calls := explicitDialer.Calls()
	if len(calls) != 1 || calls[0].Method != "/backend.Backend/TTS" || calls[0].Text != "explicit override" {
		t.Fatalf("explicit dialer calls = %#v, want one private TTS call", calls)
	}
	if got := scenario.generic.Calls(); got != 0 {
		t.Fatalf("generic TTS fallback calls = %d, want zero", got)
	}
	assertProductionDialerRelease(t, scenario, audioPath)
	t.Logf("G-DIAL override evidence root=BuildProcess explicitDialer=true productionListenerCalls=0 privateMethod=%s mediaType=audio/wav bytes=%d sha256=%s", calls[0].Method, len(audio), ttsDigest(audio))
}

// TestModelsUnavailableProductionDialerCleansAndRecovers proves the default
// dialer turns a controlled unavailable LocalAI backend at a loopback endpoint
// into a typed, redacted failure without publishing partial output, then
// succeeds on a later healthy root process attempt.
func TestModelsUnavailableProductionDialerCleansAndRecovers(t *testing.T) {
	t.Parallel()

	failedFixture := functionalStartLocalAI(t, localai.Options{Mode: localai.ModeUnavailable})
	failed := setupProductionDialerTTSScenario(t, failedFixture.Endpoint(), nil)
	failurePath := filepath.Join(failed.dir, "unavailable.wav")
	var stdout, stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--text", "unavailable secret", "--output", failurePath,
	})
	inputs.Input.Env = failed.environment
	inputs.Input.WorkingDirectory = failed.dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	err := failed.process.Execute(inputs.Input)
	if err == nil {
		t.Fatal("Process.Execute(unavailable production dialer) error = nil, want bounded failure")
	}
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendReadiness {
		t.Fatalf("unavailable production dialer error = %v failure = %#v, want typed readiness failure", err, failure)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unavailable production dialer stdout = %q, want no output", stdout.String())
	}
	var diagnostic struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := jsonUnmarshalFunctional(stderr.String(), &diagnostic); err != nil {
		t.Fatalf("decode unavailable production dialer diagnostic: %v\nstderr=%q", err, stderr.String())
	}
	if diagnostic.Code != "MODEL_BACKEND_NOT_READY" || diagnostic.Message != "TTS backend is unavailable" {
		t.Fatalf("unavailable production dialer diagnostic = %#v, want typed readiness response", diagnostic)
	}
	if _, statErr := os.Stat(failurePath); !os.IsNotExist(statErr) {
		t.Fatalf("unavailable production dialer output stat = %v, want no published output", statErr)
	}
	assertRedactedTTSFailure(t, err, stdout.String(), stderr.String(), failurePath, "unavailable secret")
	assertProductionDialerRelease(t, failed, failurePath)

	healthyFixture := functionalStartLocalAI(t)
	recovered := setupProductionDialerTTSScenario(t, healthyFixture.Endpoint(), nil)
	audioPath, audio := executeProductionDialerTTS(t, recovered, "recovered")
	assertSemanticTTSAudio(t, audio, "recovered production output")
	if calls := healthyFixture.Calls(); len(calls) != 1 || calls[0].Method != "TTS" {
		t.Fatalf("healthy recovery fixture calls = %#v, want one TTS call", calls)
	}
	assertProductionDialerRelease(t, recovered, audioPath)
	t.Logf("G-DIAL recovery evidence unavailableClass=%s outputExists=false cleanup=once healthyLaterCall=true mediaType=audio/wav bytes=%d sha256=%s", failure.Class, len(audio), ttsDigest(audio))
}

type productionDialerTTSScenario struct {
	process     support.ApplicationProcess
	dir         string
	environment []string
	temp        *ttsTempEffects
	generic     *ttsGenericBackendTrap
	host        *ttsHostLauncher
}

func setupProductionDialerTTSScenario(
	t *testing.T,
	endpoint string,
	dialer platformgrpc.Dialer,
) productionDialerTTSScenario {
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
	tempDirectory := filepath.Join(home, "tts-runtime-temp")
	if err := os.MkdirAll(tempDirectory, 0o755); err != nil {
		t.Fatalf("create TTS runtime temp directory: %v", err)
	}
	writeGenericBuiltinTTSCache(t, home)
	writeGenericBuiltinTTSManagedRuntimeCache(t, home)
	writeGenericBuiltinTTSBackendCache(t, home)

	temp := &ttsTempEffects{directory: tempDirectory}
	generic := &ttsGenericBackendTrap{}
	host := &ttsHostLauncher{endpoint: endpoint}
	assetFiles := functionalModelAssetFileSystem{home: home}
	rejectingNetwork := &rejectingModelAssetHTTP{}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	selection := pinnedTTSBackendSelection()
	dir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())

	edges := serviceedges.Edges{
		ModelAssetHTTPClient:           rejectingNetwork,
		ModelAssetMakeDirectories:      assetFiles.MkdirAll,
		ModelAssetInspectPath:          assetFiles.Stat,
		ModelAssetResolveHomeDirectory: assetFiles.UserHomeDir,
		ModelAssetResolveEnvironment:   func(string) string { return "" },
		ModelAssetWriteFile:            assetFiles.WriteFile,
		ModelAssetRenamePath:           assetFiles.Rename,
		ModelAssetRemovePath:           temp.Remove,
		ModelAssetReadFile:             assetFiles.ReadFile,
		ModelAssetReadDirectory:        assetFiles.ReadDir,
		ModelAssetCreateFile:           assetFiles.Create,
		ModelAssetOpenFile:             assetFiles.Open,
		ModelHostProcessLauncher:       host,
		ModelHostProtocolNegotiator:    protocol,
		ModelHostCompatibilityChecker:  compatibility,
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelResolveBackendArtifact: func(context.Context, serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
			return selection, nil
		},
		ModelInvocationBackend:     generic.Invoke,
		ModelHostHTTPClient:        modelServer.Client(),
		ModelRuntimeHTTPClient:     modelServer.Client(),
		ModelRuntimeTempDirectory:  func() string { return temp.directory },
		ModelRuntimeCreateTempFile: temp.CreateTemp,
		ModelRuntimeInspectFile:    os.Stat,
	}
	if dialer != nil {
		edges.ModelInvocationGRPCDialer = dialer
	}

	process := functionalBuildProcess(t, edges)
	return productionDialerTTSScenario{
		process: process, dir: dir, environment: functionalHomeEnvironment(home),
		temp: temp, generic: generic, host: host,
	}
}

func executeProductionDialerTTS(
	t *testing.T,
	scenario productionDialerTTSScenario,
	text string,
) (string, []byte) {
	t.Helper()
	outputPath := filepath.Join(scenario.dir, "speech.wav")
	var stdout, stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--text", text, "--output", outputPath,
	})
	inputs.Input.Env = scenario.environment
	inputs.Input.WorkingDirectory = scenario.dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	if err := scenario.process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(production TTS %q) error = %v\nstdout=%q\nstderr=%q", text, err, stdout.String(), stderr.String())
	}
	if stdout.String() != "Wrote audio: "+outputPath+"\n" || stderr.Len() != 0 {
		t.Fatalf("production TTS streams = stdout:%q stderr:%q, want status-only success", stdout.String(), stderr.String())
	}
	audio, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read production TTS output: %v", err)
	}
	return outputPath, audio
}

func assertProductionDialerRelease(
	t *testing.T,
	scenario productionDialerTTSScenario,
	outputPath string,
) {
	t.Helper()
	closeRootProcess(t, scenario.process, "close production dialer process")
	created, removed, duplicateRemoves := scenario.temp.Snapshot()
	if created == 0 || created != removed || duplicateRemoves != 0 {
		t.Fatalf("production dialer staging release for %s = created:%d removed:%d duplicateRemoves:%d, want exactly-once cleanup", outputPath, created, removed, duplicateRemoves)
	}
	starts, stops, waits, active := scenario.host.Snapshot()
	if starts == 0 || starts != stops || stops != waits || active != 0 {
		t.Fatalf("production dialer host release = starts:%d stops:%d waits:%d active:%d, want exactly-once stop/wait", starts, stops, waits, active)
	}
}
