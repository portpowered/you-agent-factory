package root

import (
	"context"
	io "io"
	fs "io/fs"
	http "net/http"
	"strings"
	"testing"
	time "time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	portablefiles "github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	work "github.com/portpowered/infinite-you/pkg/services/work"
	workers "github.com/portpowered/infinite-you/pkg/services/workers"
)

// Each fixture is a supplied nil pointer implementing the exact effect port.
// These assertions exercise runtime admission, without reading source or
// checking a constructor inventory at test time.
type invalidOverrideCase struct {
	field string
	edges serviceedges.Edges
}

func TestBuildProcessRejectsEverySuppliedInterface(t *testing.T) {
	t.Parallel()
	var cases []invalidOverrideCase
	cases = append(cases, invalidOverrides0()...)
	cases = append(cases, invalidOverrides1()...)
	cases = append(cases, invalidOverrides2()...)
	cases = append(cases, invalidOverrides3()...)

	for _, test := range cases {
		t.Run(test.field, func(t *testing.T) {
			test.edges.RecordingsRootObserver = func(recordings.Service) { panic("constructor observed rejected override") }
			process, err := BuildProcess(context.Background(), test.edges)
			if process != nil || err == nil {
				t.Fatalf("BuildProcess = (%v, %v), want invalid %s", process, err, test.field)
			}
			if test.field == "Clock" {
				if err.Error() != "build application process: Clock must not be typed-nil; omit it to select the default" {
					t.Fatalf("Clock diagnostic = %q", err)
				}
			} else if !strings.Contains(err.Error(), "Edges."+test.field) {
				t.Fatalf("BuildProcess error = %v, want invalid %s", err, test.field)
			}
		})
	}
}

func invalidOverrides0() []invalidOverrideCase {
	return []invalidOverrideCase{
		{"PlatformProcessClock", serviceedges.Edges{PlatformProcessClock: (*nilPlatformProcessClock)(nil)}},
		{"ProvidersExecutableLocator", serviceedges.Edges{ProvidersExecutableLocator: (*nilProvidersExecutableLocator)(nil)}},
		{"ProviderCommandRunner", serviceedges.Edges{ProviderCommandRunner: (*nilProviderCommandRunner)(nil)}},
		{"AgyPTYHost", serviceedges.Edges{AgyPTYHost: (*nilAgyPTYHost)(nil)}},
		{"AgyPTYClock", serviceedges.Edges{AgyPTYClock: (*nilAgyPTYClock)(nil)}},
		{"HostedHTTPClient", serviceedges.Edges{HostedHTTPClient: (*nilHostedHTTPClient)(nil)}},
		{"HostedLinearCheckpointStore", serviceedges.Edges{HostedLinearCheckpointStore: (*nilHostedLinearCheckpointStore)(nil)}},
		{"HostedClock", serviceedges.Edges{HostedClock: (*nilHostedClock)(nil)}},
		{"AutomationsCursorFileSystem", serviceedges.Edges{AutomationsCursorFileSystem: (*nilAutomationsCursorFileSystem)(nil)}},
		{"FactoryWebhookHTTPClient", serviceedges.Edges{FactoryWebhookHTTPClient: (*nilFactoryWebhookHTTPClient)(nil)}},
		{"FactoryWebhookClock", serviceedges.Edges{FactoryWebhookClock: (*nilFactoryWebhookClock)(nil)}},
		{"ModelAssetHTTPClient", serviceedges.Edges{ModelAssetHTTPClient: (*nilModelAssetHTTPClient)(nil)}},
		{"ModelHostProcessLauncher", serviceedges.Edges{ModelHostProcessLauncher: (*nilModelHostProcessLauncher)(nil)}},
		{"ModelHostHTTPClient", serviceedges.Edges{ModelHostHTTPClient: (*nilModelHostHTTPClient)(nil)}},
		{"ModelHostClock", serviceedges.Edges{ModelHostClock: (*nilModelHostClock)(nil)}},
		{"ModelHostProtocolNegotiator", serviceedges.Edges{ModelHostProtocolNegotiator: (*nilModelHostProtocolNegotiator)(nil)}},
		{"ModelHostGRPCDialer", serviceedges.Edges{ModelHostGRPCDialer: (*nilModelHostGRPCDialer)(nil)}},
		{"ModelHostCompatibilityChecker", serviceedges.Edges{ModelHostCompatibilityChecker: (*nilModelHostCompatibilityChecker)(nil)}},
		{"ModelRuntimeCommandRunner", serviceedges.Edges{ModelRuntimeCommandRunner: (*nilModelRuntimeCommandRunner)(nil)}},
	}
}

func invalidOverrides1() []invalidOverrideCase {
	return []invalidOverrideCase{
		{"ModelInvocationArtifactFileSystem", serviceedges.Edges{ModelInvocationArtifactFileSystem: (*nilModelInvocationArtifactFileSystem)(nil)}},
		{"ModelInvocationProtocolClient", serviceedges.Edges{ModelInvocationProtocolClient: (*nilModelInvocationProtocolClient)(nil)}},
		{"ModelInvocationGRPCDialer", serviceedges.Edges{ModelInvocationGRPCDialer: (*nilModelInvocationGRPCDialer)(nil)}},
		{"FactorySessionsWorkingDirectory", serviceedges.Edges{FactorySessionsWorkingDirectory: (*nilFactorySessionsWorkingDirectory)(nil)}},
		{"FactorySessionDirectoryInspection", serviceedges.Edges{FactorySessionDirectoryInspection: (*nilFactorySessionDirectoryInspection)(nil)}},
		{"FactorySessionCursorPersistenceFileSystem", serviceedges.Edges{FactorySessionCursorPersistenceFileSystem: (*nilFactorySessionCursorPersistenceFileSystem)(nil)}},
		{"FactorySessionRuntimePersistenceFileSystem", serviceedges.Edges{FactorySessionRuntimePersistenceFileSystem: (*nilFactorySessionRuntimePersistenceFileSystem)(nil)}},
		{"FactoryRuntimeDirectories", serviceedges.Edges{FactoryRuntimeDirectories: (*nilFactoryRuntimeDirectories)(nil)}},
		{"FactoryRuntimeInputs", serviceedges.Edges{FactoryRuntimeInputs: (*nilFactoryRuntimeInputs)(nil)}},
		{"FactoryRuntimeWorkflowSources", serviceedges.Edges{FactoryRuntimeWorkflowSources: (*nilFactoryRuntimeWorkflowSources)(nil)}},
		{"FactoryDefinitionPortableFileSystem", serviceedges.Edges{FactoryDefinitionPortableFileSystem: (*nilFactoryDefinitionPortableFileSystem)(nil)}},
		{"FactoryDefinitionLoadingFileSystem", serviceedges.Edges{FactoryDefinitionLoadingFileSystem: (*nilFactoryDefinitionLoadingFileSystem)(nil)}},
		{"FactoryDefinitionClock", serviceedges.Edges{FactoryDefinitionClock: (*nilFactoryDefinitionClock)(nil)}},
		{"FactoryDefinitionVersionFileSystem", serviceedges.Edges{FactoryDefinitionVersionFileSystem: (*nilFactoryDefinitionVersionFileSystem)(nil)}},
		{"FactoryDefinitionPackagedGoalPromptFileSystem", serviceedges.Edges{FactoryDefinitionPackagedGoalPromptFileSystem: (*nilFactoryDefinitionPackagedGoalPromptFileSystem)(nil)}},
		{"FactoryDefinitionPortableBundledFileInspection", serviceedges.Edges{FactoryDefinitionPortableBundledFileInspection: (*nilFactoryDefinitionPortableBundledFileInspection)(nil)}},
		{"FactoryDefinitionPersistenceFileSystem", serviceedges.Edges{FactoryDefinitionPersistenceFileSystem: (*nilFactoryDefinitionPersistenceFileSystem)(nil)}},
		{"FactoryDefinitionDirectoryReplacementStore", serviceedges.Edges{FactoryDefinitionDirectoryReplacementStore: (*nilFactoryDefinitionDirectoryReplacementStore)(nil)}},
		{"FactoryDefinitionNamedPathFileSystem", serviceedges.Edges{FactoryDefinitionNamedPathFileSystem: (*nilFactoryDefinitionNamedPathFileSystem)(nil)}},
		{"FactoryDefinitionNamedFactoryCatalogFileSystem", serviceedges.Edges{FactoryDefinitionNamedFactoryCatalogFileSystem: (*nilFactoryDefinitionNamedFactoryCatalogFileSystem)(nil)}},
	}
}

func invalidOverrides2() []invalidOverrideCase {
	return []invalidOverrideCase{
		{"FactoryDefinitionPackagedInstallationFileSystem", serviceedges.Edges{FactoryDefinitionPackagedInstallationFileSystem: (*nilFactoryDefinitionPackagedInstallationFileSystem)(nil)}},
		{"FactoryDefinitionAuthoredReaderFileSystem", serviceedges.Edges{FactoryDefinitionAuthoredReaderFileSystem: (*nilFactoryDefinitionAuthoredReaderFileSystem)(nil)}},
		{"FactoryDefinitionAuthoredWriterFileSystem", serviceedges.Edges{FactoryDefinitionAuthoredWriterFileSystem: (*nilFactoryDefinitionAuthoredWriterFileSystem)(nil)}},
		{"FactoryDefinitionScaffoldFileSystem", serviceedges.Edges{FactoryDefinitionScaffoldFileSystem: (*nilFactoryDefinitionScaffoldFileSystem)(nil)}},
		{"FactoryDefinitionScaffoldOutput", serviceedges.Edges{FactoryDefinitionScaffoldOutput: (*nilFactoryDefinitionScaffoldOutput)(nil)}},
		{"ProviderSessionFileSystem", serviceedges.Edges{ProviderSessionFileSystem: (*nilProviderSessionFileSystem)(nil)}},
		{"OperatorSettingsFileSystem", serviceedges.Edges{OperatorSettingsFileSystem: (*nilOperatorSettingsFileSystem)(nil)}},
		{"Clock", serviceedges.Edges{Clock: (*nilClock)(nil)}},
		{"WorkerRecordingWriter", serviceedges.Edges{WorkerRecordingWriter: (*nilWorkerRecordingWriter)(nil)}},
		{"InvocationMetricsRecorder", serviceedges.Edges{InvocationMetricsRecorder: (*nilInvocationMetricsRecorder)(nil)}},
		{"FactoryVisualizationSink", serviceedges.Edges{FactoryVisualizationSink: (*nilFactoryVisualizationSink)(nil)}},
		{"ModelPullMetricsRecorder", serviceedges.Edges{ModelPullMetricsRecorder: (*nilModelPullMetricsRecorder)(nil)}},
		{"ProviderOverride", serviceedges.Edges{ProviderOverride: (*nilProviderOverride)(nil)}},
		{"WorkersFactoryDocsFileSystem", serviceedges.Edges{WorkersFactoryDocsFileSystem: (*nilWorkersFactoryDocsFileSystem)(nil)}},
		{"WorkersExecutableLocator", serviceedges.Edges{WorkersExecutableLocator: (*nilWorkersExecutableLocator)(nil)}},
		{"WorkersExecutablePathInspector", serviceedges.Edges{WorkersExecutablePathInspector: (*nilWorkersExecutablePathInspector)(nil)}},
		{"WorkersExecutableFileReader", serviceedges.Edges{WorkersExecutableFileReader: (*nilWorkersExecutableFileReader)(nil)}},
		{"WorkersInferenceMediaFileReader", serviceedges.Edges{WorkersInferenceMediaFileReader: (*nilWorkersInferenceMediaFileReader)(nil)}},
		{"WorkersWorktreeFileSystem", serviceedges.Edges{WorkersWorktreeFileSystem: (*nilWorkersWorktreeFileSystem)(nil)}},
		{"WorkersWorktreeGit", serviceedges.Edges{WorkersWorktreeGit: (*nilWorkersWorktreeGit)(nil)}},
	}
}

func invalidOverrides3() []invalidOverrideCase {
	return []invalidOverrideCase{
		{"WorkersAgentToolFileSystem", serviceedges.Edges{WorkersAgentToolFileSystem: (*nilWorkersAgentToolFileSystem)(nil)}},
		{"WorkersMockWorkersConfigFileSystem", serviceedges.Edges{WorkersMockWorkersConfigFileSystem: (*nilWorkersMockWorkersConfigFileSystem)(nil)}},
		{"WorkersRetryRandomSource", serviceedges.Edges{WorkersRetryRandomSource: (*nilWorkersRetryRandomSource)(nil)}},
		{"WorkersWorkstationFileSystem", serviceedges.Edges{WorkersWorkstationFileSystem: (*nilWorkersWorkstationFileSystem)(nil)}},
		{"WorkersProviderTemporaryFileSystem", serviceedges.Edges{WorkersProviderTemporaryFileSystem: (*nilWorkersProviderTemporaryFileSystem)(nil)}},
		{"ScriptCommandRunner", serviceedges.Edges{ScriptCommandRunner: (*nilScriptCommandRunner)(nil)}},
		{"WorkContentStagingFileSystem", serviceedges.Edges{WorkContentStagingFileSystem: (*nilWorkContentStagingFileSystem)(nil)}},
		{"WorkContentStagingRandom", serviceedges.Edges{WorkContentStagingRandom: (*nilWorkContentStagingRandom)(nil)}},
		{"WorkContentStagingClock", serviceedges.Edges{WorkContentStagingClock: (*nilWorkContentStagingClock)(nil)}},
		{"WorkContentHTTPDoer", serviceedges.Edges{WorkContentHTTPDoer: (*nilWorkContentHTTPDoer)(nil)}},
	}
}

type nilPlatformProcessClockEffect platformprocess.Clock
type nilPlatformProcessClock struct{ nilPlatformProcessClockEffect }

type nilProvidersExecutableLocatorEffect platformprocess.ExecutableLocator
type nilProvidersExecutableLocator struct {
	nilProvidersExecutableLocatorEffect
}

type nilProviderCommandRunnerEffect platformprocess.CommandRunner
type nilProviderCommandRunner struct{ nilProviderCommandRunnerEffect }

type nilAgyPTYHostEffect platformpty.Host
type nilAgyPTYHost struct{ nilAgyPTYHostEffect }

type nilAgyPTYClockEffect platformclock.Source
type nilAgyPTYClock struct{ nilAgyPTYClockEffect }

type nilHostedHTTPClientEffect automations.HostedLinearHTTPDoer
type nilHostedHTTPClient struct{ nilHostedHTTPClientEffect }

type nilHostedLinearCheckpointStoreEffect automations.HostedLinearCheckpointStore
type nilHostedLinearCheckpointStore struct {
	nilHostedLinearCheckpointStoreEffect
}

type nilHostedClockEffect automations.HostedLinearClock
type nilHostedClock struct{ nilHostedClockEffect }

type nilAutomationsCursorFileSystemEffect interface {
	ReadFile(string) ([]byte, error)
	MkdirAll(string, fs.FileMode) error
	WriteFile(string, []byte, fs.FileMode) error
	Rename(string, string) error
}
type nilAutomationsCursorFileSystem struct {
	nilAutomationsCursorFileSystemEffect
}

type nilFactoryWebhookHTTPClientEffect interface {
	Do(*http.Request) (*http.Response, error)
}
type nilFactoryWebhookHTTPClient struct {
	nilFactoryWebhookHTTPClientEffect
}

type nilFactoryWebhookClockEffect interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}
type nilFactoryWebhookClock struct{ nilFactoryWebhookClockEffect }

type nilModelAssetHTTPClientEffect interface {
	Do(*http.Request) (*http.Response, error)
}
type nilModelAssetHTTPClient struct{ nilModelAssetHTTPClientEffect }

type nilModelHostProcessLauncherEffect interface {
	Start(context.Context, serviceedges.HostProcessStartSpec) (interface {
		HealthEndpoint() string
		Wait() error
		Stop(context.Context) error
	}, error)
}
type nilModelHostProcessLauncher struct {
	nilModelHostProcessLauncherEffect
}

type nilModelHostHTTPClientEffect interface {
	Do(*http.Request) (*http.Response, error)
}
type nilModelHostHTTPClient struct{ nilModelHostHTTPClientEffect }

type nilModelHostClockEffect interface {
	Now() time.Time
	NewTimer(time.Duration) interface {
		C() <-chan time.Time
		Stop() bool
	}
}
type nilModelHostClock struct{ nilModelHostClockEffect }

type nilModelHostProtocolNegotiatorEffect interface {
	Negotiate(context.Context, string, serviceedges.ModelHostProtocolNegotiationRequest) (serviceedges.ModelHostProtocolNegotiationResult, error)
}
type nilModelHostProtocolNegotiator struct {
	nilModelHostProtocolNegotiatorEffect
}

type nilModelHostGRPCDialerEffect interface {
	Dial(context.Context, string) (interface {
		Negotiate(context.Context, serviceedges.ModelHostProtocolNegotiationRequest) (serviceedges.ModelHostProtocolNegotiationResult, error)
		Close() error
	}, error)
}
type nilModelHostGRPCDialer struct{ nilModelHostGRPCDialerEffect }

type nilModelHostCompatibilityCheckerEffect interface {
	Check(context.Context, serviceedges.ModelHostCompatibilityRequest) error
}
type nilModelHostCompatibilityChecker struct {
	nilModelHostCompatibilityCheckerEffect
}

type nilModelRuntimeCommandRunnerEffect platformprocess.CommandRunner
type nilModelRuntimeCommandRunner struct {
	nilModelRuntimeCommandRunnerEffect
}

type nilModelInvocationArtifactFileSystemEffect interface {
	Open(string) (io.ReadCloser, error)
	Create(string) (io.WriteCloser, error)
}
type nilModelInvocationArtifactFileSystem struct {
	nilModelInvocationArtifactFileSystemEffect
}

type nilModelInvocationProtocolClientEffect interface {
	Predict(context.Context, models.InvocationProtocolRequest) (models.InvocationProtocolResponse, error)
}
type nilModelInvocationProtocolClient struct {
	nilModelInvocationProtocolClientEffect
}

type nilModelInvocationGRPCDialerEffect platformgrpc.Dialer
type nilModelInvocationGRPCDialer struct {
	nilModelInvocationGRPCDialerEffect
}

type nilFactorySessionsWorkingDirectoryEffect platformfilesystem.WorkingDirectory
type nilFactorySessionsWorkingDirectory struct {
	nilFactorySessionsWorkingDirectoryEffect
}

type nilFactorySessionDirectoryInspectionEffect factorysessions.DirectoryInspection
type nilFactorySessionDirectoryInspection struct {
	nilFactorySessionDirectoryInspectionEffect
}

type nilFactorySessionCursorPersistenceFileSystemEffect factorysessions.CursorPersistenceFileSystem
type nilFactorySessionCursorPersistenceFileSystem struct {
	nilFactorySessionCursorPersistenceFileSystemEffect
}

type nilFactorySessionRuntimePersistenceFileSystemEffect factorysessions.RuntimePersistenceFileSystem
type nilFactorySessionRuntimePersistenceFileSystem struct {
	nilFactorySessionRuntimePersistenceFileSystemEffect
}

type nilFactoryRuntimeDirectoriesEffect factoryruntime.RuntimeDirectoryFileSystem
type nilFactoryRuntimeDirectories struct {
	nilFactoryRuntimeDirectoriesEffect
}

type nilFactoryRuntimeInputsEffect interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
}
type nilFactoryRuntimeInputs struct{ nilFactoryRuntimeInputsEffect }

type nilFactoryRuntimeWorkflowSourcesEffect factoryruntime.WorkflowSourceFileSystem
type nilFactoryRuntimeWorkflowSources struct {
	nilFactoryRuntimeWorkflowSourcesEffect
}

type nilFactoryDefinitionPortableFileSystemEffect portablefiles.FileSystem
type nilFactoryDefinitionPortableFileSystem struct {
	nilFactoryDefinitionPortableFileSystemEffect
}

type nilFactoryDefinitionLoadingFileSystemEffect factorydefinitions.LoadingFileSystem
type nilFactoryDefinitionLoadingFileSystem struct {
	nilFactoryDefinitionLoadingFileSystemEffect
}

type nilFactoryDefinitionClockEffect factorydefinitions.Clock
type nilFactoryDefinitionClock struct {
	nilFactoryDefinitionClockEffect
}

type nilFactoryDefinitionVersionFileSystemEffect factorydefinitions.VersionFileSystem
type nilFactoryDefinitionVersionFileSystem struct {
	nilFactoryDefinitionVersionFileSystemEffect
}

type nilFactoryDefinitionPackagedGoalPromptFileSystemEffect factorydefinitions.PackagedGoalPromptFileSystem
type nilFactoryDefinitionPackagedGoalPromptFileSystem struct {
	nilFactoryDefinitionPackagedGoalPromptFileSystemEffect
}

type nilFactoryDefinitionPortableBundledFileInspectionEffect factorydefinitions.PortableBundledFileInspection
type nilFactoryDefinitionPortableBundledFileInspection struct {
	nilFactoryDefinitionPortableBundledFileInspectionEffect
}

type nilFactoryDefinitionPersistenceFileSystemEffect factorydefinitions.PersistenceFileSystem
type nilFactoryDefinitionPersistenceFileSystem struct {
	nilFactoryDefinitionPersistenceFileSystemEffect
}

type nilFactoryDefinitionDirectoryReplacementStoreEffect factorydefinitions.DirectoryReplacementStore
type nilFactoryDefinitionDirectoryReplacementStore struct {
	nilFactoryDefinitionDirectoryReplacementStoreEffect
}

type nilFactoryDefinitionNamedPathFileSystemEffect interface {
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
	WriteFile(string, []byte, fs.FileMode) error
}
type nilFactoryDefinitionNamedPathFileSystem struct {
	nilFactoryDefinitionNamedPathFileSystemEffect
}

type nilFactoryDefinitionNamedFactoryCatalogFileSystemEffect factorydefinitions.NamedFactoryCatalogFileSystem
type nilFactoryDefinitionNamedFactoryCatalogFileSystem struct {
	nilFactoryDefinitionNamedFactoryCatalogFileSystemEffect
}

type nilFactoryDefinitionPackagedInstallationFileSystemEffect factorydefinitions.PackagedInstallationFileSystem
type nilFactoryDefinitionPackagedInstallationFileSystem struct {
	nilFactoryDefinitionPackagedInstallationFileSystemEffect
}

type nilFactoryDefinitionAuthoredReaderFileSystemEffect factorydefinitions.AuthoredLayoutReaderFileSystem
type nilFactoryDefinitionAuthoredReaderFileSystem struct {
	nilFactoryDefinitionAuthoredReaderFileSystemEffect
}

type nilFactoryDefinitionAuthoredWriterFileSystemEffect factorydefinitions.AuthoredLayoutWriterFileSystem
type nilFactoryDefinitionAuthoredWriterFileSystem struct {
	nilFactoryDefinitionAuthoredWriterFileSystemEffect
}

type nilFactoryDefinitionScaffoldFileSystemEffect factorydefinitions.ScaffoldFileSystem
type nilFactoryDefinitionScaffoldFileSystem struct {
	nilFactoryDefinitionScaffoldFileSystemEffect
}

type nilFactoryDefinitionScaffoldOutputEffect factorydefinitions.ScaffoldOutput
type nilFactoryDefinitionScaffoldOutput struct {
	nilFactoryDefinitionScaffoldOutputEffect
}

type nilProviderSessionFileSystemEffect interface {
	Open(string) (io.ReadCloser, error)
	Stat(string) (fs.FileInfo, error)
}
type nilProviderSessionFileSystem struct {
	nilProviderSessionFileSystemEffect
}

type nilOperatorSettingsFileSystemEffect operatorsettings.FileSystem
type nilOperatorSettingsFileSystem struct {
	nilOperatorSettingsFileSystemEffect
}

type nilClockEffect platformclock.Source
type nilClock struct{ nilClockEffect }

type nilWorkerRecordingWriterEffect recordings.WorkerRecordingWriter
type nilWorkerRecordingWriter struct{ nilWorkerRecordingWriterEffect }

type nilInvocationMetricsRecorderEffect factorysessions.InvocationMetricsRecorder
type nilInvocationMetricsRecorder struct {
	nilInvocationMetricsRecorderEffect
}

type nilFactoryVisualizationSinkEffect factoryvisualization.Sink
type nilFactoryVisualizationSink struct {
	nilFactoryVisualizationSinkEffect
}

type nilModelPullMetricsRecorderEffect interface{ RecordModelPullMetric(serviceedges.PullMetric) }
type nilModelPullMetricsRecorder struct {
	nilModelPullMetricsRecorderEffect
}

type nilProviderOverrideEffect providers.Service
type nilProviderOverride struct{ nilProviderOverrideEffect }

type nilWorkersFactoryDocsFileSystemEffect platformfilesystem.ReadFileTree
type nilWorkersFactoryDocsFileSystem struct {
	nilWorkersFactoryDocsFileSystemEffect
}

type nilWorkersExecutableLocatorEffect platformprocess.ExecutableLocator
type nilWorkersExecutableLocator struct {
	nilWorkersExecutableLocatorEffect
}

type nilWorkersExecutablePathInspectorEffect platformfilesystem.PathInspector
type nilWorkersExecutablePathInspector struct {
	nilWorkersExecutablePathInspectorEffect
}

type nilWorkersExecutableFileReaderEffect platformfilesystem.ReadOpener
type nilWorkersExecutableFileReader struct {
	nilWorkersExecutableFileReaderEffect
}

type nilWorkersInferenceMediaFileReaderEffect platformfilesystem.ReadOpener
type nilWorkersInferenceMediaFileReader struct {
	nilWorkersInferenceMediaFileReaderEffect
}

type nilWorkersWorktreeFileSystemEffect workers.WorktreeFileSystem
type nilWorkersWorktreeFileSystem struct {
	nilWorkersWorktreeFileSystemEffect
}

type nilWorkersWorktreeGitEffect workers.WorktreeGitCommander
type nilWorkersWorktreeGit struct{ nilWorkersWorktreeGitEffect }

type nilWorkersAgentToolFileSystemEffect workers.AgentToolFileSystem
type nilWorkersAgentToolFileSystem struct {
	nilWorkersAgentToolFileSystemEffect
}

type nilWorkersMockWorkersConfigFileSystemEffect workers.MockWorkersConfigFileSystem
type nilWorkersMockWorkersConfigFileSystem struct {
	nilWorkersMockWorkersConfigFileSystemEffect
}

type nilWorkersRetryRandomSourceEffect platformrandom.Source
type nilWorkersRetryRandomSource struct {
	nilWorkersRetryRandomSourceEffect
}

type nilWorkersWorkstationFileSystemEffect platformfilesystem.ReadFileInspector
type nilWorkersWorkstationFileSystem struct {
	nilWorkersWorkstationFileSystemEffect
}

type nilWorkersProviderTemporaryFileSystemEffect platformfilesystem.TemporaryFileSystem
type nilWorkersProviderTemporaryFileSystem struct {
	nilWorkersProviderTemporaryFileSystemEffect
}

type nilScriptCommandRunnerEffect platformprocess.CommandRunner
type nilScriptCommandRunner struct{ nilScriptCommandRunnerEffect }

type nilWorkContentStagingFileSystemEffect work.ContentStagingFileSystem
type nilWorkContentStagingFileSystem struct {
	nilWorkContentStagingFileSystemEffect
}

type nilWorkContentStagingRandomEffect work.ContentStagingRandom
type nilWorkContentStagingRandom struct {
	nilWorkContentStagingRandomEffect
}

type nilWorkContentStagingClockEffect work.ContentStagingClock
type nilWorkContentStagingClock struct {
	nilWorkContentStagingClockEffect
}

type nilWorkContentHTTPDoerEffect work.ContentHTTPDoer
type nilWorkContentHTTPDoer struct{ nilWorkContentHTTPDoerEffect }
