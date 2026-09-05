// Package edges is the process-edge aggregator architecture exception for
// root.BuildProcess and pkg/wire construction.
//
// Edges aggregates replaceable external-effect ports so production can supply
// empty defaults and functional tests can supply typed replacements through the
// same BuildProcess bag. Merge overlays non-zero replacements onto those
// defaults. This package is not a service locator or Initializer dependency
// bag: constructed services must receive exact projected ports at the Wire /
// root composition boundary rather than importing or holding Edges.
package edges

import (
	"context"
	"database/sql"
	"io"
	"io/fs"
	"net/http"
	"time"

	platformbrowser "github.com/portpowered/infinite-you/pkg/platform/browser"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	"github.com/portpowered/infinite-you/pkg/platform/wiretranscript"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providercontract "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Edges aggregates replaceable external-effect ports for process construction
// and functional overrides. Only the process-edge boundary (pkg/services/edges,
// pkg/root, pkg/wire, and BuildProcess override tests) consumes this bag;
// constructed services take exact ports instead. It is not a service locator.
type Edges struct {
	CLIObserver                   platformprocess.CLIObserver
	PlatformProcessClock          platformprocess.Clock
	PlatformProcessCommandFactory platformprocess.CommandFactory
	ProvidersExecutableLocator    platformprocess.ExecutableLocator
	ProviderCommandRunner         platformprocess.CommandRunner
	AgyPTYHost                    platformpty.Host
	AgyPTYClock                   platformclock.Source
	HostedHTTPClient              automations.HostedLinearHTTPDoer
	HostedLinearEndpoint          string
	HostedSecretResolver          automations.HostedLinearSecretResolver
	HostedLinearCheckpointStore   automations.HostedLinearCheckpointStore
	HostedClock                   automations.HostedLinearClock
	AutomationsCursorFileSystem   interface {
		ReadFile(string) ([]byte, error)
		MkdirAll(string, fs.FileMode) error
		WriteFile(string, []byte, fs.FileMode) error
		Rename(string, string) error
	}
	FactoryWebhookHTTPClient interface {
		Do(*http.Request) (*http.Response, error)
	}
	FactoryWebhookClock interface {
		Now() time.Time
		After(time.Duration) <-chan time.Time
	}
	FactoryWebhookSecretResolver     webhooks.SecretResolver
	FactoryWebhookDeadLetterAppender webhooks.DeadLetterAppender
	ModelAssetHTTPClient             interface {
		Do(*http.Request) (*http.Response, error)
	}
	ModelAssetEndpoints                  models.RuntimeAssetEndpoints
	ModelAssetHostPlatform               models.AssetHostPlatform
	ModelResolveHuggingFaceRevision      func(context.Context, string) (string, error)
	ModelAssetResolveEnvironment         func(string) string
	ModelAssetMakeDirectories            AssetMakeDirectories
	ModelAssetInspectPath                AssetInspectPath
	ModelAssetResolveHomeDirectory       AssetResolveHomeDirectory
	ModelAssetWriteFile                  AssetWriteFile
	ModelAssetRenamePath                 AssetRenamePath
	ModelAssetRemovePath                 AssetRemovePath
	ModelAssetReadFile                   AssetReadFile
	ModelAssetReadDirectory              AssetReadDirectory
	ModelAssetCreateFile                 AssetCreateFile
	ModelAssetOpenFile                   AssetOpenFile
	ModelAssetStagingCoordinationFactory AssetStagingCoordinationFactory
	ModelCLIInputReadFile                ModelCLIInputReadFile
	ModelCLIOutputCreateTempFile         ModelCLIOutputCreateTempFile
	ModelCLIOutputInspectPath            AssetInspectPath
	ModelCLIOutputRemovePath             AssetRemovePath
	ModelCLIOutputRenamePath             AssetRenamePath
	ModelHostProcessLauncher             interface {
		Start(context.Context, HostProcessStartSpec) (interface {
			HealthEndpoint() string
			Wait() error
			Stop(context.Context) error
		}, error)
	}
	ModelHostHTTPClient interface {
		Do(*http.Request) (*http.Response, error)
	}
	ModelHostClock interface {
		Now() time.Time
		NewTimer(time.Duration) interface {
			C() <-chan time.Time
			Stop() bool
		}
	}
	ModelHostProtocolNegotiator interface {
		Negotiate(context.Context, string, ModelHostProtocolNegotiationRequest) (ModelHostProtocolNegotiationResult, error)
	}
	ModelHostGRPCDialer interface {
		Dial(context.Context, string) (interface {
			Negotiate(context.Context, ModelHostProtocolNegotiationRequest) (ModelHostProtocolNegotiationResult, error)
			Close() error
		}, error)
	}
	ModelHostCompatibilityChecker interface {
		Check(context.Context, ModelHostCompatibilityRequest) error
	}
	ModelResolveBackendArtifact ModelResolveBackendArtifact
	ModelInvocationBackend      ModelInvocationBackend
	ModelASRBackend             ModelASRBackend
	ModelEmbeddingBackend       ModelEmbeddingBackend
	ModelRuntimeCommandRunner   platformprocess.CommandRunner
	ModelRuntimeHTTPClient      interface {
		Do(*http.Request) (*http.Response, error)
	}
	ModelRuntimeInspectFile           RuntimeInspectFile
	ModelRuntimeTempDirectory         RuntimeTempDirectory
	ModelRuntimeCreateTempFile        RuntimeCreateTempFile
	ModelInvocationArtifactFileSystem interface {
		Open(string) (io.ReadCloser, error)
		Create(string) (io.WriteCloser, error)
	}
	// ModelInvocationProtocolClient is the provider-neutral generic protocol
	// edge used by fixture-backed and production model adapters. LocalAI wire
	// representations stay behind the Models wire boundary.
	ModelInvocationProtocolClient interface {
		Predict(context.Context, models.InvocationProtocolRequest) (models.InvocationProtocolResponse, error)
	}
	// ModelInvocationGRPCDialer is the policy-free transport edge used by the
	// production pinned LocalAI adapter. Backend method names and protobuf
	// messages stay behind the Models wire boundary.
	ModelInvocationGRPCDialer                  platformgrpc.Dialer
	FactorySessionsWorkingDirectory            platformfilesystem.WorkingDirectory
	FactorySessionExecutionOpeningFileSystem   factorysessions.ExecutionOpeningFileSystem
	FactorySessionDirectoryInspection          factorysessions.DirectoryInspection
	FactorySessionResolveHomeDirectory         factorysessions.HomeDirectoryResolver
	FactorySessionResolveLogicalTargetSymlinks factorysessions.LogicalTargetResolveSymlinks
	FactorySessionIDGenerator                  factorysessions.SessionIDGenerator
	FactorySessionRuntimeInstanceIDGenerator   factorysessions.RuntimeInstanceIDGenerator
	FactorySessionResponseEventIDGenerator     factorysessions.ResponseEventIDGenerator
	FactorySessionResponseEventRetentionLimits *factorysessions.ResponseEventRetentionLimits
	FactorySessionCursorPersistenceFileSystem  factorysessions.CursorPersistenceFileSystem
	FactorySessionCursorCreateTemporaryFile    factorysessions.CursorPersistenceCreateTemporaryFile
	FactorySessionRuntimePersistenceFileSystem factorysessions.RuntimePersistenceFileSystem
	FactorySessionContractFixtureReader        factorysessions.ContractFixtureReader
	FactorySessionInvocationInputReader        factorysessions.InvocationInputReader
	FactorySessionReplayRecordingReader        factorysessions.ReplayRecordingReader
	FactorySessionInitialWorkReader            factorysessions.InitialWorkReader
	FactoryRuntimeIDGenerator                  factoryruntime.IDGenerator
	FactoryRuntimeDirectories                  factoryruntime.RuntimeDirectoryFileSystem
	FactoryRuntimeInputs                       interface {
		ReadDir(string) ([]fs.DirEntry, error)
		ReadFile(string) ([]byte, error)
		Stat(string) (fs.FileInfo, error)
	}
	FactoryRuntimeInputDirectoryWalker             factoryruntime.InputDirectoryWalker
	FactoryRuntimeWorkflowSources                  factoryruntime.WorkflowSourceFileSystem
	FactoryRuntimeWorkflowSourceResolveSymlinks    factoryruntime.WorkflowSourceResolveSymlinks
	FactoryRuntimeWorkflowHome                     factoryruntime.WorkflowHomeResolver
	FactoryDefinitionPortableFileSystem            portablefiles.FileSystem
	FactoryDefinitionLoadingFileSystem             factorydefinitions.LoadingFileSystem
	FactoryDefinitionClock                         factorydefinitions.Clock
	FactoryDefinitionVersionFileSystem             factorydefinitions.VersionFileSystem
	FactoryDefinitionPackagedGoalPromptFileSystem  factorydefinitions.PackagedGoalPromptFileSystem
	FactoryDefinitionPortableBundledFileInspection factorydefinitions.PortableBundledFileInspection
	FactoryDefinitionRequiredToolPathLookup        factorydefinitions.RequiredToolPathLookup
	FactoryDefinitionRequiredToolVersionProbe      factorydefinitions.RequiredToolVersionProbe
	FactoryDefinitionPersistenceFileSystem         factorydefinitions.PersistenceFileSystem
	FactoryDefinitionDirectoryReplacementStore     factorydefinitions.DirectoryReplacementStore
	FactoryDefinitionNamedPathFileSystem           interface {
		ReadFile(string) ([]byte, error)
		Stat(string) (fs.FileInfo, error)
		MkdirAll(string, fs.FileMode) error
		WriteFile(string, []byte, fs.FileMode) error
	}
	FactoryDefinitionNamedFactoryCatalogFileSystem        factorydefinitions.NamedFactoryCatalogFileSystem
	FactoryDefinitionPackagedInstallationFileSystem       factorydefinitions.PackagedInstallationFileSystem
	FactoryDefinitionPackagedInstallationDirectoryCreator factorydefinitions.PackagedInstallationDirectoryCreator
	FactoryDefinitionAuthoredReaderFileSystem             factorydefinitions.AuthoredLayoutReaderFileSystem
	FactoryDefinitionAuthoredWriterFileSystem             factorydefinitions.AuthoredLayoutWriterFileSystem
	FactoryDefinitionScaffoldFileSystem                   factorydefinitions.ScaffoldFileSystem
	FactoryDefinitionScaffoldOutput                       factorydefinitions.ScaffoldOutput
	ProviderSessionFileSystem                             interface {
		Open(string) (io.ReadCloser, error)
		Stat(string) (fs.FileInfo, error)
	}
	ProviderSessionResolveHomeDirectory  func() (string, error)
	WorkerSessionResolveHomeDirectory    func() (string, error)
	ProviderSessionCodexWalkDirectory    func(string, fs.WalkDirFunc) error
	ProviderSessionCodexResolveSymlinks  func(string) (string, error)
	ProviderSessionCursorWalkDirectory   func(string, fs.WalkDirFunc) error
	ProviderSessionCursorResolveSymlinks func(string) (string, error)
	ProviderSessionCursorOpenDatabase    func(string, string) (*sql.DB, error)
	ProviderSessionOperatingSystem       string
	OperatorSettingsFileSystem           operatorsettings.FileSystem
	OperatorSettingsCreateTemporaryFile  operatorsettings.CreateTemporaryFile
	OperatorSettingsIDGenerator          operatorsettings.IDGenerator
	SystemInitializationInspectPath      func(string) (fs.FileInfo, error)

	// RecordingsRootObserver is a construction-time observation seam for
	// functional callers that need to exercise the public Recordings root
	// directly. It does not replace or mutate any Recordings dependency.
	RecordingsRootObserver               func(recordings.Service)
	RecordingsWorkSnapshotReaderObserver func(interface {
		ReadWorkSnapshot(context.Context, string) (work.ReadSnapshot, error)
	})
	Clock                            platformclock.Source
	ACPWireRecorder                  wiretranscript.WireRecorder
	SubmissionRecorder               recordings.SubmissionRecorder
	DispatchRecorder                 recordings.DispatchRecorder
	WorkerRecordingWriter            recordings.WorkerRecordingWriter
	RecordingWriteFile               func(string, []byte) error
	RecordingAppendFile              func(string, []byte) error
	RecordingMakeDirectories         recordings.RecordingMakeDirectories
	RecordingCreateTempFile          recordings.RecordingCreateTemporaryFile
	RecordingRemovePath              recordings.RecordingRemovePath
	RecordingRenamePath              recordings.RecordingRenamePath
	RecordingReadFile                recordings.RecordingReadFile
	RecordingOpenFile                recordings.RecordingOpenFile
	RecordingReadDirectory           func(string) ([]fs.DirEntry, error)
	APIServerStarter                 platformhttpserver.Starter
	BrowserOpener                    platformbrowser.Opener
	InvocationMetricsRecorder        factorysessions.InvocationMetricsRecorder
	RuntimeHostObserver              factorysessions.RuntimeHostObserver
	FactoryVisualizationSink         factoryvisualization.Sink
	FactoryVisualizationRootObserver factoryvisualization.RootObserver
	ModelPullMetricsRecorder         interface{ RecordModelPullMetric(PullMetric) }
	ProviderOverride                 providers.Service
	providercontract.ProviderRegistrations
	ProviderCatalogCapabilityOverrides []providercontract.CatalogCapabilityOverride
	WorkersFactoryDocsFileSystem       platformfilesystem.ReadFileTree
	WorkersResolveSymlinks             workers.ResolveExecutableSymlinks
	WorkersExecutableLocator           platformprocess.ExecutableLocator
	WorkersExecutablePathInspector     platformfilesystem.PathInspector
	WorkersExecutableFileReader        platformfilesystem.ReadOpener
	WorkersOperatingSystem             workers.OperatingSystem
	WorkersWorktreeFileSystem          workers.WorktreeFileSystem
	WorkersWorktreeGit                 workers.WorktreeGitCommander
	WorkersAgentToolFileSystem         workers.AgentToolFileSystem
	WorkersMockWorkersConfigFileSystem workers.MockWorkersConfigFileSystem
	WorkersRetryRandomSource           platformrandom.Source
	WorkersWorkstationFileSystem       platformfilesystem.ReadFileInspector
	WorkersProviderTemporaryFileSystem platformfilesystem.TemporaryFileSystem

	ScriptCommandRunner platformprocess.CommandRunner

	WorkContentStagingFileSystem   work.ContentStagingFileSystem
	WorkContentStagingRandom       work.ContentStagingRandom
	WorkContentStagingClock        work.ContentStagingClock
	WorkContentHostPlatform        work.ContentHostPlatform
	WorkContentInspectPath         work.ContentInspectPath
	WorkContentCreateTempFile      work.ContentCreateTemporaryFile
	WorkContentRemovePath          work.ContentRemovePath
	WorkContentWriteFile           work.ContentWriteFile
	WorkContentOpenFile            work.ContentOpenFile
	WorkContentHTTPDoer            work.ContentHTTPDoer
	WorkRequestIDGenerator         work.RequestIDGenerator
	WorkSubmittedFileReader        work.SubmittedFileReader
	WorkSubmittedFilePathInspector work.SubmittedFilePathInspector
}

// Merge overlays non-zero replacements onto defaults.
// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func Merge(defaults Edges, replacements Edges) Edges {
	defaults.ProviderRegistrations = append(
		append(providercontract.ProviderRegistrations(nil), defaults.ProviderRegistrations...),
		replacements.ProviderRegistrations...,
	)
	defaults.ProviderCatalogCapabilityOverrides = append(
		cloneCatalogCapabilityOverrides(defaults.ProviderCatalogCapabilityOverrides),
		cloneCatalogCapabilityOverrides(replacements.ProviderCatalogCapabilityOverrides)...,
	)
	if replacements.CLIObserver != nil {
		defaults.CLIObserver = replacements.CLIObserver
	}
	if replacements.PlatformProcessClock != nil {
		defaults.PlatformProcessClock = replacements.PlatformProcessClock
	}
	if replacements.PlatformProcessCommandFactory != nil {
		defaults.PlatformProcessCommandFactory = replacements.PlatformProcessCommandFactory
	}
	if replacements.ProvidersExecutableLocator != nil {
		defaults.ProvidersExecutableLocator = replacements.ProvidersExecutableLocator
	}
	if replacements.ProviderCommandRunner != nil {
		defaults.ProviderCommandRunner = replacements.ProviderCommandRunner
	}
	if replacements.AgyPTYHost != nil {
		defaults.AgyPTYHost = replacements.AgyPTYHost
	}
	if replacements.AgyPTYClock != nil {
		defaults.AgyPTYClock = replacements.AgyPTYClock
	}
	if replacements.HostedHTTPClient != nil {
		defaults.HostedHTTPClient = replacements.HostedHTTPClient
	}
	if replacements.HostedLinearEndpoint != "" {
		defaults.HostedLinearEndpoint = replacements.HostedLinearEndpoint
	}
	if replacements.HostedSecretResolver != nil {
		defaults.HostedSecretResolver = replacements.HostedSecretResolver
	}
	if replacements.HostedLinearCheckpointStore != nil {
		defaults.HostedLinearCheckpointStore = replacements.HostedLinearCheckpointStore
	}
	if replacements.HostedClock != nil {
		defaults.HostedClock = replacements.HostedClock
	}
	if replacements.AutomationsCursorFileSystem != nil {
		defaults.AutomationsCursorFileSystem = replacements.AutomationsCursorFileSystem
	}
	if replacements.FactoryWebhookHTTPClient != nil {
		defaults.FactoryWebhookHTTPClient = replacements.FactoryWebhookHTTPClient
	}
	if replacements.FactoryWebhookClock != nil {
		defaults.FactoryWebhookClock = replacements.FactoryWebhookClock
	}
	if replacements.FactoryWebhookSecretResolver != nil {
		defaults.FactoryWebhookSecretResolver = replacements.FactoryWebhookSecretResolver
	}
	if replacements.FactoryWebhookDeadLetterAppender != nil {
		defaults.FactoryWebhookDeadLetterAppender = replacements.FactoryWebhookDeadLetterAppender
	}
	if replacements.ModelAssetHTTPClient != nil {
		defaults.ModelAssetHTTPClient = replacements.ModelAssetHTTPClient
	}
	if replacements.ModelAssetEndpoints.BaseURL != "" {
		defaults.ModelAssetEndpoints.BaseURL = replacements.ModelAssetEndpoints.BaseURL
	}
	if replacements.ModelAssetEndpoints.APIBaseURL != "" {
		defaults.ModelAssetEndpoints.APIBaseURL = replacements.ModelAssetEndpoints.APIBaseURL
	}
	if replacements.ModelAssetHostPlatform.OperatingSystem != "" {
		defaults.ModelAssetHostPlatform.OperatingSystem = replacements.ModelAssetHostPlatform.OperatingSystem
	}
	if replacements.ModelAssetHostPlatform.Architecture != "" {
		defaults.ModelAssetHostPlatform.Architecture = replacements.ModelAssetHostPlatform.Architecture
	}
	if replacements.ModelResolveHuggingFaceRevision != nil {
		defaults.ModelResolveHuggingFaceRevision = replacements.ModelResolveHuggingFaceRevision
	}
	if replacements.ModelAssetResolveEnvironment != nil {
		defaults.ModelAssetResolveEnvironment = replacements.ModelAssetResolveEnvironment
	}
	if replacements.ModelAssetMakeDirectories != nil {
		defaults.ModelAssetMakeDirectories = replacements.ModelAssetMakeDirectories
	}
	if replacements.ModelAssetInspectPath != nil {
		defaults.ModelAssetInspectPath = replacements.ModelAssetInspectPath
	}
	if replacements.ModelAssetResolveHomeDirectory != nil {
		defaults.ModelAssetResolveHomeDirectory = replacements.ModelAssetResolveHomeDirectory
	}
	if replacements.ModelAssetWriteFile != nil {
		defaults.ModelAssetWriteFile = replacements.ModelAssetWriteFile
	}
	if replacements.ModelAssetRenamePath != nil {
		defaults.ModelAssetRenamePath = replacements.ModelAssetRenamePath
	}
	if replacements.ModelAssetRemovePath != nil {
		defaults.ModelAssetRemovePath = replacements.ModelAssetRemovePath
	}
	if replacements.ModelAssetReadFile != nil {
		defaults.ModelAssetReadFile = replacements.ModelAssetReadFile
	}
	if replacements.ModelAssetReadDirectory != nil {
		defaults.ModelAssetReadDirectory = replacements.ModelAssetReadDirectory
	}
	if replacements.ModelAssetCreateFile != nil {
		defaults.ModelAssetCreateFile = replacements.ModelAssetCreateFile
	}
	if replacements.ModelAssetOpenFile != nil {
		defaults.ModelAssetOpenFile = replacements.ModelAssetOpenFile
	}
	if replacements.ModelAssetStagingCoordinationFactory != nil {
		defaults.ModelAssetStagingCoordinationFactory = replacements.ModelAssetStagingCoordinationFactory
	}
	if replacements.ModelCLIInputReadFile != nil {
		defaults.ModelCLIInputReadFile = replacements.ModelCLIInputReadFile
	}
	if replacements.ModelCLIOutputCreateTempFile != nil {
		defaults.ModelCLIOutputCreateTempFile = replacements.ModelCLIOutputCreateTempFile
	}
	if replacements.ModelCLIOutputInspectPath != nil {
		defaults.ModelCLIOutputInspectPath = replacements.ModelCLIOutputInspectPath
	}
	if replacements.ModelCLIOutputRemovePath != nil {
		defaults.ModelCLIOutputRemovePath = replacements.ModelCLIOutputRemovePath
	}
	if replacements.ModelCLIOutputRenamePath != nil {
		defaults.ModelCLIOutputRenamePath = replacements.ModelCLIOutputRenamePath
	}
	if replacements.ModelHostProcessLauncher != nil {
		defaults.ModelHostProcessLauncher = replacements.ModelHostProcessLauncher
	}
	if replacements.ModelHostHTTPClient != nil {
		defaults.ModelHostHTTPClient = replacements.ModelHostHTTPClient
	}
	if replacements.ModelHostClock != nil {
		defaults.ModelHostClock = replacements.ModelHostClock
	}
	if replacements.ModelHostProtocolNegotiator != nil {
		defaults.ModelHostProtocolNegotiator = replacements.ModelHostProtocolNegotiator
	}
	if replacements.ModelHostGRPCDialer != nil {
		defaults.ModelHostGRPCDialer = replacements.ModelHostGRPCDialer
	}
	if replacements.ModelHostCompatibilityChecker != nil {
		defaults.ModelHostCompatibilityChecker = replacements.ModelHostCompatibilityChecker
	}
	if replacements.ModelResolveBackendArtifact != nil {
		defaults.ModelResolveBackendArtifact = replacements.ModelResolveBackendArtifact
	}
	if replacements.ModelInvocationBackend != nil {
		defaults.ModelInvocationBackend = replacements.ModelInvocationBackend
	}
	if replacements.ModelASRBackend != nil {
		defaults.ModelASRBackend = replacements.ModelASRBackend
	}
	if replacements.ModelEmbeddingBackend != nil {
		defaults.ModelEmbeddingBackend = replacements.ModelEmbeddingBackend
	}
	if replacements.ModelRuntimeCommandRunner != nil {
		defaults.ModelRuntimeCommandRunner = replacements.ModelRuntimeCommandRunner
	}
	if replacements.ModelRuntimeHTTPClient != nil {
		defaults.ModelRuntimeHTTPClient = replacements.ModelRuntimeHTTPClient
	}
	if replacements.ModelRuntimeInspectFile != nil {
		defaults.ModelRuntimeInspectFile = replacements.ModelRuntimeInspectFile
	}
	if replacements.ModelRuntimeTempDirectory != nil {
		defaults.ModelRuntimeTempDirectory = replacements.ModelRuntimeTempDirectory
	}
	if replacements.ModelRuntimeCreateTempFile != nil {
		defaults.ModelRuntimeCreateTempFile = replacements.ModelRuntimeCreateTempFile
	}
	if replacements.ModelInvocationArtifactFileSystem != nil {
		defaults.ModelInvocationArtifactFileSystem = replacements.ModelInvocationArtifactFileSystem
	}
	if replacements.ModelInvocationProtocolClient != nil {
		defaults.ModelInvocationProtocolClient = replacements.ModelInvocationProtocolClient
	}
	if replacements.ModelInvocationGRPCDialer != nil {
		defaults.ModelInvocationGRPCDialer = replacements.ModelInvocationGRPCDialer
	}
	if replacements.FactorySessionsWorkingDirectory != nil {
		defaults.FactorySessionsWorkingDirectory = replacements.FactorySessionsWorkingDirectory
	}
	if replacements.FactorySessionExecutionOpeningFileSystem != nil {
		defaults.FactorySessionExecutionOpeningFileSystem = replacements.FactorySessionExecutionOpeningFileSystem
	}
	if replacements.FactorySessionDirectoryInspection != nil {
		defaults.FactorySessionDirectoryInspection = replacements.FactorySessionDirectoryInspection
	}
	if replacements.FactorySessionResolveHomeDirectory != nil {
		defaults.FactorySessionResolveHomeDirectory = replacements.FactorySessionResolveHomeDirectory
	}
	if replacements.FactorySessionResolveLogicalTargetSymlinks != nil {
		defaults.FactorySessionResolveLogicalTargetSymlinks = replacements.FactorySessionResolveLogicalTargetSymlinks
	}
	if replacements.FactorySessionIDGenerator != nil {
		defaults.FactorySessionIDGenerator = replacements.FactorySessionIDGenerator
	}
	if replacements.FactorySessionRuntimeInstanceIDGenerator != nil {
		defaults.FactorySessionRuntimeInstanceIDGenerator = replacements.FactorySessionRuntimeInstanceIDGenerator
	}
	if replacements.FactorySessionResponseEventIDGenerator != nil {
		defaults.FactorySessionResponseEventIDGenerator = replacements.FactorySessionResponseEventIDGenerator
	}
	if replacements.FactorySessionResponseEventRetentionLimits != nil {
		defaults.FactorySessionResponseEventRetentionLimits = replacements.FactorySessionResponseEventRetentionLimits
	}
	if replacements.FactorySessionCursorPersistenceFileSystem != nil {
		defaults.FactorySessionCursorPersistenceFileSystem = replacements.FactorySessionCursorPersistenceFileSystem
	}
	if replacements.FactorySessionCursorCreateTemporaryFile != nil {
		defaults.FactorySessionCursorCreateTemporaryFile = replacements.FactorySessionCursorCreateTemporaryFile
	}
	if replacements.FactorySessionRuntimePersistenceFileSystem != nil {
		defaults.FactorySessionRuntimePersistenceFileSystem = replacements.FactorySessionRuntimePersistenceFileSystem
	}
	if replacements.FactorySessionContractFixtureReader != nil {
		defaults.FactorySessionContractFixtureReader = replacements.FactorySessionContractFixtureReader
	}
	if replacements.FactorySessionInvocationInputReader != nil {
		defaults.FactorySessionInvocationInputReader = replacements.FactorySessionInvocationInputReader
	}
	if replacements.FactorySessionReplayRecordingReader != nil {
		defaults.FactorySessionReplayRecordingReader = replacements.FactorySessionReplayRecordingReader
	}
	if replacements.FactorySessionInitialWorkReader != nil {
		defaults.FactorySessionInitialWorkReader = replacements.FactorySessionInitialWorkReader
	}
	if replacements.FactoryRuntimeIDGenerator != nil {
		defaults.FactoryRuntimeIDGenerator = replacements.FactoryRuntimeIDGenerator
	}
	if replacements.FactoryRuntimeDirectories != nil {
		defaults.FactoryRuntimeDirectories = replacements.FactoryRuntimeDirectories
	}
	if replacements.FactoryRuntimeInputs != nil {
		defaults.FactoryRuntimeInputs = replacements.FactoryRuntimeInputs
	}
	if replacements.FactoryRuntimeInputDirectoryWalker != nil {
		defaults.FactoryRuntimeInputDirectoryWalker = replacements.FactoryRuntimeInputDirectoryWalker
	}
	if replacements.FactoryRuntimeWorkflowSources != nil {
		defaults.FactoryRuntimeWorkflowSources = replacements.FactoryRuntimeWorkflowSources
	}
	if replacements.FactoryRuntimeWorkflowSourceResolveSymlinks != nil {
		defaults.FactoryRuntimeWorkflowSourceResolveSymlinks = replacements.FactoryRuntimeWorkflowSourceResolveSymlinks
	}
	if replacements.FactoryRuntimeWorkflowHome != nil {
		defaults.FactoryRuntimeWorkflowHome = replacements.FactoryRuntimeWorkflowHome
	}
	if replacements.FactoryDefinitionPortableFileSystem != nil {
		defaults.FactoryDefinitionPortableFileSystem = replacements.FactoryDefinitionPortableFileSystem
	}
	if replacements.FactoryDefinitionLoadingFileSystem != nil {
		defaults.FactoryDefinitionLoadingFileSystem = replacements.FactoryDefinitionLoadingFileSystem
	}
	if replacements.FactoryDefinitionClock != nil {
		defaults.FactoryDefinitionClock = replacements.FactoryDefinitionClock
	}
	if replacements.FactoryDefinitionVersionFileSystem != nil {
		defaults.FactoryDefinitionVersionFileSystem = replacements.FactoryDefinitionVersionFileSystem
	}
	if replacements.FactoryDefinitionPackagedGoalPromptFileSystem != nil {
		defaults.FactoryDefinitionPackagedGoalPromptFileSystem = replacements.FactoryDefinitionPackagedGoalPromptFileSystem
	}
	if replacements.FactoryDefinitionPortableBundledFileInspection != nil {
		defaults.FactoryDefinitionPortableBundledFileInspection = replacements.FactoryDefinitionPortableBundledFileInspection
	}
	if replacements.FactoryDefinitionRequiredToolPathLookup != nil {
		defaults.FactoryDefinitionRequiredToolPathLookup = replacements.FactoryDefinitionRequiredToolPathLookup
	}
	if replacements.FactoryDefinitionRequiredToolVersionProbe != nil {
		defaults.FactoryDefinitionRequiredToolVersionProbe = replacements.FactoryDefinitionRequiredToolVersionProbe
	}
	if replacements.FactoryDefinitionPersistenceFileSystem != nil {
		defaults.FactoryDefinitionPersistenceFileSystem = replacements.FactoryDefinitionPersistenceFileSystem
	}
	if replacements.FactoryDefinitionDirectoryReplacementStore != nil {
		defaults.FactoryDefinitionDirectoryReplacementStore = replacements.FactoryDefinitionDirectoryReplacementStore
	}
	if replacements.FactoryDefinitionNamedPathFileSystem != nil {
		defaults.FactoryDefinitionNamedPathFileSystem = replacements.FactoryDefinitionNamedPathFileSystem
	}
	if replacements.FactoryDefinitionNamedFactoryCatalogFileSystem != nil {
		defaults.FactoryDefinitionNamedFactoryCatalogFileSystem = replacements.FactoryDefinitionNamedFactoryCatalogFileSystem
	}
	if replacements.FactoryDefinitionPackagedInstallationFileSystem != nil {
		defaults.FactoryDefinitionPackagedInstallationFileSystem = replacements.FactoryDefinitionPackagedInstallationFileSystem
	}
	if replacements.FactoryDefinitionPackagedInstallationDirectoryCreator != nil {
		defaults.FactoryDefinitionPackagedInstallationDirectoryCreator = replacements.FactoryDefinitionPackagedInstallationDirectoryCreator
	}
	if replacements.FactoryDefinitionAuthoredReaderFileSystem != nil {
		defaults.FactoryDefinitionAuthoredReaderFileSystem = replacements.FactoryDefinitionAuthoredReaderFileSystem
	}
	if replacements.FactoryDefinitionAuthoredWriterFileSystem != nil {
		defaults.FactoryDefinitionAuthoredWriterFileSystem = replacements.FactoryDefinitionAuthoredWriterFileSystem
	}
	if replacements.FactoryDefinitionScaffoldFileSystem != nil {
		defaults.FactoryDefinitionScaffoldFileSystem = replacements.FactoryDefinitionScaffoldFileSystem
	}
	if replacements.FactoryDefinitionScaffoldOutput != nil {
		defaults.FactoryDefinitionScaffoldOutput = replacements.FactoryDefinitionScaffoldOutput
	}
	if replacements.ProviderSessionFileSystem != nil {
		defaults.ProviderSessionFileSystem = replacements.ProviderSessionFileSystem
	}
	if replacements.ProviderSessionResolveHomeDirectory != nil {
		defaults.ProviderSessionResolveHomeDirectory = replacements.ProviderSessionResolveHomeDirectory
	}
	if replacements.WorkerSessionResolveHomeDirectory != nil {
		defaults.WorkerSessionResolveHomeDirectory = replacements.WorkerSessionResolveHomeDirectory
	}
	if replacements.ProviderSessionCodexWalkDirectory != nil {
		defaults.ProviderSessionCodexWalkDirectory = replacements.ProviderSessionCodexWalkDirectory
	}
	if replacements.ProviderSessionCodexResolveSymlinks != nil {
		defaults.ProviderSessionCodexResolveSymlinks = replacements.ProviderSessionCodexResolveSymlinks
	}
	if replacements.ProviderSessionCursorWalkDirectory != nil {
		defaults.ProviderSessionCursorWalkDirectory = replacements.ProviderSessionCursorWalkDirectory
	}
	if replacements.ProviderSessionCursorResolveSymlinks != nil {
		defaults.ProviderSessionCursorResolveSymlinks = replacements.ProviderSessionCursorResolveSymlinks
	}
	if replacements.ProviderSessionCursorOpenDatabase != nil {
		defaults.ProviderSessionCursorOpenDatabase = replacements.ProviderSessionCursorOpenDatabase
	}
	if replacements.ProviderSessionOperatingSystem != "" {
		defaults.ProviderSessionOperatingSystem = replacements.ProviderSessionOperatingSystem
	}
	if replacements.OperatorSettingsFileSystem != nil {
		defaults.OperatorSettingsFileSystem = replacements.OperatorSettingsFileSystem
	}
	if replacements.OperatorSettingsCreateTemporaryFile != nil {
		defaults.OperatorSettingsCreateTemporaryFile = replacements.OperatorSettingsCreateTemporaryFile
	}
	if replacements.OperatorSettingsIDGenerator != nil {
		defaults.OperatorSettingsIDGenerator = replacements.OperatorSettingsIDGenerator
	}
	if replacements.SystemInitializationInspectPath != nil {
		defaults.SystemInitializationInspectPath = replacements.SystemInitializationInspectPath
	}
	if replacements.Clock != nil {
		defaults.Clock = replacements.Clock
	}
	if replacements.ACPWireRecorder != nil {
		defaults.ACPWireRecorder = replacements.ACPWireRecorder
	}
	if replacements.SubmissionRecorder != nil {
		defaults.SubmissionRecorder = replacements.SubmissionRecorder
	}
	if replacements.DispatchRecorder != nil {
		defaults.DispatchRecorder = replacements.DispatchRecorder
	}
	if replacements.WorkerRecordingWriter != nil {
		defaults.WorkerRecordingWriter = replacements.WorkerRecordingWriter
	}
	if replacements.RecordingWriteFile != nil {
		defaults.RecordingWriteFile = replacements.RecordingWriteFile
	}
	if replacements.RecordingAppendFile != nil {
		defaults.RecordingAppendFile = replacements.RecordingAppendFile
	}
	if replacements.RecordingMakeDirectories != nil {
		defaults.RecordingMakeDirectories = replacements.RecordingMakeDirectories
	}
	if replacements.RecordingCreateTempFile != nil {
		defaults.RecordingCreateTempFile = replacements.RecordingCreateTempFile
	}
	if replacements.RecordingRemovePath != nil {
		defaults.RecordingRemovePath = replacements.RecordingRemovePath
	}
	if replacements.RecordingRenamePath != nil {
		defaults.RecordingRenamePath = replacements.RecordingRenamePath
	}
	if replacements.RecordingReadFile != nil {
		defaults.RecordingReadFile = replacements.RecordingReadFile
	}
	if replacements.RecordingOpenFile != nil {
		defaults.RecordingOpenFile = replacements.RecordingOpenFile
	}
	if replacements.RecordingReadDirectory != nil {
		defaults.RecordingReadDirectory = replacements.RecordingReadDirectory
	}
	if replacements.RecordingsRootObserver != nil {
		defaults.RecordingsRootObserver = replacements.RecordingsRootObserver
	}
	if replacements.RecordingsWorkSnapshotReaderObserver != nil {
		defaults.RecordingsWorkSnapshotReaderObserver = replacements.RecordingsWorkSnapshotReaderObserver
	}
	if replacements.APIServerStarter != nil {
		defaults.APIServerStarter = replacements.APIServerStarter
	}
	if replacements.BrowserOpener != nil {
		defaults.BrowserOpener = replacements.BrowserOpener
	}
	if replacements.InvocationMetricsRecorder != nil {
		defaults.InvocationMetricsRecorder = replacements.InvocationMetricsRecorder
	}
	if replacements.RuntimeHostObserver != nil {
		defaults.RuntimeHostObserver = replacements.RuntimeHostObserver
	}
	if replacements.FactoryVisualizationSink != nil {
		defaults.FactoryVisualizationSink = replacements.FactoryVisualizationSink
	}
	if replacements.FactoryVisualizationRootObserver != nil {
		defaults.FactoryVisualizationRootObserver = replacements.FactoryVisualizationRootObserver
	}
	if replacements.ModelPullMetricsRecorder != nil {
		defaults.ModelPullMetricsRecorder = replacements.ModelPullMetricsRecorder
	}
	if replacements.ProviderOverride != nil {
		defaults.ProviderOverride = replacements.ProviderOverride
	}
	if replacements.WorkersFactoryDocsFileSystem != nil {
		defaults.WorkersFactoryDocsFileSystem = replacements.WorkersFactoryDocsFileSystem
	}
	if replacements.WorkersResolveSymlinks != nil {
		defaults.WorkersResolveSymlinks = replacements.WorkersResolveSymlinks
	}
	if replacements.WorkersExecutableLocator != nil {
		defaults.WorkersExecutableLocator = replacements.WorkersExecutableLocator
	}
	if replacements.WorkersExecutablePathInspector != nil {
		defaults.WorkersExecutablePathInspector = replacements.WorkersExecutablePathInspector
	}
	if replacements.WorkersExecutableFileReader != nil {
		defaults.WorkersExecutableFileReader = replacements.WorkersExecutableFileReader
	}
	if replacements.WorkersOperatingSystem != "" {
		defaults.WorkersOperatingSystem = replacements.WorkersOperatingSystem
	}
	if replacements.WorkersWorktreeFileSystem != nil {
		defaults.WorkersWorktreeFileSystem = replacements.WorkersWorktreeFileSystem
	}
	if replacements.WorkersWorktreeGit != nil {
		defaults.WorkersWorktreeGit = replacements.WorkersWorktreeGit
	}
	if replacements.WorkersAgentToolFileSystem != nil {
		defaults.WorkersAgentToolFileSystem = replacements.WorkersAgentToolFileSystem
	}
	if replacements.WorkersMockWorkersConfigFileSystem != nil {
		defaults.WorkersMockWorkersConfigFileSystem = replacements.WorkersMockWorkersConfigFileSystem
	}
	if replacements.WorkersRetryRandomSource != nil {
		defaults.WorkersRetryRandomSource = replacements.WorkersRetryRandomSource
	}
	if replacements.WorkersWorkstationFileSystem != nil {
		defaults.WorkersWorkstationFileSystem = replacements.WorkersWorkstationFileSystem
	}
	if replacements.WorkersProviderTemporaryFileSystem != nil {
		defaults.WorkersProviderTemporaryFileSystem = replacements.WorkersProviderTemporaryFileSystem
	}
	if replacements.ScriptCommandRunner != nil {
		defaults.ScriptCommandRunner = replacements.ScriptCommandRunner
	}
	if replacements.WorkContentStagingFileSystem != nil {
		defaults.WorkContentStagingFileSystem = replacements.WorkContentStagingFileSystem
	}
	if replacements.WorkContentStagingRandom != nil {
		defaults.WorkContentStagingRandom = replacements.WorkContentStagingRandom
	}
	if replacements.WorkContentStagingClock != nil {
		defaults.WorkContentStagingClock = replacements.WorkContentStagingClock
	}
	if replacements.WorkContentHostPlatform != "" {
		defaults.WorkContentHostPlatform = replacements.WorkContentHostPlatform
	}
	if replacements.WorkContentInspectPath != nil {
		defaults.WorkContentInspectPath = replacements.WorkContentInspectPath
	}
	if replacements.WorkContentCreateTempFile != nil {
		defaults.WorkContentCreateTempFile = replacements.WorkContentCreateTempFile
	}
	if replacements.WorkContentRemovePath != nil {
		defaults.WorkContentRemovePath = replacements.WorkContentRemovePath
	}
	if replacements.WorkContentWriteFile != nil {
		defaults.WorkContentWriteFile = replacements.WorkContentWriteFile
	}
	if replacements.WorkContentOpenFile != nil {
		defaults.WorkContentOpenFile = replacements.WorkContentOpenFile
	}
	if replacements.WorkContentHTTPDoer != nil {
		defaults.WorkContentHTTPDoer = replacements.WorkContentHTTPDoer
	}
	if replacements.WorkRequestIDGenerator != nil {
		defaults.WorkRequestIDGenerator = replacements.WorkRequestIDGenerator
	}
	if replacements.WorkSubmittedFileReader != nil {
		defaults.WorkSubmittedFileReader = replacements.WorkSubmittedFileReader
	}
	if replacements.WorkSubmittedFilePathInspector != nil {
		defaults.WorkSubmittedFilePathInspector = replacements.WorkSubmittedFilePathInspector
	}
	return defaults
}

func cloneCatalogCapabilityOverrides(
	overrides []providercontract.CatalogCapabilityOverride,
) []providercontract.CatalogCapabilityOverride {
	cloned := make([]providercontract.CatalogCapabilityOverride, len(overrides))
	for index, override := range overrides {
		cloned[index] = override.Clone()
	}
	return cloned
}
