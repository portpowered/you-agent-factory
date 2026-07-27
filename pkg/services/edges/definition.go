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
	platformbrowser "github.com/portpowered/infinite-you/pkg/platform/browser"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	providercontract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

// Edges aggregates replaceable external-effect ports for process construction
// and functional overrides. Only the process-edge boundary (pkg/services/edges,
// pkg/root, pkg/wire, and BuildProcess override tests) consumes this bag;
// constructed services take exact ports instead. It is not a service locator.
type Edges struct {
	CLIObserver                                     platformprocess.CLIObserver
	PlatformProcessClock                            platformprocess.Clock
	PlatformProcessCommandFactory                   platformprocess.CommandFactory
	ProviderCommandRunner                           platformprocess.CommandRunner
	AgyPTYHost                                      platformpty.Host
	AgyPTYClock                                     platformclock.Source
	HostedHTTPClient                                automations.HostedLinearHTTPDoer
	HostedLinearEndpoint                            string
	HostedSecretResolver                            automations.HostedLinearSecretResolver
	HostedLinearCheckpointStore                     automations.HostedLinearCheckpointStore
	HostedClock                                     automations.HostedLinearClock
	ModelAssetHTTPClient                            models.AssetHTTPDoer
	ModelAssetEndpoints                             models.RuntimeAssetEndpoints
	ModelAssetHostPlatform                          models.AssetHostPlatform
	ModelAssetMakeDirectories                       models.AssetMakeDirectories
	ModelAssetInspectPath                           models.AssetInspectPath
	ModelAssetResolveHomeDirectory                  models.AssetResolveHomeDirectory
	ModelAssetWriteFile                             models.AssetWriteFile
	ModelAssetRenamePath                            models.AssetRenamePath
	ModelAssetRemovePath                            models.AssetRemovePath
	ModelAssetReadFile                              models.AssetReadFile
	ModelAssetReadDirectory                         models.AssetReadDirectory
	ModelAssetCreateFile                            models.AssetCreateFile
	ModelAssetOpenFile                              models.AssetOpenFile
	ModelHostProcessLauncher                        models.HostProcessLauncher
	ModelHostHTTPClient                             models.HostHTTPDoer
	ModelHostClock                                  models.HostClock
	ModelRuntimeCommandRunner                       platformprocess.CommandRunner
	ModelRuntimeHTTPClient                          models.RuntimeHTTPDoer
	ModelRuntimeInspectFile                         models.RuntimeInspectFile
	ModelRuntimeTempDirectory                       models.RuntimeTempDirectory
	ModelRuntimeCreateTempFile                      models.RuntimeCreateTempFile
	ModelInvocationArtifactFileSystem               models.InvocationArtifactFileSystem
	FactorySessionsWorkingDirectory                 platformfilesystem.WorkingDirectory
	FactorySessionExecutionOpeningFileSystem        factorysessions.ExecutionOpeningFileSystem
	FactorySessionDirectoryInspection               factorysessions.DirectoryInspection
	FactorySessionResolveHomeDirectory              factorysessions.HomeDirectoryResolver
	FactorySessionResolveLogicalTargetSymlinks      factorysessions.LogicalTargetResolveSymlinks
	FactorySessionIDGenerator                       factorysessions.SessionIDGenerator
	FactorySessionRuntimeInstanceIDGenerator        factorysessions.RuntimeInstanceIDGenerator
	FactorySessionResponseEventIDGenerator          factorysessions.ResponseEventIDGenerator
	FactorySessionCursorPersistenceFileSystem       factorysessions.CursorPersistenceFileSystem
	FactorySessionCursorCreateTemporaryFile         factorysessions.CursorPersistenceCreateTemporaryFile
	FactorySessionRuntimePersistenceFileSystem      factorysessions.RuntimePersistenceFileSystem
	FactorySessionContractFixtureReader             factorysessions.ContractFixtureReader
	FactorySessionInvocationInputReader             factorysessions.InvocationInputReader
	FactorySessionReplayRecordingReader             factorysessions.ReplayRecordingReader
	FactorySessionInitialWorkReader                 factorysessions.InitialWorkReader
	FactoryRuntimeIDGenerator                       factoryruntime.IDGenerator
	FactoryRuntimeDirectories                       factoryruntime.RuntimeDirectoryFileSystem
	FactoryRuntimeInputs                            factoryruntime.InputFileSystem
	FactoryRuntimeInputDirectoryWalker              factoryruntime.InputDirectoryWalker
	FactoryRuntimeWorkflowSources                   factoryruntime.WorkflowSourceFileSystem
	FactoryRuntimeWorkflowSourceResolveSymlinks     factoryruntime.WorkflowSourceResolveSymlinks
	FactoryRuntimeWorkflowHome                      factoryruntime.WorkflowHomeResolver
	FactoryDefinitionPortableFileSystem             portablefiles.FileSystem
	FactoryDefinitionLoadingFileSystem              factorydefinitions.LoadingFileSystem
	FactoryDefinitionClock                          factorydefinitions.Clock
	FactoryDefinitionVersionFileSystem              factorydefinitions.VersionFileSystem
	FactoryDefinitionPackagedGoalPromptFileSystem   factorydefinitions.PackagedGoalPromptFileSystem
	FactoryDefinitionPortableBundledFileInspection  factorydefinitions.PortableBundledFileInspection
	FactoryDefinitionRequiredToolPathLookup         factorydefinitions.RequiredToolPathLookup
	FactoryDefinitionRequiredToolVersionProbe       factorydefinitions.RequiredToolVersionProbe
	FactoryDefinitionPersistenceFileSystem          factorydefinitions.PersistenceFileSystem
	FactoryDefinitionDirectoryReplacementStore      factorydefinitions.DirectoryReplacementStore
	FactoryDefinitionNamedPathFileSystem            factorydefinitions.NamedPathFileSystem
	FactoryDefinitionNamedFactoryCatalogFileSystem  factorydefinitions.NamedFactoryCatalogFileSystem
	FactoryDefinitionPackagedInstallationFileSystem factorydefinitions.PackagedInstallationFileSystem
	FactoryDefinitionAuthoredReaderFileSystem       factorydefinitions.AuthoredLayoutReaderFileSystem
	FactoryDefinitionAuthoredWriterFileSystem       factorydefinitions.AuthoredLayoutWriterFileSystem
	FactoryDefinitionScaffoldFileSystem             factorydefinitions.ScaffoldFileSystem
	FactoryDefinitionScaffoldOutput                 factorydefinitions.ScaffoldOutput
	ProviderSessionFileSystem                       providersessions.FileSystem
	ProviderSessionResolveHomeDirectory             providersessions.ResolveHomeDirectory
	ProviderSessionCodexWalkDirectory               providersessions.CodexWalkDirectory
	ProviderSessionCodexResolveSymlinks             providersessions.CodexResolveSymlinks
	ProviderSessionCursorWalkDirectory              providersessions.CursorWalkDirectory
	ProviderSessionCursorResolveSymlinks            providersessions.CursorResolveSymlinks
	ProviderSessionCursorOpenDatabase               providersessions.CursorOpenSQLDatabase
	ProviderSessionOperatingSystem                  providersessions.OperatingSystem
	OperatorSettingsFileSystem                      operatorsettings.FileSystem
	OperatorSettingsCreateTemporaryFile             operatorsettings.CreateTemporaryFile
	OperatorSettingsIDGenerator                     operatorsettings.IDGenerator
	SystemInitializationInspectPath                 systeminitialization.InspectPath
	SystemInitializationMigrationFileSystem         systeminitialization.LegacyFactoryMigrationFileSystem

	Clock                     platformclock.Source
	SubmissionRecorder        recordings.SubmissionRecorder
	DispatchRecorder          recordings.DispatchRecorder
	RecordingMakeDirectories  recordings.RecordingMakeDirectories
	RecordingCreateTempFile   recordings.RecordingCreateTemporaryFile
	RecordingRemovePath       recordings.RecordingRemovePath
	RecordingRenamePath       recordings.RecordingRenamePath
	APIServerStarter          platformhttpserver.Starter
	BrowserOpener             platformbrowser.Opener
	InvocationMetricsRecorder factorysessions.InvocationMetricsRecorder
	RuntimeHostObserver       factorysessions.RuntimeHostObserver
	ModelPullMetricsRecorder  models.PullMetricsRecorder
	ProviderOverride          providercontract.Provider
	providercontract.ProviderRegistrations
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

	WorkContentStagingFileSystem work.ContentStagingFileSystem
	WorkContentStagingRandom     work.ContentStagingRandom
	WorkContentStagingClock      work.ContentStagingClock
	WorkContentHostPlatform      work.ContentHostPlatform
	WorkContentInspectPath       work.ContentInspectPath
	WorkContentCreateTempFile    work.ContentCreateTemporaryFile
	WorkContentRemovePath        work.ContentRemovePath
	WorkContentWriteFile         work.ContentWriteFile
	WorkContentOpenFile          work.ContentOpenFile
	WorkContentHTTPDoer          work.ContentHTTPDoer
	WorkRequestIDGenerator       work.RequestIDGenerator
	WorkSubmittedFileReader      work.SubmittedFileReader
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
	if replacements.CLIObserver != nil {
		defaults.CLIObserver = replacements.CLIObserver
	}
	if replacements.PlatformProcessClock != nil {
		defaults.PlatformProcessClock = replacements.PlatformProcessClock
	}
	if replacements.PlatformProcessCommandFactory != nil {
		defaults.PlatformProcessCommandFactory = replacements.PlatformProcessCommandFactory
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
	if replacements.ModelHostProcessLauncher != nil {
		defaults.ModelHostProcessLauncher = replacements.ModelHostProcessLauncher
	}
	if replacements.ModelHostHTTPClient != nil {
		defaults.ModelHostHTTPClient = replacements.ModelHostHTTPClient
	}
	if replacements.ModelHostClock != nil {
		defaults.ModelHostClock = replacements.ModelHostClock
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
	if replacements.SystemInitializationMigrationFileSystem != nil {
		defaults.SystemInitializationMigrationFileSystem = replacements.SystemInitializationMigrationFileSystem
	}
	if replacements.Clock != nil {
		defaults.Clock = replacements.Clock
	}
	if replacements.SubmissionRecorder != nil {
		defaults.SubmissionRecorder = replacements.SubmissionRecorder
	}
	if replacements.DispatchRecorder != nil {
		defaults.DispatchRecorder = replacements.DispatchRecorder
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
	return defaults
}
