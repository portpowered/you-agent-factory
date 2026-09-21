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
	if manifest.Schema != asrLiveCorrelationManifestSchema ||
		!safeHarnessRunID(manifest.RunID) || !validHarnessScenario(manifest.Scenario) ||
		!validHarnessGitID(manifest.SourceCommit) || !validHarnessGitID(manifest.SourceTree) ||
		!validHarnessDigest(manifest.ExecutableSHA256) || !validHarnessDigest(manifest.WAVSHA256) ||
		!validHarnessDigest(manifest.ModelIdentitySHA256) || !validHarnessDigest(manifest.BackendArtifactSHA256) ||
		!validHarnessDigest(manifest.CacheManifestSHA256) || manifest.TimeoutSeconds < 1 || manifest.TimeoutSeconds > 180 {
		return errors.New("manifest identity or bounded timeout is invalid")
	}
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
	fixture := verifyASRLiveCorrelationInputs(t, ctx, manifest)
	endpoint := reserveASRLiveCorrelationEndpoint(t)
	controller, err := modelseffects.NewASRLiveCorrelationController(
		modelseffects.WindowsASRLiveCorrelationListenerPIDLookup,
	)
	if err != nil {
		t.Fatalf("construct ASR live-correlation controller: %v", err)
	}
	evidenceSink := &asrLiveCorrelationRuntimeEvidenceSink{}
	launcher := &asrLiveCorrelationProcessLauncher{
		controller: controller, endpoint: endpoint, manifest: manifest,
	}
	service := newASRLiveCorrelationModelsService(t, manifest, launcher, evidenceSink)
	opened, err := service.OpenRuntimeScope(ctx, models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{
			CacheDirectory: manifest.CacheRoot,
			Runtime:        asrLiveCorrelationRuntimeConfig(manifest.ExecutablePath, endpoint),
		},
	})
	if err != nil {
		t.Fatalf("open isolated ASR Runtime Scope: %v", err)
	}
	closed := false
	defer func() {
		if process := launcher.process(); process != nil {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = process.Stop(cleanupCtx)
			cleanupCancel()
		}
		if !closed {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			_, _ = service.StopModelHost(cleanupCtx, models.StopModelHostRequest{Scope: opened.Scope, Name: models.BuiltInModelNameASR})
			_, _ = service.CloseRuntimeScope(cleanupCtx, models.CloseRuntimeScopeRequest{Scope: opened.Scope})
		}
	}()

	invocations := make(chan asrLiveCorrelationInvocation, 1)
	processStopped := false
	go func() {
		result, invokeErr := service.InvokeModel(
			modelseffects.WithASRLiveCorrelation(ctx, controller),
			asrLiveCorrelationRequest(opened.Scope, fixture.audio),
		)
		invocations <- asrLiveCorrelationInvocation{result: result, err: invokeErr}
	}()
	endpointEvent := awaitASRLiveCorrelationSignal(t, ctx, controller, modelseffects.ASRLiveCorrelationEndpointObserved)
	terminalEvent := awaitASRLiveCorrelationSignal(t, ctx, controller, modelseffects.ASRLiveCorrelationRPCTerminal)
	if endpointEvent.ProcessID <= 0 || endpointEvent.Endpoint.Port == 7437 || terminalEvent.Sequence <= endpointEvent.Sequence {
		t.Fatalf("ASR endpoint/RPC facts are not owned and ordered: endpoint=%#v terminal=%#v", endpointEvent, terminalEvent)
	}

	var invocation asrLiveCorrelationInvocation
	var transcript string
	if manifest.Scenario == modelseffects.ASRLiveCorrelationScenarioResponseFirst {
		if err := controller.ReleaseResponse(); err != nil {
			t.Fatalf("release response-first decoded ASR result: %v", err)
		}
		invocation = awaitASRLiveCorrelationInvocation(t, ctx, invocations)
		transcript = assertASRLiveCorrelationResponseFirst(t, invocation)
	} else {
		process := launcher.process()
		if process == nil || process.PID() != endpointEvent.ProcessID {
			t.Fatalf("owned process does not match endpoint PID: process=%v endpoint=%#v", process, endpointEvent)
		}
		if err := process.Stop(ctx); err != nil {
			t.Fatalf("stop owned LocalAI child after RPC terminal: %v", err)
		}
		processStopped = true
		awaitASRLiveCorrelationSignal(t, ctx, controller, modelseffects.ASRLiveCorrelationChildWaited)
		awaitASRLiveCorrelationSignal(t, ctx, controller, modelseffects.ASRLiveCorrelationHostFailureSeen)
		if err := controller.ReleaseResponse(); err != nil {
			t.Fatalf("release exit-first decoded ASR result after host failure: %v", err)
		}
		invocation = awaitASRLiveCorrelationInvocation(t, ctx, invocations)
		assertASRLiveCorrelationResponseFirstFailure(t, invocation)
	}
	process := launcher.process()
	if process == nil || process.PID() != endpointEvent.ProcessID {
		t.Fatalf("managed process PID does not match endpoint ownership: process=%v endpoint=%#v", process, endpointEvent)
	}
	if !processStopped {
		if err := process.Stop(ctx); err != nil {
			t.Fatalf("stop owned LocalAI child after ASR result: %v", err)
		}
	}
	awaitASRLiveCorrelationSignal(t, ctx, controller, modelseffects.ASRLiveCorrelationChildWaited)
	awaitASRLiveCorrelationSignal(t, ctx, controller, modelseffects.ASRLiveCorrelationHostFailureSeen)
	if process.waitCount() != 1 || process.stopCount() != 1 {
		t.Fatalf("owned process Wait/Stop counts = %d/%d, want one each", process.waitCount(), process.stopCount())
	}
	stopASRLiveCorrelationScope(t, ctx, service, opened.Scope)
	closed = true
	assertASRLiveCorrelationCleanup(t, manifest, endpointEvent.Endpoint.Port)
	protocol := asrLiveCorrelationProtocolEvidence(t, evidenceSink.snapshot())
	if protocol.ModelIdentitySHA256 != manifest.ModelIdentitySHA256 ||
		protocol.BackendArtifactSHA256 != manifest.BackendArtifactSHA256 ||
		protocol.RPCMethod != modelseffects.RuntimeASRPCMethodAudioTranscription ||
		protocol.RPCStatus != "OK" || !protocol.ResponseDecoded {
		t.Fatalf("live ASR protocol evidence mismatches the immutable manifest: %#v", protocol)
	}
	writeASRLiveCorrelationEvidence(t, manifest, fixture, endpoint, controller.Snapshot(), protocol, invocation, transcript)
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

func asrLiveCorrelationRuntimeConfig(executable, endpoint string) models.RuntimeConfig {
	return models.RuntimeConfig{
		Workers: []models.RuntimeWorker{{
			Name: "asr-worker", Type: models.RuntimeWorkerTypeInference,
			Model: models.BuiltInModelNameASR, ModelLocality: models.RuntimeModelLocalityLocal,
			Command: executable, Args: []string{"--grpc-endpoint", endpoint},
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
	if launcher.started || spec.HealthEndpoint != launcher.endpoint || !sameWindowsPath(spec.Command, launcher.manifest.ExecutablePath) {
		return nil, errors.New("ASR managed process ownership or executable identity mismatch")
	}
	launcher.started = true
	childContext := context.Background()
	if ctx != nil {
		childContext = context.WithoutCancel(ctx)
	}
	process, err := managedchild.Start(childContext, managedchild.Spec{
		Command: spec.Command, Args: append([]string(nil), spec.Args...),
		Env: asrLiveCorrelationOfflineEnvironment(spec.Env), WorkDir: spec.WorkDir,
	})
	if err != nil {
		return nil, errors.New("could not start the manifest-owned LocalAI executable")
	}
	owned := &asrLiveCorrelationManagedProcess{
		child: process, endpoint: spec.HealthEndpoint, controller: launcher.controller,
		pid: process.PID(), waitDone: make(chan struct{}),
	}
	if err := launcher.controller.RecordManagedChildStarted(owned.pid, spec.HealthEndpoint); err != nil {
		_ = process.Stop(context.Background())
		return nil, err
	}
	launcher.owned = owned
	return owned, nil
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

func sameWindowsPath(left, right string) bool {
	leftPath, leftErr := filepath.Abs(strings.TrimSpace(left))
	rightPath, rightErr := filepath.Abs(strings.TrimSpace(right))
	return leftErr == nil && rightErr == nil && strings.EqualFold(filepath.Clean(leftPath), filepath.Clean(rightPath))
}

func awaitASRLiveCorrelationSignal(
	t *testing.T,
	ctx context.Context,
	controller *modelseffects.ASRLiveCorrelationController,
	kind modelseffects.ASRLiveCorrelationEventKind,
) modelseffects.ASRLiveCorrelationEvent {
	t.Helper()
	event, err := controller.WaitForSignal(ctx, kind)
	if err != nil {
		t.Fatalf("wait for ASR live-correlation signal %s: %v", kind, err)
	}
	return event
}

func awaitASRLiveCorrelationInvocation(
	t *testing.T,
	ctx context.Context,
	invocations <-chan asrLiveCorrelationInvocation,
) asrLiveCorrelationInvocation {
	t.Helper()
	select {
	case invocation := <-invocations:
		return invocation
	case <-ctx.Done():
		t.Fatalf("ASR invocation did not finish before its bounded ceiling: %v", ctx.Err())
		return asrLiveCorrelationInvocation{}
	}
}

func assertASRLiveCorrelationResponseFirst(
	t *testing.T,
	invocation asrLiveCorrelationInvocation,
) string {
	t.Helper()
	if invocation.err != nil || invocation.result.Status != models.ModelInvocationStatusCompleted ||
		invocation.result.LeaseDisposition != models.InvocationLeaseReleased || len(invocation.result.Outputs) != 2 {
		t.Fatalf("response-first Models result = %#v error=%v, want completed two-output result", invocation.result, invocation.err)
	}
	var transcript string
	var segments []models.ASRBackendSegment
	for _, content := range invocation.result.Content {
		switch content.Name {
		case "transcript":
			transcript = strings.TrimSpace(content.Content)
		case "segments":
			if err := json.Unmarshal([]byte(content.Content), &segments); err != nil {
				t.Fatalf("decode normalized ASR segments: %v", err)
			}
		}
	}
	if transcript == "" || len(segments) == 0 {
		t.Fatalf("response-first ASR semantic output is empty: transcriptBytes=%d segments=%d", len(transcript), len(segments))
	}
	previousStart, previousEnd := int64(-1), int64(-1)
	for index, segment := range segments {
		if segment.Start < 0 || segment.End < segment.Start ||
			index > 0 && (segment.Start < previousStart || segment.Start < previousEnd) {
			t.Fatalf("ASR segment[%d] is not finite, nonnegative and ordered: %#v", index, segment)
		}
		previousStart, previousEnd = segment.Start, segment.End
	}
	return transcript
}

func assertASRLiveCorrelationResponseFirstFailure(
	t *testing.T,
	invocation asrLiveCorrelationInvocation,
) {
	t.Helper()
	if invocation.err == nil || !errors.Is(invocation.err, models.ErrInferenceFailed) ||
		!errors.Is(invocation.err, models.ErrHostLeaseExpired) ||
		invocation.result.Status != models.ModelInvocationStatusFailed ||
		invocation.result.LeaseDisposition != models.InvocationLeaseExpired ||
		len(invocation.result.Content) != 0 || len(invocation.result.Outputs) != 0 {
		t.Fatalf("exit-first Models result = %#v error=%v, want typed lease failure with zero outputs", invocation.result, invocation.err)
	}
}

func stopASRLiveCorrelationScope(
	t *testing.T,
	ctx context.Context,
	service models.Service,
	scope models.RuntimeScopeRef,
) {
	t.Helper()
	if _, err := service.StopModelHost(ctx, models.StopModelHostRequest{Scope: scope, Name: models.BuiltInModelNameASR}); err != nil &&
		!errors.Is(err, models.ErrHostRuntimeNotReady) {
		t.Fatalf("stop ASR Runtime Host: %v", err)
	}
	if _, err := service.CloseRuntimeScope(ctx, models.CloseRuntimeScopeRequest{Scope: scope}); err != nil {
		t.Fatalf("close ASR Runtime Scope: %v", err)
	}
}

func assertASRLiveCorrelationCleanup(
	t *testing.T,
	manifest asrLiveCorrelationHarnessManifest,
	port int,
) {
	t.Helper()
	entries, err := os.ReadDir(manifest.StagingRoot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("ASR staged audio cleanup entries=%v err=%v", entries, err)
	}
	_, err = modelseffects.WindowsASRLiveCorrelationListenerPIDLookup(context.Background(), "127.0.0.1", port)
	if !errors.Is(err, modelseffects.ErrASRLiveCorrelationListenerAbsent) {
		t.Fatalf("owned ASR listener remains or could not be classified: %v", err)
	}
}

func (sink *asrLiveCorrelationRuntimeEvidenceSink) RecordRuntimeEvidence(record modelseffects.RuntimeEvidenceRecord) {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	copyRecord := record
	if record.ASRProtocol != nil {
		protocol := *record.ASRProtocol
		copyRecord.ASRProtocol = &protocol
	}
	sink.records = append(sink.records, copyRecord)
}

func (sink *asrLiveCorrelationRuntimeEvidenceSink) snapshot() []modelseffects.RuntimeEvidenceRecord {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return append([]modelseffects.RuntimeEvidenceRecord(nil), sink.records...)
}

func asrLiveCorrelationProtocolEvidence(
	t *testing.T,
	records []modelseffects.RuntimeEvidenceRecord,
) modelseffects.RuntimeASRProtocolObservation {
	t.Helper()
	for _, record := range records {
		if record.Kind == modelseffects.RuntimeEvidenceKindASRProtocol && record.ASRProtocol != nil {
			return *record.ASRProtocol
		}
	}
	t.Fatal("Models runtime did not emit the private ASR protocol observation")
	return modelseffects.RuntimeASRProtocolObservation{}
}

func writeASRLiveCorrelationEvidence(
	t *testing.T,
	manifest asrLiveCorrelationHarnessManifest,
	fixture asrLiveCorrelationFixture,
	endpoint string,
	events []modelseffects.ASRLiveCorrelationEvent,
	protocol modelseffects.RuntimeASRProtocolObservation,
	invocation asrLiveCorrelationInvocation,
	transcript string,
) {
	t.Helper()
	endpointEvent := findASRLiveCorrelationEvent(t, events, modelseffects.ASRLiveCorrelationEndpointObserved)
	terminalEvent := findASRLiveCorrelationEvent(t, events, modelseffects.ASRLiveCorrelationRPCTerminal)
	childEvent := findASRLiveCorrelationEvent(t, events, modelseffects.ASRLiveCorrelationChildWaited)
	application := modelseffects.ASRLiveCorrelationApplication{
		Outcome: "COMPLETED", OutputCount: len(invocation.result.Outputs),
	}
	if invocation.err != nil {
		application.Outcome = "FAILED"
		application.ErrorClasses = []string{"INFERENCE_FAILED", "HOST_LEASE_EXPIRED"}
		application.OutputCount = len(invocation.result.Outputs)
	}
	evidence := modelseffects.ASRLiveCorrelationEvidence{
		Schema: modelseffects.ASRLiveCorrelationEvidenceSchema,
		RunID:  manifest.RunID, Scenario: manifest.Scenario,
		SourceCommit: manifest.SourceCommit, SourceTree: manifest.SourceTree,
		GoToolchain:   runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH,
		ExecutableSHA: manifest.ExecutableSHA256, WAVSHA: manifest.WAVSHA256,
		ModelSHA: protocol.ModelIdentitySHA256, BackendSHA: protocol.BackendArtifactSHA256,
		CacheSHA: manifest.CacheManifestSHA256, Endpoint: endpointEvent.Endpoint,
		RPC: modelseffects.ASRLiveCorrelationRPC{
			Method: modelseffects.RuntimeASRPCMethodAudioTranscription, Status: "OK",
			TerminalSequence: terminalEvent.Sequence, ResponseDecoded: protocol.ResponseDecoded,
			RequestSemanticSHA256:  terminalEvent.RequestSemanticSHA256,
			ResponseSemanticSHA256: terminalEvent.ResponseSemanticSHA256,
		},
		Child: modelseffects.ASRLiveCorrelationChild{
			ProcessID: childEvent.ProcessID, WaitSequence: childEvent.Sequence,
			ExitClass: childEvent.ExitClass, ExitCodeKnown: childEvent.ExitCodeKnown,
			ExitCode: childEvent.ExitCode,
		},
		Application: application, RedactionPassed: true,
	}
	if err := modelseffects.ValidateASRLiveCorrelationEvidence(evidence); err != nil {
		t.Fatalf("live ASR evidence does not meet the additive schema: %v", err)
	}
	encoded, err := modelseffects.MarshalASRLiveCorrelationEvidence(evidence)
	if err != nil {
		t.Fatalf("serialize redacted live ASR evidence: %v", err)
	}
	endpointAddress := endpoint
	privateValues := [][]byte{
		[]byte(manifest.SourceRoot), []byte(manifest.ExecutablePath), []byte(manifest.WAVPath),
		[]byte(manifest.CacheRoot), []byte(manifest.CacheManifestPath), []byte(manifest.StateRoot),
		[]byte(manifest.StagingRoot), []byte(manifest.EvidenceOutput), []byte(endpointAddress),
		fixture.audio, []byte(transcript),
	}
	if invocation.err != nil {
		privateValues = append(privateValues, []byte(invocation.err.Error()))
	}
	for _, privateValue := range privateValues {
		if len(privateValue) != 0 && bytes.Contains(encoded, privateValue) {
			t.Fatalf("serialized ASR evidence contains a raw private value")
		}
	}
	if err := writeASRLiveCorrelationFileExclusive(manifest.EvidenceOutput, encoded); err != nil {
		t.Fatalf("write exclusive ASR evidence output: %v", err)
	}
	t.Logf("ASR_LIVE_CORRELATION scenario=%s request_sha256=%s response_sha256=%s endpoint_port=%d listener_pid=%d output_count=%d", manifest.Scenario, terminalEvent.RequestSemanticSHA256, terminalEvent.ResponseSemanticSHA256, endpointEvent.Endpoint.Port, endpointEvent.ProcessID, application.OutputCount)
}

func findASRLiveCorrelationEvent(
	t *testing.T,
	events []modelseffects.ASRLiveCorrelationEvent,
	kind modelseffects.ASRLiveCorrelationEventKind,
) modelseffects.ASRLiveCorrelationEvent {
	t.Helper()
	for _, event := range events {
		if event.Kind == kind {
			return event
		}
	}
	t.Fatalf("ASR live-correlation event %s is missing", kind)
	return modelseffects.ASRLiveCorrelationEvent{}
}

func writeASRLiveCorrelationFileExclusive(path string, contents []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.New("could not prepare private evidence directory")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("could not create a fresh private evidence file")
	}
	if _, err := file.Write(append(contents, '\n')); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return errors.New("could not write private evidence")
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return errors.New("could not sync private evidence")
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return errors.New("could not close private evidence")
	}
	return nil
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
