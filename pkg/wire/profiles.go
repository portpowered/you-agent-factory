package wire

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

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
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	platformruntimeartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/sessionexecution"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
	systeminitializationwire "github.com/portpowered/infinite-you/pkg/services/system_initialization/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
	httpapplication "github.com/portpowered/infinite-you/pkg/transports/http/application"
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

func provideLiveRecordingTargetPlanner() recordings.LiveRecordingTargetPlanner {
	return recordingswire.NewLiveRecordingTargetPlanner(
		platformclock.Real{},
		uuid.NewString,
		filepath.Join,
	)
}

func provideCLIRunDefaults(
	recordingTargets recordings.LiveRecordingTargetPlanner,
	recordingsCLI recordingscli.Adapter,
) runcli.RunConfig {
	return runcli.RunConfig{
		RuntimeLogConfig:       logging.DefaultRuntimeLogConfig(),
		RuntimeMetricsConfig:   platformmetrics.DefaultRuntimeMetricsConfig(),
		RecordingTargetPlanner: recordingTargets,
		RecordingsCLI:          recordingsCLI,
		Clock:                  platformclock.Real{},
	}
}

func provideRunDirectoryCreator() platformfilesystem.DirectoryCreator {
	return platformfilesystem.Local{}
}

func provideBrowserOpener(edges serviceedges.Edges) platformbrowser.Opener {
	if edges.BrowserOpener != nil {
		return edges.BrowserOpener
	}
	return platformbrowser.NewHost(runtime.GOOS).Open
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
	return platformfilesystem.Local{}
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
	providers workers.ProviderRegistry,
) operatorsettings.ProviderCatalog {
	return func(value string) (string, bool) {
		canonical, err := providers.CanonicalIdentity(value)
		return canonical, err == nil
	}
}

func provideOperatorConfigDocumentService(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	providers operatorsettings.ProviderCatalog,
	decode operatorsettings.ConfigDecoder,
	encode operatorsettings.ConfigEncoder,
) operatorsettings.ConfigDocumentService {
	return settingswire.NewConfigDocumentService(
		files,
		createTemp,
		decode,
		encode,
		providers,
		&sync.Mutex{},
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
) systeminitialization.InspectPath {
	if edges.SystemInitializationInspectPath != nil {
		return edges.SystemInitializationInspectPath
	}
	return os.Stat
}

func provideSystemInitializationLegacyFactoryMigrationFileSystem(
	edges serviceedges.Edges,
) systeminitialization.LegacyFactoryMigrationFileSystem {
	if edges.SystemInitializationMigrationFileSystem != nil {
		return edges.SystemInitializationMigrationFileSystem
	}
	return platformfilesystem.Local{}
}

func provideOperatorConfigLoader(files operatorsettings.FileSystem, decode operatorsettings.ConfigDecoder) operatorsettings.ConfigLoader {
	return func(path string) (operatorsettings.Config, error) {
		return operatorsettings.LoadFileConfig(files, decode, path)
	}
}

func provideOperatorBackendScopeEnsurer(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	generateID operatorsettings.IDGenerator,
	decode operatorsettings.ConfigDecoder,
	encode operatorsettings.ConfigEncoder,
) operatorsettings.BackendScopeEnsurer {
	return func(path string) (operatorsettings.ResolvedBackendScope, error) {
		return operatorsettings.EnsureLocalBackendScope(files, createTemp, generateID, decode, encode, path)
	}
}

func provideModelInvocationArtifactExporter(
	edges serviceedges.Edges,
) (models.InvocationArtifactExporter, error) {
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
	persistence factorydefinitions.Persistence,
	packagedInstallationFileSystem factorydefinitions.PackagedInstallationFileSystem,
	packagedCatalog factorydefinitions.PackagedFactoryCatalogOperations,
	loadOperatorConfig operatorsettings.ConfigLoader,
	ensureOperatorBackendScope operatorsettings.BackendScopeEnsurer,
	inspectPath systeminitialization.InspectPath,
	migrationFiles systeminitialization.LegacyFactoryMigrationFileSystem,
) (systeminitialization.Service, error) {
	return systeminitializationwire.NewService(
		systeminitialization.OperatorSettingsFunctions{
			Load:   loadOperatorConfig,
			Ensure: ensureOperatorBackendScope,
		},
		packagedCatalog,
		factorydefinitionswire.NewPackagedFactoryInstaller(persistence, packagedInstallationFileSystem),
		inspectPath,
		migrationFiles,
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
	persistence factorydefinitions.Persistence,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
) factorydefinitions.PackagedFactoryInstallationOperations {
	installer := factorydefinitionswire.NewPackagedFactoryInstallationService(persistence, fileSystem)
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
		provider workers.Provider,
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

func provideWorkerExecutionFactory() factorysessionwire.WorkerExecutionFactory {
	return factorysessionwire.NewWorkerExecutionRuntime
}

func provideFactoryRuntimeClockResolver() factoryruntime.ClockResolver {
	return func(clock factoryruntime.Clock) factoryruntime.Clock {
		if clock != nil {
			return clock
		}
		return platformclock.Real{}
	}
}

func provideFactoryRuntimeSessionLoggerFactory() factoryruntime.SessionLoggerFactory {
	return factoryruntime.NewSessionLogger
}

func provideAPIServerStarter(edges serviceedges.Edges) (platformhttpserver.Starter, error) {
	if edges.APIServerStarter != nil {
		return edges.APIServerStarter, nil
	}
	return platformhttpserver.NewStarter(net.Listen)
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
	) (*factoryvisualization.Service, error) {
		return factoryvisualization.New(
			factoryvisualization.NewCurrentRuntimeSource(reader),
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

type runtimeArtifactClock func() time.Time
type runtimeArtifactIDGenerator func() string

func provideRuntimeArtifactClock() runtimeArtifactClock             { return time.Now }
func provideRuntimeArtifactIDGenerator() runtimeArtifactIDGenerator { return uuid.NewString }

func provideRuntimeLoggerFactory() factoryruntime.RuntimeLoggerFactory {
	return func(logger *zap.Logger, verbose bool) factoryruntime.Logger {
		return logging.NewZapLogger(logger, verbose)
	}
}

func provideRuntimeArtifactRootResolver() factoryruntime.RuntimeArtifactRootResolver {
	return func(home string) factoryruntime.RuntimeArtifactRoots {
		if strings.TrimSpace(home) == "" {
			return factoryruntime.RuntimeArtifactRoots{}
		}
		return factoryruntime.RuntimeArtifactRoots{
			Logs: logging.RuntimeLogsRoot(home), Metrics: platformmetrics.RuntimeMetricsRoot(home),
		}
	}
}

func provideRuntimeArtifactPathReserver() (platformruntimeartifact.Reserver, error) {
	return platformruntimeartifact.NewReserver(platformfilesystem.Local{})
}

func provideRuntimeLogSinkFactory(
	clock runtimeArtifactClock,
	newID runtimeArtifactIDGenerator,
	paths platformruntimeartifact.Reserver,
) (factoryruntime.RuntimeLogSinkFactory, error) {
	opener, err := logging.NewRuntimeLogOpener(paths)
	if err != nil {
		return nil, err
	}
	return func(
		base *zap.Logger,
		runtimeInstanceID string,
		rootDir string,
		config factoryruntime.RuntimeLogStorageConfig,
	) (factoryruntime.RuntimeLogSink, error) {
		opened, err := opener.Open(logging.RuntimeLogOpeningRequest{
			BaseLogger: base, RuntimeInstanceID: runtimeInstanceID,
			RootDirectory: rootDir, StartTimeUTC: clock(), CollisionID: newID(),
			Config: logging.RuntimeLogConfig{
				MaxSize: config.MaxSize, MaxBackups: config.MaxBackups,
				MaxAge: config.MaxAge, Compress: config.Compress,
			},
		})
		if err != nil {
			return nil, err
		}
		return runtimeLogSinkAdapter{sink: opened}, nil
	}, nil
}

type runtimeLogSinkAdapter struct{ sink *logging.RuntimeLogSink }

func (adapter runtimeLogSinkAdapter) Logger() *zap.Logger { return adapter.sink.Logger() }
func (adapter runtimeLogSinkAdapter) Close() error        { return adapter.sink.Close() }
func (adapter runtimeLogSinkAdapter) Artifact() factoryruntime.RuntimeLogArtifact {
	artifact := adapter.sink.Artifact()
	return factoryruntime.RuntimeLogArtifact{
		Path: artifact.Path, RootDir: artifact.RootDir, StartTimeUTC: artifact.StartTimeUTC,
		Config: factoryruntime.RuntimeLogStorageConfig{
			MaxSize: artifact.Config.MaxSize, MaxBackups: artifact.Config.MaxBackups,
			MaxAge: artifact.Config.MaxAge, Compress: artifact.Config.Compress,
		},
	}
}

func provideRuntimeMetricsSinkFactory(
	clock runtimeArtifactClock,
	newID runtimeArtifactIDGenerator,
	paths platformruntimeartifact.Reserver,
) (factoryruntime.RuntimeMetricsSinkFactory, error) {
	opener, err := platformmetrics.NewRuntimeMetricsOpener(paths)
	if err != nil {
		return nil, err
	}
	return func(
		scope factoryruntime.RuntimeMetricsScope,
		rootDir string,
		config factoryruntime.RuntimeMetricsStorageConfig,
	) (factoryruntime.RuntimeMetricsSink, error) {
		writer, err := opener.Open(platformmetrics.RuntimeMetricsOpeningRequest{
			SessionID: scope.SessionID, RuntimeInstanceID: scope.RuntimeInstanceID,
			FolderPath: scope.FolderPath, FactoryDirectory: scope.FactoryDir,
			RootDirectory: rootDir, StartTimeUTC: clock(), CollisionID: newID(),
			Config: platformmetrics.RuntimeMetricsConfig{
				MaxSize: config.MaxSize, MaxBackups: config.MaxBackups,
				MaxAge: config.MaxAge, Compress: config.Compress,
			},
		})
		if err != nil {
			return nil, err
		}
		return factoryruntime.NewRuntimeMetricsSink(
			runtimeMetricRecordWriterAdapter{writer: writer},
			scope,
			clock,
			factoryruntime.RuntimeMetricsArtifact{
				Path: writer.Path(), RootDir: writer.RootDir(),
				StartTimeUTC: writer.StartTimeUTC(),
			},
		)
	}, nil
}

type runtimeMetricRecordWriterAdapter struct {
	writer *platformmetrics.RuntimeMetricsSink
}

func (a runtimeMetricRecordWriterAdapter) WriteMetric(
	ctx context.Context,
	record factoryruntime.RuntimeMetricRecord,
) error {
	return a.writer.WriteMetric(ctx, record)
}

func (a runtimeMetricRecordWriterAdapter) Close() error {
	return a.writer.Close()
}

func provideApplicationRuntimeAdapter(
	visualizationFactory factoryvisualization.RuntimeFactory,
	httpHandler *httpapplication.Handler,
	newRunner lifecycle.RunnerFactory,
) (factorysessionwire.RuntimeAdapter, error) {
	if visualizationFactory == nil || httpHandler == nil || newRunner == nil {
		return nil, errors.New("Factory visualization, HTTP handler, and lifecycle component operations are required")
	}
	return func(
		opened factorysessionwire.OpenedApplicationRuntime,
		effects factorysessionwire.RuntimeOpeningExternalEffects,
		sink factoryvisualization.Sink,
	) (factorysessions.BoundProcessComponents, error) {
		var visualization lifecycle.Component
		var err error
		if sink != nil {
			logger := opened.Resources.Logger
			if logger == nil {
				logger = zap.NewNop()
			}
			visualized, err := visualizationFactory(
				opened.Visualization.Reader, opened.Visualization.Projections, effects.Clock, sink,
				func(err error) {
					logger.Error("Factory visualization failed", zap.Error(err))
				},
			)
			if err != nil {
				return factorysessions.BoundProcessComponents{}, err
			}
			if effects.FactoryVisualizationRootObserver != nil {
				effects.FactoryVisualizationRootObserver(visualized)
			}
			// Factory Session lifecycle must not auto-activate Visualization.
			// Peers leave the composed root inert until explicit Activate.
			visualization = lifecycle.Functions{
				StartFunc: func(context.Context) error { return nil },
				StopFunc:  visualized.Stop,
			}
		}
		handler, err := httpHandler.Bind(opened.HTTP)
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

func provideManagedRunnerFactory() runtimeapplication.ManagedRunnerFactory {
	return runtimeapplication.NewManagedRunner
}

func provideFixtureStdioApplicationBuilder(
	build initializerapplication.StdioRunnerBuilder,
	newRunner lifecycle.RunnerFactory,
	open mcpstdio.Opener,
	prepare factorysessionwire.RequestPreparation,
	workflowPreview factoryruntime.WorkflowPreviewOperation,
) factorysessionwire.FixtureStdioApplicationBuilder {
	return func(
		ctx context.Context,
		execution factorysessions.ExecutionService,
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
			session, err := open(execution, prepare, workflowPreview, sessionInput, sessionOutput)
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
				session, err := open(opened.Execution, prepare, opened.WorkflowPreview, sessionInput, sessionOutput)
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
	open factorysessionwire.StdioOpeningOperation
}

func (adapter stdioApplicationOpener) OpenStdio(
	ctx context.Context,
	intent processcontract.MCPIntent,
) (initializer.RunApplication, error) {
	return adapter.open.OpenStdio(ctx, factorysessions.StdioOpeningRequest{
		FixtureCatalogPath: intent.FixtureCatalogPath,
		RuntimeBacked:      intent.RuntimeBacked,
		ProjectRoot:        intent.ProjectRoot,
		SystemConfigHome:   intent.HomeDir,
		Input:              intent.Stdin,
		Output:             intent.Stdout,
	})
}

func provideStdioApplicationOpener(
	open factorysessionwire.StdioOpeningOperation,
) (processcontract.StdioApplicationOpener, error) {
	if open == nil {
		return nil, errors.New("Factory Session stdio opening operation is required")
	}
	return stdioApplicationOpener{open: open}, nil
}

func provideLifecycleRunnerFactory() lifecycle.RunnerFactory {
	return lifecycle.NewRunner
}

func provideRunOpener(
	prepareWorkTarget work.SingleWorkTargetPreparation,
	loadMockWorkers workers.MockWorkersConfigLoader,
	buildRuntimeRequest runcli.RuntimeOpeningRequestFactory,
) runcli.Opener {
	return func(
		ctx context.Context,
		cfg runcli.RunConfig,
		buildRunner runcli.RuntimeRunnerBuilder,
		invocation runcli.InvocationOperation,
		presentation factoryvisualization.ResponsePresentation,
	) (*runcli.Operation, error) {
		return runcli.Open(
			ctx, cfg, buildRunner, invocation, presentation,
			prepareWorkTarget, loadMockWorkers, buildRuntimeRequest,
		)
	}
}

func provideRunInvocationOperation(
	invocation factorysessionwire.InvocationOperation,
) runcli.InvocationOperation {
	return invocation
}

func provideModelsCLIInvocationOperation(
	invocation factorysessionwire.InvocationOperation,
) modelscli.InvocationOperation {
	return invocation
}

func provideRunSelectionFactory(
	open runcli.Opener,
	buildRunner runcli.RuntimeRunnerBuilder,
	invocation factorysessionwire.InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
	directJavaScript factorysessionwire.DirectJavaScriptRunOperation,
	buildApplication initializer.RuntimeRunnerBuilder,
) (runcli.SelectionFactory, error) {
	return runcli.NewSelectionFactory(
		open, buildRunner, invocation, presentation, directJavaScript, buildApplication,
	)
}

func provideRunRuntimeRunnerBuilder(
	build initializer.RuntimeRunnerBuilder,
	open *factorysessionwire.ApplicationService,
) (runcli.RuntimeRunnerBuilder, error) {
	if build == nil || open == nil {
		return nil, errors.New("run application lifecycle builder and Factory Session opener are required")
	}
	return func(
		ctx context.Context,
		request factorysessionwire.ApplicationOpeningRequest,
		logger *zap.Logger,
		sink factoryvisualization.Sink,
	) (initializer.LocalRuntimeRunner, error) {
		return build(ctx, func(openCtx context.Context) (initializer.OpenedApplication, error) {
			opened, err := open.OpenApplication(openCtx, request, logger, sink)
			if err != nil {
				return initializer.OpenedApplication{}, err
			}
			return initializer.OpenedApplication{
				Plan:        opened.Plan,
				Diagnostics: runtimeartifact.Diagnostics(opened.Diagnostics),
			}, nil
		})
	}, nil
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
	return factoryvisualization.NewResponsePresentation()
}

func provideRuntimeOpener(factory *factorysessionwire.RuntimeOpeningFactory) factorysessionwire.RuntimeOpener {
	return factory
}

func provideDirectJavaScriptSyncRunner() factorysessionwire.DirectJavaScriptSyncRunner {
	return sessionexecutioncli.RunNormalizedSync
}

func provideDirectJavaScriptHostAdapter(
	httpHandler *httpapplication.Handler,
	start platformhttpserver.Starter,
	newRunner lifecycle.RunnerFactory,
) (factorysessionwire.DirectJavaScriptHostAdapter, error) {
	if httpHandler == nil || start == nil || newRunner == nil {
		return nil, errors.New("direct JavaScript HTTP handler, starter, and lifecycle runner are required")
	}
	return func(
		execution factorysessionwire.OwnedExecutionService,
		executionLifecycle factorysessionwire.DirectJavaScriptLifecycle,
		request factorysessions.DirectJavaScriptRunRequest,
	) (lifecycle.Component, error) {
		if request.Host == nil {
			return nil, errors.New("direct JavaScript host request is required")
		}
		handler, err := httpHandler.BindDurableExecution(
			execution, executionLifecycle, request.Logger,
		)
		if err != nil {
			return nil, err
		}
		return newRunner(func(ctx context.Context) error {
			return start(ctx, platformhttpserver.StartRequest{
				Handler: handler, Host: request.Host.Host, Port: request.Host.Port,
				AutoPort: request.Host.AutoPort, Logger: request.Logger,
				OnBound: func(binding platformhttpserver.Binding) {
					if request.RuntimeHostObserver != nil {
						request.RuntimeHostObserver(factorysessions.RuntimeHostBinding{
							Host: binding.Host, Port: binding.Port,
						})
					}
				},
			})
		}), nil
	}, nil
}
