package wire

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	"github.com/portpowered/infinite-you/pkg/initializer"
	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	runtimeapplication "github.com/portpowered/infinite-you/pkg/initializer/runtimeapplication"
	platformbrowser "github.com/portpowered/infinite-you/pkg/platform/browser"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformcontentstaging "github.com/portpowered/infinite-you/pkg/platform/contentstaging"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	platformprocessmemory "github.com/portpowered/infinite-you/pkg/platform/processmemory"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/sessionexecution"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	factorysessionmcp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryvisualizationwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/wire"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
	recordingmcp "github.com/portpowered/infinite-you/pkg/services/recordings/transports/mcp"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
	systeminitializationwire "github.com/portpowered/infinite-you/pkg/services/system_initialization/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	mcpserver "github.com/portpowered/infinite-you/pkg/transports/mcp/server"
	mcpstdio "github.com/portpowered/infinite-you/pkg/transports/mcp/stdio"
	"go.uber.org/zap"
)

func provideCurrentFactoryDirectoryResolver(
	namedPaths factorydefinitions.NamedPathResolver,
) factorydefinitions.CurrentFactoryDirectoryResolver {
	return func(rootDir string) (string, error) {
		return factorydefinitionswire.ResolveCurrent(namedPaths, rootDir)
	}
}

func provideTerminalLoggerBuilder() terminalpolicy.LoggerBuilder {
	return func(mode terminalpolicy.Mode, debug bool) (*zap.Logger, error) {
		return logging.BuildTerminalLogger(string(mode), debug)
	}
}

func provideLiveRecordingTargetPlanner(
	reserver runtimeartifact.Reserver,
) recordings.LiveRecordingTargetPlanner {
	return recordingswire.NewLiveRecordingTargetPlanner(
		platformclock.Real{},
		reserver,
		filepath.Join,
	)
}

func provideCLIRunDefaults(
	recordingTargets recordings.LiveRecordingTargetPlanner,
	recordingsCLI recordingscli.Adapter,
) runcli.RunConfig {
	return runcli.RunConfig{
		RuntimeLogConfig:            logging.DefaultRuntimeLogConfig(),
		RuntimeMetricsConfig:        platformmetrics.DefaultRuntimeMetricsConfig(),
		RecordingTargetPlanner:      recordingTargets,
		CanonicalSessionIDGenerator: uuid.NewString,
		RecordingsCLI:               recordingsCLI,
		Clock:                       platformclock.Real{},
	}
}

func provideRunDirectoryCreator() platformfilesystem.DirectoryCreator {
	return platformfilesystem.Local{}
}

func provideBrowserOpener(edges serviceedges.Edges) platformbrowser.Opener {
	return provideBrowserOpenerWith(
		edges,
		os.LookupEnv,
		func() platformbrowser.Opener {
			return platformbrowser.NewHost(runtime.GOOS).Open
		},
	)
}

const browserOpenOptOutEnvironment = "YOU_NO_BROWSER_OPEN"

func provideBrowserOpenerWith(
	edges serviceedges.Edges,
	lookupEnv func(string) (string, bool),
	hostFactory func() platformbrowser.Opener,
) platformbrowser.Opener {
	if edges.BrowserOpener != nil {
		return edges.BrowserOpener
	}
	if value, ok := lookupEnv(browserOpenOptOutEnvironment); ok && value == "1" {
		return func(context.Context, string) error { return nil }
	}
	return hostFactory()
}

func provideFactorySessionsWorkingDirectory(
	edges serviceedges.Edges,
) platformfilesystem.WorkingDirectory {
	if edges.FactorySessionsWorkingDirectory != nil {
		return edges.FactorySessionsWorkingDirectory
	}
	return platformfilesystem.Local{}
}

func provideFactorySessionExecutionOpeningFileSystem(
	edges serviceedges.Edges,
) factorysessionwire.ExecutionOpeningFileSystem {
	if edges.FactorySessionExecutionOpeningFileSystem != nil {
		return edges.FactorySessionExecutionOpeningFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactorySessionDirectoryInspection(
	edges serviceedges.Edges,
) factorysessionwire.DirectoryInspection {
	if edges.FactorySessionDirectoryInspection != nil {
		return edges.FactorySessionDirectoryInspection
	}
	return platformfilesystem.Local{}
}

func provideFactorySessionContractFixtureReader(edges serviceedges.Edges) factorysessionwire.ContractFixtureReader {
	if edges.FactorySessionContractFixtureReader != nil {
		return edges.FactorySessionContractFixtureReader
	}
	return factorysessionwire.ContractFixtureReader(platformfilesystem.Local{}.ReadFile)
}

func provideFactorySessionInvocationInputReader(edges serviceedges.Edges) factorysessionwire.InvocationInputReader {
	if edges.FactorySessionInvocationInputReader != nil {
		return edges.FactorySessionInvocationInputReader
	}
	return factorysessionwire.InvocationInputReader(platformfilesystem.Local{}.ReadFile)
}

func provideFactorySessionReplayRecordingReader(edges serviceedges.Edges) factorysessionwire.ReplayRecordingReader {
	if edges.FactorySessionReplayRecordingReader != nil {
		return edges.FactorySessionReplayRecordingReader
	}
	return factorysessionwire.ReplayRecordingReader(platformfilesystem.Local{}.ReadFile)
}

func provideFactorySessionInitialWorkReader(edges serviceedges.Edges) factorysessionwire.InitialWorkReader {
	if edges.FactorySessionInitialWorkReader != nil {
		return edges.FactorySessionInitialWorkReader
	}
	return factorysessionwire.InitialWorkReader(platformfilesystem.Local{}.ReadFile)
}

func provideFactorySessionResolveHomeDirectory(
	edges serviceedges.Edges,
) factorysessions.HomeDirectoryResolver {
	if edges.FactorySessionResolveHomeDirectory != nil {
		return edges.FactorySessionResolveHomeDirectory
	}
	return os.UserHomeDir
}

func provideFactorySessionResolveLogicalTargetSymlinks(
	edges serviceedges.Edges,
) factorysessions.LogicalTargetResolveSymlinks {
	if edges.FactorySessionResolveLogicalTargetSymlinks != nil {
		return edges.FactorySessionResolveLogicalTargetSymlinks
	}
	return filepath.EvalSymlinks
}

func provideFactorySessionCursorPersistenceFileSystem(
	edges serviceedges.Edges,
) factorysessionwire.CursorPersistenceFileSystem {
	if edges.FactorySessionCursorPersistenceFileSystem != nil {
		return edges.FactorySessionCursorPersistenceFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactorySessionCursorCreateTemporaryFile(
	edges serviceedges.Edges,
) factorysessionwire.CursorPersistenceCreateTemporaryFile {
	if edges.FactorySessionCursorCreateTemporaryFile != nil {
		return edges.FactorySessionCursorCreateTemporaryFile
	}
	return func(dir, pattern string) (factorysessionwire.CursorPersistenceTemporaryFile, error) {
		return os.CreateTemp(dir, pattern)
	}
}

func provideFactorySessionCursorStoreFactory(
	files factorysessionwire.CursorPersistenceFileSystem,
	createTemporaryFile factorysessionwire.CursorPersistenceCreateTemporaryFile,
) factorysessionwire.CursorStoreFactory {
	return func(dir string) (factorysessions.CursorStore, error) {
		return factorysessionwire.NewCursorFileStore(dir, files, createTemporaryFile)
	}
}

func provideFactorySessionRuntimePersistenceFileSystem(
	edges serviceedges.Edges,
) factorysessionwire.RuntimePersistenceFileSystem {
	if edges.FactorySessionRuntimePersistenceFileSystem != nil {
		return edges.FactorySessionRuntimePersistenceFileSystem
	}
	return runtimePersistenceFileSystem{
		directories: platformfilesystem.Local{},
		storage:     platformreplay.NewLocal(runtime.GOOS),
	}
}

// runtimePersistenceFileSystem preserves the established Sessions directory
// lifecycle effect while routing snapshot bytes through replay.Storage's
// temp-write, sync, close, and replace implementation.
type runtimePersistenceFileSystem struct {
	directories platformfilesystem.DirectoryCreator
	storage     platformreplay.Storage
}

func (files runtimePersistenceFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	return files.directories.MkdirAll(path, mode)
}

func (files runtimePersistenceFileSystem) ReadFile(path string) ([]byte, error) {
	return files.storage.ReadFile(path)
}

func (files runtimePersistenceFileSystem) WriteFile(path string, data []byte, _ fs.FileMode) error {
	return files.storage.WriteFile(path, data)
}

func provideFactorySessionRuntimePersistenceStoreFactory(
	files factorysessionwire.RuntimePersistenceFileSystem,
) factorysessionwire.RuntimePersistenceStoreFactory {
	return func(projectRoot string) (factorysessionwire.RuntimePersistenceStore, error) {
		return factorysessionwire.NewRuntimeProjectStore(projectRoot, files)
	}
}

func provideOperatorSettingsFileSystem(edges serviceedges.Edges) operatorsettings.FileSystem {
	if edges.OperatorSettingsFileSystem != nil {
		return edges.OperatorSettingsFileSystem
	}
	return platformfilesystem.Local{}
}

func provideOperatorSettingsCreateTemporaryFile(edges serviceedges.Edges) operatorsettings.CreateTemporaryFile {
	if edges.OperatorSettingsCreateTemporaryFile != nil {
		return edges.OperatorSettingsCreateTemporaryFile
	}
	return func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
		return os.CreateTemp(dir, pattern)
	}
}

func provideOperatorSettingsProviderCatalog(
	providersService providers.Service,
) operatorsettings.ProviderCatalog {
	return func(value string) (string, bool) {
		if providersService == nil {
			return "", false
		}
		resolved, err := providersService.ResolveIdentity(
			context.Background(),
			providers.ResolveIdentityRequest{Identity: value},
		)
		if err != nil {
			return "", false
		}
		return resolved.ID.String(), true
	}
}

// provideOperatorSettingsLogger converts the canonical process-wide zap
// logger into the logging.Logger abstraction accepted by the Operator
// Settings service, so ResolveACPAgentProfile/UpdateACPAgentProfile emit
// operation logs through the same logger the rest of the process uses.
func provideOperatorSettingsLogger(logger *zap.Logger) logging.Logger {
	return logging.NewZapLogger(logger, false)
}

func provideOperatorSettingsService(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	providerCatalog operatorsettings.ProviderCatalog,
	decode operatorsettings.ConfigDecoder,
	diagnosticDecode operatorsettings.ConfigDiagnosticsDecoder,
	encode operatorsettings.ConfigEncoder,
	idGenerator operatorsettings.IDGenerator,
	providersRoot providers.Service,
	logger logging.Logger,
) (operatorsettings.Service, error) {
	return settingswire.NewServiceFromConfigDocument(
		operatorsettings.ConfigDocumentService{
			Files:                 files,
			CreateTemp:            createTemp,
			Providers:             providerCatalog,
			Decoder:               decode,
			DiagnosticDecoder:     diagnosticDecode,
			Encoder:               encode,
			PreserveUnknownFields: globalconfigmapping.PreserveUnknownFields,
		},
		providersRoot,
		idGenerator,
		logger,
	)
}

func provideOperatorSettingsIDGenerator(edges serviceedges.Edges) operatorsettings.IDGenerator {
	if edges.OperatorSettingsIDGenerator != nil {
		return edges.OperatorSettingsIDGenerator
	}
	return uuid.NewString
}

func provideSystemInitializationInspectPath(
	edges serviceedges.Edges,
) systeminitializationwire.InspectPath {
	if edges.SystemInitializationInspectPath != nil {
		return edges.SystemInitializationInspectPath
	}
	return os.Stat
}

func provideOperatorConfigLoader(settings operatorsettings.Service) operatorsettings.ConfigLoader {
	return func(path string) (operatorsettings.Config, error) {
		return settings.LoadFileConfig(path)
	}
}

func provideOperatorBackendScopeEnsurer(settings operatorsettings.Service) operatorsettings.BackendScopeEnsurer {
	return func(path string) (operatorsettings.ResolvedBackendScope, error) {
		return settings.EnsureLocalBackendScope(path)
	}
}

func provideModelInvocationArtifactExporter(
	edges serviceedges.Edges,
) (modelswire.InvocationArtifactExporter, error) {
	filesystem := edges.ModelInvocationArtifactFileSystem
	if filesystem == nil {
		filesystem = platformfilesystem.Local{}
	}
	return modelswire.NewInvocationArtifactExporter(filesystem)
}

func provideModelInvocationTimeout() factorysessions.ModelInvocationTimeout {
	return factorysessions.DefaultModelInvocationTimeout
}

func provideModelInvocationOperation(
	operation factorysessionwire.InvocationOperation,
) factorysessionwire.ModelInvocationOperation {
	return operation
}

func provideSystemInitializationService(
	persistence factorydefinitions.PackagedFactoryPersistence,
	packagedInstallationFileSystem factorydefinitions.PackagedInstallationFileSystem,
	packagedInstallationDirectoryCreator factorydefinitions.PackagedInstallationDirectoryCreator,
	packagedCatalog factorydefinitions.PackagedFactoryCatalogOperations,
	loadOperatorConfig operatorsettings.ConfigLoader,
	ensureOperatorBackendScope operatorsettings.BackendScopeEnsurer,
	inspectPath systeminitializationwire.InspectPath,
	logger logging.Logger,
) (systeminitialization.Service, error) {
	return systeminitializationwire.NewService(
		systeminitializationwire.OperatorSettingsFunctions{
			Load:   loadOperatorConfig,
			Ensure: ensureOperatorBackendScope,
		},
		packagedCatalog,
		factorydefinitionswire.NewPackagedFactoryInstaller(
			persistence,
			packagedInstallationFileSystem,
			packagedInstallationDirectoryCreator,
			logger,
		),
		inspectPath,
	)
}

func provideSystemInitializationOperation(
	service systeminitialization.Service,
) initializerapplication.SystemInitializationOperation {
	return func(ctx context.Context, homeDir string) error {
		_, err := service.Initialize(ctx, systeminitialization.Request{HomeDir: homeDir})
		return err
	}
}

func providePackagedFactoryDefinitions() ([]factorydefinitions.PackagedDefinition, error) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		return nil, err
	}
	return catalog.All(), nil
}

func providePackagedFactoryCatalog(
	definitions []factorydefinitions.PackagedDefinition,
) (factorydefinitions.PackagedFactoryCatalogOperations, error) {
	return factorydefinitionswire.NewPackagedFactoryCatalog(definitions)
}

func providePackagedFactoryInstallation(
	persistence factorydefinitions.PackagedFactoryPersistence,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
	directoryCreator factorydefinitions.PackagedInstallationDirectoryCreator,
	logger logging.Logger,
) factorydefinitions.PackagedFactoryInstallationOperations {
	installer := factorydefinitionswire.NewPackagedFactoryInstallationService(persistence, fileSystem, directoryCreator, logger)
	return factorydefinitions.PackagedFactoryInstallationOperations{
		Install: installer.InstallPackagedFactory,
	}
}

func provideDurableExecutionFactory(loadOperatorConfig operatorsettings.ConfigLoader) factorysessionwire.DurableExecutionFactory {
	return func(
		definition factorydefinitions.RuntimeOpeningRequest,
		session factorysessions.SessionRuntimeOpeningRequest,
		defaults operatorsettings.ResolvedDefaults,
		root factorysessionwire.RuntimeRoot,
		clock factoryruntime.Clock,
		provider providers.Service,
		mockWorkersConfig *workers.MockWorkersConfig,
		factory factorysessionwire.FactorySessionExecutionFactory,
		providerIdentities factorysessions.ProviderIdentityResolver,
	) (factorysessionwire.DurableExecution, error) {
		return factorysessionwire.NewDurableExecutionRuntime(
			loadOperatorConfig,
			definition,
			session,
			defaults,
			root,
			clock,
			provider,
			mockWorkersConfig,
			factory,
			providerIdentities,
		)
	}
}

func provideFactoryRuntimeClockResolver() factoryruntime.ClockResolver {
	return func(clock factoryruntime.Clock) factoryruntime.Clock {
		if clock != nil {
			return clock
		}
		return platformclock.Real{}
	}
}

func provideFactoryRuntimeMetricsClock(edges serviceedges.Edges) platformclock.TimerSource {
	if clock, ok := edges.Clock.(platformclock.TimerSource); ok {
		return clock
	}
	return platformclock.Real{}
}

func providePprofCommandLineReader() platformhttpserver.CommandLineReader {
	return func() []string {
		return append([]string(nil), os.Args...)
	}
}

func provideFactoryRuntimeSessionLoggerFactory() factoryruntime.SessionLoggerFactory {
	return factoryruntime.NewSessionLogger
}

func provideAPIServerStarter(
	edges serviceedges.Edges,
	commandLineReader platformhttpserver.CommandLineReader,
) (platformhttpserver.Starter, error) {
	if edges.APIServerStarter != nil {
		return edges.APIServerStarter, nil
	}
	return platformhttpserver.NewStarter(net.Listen, platformprocessmemory.CurrentCommit, commandLineReader)
}

func provideRuntimeHostOperation(
	starter platformhttpserver.Starter,
) factorysessionwire.RuntimeHostOperation {
	return factorysessionwire.NewRuntimeHostService(starter)
}

func provideProcessRuntimeFactory(
	host factorysessionwire.RuntimeHostOperation,
) (factorysessionwire.ProcessRuntimeFactory, error) {
	return factorysessionwire.NewProcessLifecycleFactory(host)
}

func provideFactoryVisualizationFactory() factoryvisualization.RuntimeFactory {
	return func(
		reader factoryvisualization.RuntimeReader,
		projections recordings.ProjectionService,
		clock factoryvisualization.Clock,
		sink factoryvisualization.Sink,
		reportError factoryvisualization.ErrorReporter,
	) (factoryvisualization.Service, error) {
		return factoryvisualizationwire.NewRoot(
			factoryvisualizationwire.NewCurrentRuntimeSource(reader),
			projections,
			clock,
			sink,
			reportError,
		)
	}
}

func provideWorkContentStagingService(
	edges serviceedges.Edges,
) (work.ContentStagingService, error) {
	filesystem := edges.WorkContentStagingFileSystem
	if filesystem == nil {
		filesystem = platformcontentstaging.FileSystem{}
	}
	random := edges.WorkContentStagingRandom
	if random == nil {
		random = platformcontentstaging.Random{}
	}
	clock := edges.WorkContentStagingClock
	if clock == nil {
		clock = platformclock.Real{}
	}
	return workwire.NewContentStagingService(filesystem, random, clock, 0)
}

func provideApplicationRuntimeAdapter(
	edges serviceedges.Edges,
	visualizationFactory factoryvisualization.RuntimeFactory,
	visualizationSinks factoryvisualization.RuntimeSinkOwner,
	httpBinding httpRuntimeBinding,
	newRunner lifecycle.RunnerFactory,
) (factorysessionwire.RuntimeAdapter, error) {
	if visualizationFactory == nil || visualizationSinks == nil || httpBinding == nil || newRunner == nil {
		return nil, errors.New("Factory visualization, HTTP binding, and lifecycle component operations are required")
	}
	fixedSink := edges.FactoryVisualizationSink
	fixedRootObserver := edges.FactoryVisualizationRootObserver
	return func(
		opened factorysessionwire.OpenedApplicationRuntime,
		sinkID factorysessions.VisualizationSinkID,
	) (factorysessions.BoundProcessComponents, error) {
		sink, err := selectVisualizationSink(visualizationSinks, sinkID)
		if err != nil {
			return factorysessions.BoundProcessComponents{}, err
		}
		if fixedSink != nil {
			sink = fixedSink
		}
		var visualization lifecycle.Component
		if sink != nil {
			logger := opened.Resources.Logger
			if logger == nil {
				logger = zap.NewNop()
			}
			visualized, err := visualizationFactory(
				opened.Visualization.Reader, opened.Visualization.Projections, opened.Resources.Clock, sink,
				func(err error) {
					logger.Error("Factory visualization failed", zap.Error(err))
				},
			)
			if err != nil {
				return factorysessions.BoundProcessComponents{}, err
			}
			if fixedRootObserver != nil {
				fixedRootObserver(visualized)
			}
			// Factory Session lifecycle must not auto-activate Visualization.
			// Peers leave the composed root inert until explicit Activate.
			visualization = lifecycle.Functions{
				StartFunc: func(context.Context) error { return nil },
				StopFunc: func(ctx context.Context) error {
					_, err := visualized.StopDrain(ctx, factoryvisualization.StopDrainRequest{})
					return err
				},
			}
		}
		handler, err := httpBinding(opened)
		if err != nil {
			return factorysessions.BoundProcessComponents{}, err
		}
		transport := newRunner(func(ctx context.Context) error {
			return opened.Process.RunTransport(ctx, handler)
		})
		return factorysessions.BoundProcessComponents{
			Transport:     transport,
			Visualization: visualization,
		}, nil
	}, nil
}

// selectVisualizationSink resolves the transport-selected visualization sink
// the opening request carries. Factory Sessions carries only the opaque
// selection, so the composition root that owns the sink registry is the only
// place that can turn it back into a presentation sink.
func selectVisualizationSink(
	sinks factoryvisualization.RuntimeSinkOwner,
	sinkID factorysessions.VisualizationSinkID,
) (factoryvisualization.Sink, error) {
	if sinkID == "" {
		return nil, nil
	}
	sink, ok := sinks.RuntimeSink(factoryvisualization.RuntimeSinkID(sinkID))
	if !ok {
		return nil, fmt.Errorf("Factory Visualization sink %q is unavailable", sinkID)
	}
	return sink, nil
}

func provideManagedRunnerFactory() runtimeapplication.ManagedRunnerFactory {
	return runtimeapplication.NewManagedRunner
}

type mcpServerBuilder func(
	factorysessionwire.DurableExecutionService,
	recordings.Service,
	factorysessionwire.RequestPreparation,
	factoryruntime.WorkflowPreviewOperation,
) (*mcpserver.Server, error)

// provideMCPServerBuilder composes owner adapters at the Wire boundary. The
// protocol stdio package receives only the resulting inert server and caller
// streams; it does not construct Factory Sessions, Recordings, or workflow
// services while an opening is being selected.
func provideMCPServerBuilder() mcpServerBuilder {
	return func(
		execution factorysessionwire.DurableExecutionService,
		recordingsService recordings.Service,
		prepare factorysessionwire.RequestPreparation,
		workflowPreview factoryruntime.WorkflowPreviewOperation,
	) (*mcpserver.Server, error) {
		inspection := factorysessionmcp.RecordingsInspection(recordingsService)
		if inspection == nil {
			if bridge := factorysessionmapping.NewDurableInspectionBridge(execution); bridge != nil {
				inspection = recordingmcp.NewLegacyFactorySessionInspection(bridge)
			}
		}
		return mcpserver.New(mcpserver.Options{
			ToolOperation: mcpserver.ToolOperation(factorysessionmcp.BindToolOperation(
				execution, inspection, prepare, workflowPreview,
			)),
		})
	}
}

func provideFixtureStdioApplicationBuilder(
	build initializerapplication.StdioRunnerBuilder,
	newRunner lifecycle.RunnerFactory,
	open mcpstdio.Opener,
	buildServer mcpServerBuilder,
	prepare factorysessionwire.RequestPreparation,
	workflowPreview factoryruntime.WorkflowPreviewOperation,
) factorysessionwire.FixtureStdioApplicationBuilder {
	return func(
		ctx context.Context,
		execution factorysessionwire.DurableExecutionService,
		input io.Reader,
		output io.Writer,
	) (factorysessionwire.StdioApplication, error) {
		openSession := initializer.StdioSessionOpener(func(
			sessionCtx context.Context,
			sessionInput io.Reader,
			sessionOutput io.Writer,
		) (initializer.OpenedApplication, error) {
			if sessionCtx == nil {
				return initializer.OpenedApplication{}, errors.New("MCP stdio context is required")
			}
			if err := sessionCtx.Err(); err != nil {
				return initializer.OpenedApplication{}, err
			}
			server, err := buildServer(execution, nil, prepare, workflowPreview)
			if err != nil {
				return initializer.OpenedApplication{}, err
			}
			session, err := open(server, sessionInput, sessionOutput)
			if err != nil {
				return initializer.OpenedApplication{}, err
			}
			return stdioLifecycleOpening(newRunner(session.Run), runtimeartifact.Diagnostics{}, nil), nil
		})
		return build(ctx, openSession, input, output)
	}
}

func provideRuntimeStdioApplicationBuilder(
	build initializerapplication.OpenedStdioRunnerBuilder,
	newRunner lifecycle.RunnerFactory,
	open mcpstdio.Opener,
	buildServer mcpServerBuilder,
	prepare factorysessionwire.RequestPreparation,
) factorysessionwire.RuntimeStdioApplicationBuilder {
	return func(
		ctx context.Context,
		opened factorysessionwire.OpenedExecutionRuntime,
		input io.Reader,
		output io.Writer,
	) (factorysessionwire.StdioApplication, error) {
		neutral := initializer.OpenedStdioApplication{
			OpenSession: func(
				sessionCtx context.Context,
				sessionInput io.Reader,
				sessionOutput io.Writer,
			) (initializer.OpenedApplication, error) {
				if sessionCtx == nil {
					return initializer.OpenedApplication{}, errors.New("MCP stdio context is required")
				}
				if err := sessionCtx.Err(); err != nil {
					return initializer.OpenedApplication{}, err
				}
				server, err := buildServer(opened.Execution, opened.Recordings, prepare, opened.WorkflowPreview)
				if err != nil {
					return initializer.OpenedApplication{}, err
				}
				session, err := open(server, sessionInput, sessionOutput)
				if err != nil {
					return initializer.OpenedApplication{}, err
				}
				return stdioLifecycleOpening(
					newRunner(session.Run), opened.Resources.Diagnostics, opened.Resources.Close,
				), nil
			},
		}
		return build(ctx, neutral, input, output)
	}
}

func stdioLifecycleOpening(
	transport lifecycle.Component,
	diagnostics runtimeartifact.Diagnostics,
	close func() error,
) initializer.OpenedApplication {
	plan := lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: "stdio transport", Component: transport, Primary: true,
	}}}
	if close != nil {
		plan.Resources = []lifecycle.NamedResource{{
			Name: "runtime application", Resource: lifecycle.CloserFunc(close),
		}}
	}
	return initializer.OpenedApplication{Plan: plan, Diagnostics: diagnostics}
}

type stdioApplicationOpener struct {
	open          factorysessionwire.StdioOpeningOperation
	presentations factorysessions.OpeningPresentationOwner
}

func (adapter stdioApplicationOpener) OpenStdio(
	ctx context.Context,
	intent processcontract.MCPIntent,
) (initializer.RunApplication, error) {
	request := factorysessions.StdioOpeningRequest{
		FixtureCatalogPath: intent.FixtureCatalogPath,
		RuntimeBacked:      intent.RuntimeBacked,
		ProjectRoot:        intent.ProjectRoot,
		SystemConfigHome:   intent.HomeDir,
	}
	var scopeID factorysessions.OpeningScopeID
	var err error
	if adapter.presentations != nil {
		scopeID, err = adapter.presentations.RegisterStdio(factorysessions.StdioOpeningScope{
			Input: intent.Stdin, Output: intent.Stdout,
		})
		if err != nil {
			return nil, fmt.Errorf("register stdio opening presentation: %w", err)
		}
		request.ScopeID = scopeID
	}
	application, err := adapter.open.OpenStdio(
		ctx,
		request,
	)
	if err != nil {
		if adapter.presentations != nil {
			adapter.presentations.Close(scopeID)
		}
		return nil, err
	}
	if application == nil {
		if adapter.presentations != nil {
			adapter.presentations.Close(scopeID)
		}
		return nil, errors.New("stdio opening returned nil application")
	}
	if adapter.presentations == nil {
		return application, nil
	}
	return scopedRunApplication{
		application: application,
		close:       func() { adapter.presentations.Close(scopeID) },
	}, nil
}

type scopedRunApplication struct {
	application initializer.RunApplication
	close       func()
}

func (application scopedRunApplication) Run(ctx context.Context) error {
	defer application.close()
	return application.application.Run(ctx)
}

func provideStdioApplicationOpener(
	open factorysessionwire.StdioOpeningOperation,
	presentations factorysessions.OpeningPresentationOwner,
) (processcontract.StdioApplicationOpener, error) {
	if open == nil {
		return nil, errors.New("Factory Session stdio opening operation is required")
	}
	return stdioApplicationOpener{open: open, presentations: presentations}, nil
}

func provideLifecycleRunnerFactory() lifecycle.RunnerFactory {
	return lifecycle.NewRunner
}

func provideRunInvocationOperation(
	invocation factorysessionwire.InvocationOperation,
) runcli.InvocationOperation {
	return invocation
}

func provideRunSelectionFactory(
	open runcli.Opener,
	buildRunner runcli.RuntimeRunnerBuilder,
	invocation factorysessionwire.InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
	directJavaScript factorysessionwire.DirectJavaScriptRunOperation,
	buildApplication initializer.RuntimeRunnerBuilder,
	presentations factorysessions.OpeningPresentationOwner,
) (runcli.SelectionFactory, error) {
	return runcli.NewSelectionFactory(
		open, buildRunner, invocation, presentation, directJavaScript, buildApplication,
		presentations,
	)
}

func provideFactorySessionHTTPRequestPreparation(
	prepare factorysessionwire.RequestPreparation,
) factorysessionshttp.RequestPreparation {
	return prepare
}

func provideWorkStopSummaryProjector() factorysessions.WorkStopSummaryProjector {
	return factorysessionwire.NewWorkStopSummaryProjector()
}

func provideResponsePresentation() factoryvisualization.ResponsePresentation {
	return factoryvisualizationwire.NewResponsePresentation()
}

func provideDirectJavaScriptSyncRunner() factorysessionwire.DirectJavaScriptSyncRunner {
	return sessionexecutioncli.RunNormalizedSync
}
