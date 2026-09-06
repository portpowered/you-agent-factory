package wire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformlocking "github.com/portpowered/infinite-you/pkg/platform/locking"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
	managedbackend "github.com/portpowered/infinite-you/pkg/wire/internal/managedbackend"
	"go.uber.org/zap"
)

const (
	// Asset responses can carry multi-gigabyte model files, so the asset client
	// deliberately has no whole-request deadline. These phase limits protect
	// connection setup and response headers while the caller's context remains
	// responsible for stopping an active transfer.
	modelAssetDialTimeout           = 15 * time.Second
	modelAssetKeepAlive             = 30 * time.Second
	modelAssetTLSHandshakeTimeout   = 15 * time.Second
	modelAssetResponseHeaderTimeout = 30 * time.Second
	modelHostHTTPTimeout            = 2 * time.Second
	modelRuntimeHTTPTimeout         = 5 * time.Minute
	modelRuntimeEvidenceEnvironment = "INFINITE_YOU_INTEGRATION_MODEL_RUNTIME_EVIDENCE"
)

type modelRuntimeEvidenceFileRecorder struct {
	mu   sync.Mutex
	path string
}

func (recorder *modelRuntimeEvidenceFileRecorder) RecordRuntimeEvidence(
	record modelswire.RuntimeEvidenceRecord,
) {
	if recorder == nil || strings.TrimSpace(recorder.path) == "" {
		return
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return
	}
	payload = append(payload, '\n')

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	file, err := os.OpenFile(
		recorder.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600,
	)
	if err != nil {
		return
	}
	defer file.Close()
	_ = file.Chmod(0o600)
	_, _ = file.Write(payload)
}

func provideModelRuntimeEvidenceRecorder() (modelswire.RuntimeEvidenceRecorder, error) {
	path := strings.TrimSpace(os.Getenv(modelRuntimeEvidenceEnvironment))
	if path == "" {
		return nil, nil
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("%s must be an absolute path", modelRuntimeEvidenceEnvironment)
	}
	path = filepath.Clean(path)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open model runtime evidence path: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("set model runtime evidence permissions: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close model runtime evidence path: %w", err)
	}
	return modelswire.NewOrderedRuntimeEvidenceRecorder(
		&modelRuntimeEvidenceFileRecorder{path: path},
	), nil
}

// TODO: this should be decomposed, we should inject these independently.
// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func provideModelsService(edges serviceedges.Edges) (models.Service, error) {
	processClock := edges.Clock
	if processClock == nil {
		processClock = platformclock.Real{}
	}
	assetPlatform := provideModelAssetHostPlatform(edges)
	assetEndpoints := edges.ModelAssetEndpoints
	assetEnvironment := edges.ModelAssetResolveEnvironment
	if assetEnvironment == nil {
		assetEnvironment = os.Getenv
	}

	assetHTTP := edges.ModelAssetHTTPClient
	if assetHTTP == nil {
		assetHTTP = newModelAssetHTTPClient()
	}
	assetMkdirAll := edges.ModelAssetMakeDirectories
	if assetMkdirAll == nil {
		assetMkdirAll = os.MkdirAll
	}
	assetStat := edges.ModelAssetInspectPath
	if assetStat == nil {
		assetStat = os.Stat
	}
	assetHome := edges.ModelAssetResolveHomeDirectory
	if assetHome == nil {
		assetHome = os.UserHomeDir
	}
	assetWriteFile := edges.ModelAssetWriteFile
	if assetWriteFile == nil {
		assetWriteFile = os.WriteFile
	}
	assetRename := edges.ModelAssetRenamePath
	if assetRename == nil {
		assetRename = os.Rename
	}
	assetRemove := edges.ModelAssetRemovePath
	if assetRemove == nil {
		assetRemove = os.Remove
	}
	assetReadFile := edges.ModelAssetReadFile
	if assetReadFile == nil {
		assetReadFile = os.ReadFile
	}
	assetReadDir := edges.ModelAssetReadDirectory
	if assetReadDir == nil {
		assetReadDir = os.ReadDir
	}
	assetCreate := edges.ModelAssetCreateFile
	if assetCreate == nil {
		assetCreate = func(path string) (io.WriteCloser, error) { return os.Create(path) }
	}
	assetOpen := edges.ModelAssetOpenFile
	if assetOpen == nil {
		assetOpen = func(path string) (io.ReadCloser, error) { return os.Open(path) }
	}
	var assetCoordination modelswire.AssetStagingCoordination
	var coordinationErr error
	if factory := edges.ModelAssetStagingCoordinationFactory; factory != nil {
		assetCoordination, coordinationErr = factory()
	} else {
		assetCoordination, coordinationErr = platformlocking.New(platformlocking.LocalFileSystem{})
	}
	if coordinationErr != nil {
		return nil, fmt.Errorf("construct Models asset staging coordination: %w", coordinationErr)
	}

	launcher := edges.ModelHostProcessLauncher
	if launcher == nil {
		launcher = modelsProcessLauncher{}
	}
	hostHTTP := edges.ModelHostHTTPClient
	if hostHTTP == nil {
		hostHTTP = &http.Client{Timeout: modelHostHTTPTimeout}
	}
	hostClock := edges.ModelHostClock
	if hostClock == nil {
		hostClock = modelsClock{source: processClock}
	}
	protocolDialer := edges.ModelInvocationGRPCDialer
	protocolNegotiator := adaptModelHostProtocolNegotiator(edges.ModelHostProtocolNegotiator)
	if protocolNegotiator == nil && !isNilModelEdgeDependency(edges.ModelHostGRPCDialer) {
		protocolNegotiator = modelswire.PinnedGRPCNegotiator{
			Dialer: modelHostGRPCDialerAdapter{next: edges.ModelHostGRPCDialer},
		}
	}
	if protocolNegotiator == nil {
		protocolNegotiator = modelswire.NewPinnedGRPCHostProtocolNegotiator(
			protocolDialer, platformfilesystem.Local{}.EvalSymlinks,
		)
	}
	compatibilityChecker, compatibilityErr := provideModelHostCompatibilityChecker(edges)
	if compatibilityErr != nil {
		return nil, compatibilityErr
	}
	backendArtifactResolver := adaptModelBackendArtifactResolver(edges.ModelResolveBackendArtifact)
	if backendArtifactResolver == nil {
		var resolverErr error
		backendArtifactResolver, resolverErr = modelswire.NewDefaultBackendArtifactResolver()
		if resolverErr != nil {
			return nil, fmt.Errorf("construct Models backend artifact selector: %w", resolverErr)
		}
	}
	runtimeRunner := edges.ModelRuntimeCommandRunner
	if runtimeRunner == nil {
		var runnerErr error
		runtimeRunner, runnerErr = providePlatformProcessCommandRunner(edges)
		if runnerErr != nil {
			return nil, runnerErr
		}
	}
	runtimeHTTP := edges.ModelRuntimeHTTPClient
	if runtimeHTTP == nil {
		runtimeHTTP = &http.Client{Timeout: modelRuntimeHTTPTimeout}
	}
	runtimeInspect := edges.ModelRuntimeInspectFile
	if runtimeInspect == nil {
		runtimeInspect = os.Stat
	}
	runtimeTempDir := edges.ModelRuntimeTempDirectory
	if runtimeTempDir == nil {
		runtimeTempDir = os.TempDir
	}
	runtimeTempFile := edges.ModelRuntimeCreateTempFile
	if runtimeTempFile == nil {
		runtimeTempFile = func(dir, pattern string) (interface {
			Close() error
			Name() string
		}, error) {
			return os.CreateTemp(dir, pattern)
		}
	}
	runtimeEvidence, err := provideModelRuntimeEvidenceRecorder()
	if err != nil {
		return nil, fmt.Errorf("construct Models runtime evidence recorder: %w", err)
	}

	return modelswire.NewServiceWithBackendArtifactResolverAndInvocationProtocolAndDialerAndRuntimeEvidence(
		assetPlatform,
		assetHTTP,
		assetEndpoints,
		modelswire.AssetMakeDirectories(assetMkdirAll),
		modelswire.AssetInspectPath(assetStat),
		modelswire.AssetResolveHomeDirectory(assetHome),
		modelswire.AssetWriteFile(assetWriteFile),
		modelswire.AssetRenamePath(assetRename),
		modelswire.AssetRemovePath(assetRemove),
		modelswire.AssetReadFile(assetReadFile),
		modelswire.AssetReadDirectory(assetReadDir),
		modelswire.AssetCreateFile(assetCreate),
		modelswire.AssetOpenFile(assetOpen),
		adaptModelHostProcessLauncher(launcher),
		hostHTTP,
		adaptModelHostClock(hostClock),
		runtimeRunner,
		runtimeHTTP,
		modelswire.RuntimeInspectFile(runtimeInspect),
		modelswire.RuntimeTempDirectory(runtimeTempDir),
		adaptModelRuntimeTempFile(runtimeTempFile),
		zap.NewNop(),
		processClock.Now,
		platformrandom.CryptoSource{},
		adaptModelsPullMetricsRecorder(edges.ModelPullMetricsRecorder),
		modelswire.HostDiagnosticLogger(factorysessionwire.ModelHostDiagnosticLogger(zap.NewNop())),
		modelswire.HostMetricsRecorder(factorysessionwire.ModelHostDiagnosticMetrics(edges.InvocationMetricsRecorder)),
		modelLocalRuntimeHooks(workerswire.LocalRuntimeHooks()),
		assetEnvironment,
		protocolNegotiator,
		compatibilityChecker,
		assetCoordination,
		platformfilesystem.Local{}.EvalSymlinks,
		backendArtifactResolver,
		edges.ModelInvocationProtocolClient,
		protocolDialer,
		adaptModelInvocationBackend(edges.ModelInvocationBackend),
		adaptModelASRBackend(edges.ModelASRBackend),
		adaptModelEmbeddingBackend(edges.ModelEmbeddingBackend),
		runtimeEvidence,
		edges.ModelResolveHuggingFaceRevision,
	)
}

func provideModelHostCompatibilityChecker(
	edges serviceedges.Edges,
) (modelswire.HostCompatibilityChecker, error) {
	checker := adaptModelHostCompatibilityChecker(edges.ModelHostCompatibilityChecker)
	if checker != nil {
		return checker, nil
	}
	checker, err := modelswire.NewDefaultHostCompatibilityChecker()
	if err != nil {
		return nil, fmt.Errorf("construct Models host compatibility checker: %w", err)
	}
	return checker, nil
}

func newModelAssetHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   modelAssetDialTimeout,
		KeepAlive: modelAssetKeepAlive,
	}).DialContext
	transport.TLSHandshakeTimeout = modelAssetTLSHandshakeTimeout
	transport.ResponseHeaderTimeout = modelAssetResponseHeaderTimeout
	return &http.Client{Transport: transport}
}

func adaptModelInvocationBackend(
	next serviceedges.ModelInvocationBackend,
) modelswire.InvocationBackend {
	if next == nil {
		return nil
	}
	return func(
		ctx context.Context,
		request models.InvokeModelRequest,
	) ([]models.InferenceContent, []models.InferenceArtifact, error) {
		return next(ctx, request)
	}
}

func adaptModelASRBackend(
	next serviceedges.ModelASRBackend,
) modelswire.ASRBackend {
	if next == nil {
		return nil
	}
	return func(
		ctx context.Context,
		request models.ASRBackendRequest,
	) (models.ASRBackendResponse, error) {
		return next(ctx, request)
	}
}

func adaptModelEmbeddingBackend(
	next serviceedges.ModelEmbeddingBackend,
) modelswire.EmbeddingBackend {
	if next == nil {
		return nil
	}
	return func(
		ctx context.Context,
		request models.EmbeddingBackendRequest,
	) (models.EmbeddingBackendResponse, error) {
		return next(ctx, request)
	}
}

func adaptModelBackendArtifactResolver(
	next serviceedges.ModelResolveBackendArtifact,
) modelswire.BackendArtifactResolver {
	if next == nil {
		return nil
	}
	return func(
		ctx context.Context,
		request modelswire.BackendArtifactSelectionRequest,
	) (modelswire.BackendArtifactSelection, error) {
		selection, err := next(ctx, serviceedges.ModelBackendArtifactSelectionRequest{
			Backend:         request.Backend,
			Platform:        request.Platform,
			ProtocolVersion: request.ProtocolVersion,
		})
		return modelswire.BackendArtifactSelection{
			Name:     selection.Name,
			Location: selection.Location,
			Bytes:    selection.Bytes,
			SHA256:   selection.SHA256,
		}, err
	}
}

type modelHostProtocolNegotiatorAdapter struct {
	next modelHostProtocolEdge
}

func (adapter modelHostProtocolNegotiatorAdapter) Negotiate(
	ctx context.Context,
	endpoint string,
	request modelswire.HostProtocolNegotiationRequest,
) (modelswire.HostProtocolNegotiationResult, error) {
	result, err := adapter.next.Negotiate(ctx, endpoint, serviceedges.ModelHostProtocolNegotiationRequest{
		ProtocolVersion: request.ProtocolVersion,
		Backend:         request.Backend,
		ModelName:       request.ModelName,
		Revision:        request.Revision,
		Platform:        request.Platform,
		ModelPath:       request.ModelPath,
		ModelFiles:      append([]string(nil), request.ModelFiles...),
	})
	return modelswire.HostProtocolNegotiationResult{
		ProtocolVersion: result.ProtocolVersion,
		Backend:         result.Backend,
		Ready:           result.Ready,
	}, err
}

type modelHostGRPCDialerAdapter struct {
	next modelHostGRPCDialerEdge
}

func (adapter modelHostGRPCDialerAdapter) Dial(
	ctx context.Context,
	endpoint string,
) (modelswire.HostGRPCConnection, error) {
	connection, err := adapter.next.Dial(ctx, endpoint)
	if err != nil || connection == nil {
		return nil, err
	}
	return modelHostGRPCConnectionAdapter{next: connection}, nil
}

type modelHostGRPCConnectionAdapter struct {
	next modelHostGRPCConnectionEdge
}

func (adapter modelHostGRPCConnectionAdapter) Negotiate(
	ctx context.Context,
	request modelswire.HostProtocolNegotiationRequest,
) (modelswire.HostProtocolNegotiationResult, error) {
	result, err := adapter.next.Negotiate(ctx, serviceedges.ModelHostProtocolNegotiationRequest{
		ProtocolVersion: request.ProtocolVersion,
		Backend:         request.Backend,
		ModelName:       request.ModelName,
		Revision:        request.Revision,
		Platform:        request.Platform,
		ModelPath:       request.ModelPath,
		ModelFiles:      append([]string(nil), request.ModelFiles...),
	})
	return modelswire.HostProtocolNegotiationResult{
		ProtocolVersion: result.ProtocolVersion,
		Backend:         result.Backend,
		Ready:           result.Ready,
	}, err
}

func (adapter modelHostGRPCConnectionAdapter) Close() error {
	return adapter.next.Close()
}

type modelHostCompatibilityCheckerAdapter struct {
	next modelHostCompatibilityEdge
}

func (adapter modelHostCompatibilityCheckerAdapter) Check(
	ctx context.Context,
	request modelswire.HostCompatibilityRequest,
) error {
	return adapter.next.Check(ctx, serviceedges.ModelHostCompatibilityRequest{
		Backend:   request.Backend,
		ModelName: request.ModelName,
		Revision:  request.Revision,
		Platform:  request.Platform,
	})
}

func adaptModelHostProtocolNegotiator(
	negotiator modelHostProtocolEdge,
) modelswire.HostProtocolNegotiator {
	if isNilModelEdgeDependency(negotiator) {
		return nil
	}
	return modelHostProtocolNegotiatorAdapter{next: negotiator}
}

func adaptModelHostCompatibilityChecker(
	checker modelHostCompatibilityEdge,
) modelswire.HostCompatibilityChecker {
	if isNilModelEdgeDependency(checker) {
		return nil
	}
	return modelHostCompatibilityCheckerAdapter{next: checker}
}

type modelHostProtocolEdge interface {
	Negotiate(
		context.Context,
		string,
		serviceedges.ModelHostProtocolNegotiationRequest,
	) (serviceedges.ModelHostProtocolNegotiationResult, error)
}

type modelHostGRPCDialerEdge interface {
	Dial(context.Context, string) (interface {
		Negotiate(
			context.Context,
			serviceedges.ModelHostProtocolNegotiationRequest,
		) (serviceedges.ModelHostProtocolNegotiationResult, error)
		Close() error
	}, error)
}

type modelHostGRPCConnectionEdge interface {
	Negotiate(
		context.Context,
		serviceedges.ModelHostProtocolNegotiationRequest,
	) (serviceedges.ModelHostProtocolNegotiationResult, error)
	Close() error
}

type modelHostCompatibilityEdge interface {
	Check(context.Context, serviceedges.ModelHostCompatibilityRequest) error
}

func providePlatformProcessCommandRunner(edges serviceedges.Edges) (platformprocess.CommandRunner, error) {
	clock := edges.PlatformProcessClock
	if clock == nil {
		clock = platformclock.Real{}
	}
	newCommand := edges.PlatformProcessCommandFactory
	if newCommand == nil {
		newCommand = exec.Command
	}
	processStateReader := platformprocess.NewProcfsProcessStateReader(os.ReadFile)
	runner, err := platformprocess.NewExecCommandRunner(newCommand, clock, nil, processStateReader)
	if err != nil {
		return nil, err
	}
	runner.CommandLineLimit = hostCommandLineLimit()
	return runner, nil
}

// hostCommandLineLimit selects the composed command-line bound the running
// host's process loader enforces for one spawn. The operating system is read
// here, at the canonical injection boundary, so pkg/platform/process can name
// an oversized-command-line spawn failure without selecting a host policy of
// its own. Hosts other than Windows report 0 because they bound the total
// argument block and each individual argument rather than the composed line,
// so their spawn failures are named from the operating system error alone.
func hostCommandLineLimit() int {
	if runtime.GOOS == "windows" {
		return platformprocess.WindowsCommandLineLimit
	}
	return 0
}

func provideModelAssetHostPlatform(edges serviceedges.Edges) models.AssetHostPlatform {
	platform := edges.ModelAssetHostPlatform
	if strings.TrimSpace(platform.OperatingSystem) == "" {
		platform.OperatingSystem = runtime.GOOS
	}
	if strings.TrimSpace(platform.Architecture) == "" {
		platform.Architecture = runtime.GOARCH
	}
	return platform
}

type modelsClock struct{ source platformclock.Source }

func (clock modelsClock) Now() time.Time {
	if clock.source != nil {
		return clock.source.Now()
	}
	return time.Time{}
}

func (modelsClock) NewTimer(duration time.Duration) interface {
	C() <-chan time.Time
	Stop() bool
} {
	return modelsTimer{Timer: time.NewTimer(duration)}
}

type modelsTimer struct{ *time.Timer }

func (timer modelsTimer) C() <-chan time.Time { return timer.Timer.C }

type modelsProcessLauncher struct{}

func (modelsProcessLauncher) Start(ctx context.Context, spec serviceedges.HostProcessStartSpec) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	launch, err := managedbackend.ResolveManagedBackendLaunch(ctx, spec)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(launch.Command, launch.Args...)
	if env := appendManagedBackendEnvironment(spec.Env, launch.Env); len(env) > 0 {
		cmd.Env = env
	}
	if launch.WorkDir != "" {
		cmd.Dir = launch.WorkDir
	}
	if err := cmd.Start(); err != nil {
		launch.Cleanup()
		return nil, managedbackend.WrapBackendStartFailure(err)
	}
	managed := &modelsManagedProcess{
		cmd:            cmd,
		healthEndpoint: launch.Endpoint,
		cleanup:        launch.Cleanup,
		finished:       make(chan struct{}),
	}
	go func() {
		waitErr := cmd.Wait()
		managed.cleanupResources()
		managed.mu.Lock()
		managed.waitErr = waitErr
		close(managed.finished)
		managed.mu.Unlock()
	}()
	return managed, nil
}

func appendManagedBackendEnvironment(base, additions []string) []string {
	if len(additions) == 0 {
		return append([]string(nil), base...)
	}
	environment := append([]string(nil), base...)
	if len(environment) == 0 {
		environment = os.Environ()
	}
	for _, addition := range additions {
		key, _, ok := strings.Cut(addition, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		replaced := false
		for index, existing := range environment {
			existingKey, _, hasValue := strings.Cut(existing, "=")
			if hasValue && strings.EqualFold(existingKey, key) {
				environment[index] = addition
				replaced = true
				break
			}
		}
		if !replaced {
			environment = append(environment, addition)
		}
	}
	return environment
}

type modelHostProcessLauncherAdapter struct {
	next interface {
		Start(context.Context, serviceedges.HostProcessStartSpec) (interface {
			HealthEndpoint() string
			Wait() error
			Stop(context.Context) error
		}, error)
	}
}

func adaptModelHostProcessLauncher(next interface {
	Start(context.Context, serviceedges.HostProcessStartSpec) (interface {
		HealthEndpoint() string
		Wait() error
		Stop(context.Context) error
	}, error)
}) modelswire.HostProcessLauncher {
	if isNilModelEdgeDependency(next) {
		return nil
	}
	return modelHostProcessLauncherAdapter{next: next}
}

func (adapter modelHostProcessLauncherAdapter) Start(
	ctx context.Context,
	spec modelswire.HostProcessStartSpec,
) (modelswire.HostManagedProcess, error) {
	process, err := adapter.next.Start(ctx, serviceedges.HostProcessStartSpec{
		Command:        spec.Command,
		Args:           spec.Args,
		Env:            spec.Env,
		WorkDir:        spec.WorkDir,
		HealthEndpoint: spec.HealthEndpoint,
		Backend:        spec.Backend,
		ModelPath:      spec.ModelPath,
		ModelFiles:     append([]string(nil), spec.ModelFiles...),
		BackendFiles:   append([]string(nil), spec.BackendFiles...),
	})
	if err != nil || process == nil {
		return modelswire.HostManagedProcess(process), err
	}
	return modelswire.HostManagedProcess(process), nil
}

type modelHostClockAdapter struct {
	next interface {
		Now() time.Time
		NewTimer(time.Duration) interface {
			C() <-chan time.Time
			Stop() bool
		}
	}
}

func adaptModelHostClock(next interface {
	Now() time.Time
	NewTimer(time.Duration) interface {
		C() <-chan time.Time
		Stop() bool
	}
}) modelswire.HostClock {
	if isNilModelEdgeDependency(next) {
		return nil
	}
	return modelHostClockAdapter{next: next}
}

func (adapter modelHostClockAdapter) Now() time.Time { return adapter.next.Now() }

func (adapter modelHostClockAdapter) NewTimer(duration time.Duration) modelswire.HostTimer {
	return modelswire.HostTimer(adapter.next.NewTimer(duration))
}

func adaptModelRuntimeTempFile(next serviceedges.RuntimeCreateTempFile) modelswire.RuntimeCreateTempFile {
	if next == nil {
		return nil
	}
	return func(dir, pattern string) (modelswire.RuntimeTempFile, error) {
		file, err := next(dir, pattern)
		return modelswire.RuntimeTempFile(file), err
	}
}

type modelsPullMetricsAdapter struct {
	next interface {
		RecordModelPullMetric(serviceedges.PullMetric)
	}
}

func adaptModelsPullMetricsRecorder(next interface {
	RecordModelPullMetric(serviceedges.PullMetric)
}) modelswire.PullMetricsRecorder {
	if next == nil {
		return nil
	}
	return modelsPullMetricsAdapter{next: next}
}

// isNilModelEdgeDependency preserves Models Wire's typed-nil validation when
// an edge-owned interface is wrapped by a canonical adapter. Without this
// check, a nil pointer would become a non-nil adapter value and fail later at
// invocation rather than during inert construction.
func isNilModelEdgeDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (adapter modelsPullMetricsAdapter) RecordModelPullMetric(metric modelswire.PullMetric) {
	labels := make(map[string]string, len(metric.Labels))
	for key, value := range metric.Labels {
		labels[key] = value
	}
	adapter.next.RecordModelPullMetric(serviceedges.PullMetric{
		Name:   metric.Name,
		Labels: labels,
	})
}

func modelLocalRuntimeHooks(hooks workers.LocalRuntimeHooks) modelswire.LocalRuntimeHooks {
	return modelswire.LocalRuntimeHooks{
		MarkResourceWaitStarted:  hooks.MarkResourceWaitStarted,
		MarkResourceWaitFinished: hooks.MarkResourceWaitFinished,
		MarkLoadRequested:        hooks.MarkLoadRequested,
		MarkLoadFinished:         hooks.MarkLoadFinished,
		MarkLoadReused:           hooks.MarkLoadReused,
	}
}

type modelsManagedProcess struct {
	mu             sync.Mutex
	cmd            *exec.Cmd
	healthEndpoint string
	cleanup        func()
	cleanupOnce    sync.Once
	// finished is broadcast to both the supervisor's Wait observer and the
	// application lifecycle closer; a one-shot error channel would let one
	// consumer strand the other during normal teardown.
	finished chan struct{}
	waitErr  error
	stopped  bool
}

func (p *modelsManagedProcess) cleanupResources() {
	if p == nil || p.cleanup == nil {
		return
	}
	p.cleanupOnce.Do(p.cleanup)
}

func (p *modelsManagedProcess) HealthEndpoint() string { return p.healthEndpoint }

func (p *modelsManagedProcess) Wait() error {
	if p == nil || p.finished == nil {
		return nil
	}
	<-p.finished
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.waitErr
}

func (p *modelsManagedProcess) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.mu.Lock()
	if p.stopped || p.cmd == nil || p.cmd.Process == nil {
		p.mu.Unlock()
		return nil
	}
	p.stopped = true
	command := p.cmd
	p.mu.Unlock()
	if !p.processFinished() {
		if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) && !p.processFinished() {
			return err
		}
	}
	if p.finished == nil {
		return nil
	}
	select {
	case <-p.finished:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *modelsManagedProcess) processFinished() bool {
	if p == nil {
		return true
	}
	if p.finished != nil {
		select {
		case <-p.finished:
			return true
		default:
		}
	}
	return p.cmd != nil && p.cmd.ProcessState != nil
}

func provideModelsCLIInvocationOperation(
	invocation factorysessionwire.InvocationOperation,
) modelscli.InvocationOperation {
	if invocation == nil {
		return nil
	}
	return modelsCLIInvocationOperation{invocation: invocation}
}

func provideModelsCLIInputFileReader(edges serviceedges.Edges) modelscli.InputFileReader {
	if readFile := edges.ModelCLIInputReadFile; readFile != nil {
		return modelscli.InputFileReader(readFile)
	}
	if readFile := edges.ModelAssetReadFile; readFile != nil {
		return func(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			data, err := readFile(path)
			if err != nil {
				return nil, err
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if int64(len(data)) > maxBytes {
				return nil, fmt.Errorf("file content exceeds the %d-byte limit", maxBytes)
			}
			return data, nil
		}
	}
	return readModelsCLIInputFile
}

func readModelsCLIInputFile(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("file content limit must be positive")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	readLimit := maxBytes
	if maxBytes < int64(^uint64(0)>>1) {
		readLimit++
	}
	data, err := io.ReadAll(io.LimitReader(modelsCLIInputContextReader{
		ctx: ctx, reader: file,
	}, readLimit))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("file content exceeds the %d-byte limit", maxBytes)
	}
	return data, nil
}

type modelsCLIInputContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader modelsCLIInputContextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	read, err := reader.reader.Read(buffer)
	if contextErr := reader.ctx.Err(); contextErr != nil {
		return read, contextErr
	}
	return read, err
}

// modelsCLIInvocationOperation maps the four invocation inputs the Models CLI
// resolves onto the fuller Factory Sessions invocation target. Owning the
// mapping here keeps the Models CLI transport free of a Factory Sessions
// import while preserving the exact values the command sent before.
type modelsCLIInvocationOperation struct {
	invocation factorysessionwire.InvocationOperation
}

func (o modelsCLIInvocationOperation) InvokeModel(
	ctx context.Context,
	target modelscli.InvocationTarget,
	modelName string,
	request models.Request,
) (models.Result, error) {
	return o.invocation.InvokeModel(ctx, factorysessions.InvocationTarget{
		FactoryDir:       target.FactoryDir,
		HomeDir:          target.HomeDir,
		OperatorDefaults: target.OperatorDefaults,
		Verbose:          target.Verbose,
	}, modelName, request)
}

func (o modelsCLIInvocationOperation) ResolveModelInvocationFactoryDir(
	factoryDir string,
) (string, error) {
	return o.invocation.ResolveModelInvocationFactoryDir(factoryDir)
}

func (o modelsCLIInvocationOperation) ResolveModelInvocationFactoryDirForWorkingDirectory(
	factoryDir string,
	workingDirectory string,
) (string, error) {
	resolver, ok := o.invocation.(interface {
		ResolveModelInvocationFactoryDirForWorkingDirectory(string, string) (string, error)
	})
	if !ok {
		return o.invocation.ResolveModelInvocationFactoryDir(factoryDir)
	}
	return resolver.ResolveModelInvocationFactoryDirForWorkingDirectory(factoryDir, workingDirectory)
}

func (o modelsCLIInvocationOperation) ExportModelInvocationArtifact(
	source string,
	destination string,
) error {
	return o.invocation.ExportModelInvocationArtifact(source, destination)
}
