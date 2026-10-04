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
	"runtime"
	"strings"
	"sync"
	"time"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
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
	managedChildEvidenceKind        = "MANAGED_CHILD"
	managedChildPhaseStarted        = "PROCESS_STARTED"
	managedChildPhaseExited         = "PROCESS_EXITED"
	managedChildExitClassExited     = "EXITED"
	managedChildExitClassNonzero    = "NONZERO_EXIT"
	managedChildExitClassWaitFailed = "WAIT_FAILED"
)

type modelRuntimeEvidenceFileRecorder struct {
	mu       sync.Mutex
	path     string
	sequence uint64
}

func (recorder *modelRuntimeEvidenceFileRecorder) RecordRuntimeEvidence(
	record modelswire.RuntimeEvidenceRecord,
) {
	if recorder == nil || strings.TrimSpace(recorder.path) == "" {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.appendJSONLineLocked(func(sequence uint64) any {
		record.Sequence = sequence
		return record
	})
}

type managedChildEnvironmentEvidence struct {
	Sequence             uint64                   `json:"sequence"`
	Kind                 string                   `json:"kind"`
	Backend              string                   `json:"backend"`
	ProcessID            int                      `json:"process_id"`
	Phase                string                   `json:"phase"`
	Environment          []managedEnvironmentFact `json:"environment,omitempty"`
	ExitClass            string                   `json:"exit_class,omitempty"`
	ExitCode             int                      `json:"exit_code,omitempty"`
	ExitCodeKnown        bool                     `json:"exit_code_known,omitempty"`
	StdoutBytes          uint64                   `json:"stdout_bytes,omitempty"`
	StdoutSHA256         string                   `json:"stdout_sha256,omitempty"`
	StdoutTruncated      bool                     `json:"stdout_truncated,omitempty"`
	StderrBytes          uint64                   `json:"stderr_bytes,omitempty"`
	StderrSHA256         string                   `json:"stderr_sha256,omitempty"`
	StderrTruncated      bool                     `json:"stderr_truncated,omitempty"`
	CauseCode            string                   `json:"cause_code,omitempty"`
	CauseMessage         string                   `json:"cause_message,omitempty"`
	CauseMessageRedacted bool                     `json:"cause_message_redacted,omitempty"`
}

type managedEnvironmentFact struct {
	Name        string `json:"name"`
	Present     bool   `json:"present"`
	ValueSHA256 string `json:"value_sha256,omitempty"`
}

type managedChildEnvironmentRecorder interface {
	RecordManagedChildEnvironment(managedChildEnvironmentEvidence)
}

func (recorder *modelRuntimeEvidenceFileRecorder) RecordManagedChildEnvironment(
	record managedChildEnvironmentEvidence,
) {
	if recorder == nil || strings.TrimSpace(recorder.path) == "" {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.appendJSONLineLocked(func(sequence uint64) any {
		record.Sequence = sequence
		return record
	})
}

func (recorder *modelRuntimeEvidenceFileRecorder) appendJSONLineLocked(
	value func(uint64) any,
) {
	if recorder == nil || strings.TrimSpace(recorder.path) == "" {
		return
	}
	file, err := os.OpenFile(
		recorder.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600,
	)
	if err != nil {
		return
	}
	defer file.Close()
	_ = file.Chmod(0o600)
	recorder.sequence++
	payload, err := json.Marshal(value(recorder.sequence))
	if err != nil {
		return
	}
	payload = append(payload, '\n')
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
	return &modelRuntimeEvidenceFileRecorder{path: path}, nil
}

// These distinct effect types keep asset-disabled revision resolution and
// the legacy Root override independent, and preserve the Models runner selection.
type modelAssetRevisionResolver func(context.Context, string) (string, error)
type modelNow func() time.Time
type modelRuntimeRunner interface{ platformprocess.CommandRunner }

// Child-process observations retain access to the original evidence sink.
type modelRuntimeEvidenceSource interface {
	modelswire.RuntimeEvidenceRecorder
}

func provideModelHostHTTP(edges serviceedges.Edges) modelswire.HostHTTPDoer {
	if selected := edges.ModelHostHTTPClient; selected != nil {
		return modelswire.HostHTTPDoer(selected)
	}
	return &http.Client{Timeout: modelHostHTTPTimeout}
}

func provideModelRuntimeHTTP(edges serviceedges.Edges) modelswire.RuntimeHTTPDoer {
	if selected := edges.ModelRuntimeHTTPClient; selected != nil {
		return modelswire.RuntimeHTTPDoer(selected)
	}
	return &http.Client{Timeout: modelRuntimeHTTPTimeout}
}

func provideModelRuntimeInspectFile(edges serviceedges.Edges) modelswire.RuntimeInspectFile {
	if selected := edges.ModelRuntimeInspectFile; selected != nil {
		return modelswire.RuntimeInspectFile(selected)
	}
	return os.Stat
}

func provideModelRuntimeTempDirectory(edges serviceedges.Edges) modelswire.RuntimeTempDirectory {
	if selected := edges.ModelRuntimeTempDirectory; selected != nil {
		return modelswire.RuntimeTempDirectory(selected)
	}
	return os.TempDir
}

func provideModelHostClock(edges serviceedges.Edges) modelswire.HostClock {

	if selected := edges.ModelHostClock; selected != nil {
		return adaptModelHostClock(selected)
	}
	source := edges.Clock
	if source == nil {
		source = platformclock.Real{}
	}
	return adaptModelHostClock(modelsClock{source: source})
}

func provideModelNow(edges serviceedges.Edges) modelNow {

	source := edges.Clock
	if source == nil {
		source = platformclock.Real{}
	}
	return source.Now
}

func provideModelRuntimeRunner(edges serviceedges.Edges) (modelRuntimeRunner, error) {

	if selected := edges.ModelRuntimeCommandRunner; selected != nil {
		return selected, nil
	}
	return providePlatformProcessCommandRunner(edges)
}

func provideModelHostLauncher(edges serviceedges.Edges, evidence modelRuntimeEvidenceSource) modelswire.HostProcessLauncher {

	if selected := edges.ModelHostProcessLauncher; selected != nil {
		return adaptModelHostProcessLauncher(selected)
	}
	var childRecorder managedChildEnvironmentRecorder
	childRecorder, _ = evidence.(managedChildEnvironmentRecorder)
	return adaptModelHostProcessLauncher(modelsProcessLauncher{recorder: childRecorder})
}

func provideModelRuntimeEvidenceSource() (modelRuntimeEvidenceSource, error) {

	source, err := provideModelRuntimeEvidenceRecorder()
	if err != nil {
		return nil, fmt.Errorf("construct Models runtime evidence recorder: %w", err)
	}
	return source, nil
}

func provideModelOrderedRuntimeEvidence(source modelRuntimeEvidenceSource) modelswire.RuntimeEvidenceRecorder {
	return modelswire.NewOrderedRuntimeEvidenceRecorder(source)
}

func provideModelHostSymlinks() modelswire.HostResolveSymlinks {
	return platformfilesystem.Local{}.EvalSymlinks
}

func provideModelHostProtocol(edges serviceedges.Edges, symlinks modelswire.HostResolveSymlinks) modelswire.HostProtocolNegotiator {

	protocol := adaptModelHostProtocolNegotiator(edges.ModelHostProtocolNegotiator)
	if protocol == nil && !isNilModelEdgeDependency(edges.ModelHostGRPCDialer) {
		protocol = modelswire.PinnedGRPCNegotiator{Dialer: modelHostGRPCDialerAdapter{next: edges.ModelHostGRPCDialer}}
	}
	if protocol == nil {
		protocol = modelswire.NewPinnedGRPCHostProtocolNegotiator(edges.ModelInvocationGRPCDialer, symlinks)
	}
	return protocol
}

func provideModelHostCompatibility(edges serviceedges.Edges) (modelswire.HostCompatibilityChecker, error) {
	return provideModelHostCompatibilityChecker(edges, runtime.GOOS == "linux" && runtime.GOARCH == "amd64")
}

func provideModelBackendArtifactResolver(edges serviceedges.Edges, runner modelRuntimeRunner, client modelswire.AssetHTTPDoer) (modelswire.BackendArtifactResolver, error) {

	if selected := adaptModelBackendArtifactResolver(edges.ModelResolveBackendArtifact); selected != nil {
		return selected, nil
	}
	var resolver modelswire.BackendArtifactResolver
	var err error
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		var installer modelswire.GalleryBackendInstaller
		installer, err = newLocalAIGalleryInstaller(runner, client)
		if err == nil {
			resolver, err = newLinuxBackendArtifactResolver(installer, client)
		}
	} else {
		resolver, err = modelswire.NewPublishedBackendArtifactResolver(client)
	}
	if err != nil {
		return nil, fmt.Errorf("construct Models backend artifact selector: %w", err)
	}
	return resolver, nil
}

func provideModelAssetRevision(edges serviceedges.Edges) modelAssetRevisionResolver {

	if selected := edges.ModelResolveHuggingFaceRevision; selected != nil {
		return selected
	}
	return modelswire.NewUnresolvedAssetRevisionResolver()
}

func provideModelRuntimeScopes() (modelswire.RuntimeScopes, error) {
	return modelswire.NewRuntimeScopes(platformrandom.CryptoSource{})
}

func provideModelAssets(scopes modelswire.RuntimeScopes, platform models.AssetHostPlatform, client modelswire.AssetHTTPDoer,
	endpoints models.RuntimeAssetEndpoints, mkdir modelswire.AssetMakeDirectories, stat modelswire.AssetInspectPath,
	home modelswire.AssetResolveHomeDirectory, write modelswire.AssetWriteFile, rename modelswire.AssetRenamePath,
	remove modelswire.AssetRemovePath, read modelswire.AssetReadFile, readDir modelswire.AssetReadDirectory,
	create modelswire.AssetCreateFile, open modelswire.AssetOpenFile, env modelswire.AssetResolveEnvironment,
	revision modelAssetRevisionResolver, coordination modelswire.AssetStagingCoordination) (modelswire.Assets, error) {

	return modelswire.NewAssets(scopes, platform, client, endpoints, mkdir, stat, home, write, rename, remove,
		read, readDir, create, open, env, revision, coordination)
}

func provideModelSlotFacts(state *modelswire.SlotState, scopes modelswire.RuntimeScopes, assets modelswire.Assets) modelswire.SlotFactsProvider {
	return modelswire.NewSlotFacts(scopes, assets, state)
}

func provideModelHostLogger(logger *zap.Logger) modelswire.HostDiagnosticLogger {
	return modelswire.HostDiagnosticLogger(factorysessionwire.ModelHostDiagnosticLogger(logger))
}

func provideModelHostMetrics(edges serviceedges.Edges) modelswire.HostMetricsRecorder {
	return modelswire.HostMetricsRecorder(factorysessionwire.ModelHostDiagnosticMetrics(edges.InvocationMetricsRecorder))
}

func provideModelSlotCoordinator(state *modelswire.SlotState, scopes modelswire.RuntimeScopes, clock modelswire.HostClock, logger modelswire.HostDiagnosticLogger, metrics modelswire.HostMetricsRecorder) modelswire.SlotCapacityCoordinator {
	return modelswire.NewSlotCoordinator(state, scopes, clock, logger, metrics, 0)
}

func provideModelRuntimeHost(scopes modelswire.RuntimeScopes, assets modelswire.Assets, leases modelswire.HostLeases,
	state *modelswire.SlotState, launcher modelswire.HostProcessLauncher, client modelswire.HostHTTPDoer,
	clock modelswire.HostClock, logger modelswire.HostDiagnosticLogger, metrics modelswire.HostMetricsRecorder,
	platform models.AssetHostPlatform, protocol modelswire.HostProtocolNegotiator, compatibility modelswire.HostCompatibilityChecker,
	symlinks modelswire.HostResolveSymlinks, evidence modelswire.RuntimeEvidenceRecorder) (modelswire.RuntimeHost, error) {

	return modelswire.NewRuntimeHost(scopes, assets, leases, state, launcher, client, clock, logger, metrics,
		platform, protocol, compatibility, symlinks, evidence, 0, 0)
}

func provideModelInvocationRuntime(edges serviceedges.Edges, runner modelRuntimeRunner, temp modelswire.RuntimeTempDirectory,
	create modelswire.RuntimeCreateTempFile, write modelswire.AssetWriteFile, inspect modelswire.RuntimeInspectFile,
	read modelswire.AssetReadFile, remove modelswire.AssetRemovePath) (modelswire.InvocationRuntime, error) {

	return modelswire.NewInvocationRuntime(adaptModelInvocationBackend(edges.ModelInvocationBackend),
		adaptModelASRBackend(edges.ModelASRBackend), adaptModelEmbeddingBackend(edges.ModelEmbeddingBackend),
		edges.ModelInvocationProtocolClient, edges.ModelInvocationGRPCDialer, runner, temp, create, write, inspect, read, remove)
}
func provideModelInference(scopes modelswire.RuntimeScopes, assets modelswire.Assets, catalog modelswire.Catalog,
	host modelswire.RuntimeHost, runtime modelswire.InvocationRuntime, registrar *modelswire.InvocationArtifactRegistrar,
	now modelNow, executionDeadline func() time.Duration) (modelswire.Inference, error) {
	return modelswire.NewInference(scopes, assets, catalog, host, runtime, registrar, now, executionDeadline)
}
func provideModelLocalRuntime(runner modelRuntimeRunner, client modelswire.RuntimeHTTPDoer, inspect modelswire.RuntimeInspectFile,
	temp modelswire.RuntimeTempDirectory, create modelswire.RuntimeCreateTempFile) (modelswire.LocalRuntime, error) {
	return modelswire.NewLocalRuntime(runner, client, inspect, temp, create)
}
func provideModelResourceLimiter(now modelNow) (*modelswire.ResourceLimiter, error) {
	return modelswire.NewResourceLimiter(modelLocalRuntimeHooks(workerswire.LocalRuntimeHooks()), now)
}
func provideModelsService(edges serviceedges.Edges, scopes modelswire.RuntimeScopes, assets modelswire.Assets,
	catalog modelswire.Catalog, host modelswire.RuntimeHost, inference modelswire.Inference,
	launcher modelswire.HostProcessLauncher, hostHTTP modelswire.HostHTTPDoer, clock modelswire.HostClock,
	localRuntime modelswire.LocalRuntime, resources *modelswire.ResourceLimiter, now modelNow,
	logger modelswire.HostDiagnosticLogger, metrics modelswire.HostMetricsRecorder, evidence modelswire.RuntimeEvidenceRecorder,
	resolver modelswire.BackendArtifactResolver, platform models.AssetHostPlatform, backend *zap.Logger) (models.Service, error) {
	return modelswire.NewService(scopes, assets, catalog, host, inference, launcher, hostHTTP, clock, localRuntime, resources,
		backend, now, adaptModelsPullMetricsRecorder(edges.ModelPullMetricsRecorder),
		logger, metrics, modelLocalRuntimeHooks(workerswire.LocalRuntimeHooks()), evidence,
		edges.ModelResolveHuggingFaceRevision, resolver, platform)
}

func provideModelHostCompatibilityChecker(
	edges serviceedges.Edges,
	useGallery bool,
) (modelswire.HostCompatibilityChecker, error) {
	checker := adaptModelHostCompatibilityChecker(edges.ModelHostCompatibilityChecker)
	if checker != nil {
		return checker, nil
	}
	if useGallery {
		return modelswire.NewGalleryHostCompatibilityChecker(), nil
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
		request modelswire.ResolvedHostConfiguration,
		offline bool,
	) (modelswire.BackendArtifactSelection, error) {
		selection, err := next(ctx, serviceedges.ModelBackendArtifactSelectionRequest{
			Backend:         request.Backend,
			Offline:         offline,
			Platform:        request.Platform,
			ProtocolVersion: request.ProtocolVersion,
		})
		return modelswire.BackendArtifactSelection{
			Name:          selection.Name,
			Location:      selection.Location,
			Bytes:         selection.Bytes,
			SHA256:        selection.SHA256,
			InstalledPath: selection.InstalledPath,
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
	configuration := request.Configuration.Clone()
	result, err := adapter.next.Negotiate(ctx, endpoint, serviceedges.ModelHostProtocolNegotiationRequest{
		ProtocolVersion: configuration.ProtocolVersion,
		Backend:         configuration.Backend,
		ModelName:       configuration.ModelName,
		Revision:        configuration.Revision,
		Platform:        configuration.Platform,
		ModelPath:       configuration.ModelPath,
		MMProjPath:      configuration.MMProjPath,
		ModelFiles:      append([]string(nil), configuration.ModelFiles...),
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
	configuration := request.Configuration.Clone()
	result, err := adapter.next.Negotiate(ctx, serviceedges.ModelHostProtocolNegotiationRequest{
		ProtocolVersion: configuration.ProtocolVersion,
		Backend:         configuration.Backend,
		ModelName:       configuration.ModelName,
		Revision:        configuration.Revision,
		Platform:        configuration.Platform,
		ModelPath:       configuration.ModelPath,
		MMProjPath:      configuration.MMProjPath,
		ModelFiles:      append([]string(nil), configuration.ModelFiles...),
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
	configuration := request.Configuration.Clone()
	return adapter.next.Check(ctx, serviceedges.ModelHostCompatibilityRequest{
		Backend:   configuration.Backend,
		ModelName: configuration.ModelName,
		Revision:  configuration.Revision,
		Platform:  configuration.Platform,
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
	if platform.Accelerator == "cuda" {
		platform.CUDAAvailable = true
	}
	if platform.Accelerator == "" {
		if runtime.GOOS == "linux" && platform.OperatingSystem == "linux" && platform.Architecture == "amd64" {
			platform.CUDAAvailable = linuxCUDAAvailable(os.Stat, func(ctx context.Context) ([]byte, error) {
				return exec.CommandContext(ctx, "nvidia-smi", "-L").Output()
			})
		} else if runtime.GOOS == "windows" && platform.OperatingSystem == "windows" && platform.Architecture == "amd64" {
			platform.CUDAAvailable = windowsCUDAAvailable(func(ctx context.Context) ([]byte, error) {
				return exec.CommandContext(ctx, "nvidia-smi", "-L").Output()
			})
		}
		if platform.CUDAAvailable && platform.OperatingSystem == "linux" {
			platform.Accelerator = "cuda"
		}
		// Leave Windows accelerator selection automatic. The published backend
		// resolver prefers CUDA when an archive exists and can fall back to CPU
		// when the current publication has no Windows CUDA archive.
	}
	return platform
}

func linuxCUDAAvailable(
	stat func(string) (os.FileInfo, error),
	probe func(context.Context) ([]byte, error),
) bool {
	if _, err := stat("/dev/nvidiactl"); err != nil {
		if _, wslErr := stat("/dev/dxg"); wslErr != nil {
			return false
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := probe(ctx)
	return err == nil && nvidiaGPUListed(output)
}

func windowsCUDAAvailable(probe func(context.Context) ([]byte, error)) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := probe(ctx)
	return err == nil && nvidiaGPUListed(output)
}

func nvidiaGPUListed(output []byte) bool {
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "GPU ") && strings.Contains(line, ":") {
			return true
		}
	}
	return false
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
			if maxBytes > 0 && int64(len(data)) > maxBytes {
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
	if maxBytes < 0 {
		return nil, fmt.Errorf("file content limit must not be negative")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var reader io.Reader = modelsCLIInputContextReader{ctx: ctx, reader: file}
	if maxBytes > 0 {
		readLimit := maxBytes
		if maxBytes < int64(^uint64(0)>>1) {
			readLimit++
		}
		reader = io.LimitReader(reader, readLimit)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maxBytes > 0 && int64(len(data)) > maxBytes {
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

func (composition modelsCLIComposition) openModelsPresentationScope(
	ctx context.Context,
	cfg modelscli.InvokeConfig,
	modelCacheDir string,
) (modelscli.InvokeRuntimeScope, error) {
	request := models.PresentationScopeRequest{
		FactoryDir:       cfg.FactoryDir,
		WorkingDirectory: cfg.WorkingDirectory,
		HomeDir:          cfg.HomeDir,
		OperatorDefaults: models.PresentationOperatorDefaults{
			WorkerModelProvider: cfg.OperatorDefaults.WorkerModelProvider,
			WorkerModel:         cfg.OperatorDefaults.WorkerModel,
		},
		Logger:        cfg.Logger,
		Verbose:       cfg.Verbose,
		ModelCacheDir: modelCacheDir,
	}
	opened, err := composition.source.OpenModelsPresentationScope(ctx, request)
	if err == nil {
		return modelscli.InvokeRuntimeScope{Scope: opened.Scope, Close: opened.Close}, nil
	}
	if strings.TrimSpace(cfg.FactoryDir) != "" ||
		!errors.Is(err, factorydefinitions.ErrFactoryLayoutNotFound) {
		return modelscli.InvokeRuntimeScope{}, err
	}
	return composition.openStandaloneModelsScope(ctx, modelCacheDir, cfg.HomeDir)
}

func (composition modelsCLIComposition) openCatalogModelsScope(
	ctx context.Context,
	modelCacheDir string,
) (modelscli.InvokeRuntimeScope, error) {
	workingDirectory := startupcli.WorkingDirectory(ctx)
	homeDirectory := startupcli.HomeDirectory(ctx)
	if strings.TrimSpace(workingDirectory) == "" || strings.TrimSpace(homeDirectory) == "" ||
		!catalogOperatorConfigPresent(homeDirectory) {
		return composition.openStandaloneModelsScope(ctx, modelCacheDir, homeDirectory)
	}
	opened, err := composition.source.OpenModelsPresentationScope(ctx, models.PresentationScopeRequest{
		WorkingDirectory: workingDirectory,
		HomeDir:          homeDirectory,
		ModelCacheDir:    modelCacheDir,
	})
	if err == nil {
		return modelscli.InvokeRuntimeScope{Scope: opened.Scope, Close: opened.Close}, nil
	}
	if !errors.Is(err, factorydefinitions.ErrFactoryLayoutNotFound) {
		return modelscli.InvokeRuntimeScope{}, err
	}
	return composition.openStandaloneModelsScope(ctx, modelCacheDir, homeDirectory)
}

func catalogOperatorConfigPresent(homeDirectory string) bool {
	path := filepath.Join(strings.TrimSpace(homeDirectory), ".you-agent-factory", "config.json")
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func (composition modelsCLIComposition) openStandaloneModelsScope(
	ctx context.Context,
	modelCacheDir string,
	homeDirectory string,
) (modelscli.InvokeRuntimeScope, error) {
	operatorModels, err := composition.loadStandaloneOperatorModels(homeDirectory)
	if err != nil {
		return modelscli.InvokeRuntimeScope{}, err
	}
	opened, err := composition.root.OpenRuntimeScope(ctx, models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{
			CacheDirectory: modelCacheDir,
			Runtime:        models.RuntimeConfig{},
			OperatorModels: operatorModels,
		},
	})
	if err != nil {
		return modelscli.InvokeRuntimeScope{}, err
	}
	return modelscli.InvokeRuntimeScope{
		Scope: opened.Scope,
		Close: func(closeCtx context.Context) error {
			closed, closeErr := composition.root.CloseRuntimeScope(
				context.WithoutCancel(closeCtx),
				models.CloseRuntimeScopeRequest{Scope: opened.Scope},
			)
			if closeErr != nil {
				return closeErr
			}
			if !closed.Closed {
				return errors.New("close standalone Models scope: scope was not closed")
			}
			return nil
		},
	}, nil
}

func (composition modelsCLIComposition) loadStandaloneOperatorModels(
	homeDirectory string,
) (map[string]models.ModelOverlay, error) {
	if composition.loadOperatorConfig == nil || strings.TrimSpace(homeDirectory) == "" {
		return nil, nil
	}
	config, err := composition.loadOperatorConfig(operatorsettings.DefaultConfigPath(homeDirectory))
	if err != nil {
		return nil, fmt.Errorf("load standalone Models operator config: %w", err)
	}
	return projectModelsOperatorOverlays(config.Models), nil
}

func projectModelsOperatorOverlays(
	configured map[string]operatorsettings.ModelConfig,
) map[string]models.ModelOverlay {
	if len(configured) == 0 {
		return nil
	}
	projected := make(map[string]models.ModelOverlay, len(configured))
	for name, config := range configured {
		overlay := models.ModelOverlay{
			Source:     cloneModelsOperatorString(config.Source),
			Backend:    cloneModelsOperatorString(config.Backend),
			Operations: append([]string(nil), config.Operations...),
		}
		if config.LoadPolicy != nil {
			policy := models.LoadPolicy(*config.LoadPolicy)
			overlay.LoadPolicy = &policy
		}
		projected[name] = overlay
	}
	return projected
}

func cloneModelsOperatorString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
