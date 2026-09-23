//go:build windows && managed_process_integration

package asrlivecorrelation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	platformlocking "github.com/portpowered/infinite-you/pkg/platform/locking"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/platform/process/managedchild"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	"go.uber.org/zap"
)

const (
	asrLiveCorrelationManifestEnvironment = "INFINITE_YOU_ASR_LIVE_CORRELATION_MANIFEST"
	asrLiveCorrelationManifestSchema      = "asr-live-correlation-harness/v1"
	asrLiveCorrelationFixtureBytes        = 10_340
	asrLiveCorrelationFixtureSHA256       = "eea86018ce1730baaf7f5dd6ec88c1f727dd90203521a9115b489310a248ea05"
)

// TestASRLiveCorrelationCompiledHarness is the entry point compiled once by
// the Windows managed-process integration lane. Without an explicit manifest
// it is inert, so ordinary focused suites never start a process or make a call.
func TestASRLiveCorrelationCompiledHarness(t *testing.T) {
	manifestPath := strings.TrimSpace(os.Getenv(asrLiveCorrelationManifestEnvironment))
	if manifestPath == "" {
		t.Skip("ASR live-correlation manifest is not configured")
	}
	manifest := readASRLiveCorrelationHarnessManifest(t, manifestPath)
	runASRLiveCorrelationHarness(t, manifest)
}

type asrLiveCorrelationHarnessManifest struct {
	Schema                string `json:"schema"`
	RunID                 string `json:"run_id"`
	Scenario              string `json:"scenario"`
	SourceRoot            string `json:"source_root"`
	SourceCommit          string `json:"source_commit"`
	SourceTree            string `json:"source_tree"`
	ExecutablePath        string `json:"executable_path"`
	ExecutableSHA256      string `json:"executable_sha256"`
	WAVPath               string `json:"wav_path"`
	WAVSHA256             string `json:"wav_sha256"`
	ModelIdentitySHA256   string `json:"model_identity_sha256"`
	BackendArtifactSHA256 string `json:"backend_artifact_sha256"`
	CacheRoot             string `json:"cache_root"`
	CacheManifestPath     string `json:"cache_manifest_path"`
	CacheManifestSHA256   string `json:"cache_manifest_sha256"`
	StateRoot             string `json:"state_root"`
	StagingRoot           string `json:"staging_root"`
	EvidenceOutput        string `json:"evidence_output"`
	TimeoutSeconds        int    `json:"timeout_seconds"`
}

type asrLiveCorrelationInvocation struct {
	result models.InvokeModelResult
	err    error
}

type asrLiveCorrelationRuntimeEvidenceSink struct {
	mu      sync.Mutex
	records []modelseffects.RuntimeEvidenceRecord
}

func readASRLiveCorrelationHarnessManifest(
	t *testing.T,
	path string,
) asrLiveCorrelationHarnessManifest {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ASR live-correlation manifest: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var manifest asrLiveCorrelationHarnessManifest
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatalf("decode ASR live-correlation manifest: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		t.Fatalf("ASR live-correlation manifest has trailing content: %v", err)
	}
	if err := validateASRLiveCorrelationHarnessManifest(manifest); err != nil {
		t.Fatalf("invalid ASR live-correlation manifest: %v", err)
	}
	return manifest
}

func validateASRLiveCorrelationHarnessManifest(manifest asrLiveCorrelationHarnessManifest) error {
	if !validASRLiveCorrelationManifestIdentity(manifest) {
		return errors.New("manifest identity or bounded timeout is invalid")
	}
	return validateASRLiveCorrelationManifestPaths(manifest)
}

func validASRLiveCorrelationManifestIdentity(manifest asrLiveCorrelationHarnessManifest) bool {
	return validASRLiveCorrelationManifestRun(manifest) && validASRLiveCorrelationManifestArtifacts(manifest) &&
		manifest.TimeoutSeconds >= 1 && manifest.TimeoutSeconds <= 180
}

func validASRLiveCorrelationManifestRun(manifest asrLiveCorrelationHarnessManifest) bool {
	return manifest.Schema == asrLiveCorrelationManifestSchema && safeHarnessRunID(manifest.RunID) &&
		validHarnessScenario(manifest.Scenario) && validHarnessGitID(manifest.SourceCommit) &&
		validHarnessGitID(manifest.SourceTree)
}

func validASRLiveCorrelationManifestArtifacts(manifest asrLiveCorrelationHarnessManifest) bool {
	return validHarnessDigest(manifest.ExecutableSHA256) && validHarnessDigest(manifest.WAVSHA256) &&
		validHarnessDigest(manifest.ModelIdentitySHA256) && validHarnessDigest(manifest.BackendArtifactSHA256) &&
		validHarnessDigest(manifest.CacheManifestSHA256)
}

func validateASRLiveCorrelationManifestPaths(manifest asrLiveCorrelationHarnessManifest) error {
	for _, path := range []string{
		manifest.SourceRoot, manifest.ExecutablePath, manifest.WAVPath, manifest.CacheRoot,
		manifest.CacheManifestPath, manifest.StateRoot, manifest.StagingRoot, manifest.EvidenceOutput,
	} {
		if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
			return errors.New("manifest paths must be absolute")
		}
	}
	return nil
}

func runASRLiveCorrelationHarness(
	t *testing.T,
	manifest asrLiveCorrelationHarnessManifest,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(manifest.TimeoutSeconds)*time.Second)
	defer cancel()
	harness := newASRLiveCorrelationHarness(t, ctx, manifest)
	t.Cleanup(func() { harness.cleanup(t) })
	invocations := harness.startInvocation(ctx)
	endpointEvent, terminalEvent := awaitASRLiveCorrelationTerminal(t, ctx, harness)
	result := runASRLiveCorrelationSchedule(t, ctx, harness, endpointEvent, invocations)
	finishASRLiveCorrelationHarness(t, ctx, harness, endpointEvent, terminalEvent, result)
}

type asrLiveCorrelationHarness struct {
	manifest   asrLiveCorrelationHarnessManifest
	fixture    asrLiveCorrelationFixture
	endpoint   string
	controller *modelseffects.ASRLiveCorrelationController
	evidence   *asrLiveCorrelationRuntimeEvidenceSink
	launcher   *asrLiveCorrelationProcessLauncher
	service    models.Service
	scope      models.RuntimeScopeRef
	closed     bool
}

type asrLiveCorrelationResult struct {
	invocation asrLiveCorrelationInvocation
	transcript string
}

func newASRLiveCorrelationHarness(
	t *testing.T,
	ctx context.Context,
	manifest asrLiveCorrelationHarnessManifest,
) *asrLiveCorrelationHarness {
	fixture := verifyASRLiveCorrelationInputs(t, ctx, manifest)
	endpoint := reserveASRLiveCorrelationEndpoint(t)
	controller, err := modelseffects.NewASRLiveCorrelationController(modelseffects.WindowsASRLiveCorrelationListenerPIDLookup)
	if err != nil {
		t.Fatalf("construct ASR live-correlation controller: %v", err)
	}
	evidence := &asrLiveCorrelationRuntimeEvidenceSink{}
	launcher := &asrLiveCorrelationProcessLauncher{controller: controller, endpoint: endpoint, manifest: manifest}
	service := newASRLiveCorrelationModelsService(t, manifest, launcher, evidence)
	opened, err := service.OpenRuntimeScope(ctx, models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{CacheDirectory: manifest.CacheRoot, Runtime: asrLiveCorrelationRuntimeConfig(endpoint)},
	})
	if err != nil {
		t.Fatalf("open isolated ASR Runtime Scope: %v", err)
	}
	return &asrLiveCorrelationHarness{
		manifest: manifest, fixture: fixture, endpoint: endpoint, controller: controller,
		evidence: evidence, launcher: launcher, service: service, scope: opened.Scope,
	}
}

func (harness *asrLiveCorrelationHarness) startInvocation(ctx context.Context) <-chan asrLiveCorrelationInvocation {
	invocations := make(chan asrLiveCorrelationInvocation, 1)
	go func() {
		result, err := harness.service.InvokeModel(
			modelseffects.WithASRLiveCorrelation(ctx, harness.controller),
			asrLiveCorrelationRequest(harness.scope, harness.fixture.audio),
		)
		invocations <- asrLiveCorrelationInvocation{result: result, err: err}
	}()
	return invocations
}

func awaitASRLiveCorrelationTerminal(
	t *testing.T,
	ctx context.Context,
	harness *asrLiveCorrelationHarness,
) (modelseffects.ASRLiveCorrelationEvent, modelseffects.ASRLiveCorrelationEvent) {
	t.Helper()
	endpoint, err := harness.controller.WaitForSignal(ctx, modelseffects.ASRLiveCorrelationEndpointObserved)
	if err != nil {
		failASRLiveCorrelationEndpointWait(t, harness, err)
	}
	terminal := awaitASRLiveCorrelationSignal(t, ctx, harness.controller, modelseffects.ASRLiveCorrelationRPCTerminal)
	if endpoint.ProcessID <= 0 || endpoint.Endpoint.Port == 7437 || terminal.Sequence <= endpoint.Sequence {
		t.Fatalf("ASR endpoint/RPC facts are not owned and ordered: endpoint=%#v terminal=%#v", endpoint, terminal)
	}
	return endpoint, terminal
}

func failASRLiveCorrelationEndpointWait(
	t *testing.T,
	harness *asrLiveCorrelationHarness,
	cause error,
) {
	t.Helper()
	process := harness.launcher.process()
	if process == nil {
		t.Fatalf("wait for ASR live-correlation endpoint: %v; managed process was not recorded", cause)
	}
	if snapshot, terminal := process.child.Snapshot(); terminal {
		t.Fatalf("wait for ASR live-correlation endpoint: %v; managed process pid=%d terminated=%#v", cause, process.pid, snapshot)
	}
	t.Fatalf("wait for ASR live-correlation endpoint: %v; managed process pid=%d still running", cause, process.pid)
}

func runASRLiveCorrelationSchedule(
	t *testing.T,
	ctx context.Context,
	harness *asrLiveCorrelationHarness,
	endpoint modelseffects.ASRLiveCorrelationEvent,
	invocations <-chan asrLiveCorrelationInvocation,
) asrLiveCorrelationResult {
	t.Helper()
	if harness.manifest.Scenario == modelseffects.ASRLiveCorrelationScenarioResponseFirst {
		return releaseASRLiveCorrelationFirst(t, ctx, harness, invocations)
	}
	return exitASRLiveCorrelationFirst(t, ctx, harness, endpoint, invocations)
}

func releaseASRLiveCorrelationFirst(
	t *testing.T,
	ctx context.Context,
	harness *asrLiveCorrelationHarness,
	invocations <-chan asrLiveCorrelationInvocation,
) asrLiveCorrelationResult {
	if err := harness.controller.ReleaseResponse(); err != nil {
		t.Fatalf("release response-first decoded ASR result: %v", err)
	}
	invocation := awaitASRLiveCorrelationInvocation(t, ctx, invocations)
	transcript := assertASRLiveCorrelationResponseFirst(t, invocation)
	return asrLiveCorrelationResult{invocation: invocation, transcript: transcript}
}

func exitASRLiveCorrelationFirst(
	t *testing.T,
	ctx context.Context,
	harness *asrLiveCorrelationHarness,
	endpoint modelseffects.ASRLiveCorrelationEvent,
	invocations <-chan asrLiveCorrelationInvocation,
) asrLiveCorrelationResult {
	process := requireASRLiveCorrelationOwnedProcess(t, harness.launcher, endpoint.ProcessID)
	if err := process.Stop(ctx); err != nil {
		t.Fatalf("stop owned LocalAI child after RPC terminal: %v", err)
	}
	awaitASRLiveCorrelationSignal(t, ctx, harness.controller, modelseffects.ASRLiveCorrelationChildWaited)
	awaitASRLiveCorrelationSignal(t, ctx, harness.controller, modelseffects.ASRLiveCorrelationHostFailureSeen)
	if err := harness.controller.ReleaseResponse(); err != nil {
		t.Fatalf("release exit-first decoded ASR result after host failure: %v", err)
	}
	invocation := awaitASRLiveCorrelationInvocation(t, ctx, invocations)
	assertASRLiveCorrelationResponseFirstFailure(t, invocation)
	return asrLiveCorrelationResult{invocation: invocation}
}

func requireASRLiveCorrelationOwnedProcess(
	t *testing.T,
	launcher *asrLiveCorrelationProcessLauncher,
	wantPID int,
) *asrLiveCorrelationManagedProcess {
	t.Helper()
	process := launcher.process()
	if process == nil || process.PID() != wantPID {
		t.Fatalf("managed process does not match endpoint PID %d", wantPID)
	}
	return process
}

func finishASRLiveCorrelationHarness(
	t *testing.T,
	ctx context.Context,
	harness *asrLiveCorrelationHarness,
	endpoint modelseffects.ASRLiveCorrelationEvent,
	terminal modelseffects.ASRLiveCorrelationEvent,
	result asrLiveCorrelationResult,
) {
	process := requireASRLiveCorrelationOwnedProcess(t, harness.launcher, endpoint.ProcessID)
	if _, terminalSnapshot := process.child.Snapshot(); !terminalSnapshot {
		if err := process.Stop(ctx); err != nil {
			t.Fatalf("stop owned LocalAI child after ASR result: %v", err)
		}
	}
	awaitASRLiveCorrelationSignal(t, ctx, harness.controller, modelseffects.ASRLiveCorrelationChildWaited)
	awaitASRLiveCorrelationSignal(t, ctx, harness.controller, modelseffects.ASRLiveCorrelationHostFailureSeen)
	if process.waitCount() != 1 || process.stopCount() != 1 {
		t.Fatalf("owned process Wait/Stop counts = %d/%d, want one each", process.waitCount(), process.stopCount())
	}
	stopASRLiveCorrelationScope(t, ctx, harness.service, harness.scope)
	harness.closed = true
	assertASRLiveCorrelationCleanup(t, harness.manifest, endpoint.Endpoint.Port)
	protocol := asrLiveCorrelationProtocolEvidence(t, harness.evidence.snapshot())
	assertASRLiveCorrelationProtocol(t, protocol, harness.manifest)
	writeASRLiveCorrelationEvidence(t, harness.manifest, harness.fixture, harness.endpoint, harness.controller.Snapshot(), protocol, result.invocation, result.transcript)
	if result.transcript != "" && terminal.ResponseSemanticSHA256 == "" {
		t.Fatal("response-first ASR evidence is missing the response semantic digest")
	}
}

func assertASRLiveCorrelationProtocol(
	t *testing.T,
	protocol modelseffects.RuntimeASRProtocolObservation,
	manifest asrLiveCorrelationHarnessManifest,
) {
	if protocol.ModelIdentitySHA256 != manifest.ModelIdentitySHA256 ||
		protocol.BackendArtifactSHA256 != manifest.BackendArtifactSHA256 ||
		protocol.RPCMethod != modelseffects.RuntimeASRPCMethodAudioTranscription ||
		protocol.RPCStatus != "OK" || !protocol.ResponseDecoded {
		t.Fatalf("live ASR protocol evidence mismatches the immutable manifest: %#v", protocol)
	}
}

func (harness *asrLiveCorrelationHarness) cleanup(t *testing.T) {
	if process := harness.launcher.process(); process != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = process.Stop(cleanupCtx)
		cancel()
	}
	if !harness.closed {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = harness.service.StopModelHost(cleanupCtx, models.StopModelHostRequest{Scope: harness.scope, Name: models.BuiltInModelNameASR})
		_, _ = harness.service.CloseRuntimeScope(cleanupCtx, models.CloseRuntimeScopeRequest{Scope: harness.scope})
	}
}

type asrLiveCorrelationFixture struct {
	audio []byte
}

func verifyASRLiveCorrelationInputs(
	t *testing.T,
	ctx context.Context,
	manifest asrLiveCorrelationHarnessManifest,
) asrLiveCorrelationFixture {
	t.Helper()
	if runtime.Version() != "go1.26.8" || runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Fatalf("live harness requires go1.26.8 windows/amd64, got %s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	}
	assertGitIdentity(t, ctx, manifest)
	assertFileDigest(t, manifest.ExecutablePath, manifest.ExecutableSHA256)
	assertFileDigest(t, manifest.WAVPath, manifest.WAVSHA256)
	assertFileDigest(t, manifest.CacheManifestPath, manifest.CacheManifestSHA256)
	if manifest.WAVSHA256 != asrLiveCorrelationFixtureSHA256 {
		t.Fatalf("immutable ASR WAV SHA-256 = %s, want %s", manifest.WAVSHA256, asrLiveCorrelationFixtureSHA256)
	}
	audio, err := os.ReadFile(manifest.WAVPath)
	if err != nil {
		t.Fatalf("read immutable ASR WAV: %v", err)
	}
	if len(audio) != asrLiveCorrelationFixtureBytes {
		t.Fatalf("immutable ASR WAV size = %d, want %d", len(audio), asrLiveCorrelationFixtureBytes)
	}
	if err := os.MkdirAll(manifest.StateRoot, 0o700); err != nil {
		t.Fatalf("prepare isolated Models state root: %v", err)
	}
	if err := os.MkdirAll(manifest.StagingRoot, 0o700); err != nil {
		t.Fatalf("prepare isolated ASR staging root: %v", err)
	}
	entries, err := os.ReadDir(manifest.StagingRoot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("ASR staging root must begin empty, entries=%v err=%v", entries, err)
	}
	if _, err := os.Stat(manifest.EvidenceOutput); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ASR evidence output must be a fresh path: %v", err)
	}
	return asrLiveCorrelationFixture{audio: audio}
}

func assertGitIdentity(t *testing.T, ctx context.Context, manifest asrLiveCorrelationHarnessManifest) {
	t.Helper()
	for _, item := range []struct {
		args []string
		want string
	}{
		{args: []string{"rev-parse", "HEAD"}, want: manifest.SourceCommit},
		{args: []string{"rev-parse", "HEAD^{tree}"}, want: manifest.SourceTree},
	} {
		args := append([]string{"-C", manifest.SourceRoot}, item.args...)
		output, err := exec.CommandContext(ctx, "git", args...).Output()
		if err != nil || strings.TrimSpace(string(output)) != item.want {
			t.Fatalf("Git source identity mismatch for %q: got=%q err=%v", item.args[1], strings.TrimSpace(string(output)), err)
		}
	}
	command := exec.CommandContext(ctx, "git", "-C", manifest.SourceRoot, "diff", "--exit-code", "--quiet")
	if err := command.Run(); err != nil {
		t.Fatalf("source worktree has tracked edits relative to its recorded commit: %v", err)
	}
	command = exec.CommandContext(ctx, "git", "-C", manifest.SourceRoot, "diff", "--cached", "--exit-code", "--quiet")
	if err := command.Run(); err != nil {
		t.Fatalf("source worktree has staged edits relative to its recorded commit: %v", err)
	}
}

func assertFileDigest(t *testing.T, path, want string) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open identity input: %v", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		t.Fatalf("hash identity input: %v", err)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != want {
		t.Fatalf("identity input SHA-256 = %s, want %s", got, want)
	}
}

func reserveASRLiveCorrelationEndpoint(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve isolated loopback endpoint: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release reserved loopback endpoint: %v", err)
	}
	endpoint := "grpc://" + address
	_, portText, err := net.SplitHostPort(address)
	port, portErr := strconv.Atoi(portText)
	if err != nil || portErr != nil || port == 7437 {
		t.Fatalf("reserved endpoint is invalid or prohibited: split=%v port=%v", err, portErr)
	}
	return endpoint
}

func asrLiveCorrelationRuntimeConfig(endpoint string) models.RuntimeConfig {
	return models.RuntimeConfig{
		Workers: []models.RuntimeWorker{{
			Name: "asr-worker", Type: models.RuntimeWorkerTypeInference,
			Model: models.BuiltInModelNameASR, ModelLocality: models.RuntimeModelLocalityLocal,
			Args:       []string{"--grpc-endpoint", endpoint},
			Operations: []models.RuntimeOperation{{Name: models.OperationASR}},
		}},
		Resources: []models.RuntimeResource{{
			Name: "asr-cache", Type: models.RuntimeResourceTypeModel, Capacity: 1,
			Model: models.BuiltInModelNameASR, Backend: "localai-whisper",
			LoadPolicy: string(models.LoadPolicyOnDemand), Provider: "LOCAL",
		}},
	}
}

func asrLiveCorrelationRequest(scope models.RuntimeScopeRef, audio []byte) models.InvokeModelRequest {
	return models.InvokeModelRequest{
		Scope: scope, Holder: "asr-live-correlation", ModelName: models.BuiltInModelNameASR,
		Model:     models.ModelReference{NameOrURI: models.BuiltInModelNameASR},
		Operation: models.OperationASR, Offline: true,
		Inputs: []models.InferenceInput{{
			Name: "audio", Modality: models.ModalityAudio,
			ContentType: "audio/wav", MediaType: "audio/wav", Content: string(audio),
		}},
	}
}

func newASRLiveCorrelationModelsService(
	t *testing.T,
	manifest asrLiveCorrelationHarnessManifest,
	launcher *asrLiveCorrelationProcessLauncher,
	evidenceSink modelseffects.RuntimeEvidenceRecorder,
) models.Service {
	t.Helper()
	baseBackendResolver, err := modelswire.NewDefaultBackendArtifactResolver()
	if err != nil {
		t.Fatalf("load pinned LocalAI backend manifest: %v", err)
	}
	backendResolver := modelswire.BackendArtifactResolver(func(ctx context.Context, configuration modelseffects.ResolvedHostConfiguration) (modelseffects.BackendArtifactSelection, error) {
		selection, resolveErr := baseBackendResolver(ctx, configuration)
		if resolveErr != nil {
			return modelseffects.BackendArtifactSelection{}, resolveErr
		}
		if selection.SHA256 != manifest.BackendArtifactSHA256 {
			return modelseffects.BackendArtifactSelection{}, errors.New("pinned backend artifact identity does not match the immutable manifest")
		}
		return selection, nil
	})
	compatibility, err := modelswire.NewDefaultHostCompatibilityChecker()
	if err != nil {
		t.Fatalf("construct pinned LocalAI compatibility checker: %v", err)
	}
	coordination, err := platformlocking.New(platformlocking.LocalFileSystem{})
	if err != nil {
		t.Fatalf("construct isolated asset coordination: %v", err)
	}
	commandRunner, err := platformprocess.NewExecCommandRunner(exec.Command, platformclock.Real{}, nil, nil)
	if err != nil {
		t.Fatalf("construct Models runtime command runner: %v", err)
	}
	protocol := modelswire.NewPinnedGRPCHostProtocolNegotiator(
		platformgrpc.NetworkDialer{}, platformfilesystem.Local{}.EvalSymlinks,
	)
	service, err := modelswire.NewServiceWithBackendArtifactResolverAndInvocationProtocolAndDialerAndRuntimeEvidence(
		models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64"},
		asrLiveCorrelationDenyHTTP{}, models.RuntimeAssetEndpoints{},
		os.MkdirAll, os.Stat,
		func() (string, error) { return manifest.StateRoot, nil },
		os.WriteFile, os.Rename, os.Remove, os.ReadFile, os.ReadDir,
		func(path string) (io.WriteCloser, error) { return os.Create(path) },
		func(path string) (io.ReadCloser, error) { return os.Open(path) },
		launcher, asrLiveCorrelationDenyHTTP{}, asrLiveCorrelationHostClock{}, commandRunner,
		asrLiveCorrelationDenyHTTP{}, os.Stat,
		func() string { return manifest.StagingRoot },
		func(directory, pattern string) (modelswire.RuntimeTempFile, error) {
			file, createErr := os.CreateTemp(directory, pattern)
			if createErr != nil {
				return nil, createErr
			}
			return file, nil
		},
		zap.NewNop(), time.Now, platformrandom.CryptoSource{},
		nil, nil, nil, modelseffects.LocalRuntimeHooks{}, os.Getenv,
		protocol, compatibility, coordination, platformfilesystem.Local{}.EvalSymlinks,
		backendResolver, nil, platformgrpc.NetworkDialer{}, nil, nil, nil,
		modelseffects.NewOrderedRuntimeEvidenceRecorder(evidenceSink),
	)
	if err != nil {
		t.Fatalf("construct private Models service: %v", err)
	}
	return service
}

type asrLiveCorrelationDenyHTTP struct{}

func (asrLiveCorrelationDenyHTTP) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("external HTTP is disabled for this local-real harness")
}

type asrLiveCorrelationHostClock struct{}

func (asrLiveCorrelationHostClock) Now() time.Time { return time.Now() }
func (asrLiveCorrelationHostClock) NewTimer(duration time.Duration) modelswire.HostTimer {
	return asrLiveCorrelationHostTimer{timer: time.NewTimer(duration)}
}

type asrLiveCorrelationHostTimer struct{ timer *time.Timer }

func (timer asrLiveCorrelationHostTimer) C() <-chan time.Time { return timer.timer.C }
func (timer asrLiveCorrelationHostTimer) Stop() bool          { return timer.timer.Stop() }

type asrLiveCorrelationProcessLauncher struct {
	mu         sync.Mutex
	controller *modelseffects.ASRLiveCorrelationController
	endpoint   string
	manifest   asrLiveCorrelationHarnessManifest
	started    bool
	owned      *asrLiveCorrelationManagedProcess
}

func (launcher *asrLiveCorrelationProcessLauncher) Start(
	ctx context.Context,
	spec modelseffects.HostProcessStartSpec,
) (modelseffects.HostManagedProcess, error) {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	if launcher.started || strings.TrimSpace(spec.Command) != "" || strings.TrimSpace(spec.HealthEndpoint) != "" ||
		!sameASRLiveCorrelationExecutable(launcher.manifest.ExecutablePath, launcher.manifest.ExecutablePath, launcher.manifest.ExecutableSHA256) {
		return nil, errors.New("ASR managed process ownership or executable identity mismatch")
	}
	address, err := asrLiveCorrelationEndpointAddress(launcher.endpoint)
	if err != nil {
		return nil, err
	}
	launcher.started = true
	childContext := context.Background()
	if ctx != nil {
		childContext = context.WithoutCancel(ctx)
	}
	args := append([]string(nil), spec.Args...)
	args = append(args, "--addr="+address)
	workDir := strings.TrimSpace(spec.WorkDir)
	if workDir == "" {
		workDir = filepath.Dir(launcher.manifest.ExecutablePath)
	}
	process, err := managedchild.Start(childContext, managedchild.Spec{
		Command: launcher.manifest.ExecutablePath, Args: args,
		Env: asrLiveCorrelationOfflineEnvironment(spec.Env), WorkDir: workDir,
	})
	if err != nil {
		return nil, errors.New("could not start the manifest-owned LocalAI executable")
	}
	owned := &asrLiveCorrelationManagedProcess{
		child: process, endpoint: launcher.endpoint, controller: launcher.controller,
		pid: process.PID(), waitDone: make(chan struct{}),
	}
	if err := launcher.controller.RecordManagedChildStarted(owned.pid, launcher.endpoint); err != nil {
		_ = process.Stop(context.Background())
		return nil, err
	}
	launcher.owned = owned
	return owned, nil
}

func asrLiveCorrelationEndpointAddress(endpoint string) (string, error) {
	address := strings.TrimSpace(endpoint)
	for _, prefix := range []string{"grpc://", "http://", "https://"} {
		if strings.HasPrefix(strings.ToLower(address), prefix) {
			address = strings.TrimSpace(address[len(prefix):])
			break
		}
	}
	host, portText, err := net.SplitHostPort(address)
	port, portErr := strconv.Atoi(portText)
	if err != nil || portErr != nil || net.ParseIP(host) == nil || port < 1 || port > 65535 {
		return "", errors.New("ASR live-correlation endpoint is not a host:port address")
	}
	return address, nil
}

func TestASRLiveCorrelationBackendLaunchAddress(t *testing.T) {
	for _, endpoint := range []string{
		"grpc://127.0.0.1:50052",
		"127.0.0.1:50053",
	} {
		address, err := asrLiveCorrelationEndpointAddress(endpoint)
		if err != nil || address == "" {
			t.Fatalf("normalize backend endpoint %q: %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{"grpc://127.0.0.1", "grpc://not-an-ip:50052"} {
		if _, err := asrLiveCorrelationEndpointAddress(endpoint); err == nil {
			t.Fatalf("accepted invalid backend endpoint %q", endpoint)
		}
	}
}

func (launcher *asrLiveCorrelationProcessLauncher) process() *asrLiveCorrelationManagedProcess {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.owned
}

type asrLiveCorrelationManagedProcess struct {
	child      *managedchild.Process
	endpoint   string
	controller *modelseffects.ASRLiveCorrelationController
	pid        int
	waitDone   chan struct{}
	waitOnce   sync.Once
	stopOnce   sync.Once
	waitCalls  atomic.Int32
	stopCalls  atomic.Int32
	waitErr    error
}

func (process *asrLiveCorrelationManagedProcess) HealthEndpoint() string { return process.endpoint }
func (process *asrLiveCorrelationManagedProcess) PID() int               { return process.pid }

func (process *asrLiveCorrelationManagedProcess) Wait() error {
	process.waitCalls.Add(1)
	process.waitOnce.Do(func() {
		process.waitErr = process.child.Wait()
		snapshot, ready := process.child.Snapshot()
		child := modelseffects.ASRLiveCorrelationChild{ProcessID: process.pid, ExitClass: "WAIT_FAILED"}
		if ready {
			child.ExitClass = string(snapshot.ExitClass)
			child.ExitCodeKnown = snapshot.ExitCodeKnown
			child.ExitCode = snapshot.ExitCode
		}
		process.waitErr = errors.Join(process.waitErr, process.controller.RecordChildWaited(
			child.ProcessID, child.ExitClass, child.ExitCodeKnown, child.ExitCode,
		))
		close(process.waitDone)
	})
	<-process.waitDone
	return process.waitErr
}

func (process *asrLiveCorrelationManagedProcess) RuntimeHostFailureObserved() {
	_ = process.controller.RecordHostFailureObserved(process.pid)
}

func (process *asrLiveCorrelationManagedProcess) Stop(ctx context.Context) error {
	var stopErr error
	process.stopOnce.Do(func() {
		process.stopCalls.Add(1)
		if _, terminal := process.child.Snapshot(); !terminal {
			if err := process.controller.RecordChildStopRequested(process.pid); err != nil {
				stopErr = err
				return
			}
		}
		stopErr = process.child.Stop(ctx)
	})
	return stopErr
}

func (process *asrLiveCorrelationManagedProcess) waitCount() int32 { return process.waitCalls.Load() }
func (process *asrLiveCorrelationManagedProcess) stopCount() int32 { return process.stopCalls.Load() }

func asrLiveCorrelationOfflineEnvironment(base []string) []string {
	if len(base) == 0 {
		base = os.Environ()
	}
	values := make(map[string]string, len(base)+6)
	for _, item := range base {
		key, value, ok := strings.Cut(item, "=")
		if ok && strings.TrimSpace(key) != "" {
			values[strings.ToUpper(key)] = value
		}
	}
	values["HF_HUB_OFFLINE"] = "1"
	values["TRANSFORMERS_OFFLINE"] = "1"
	values["HTTP_PROXY"] = "http://127.0.0.1:9"
	values["HTTPS_PROXY"] = "http://127.0.0.1:9"
	values["ALL_PROXY"] = "http://127.0.0.1:9"
	values["NO_PROXY"] = "127.0.0.1,localhost,::1"
	environment := make([]string, 0, len(values))
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := values[key]
		environment = append(environment, key+"="+value)
	}
	return environment
}

func sameASRLiveCorrelationExecutable(command, expected, wantSHA256 string) bool {
	if strings.TrimSpace(filepath.Base(command)) == "" ||
		!strings.EqualFold(filepath.Base(command), filepath.Base(expected)) {
		return false
	}
	file, err := os.Open(command)
	if err != nil {
		return false
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false
	}
	return strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), wantSHA256)
}

func TestASRLiveCorrelationMaterializedExecutableIdentity(t *testing.T) {
	root := t.TempDir()
	expected := filepath.Join(root, "manifest", "whisper.exe")
	actual := filepath.Join(root, "runtime", "whisper.exe")
	for _, path := range []string{expected, actual} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("prepare executable identity fixture: %v", err)
		}
	}
	contents := []byte("same pinned executable bytes")
	if err := os.WriteFile(expected, contents, 0o700); err != nil {
		t.Fatalf("write expected executable fixture: %v", err)
	}
	if err := os.WriteFile(actual, contents, 0o700); err != nil {
		t.Fatalf("write materialized executable fixture: %v", err)
	}
	hash := sha256.Sum256(contents)
	want := hex.EncodeToString(hash[:])
	if !sameASRLiveCorrelationExecutable(actual, expected, want) {
		t.Fatal("materialized executable with matching name and digest was rejected")
	}
	if sameASRLiveCorrelationExecutable(filepath.Join(root, "runtime", "other.exe"), expected, want) {
		t.Fatal("materialized executable with a different name was accepted")
	}
	if err := os.WriteFile(actual, []byte("different executable bytes"), 0o700); err != nil {
		t.Fatalf("change materialized executable fixture: %v", err)
	}
	if sameASRLiveCorrelationExecutable(actual, expected, want) {
		t.Fatal("materialized executable with a different digest was accepted")
	}
}

func validHarnessScenario(value string) bool {
	return value == modelseffects.ASRLiveCorrelationScenarioResponseFirst ||
		value == modelseffects.ASRLiveCorrelationScenarioExitFirst
}

func safeHarnessRunID(value string) bool {
	if value == "" || len(value) > 96 {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-') {
			return false
		}
	}
	return true
}

func validHarnessGitID(value string) bool {
	return len(value) == 40 && validHarnessLowerHex(value)
}

func validHarnessDigest(value string) bool {
	return len(value) == sha256.Size*2 && validHarnessLowerHex(value)
}

func validHarnessLowerHex(value string) bool {
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return value != ""
}

var _ modelswire.HostProcessLauncher = (*asrLiveCorrelationProcessLauncher)(nil)
var _ modelseffects.HostManagedProcessFailureObserver = (*asrLiveCorrelationManagedProcess)(nil)
var _ modelseffects.RuntimeEvidenceRecorder = (*asrLiveCorrelationRuntimeEvidenceSink)(nil)
