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
	"time"

	"github.com/google/uuid"
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
	factorynamedfactories "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedfactories"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packagedinstallation"
	factorypackages "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	applicationopening "github.com/portpowered/infinite-you/pkg/services/factory_sessions/applicationopening"
	factorysessioncursorpersistence "github.com/portpowered/infinite-you/pkg/services/factory_sessions/cursors/persistence"
	factorysessionruntimepersist "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/processlifecycle"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/runtimehosting"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/runtimeopening"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
	httpapplication "github.com/portpowered/infinite-you/pkg/transports/http/application"
	mcpstdio "github.com/portpowered/infinite-you/pkg/transports/mcp/stdio"
	"go.uber.org/zap"
)

func provideCurrentFactoryDirectoryResolver(
	namedPaths factorydefinitions.NamedPathResolver,
) factorydefinitions.CurrentFactoryDirectoryResolver {
	return func(rootDir string) (string, error) {
		return factorynamedfactories.ResolveCurrent(namedPaths, rootDir)
	}
}

func provideTerminalLoggerBuilder() terminalpolicy.LoggerBuilder {
	return func(mode terminalpolicy.Mode, debug bool) (*zap.Logger, error) {
		return logging.BuildTerminalLogger(string(mode), debug)
	}
}

func provideLiveRecordingTargetPlanner() recordings.LiveRecordingTargetPlanner {
	return recordings.NewLiveRecordingTargetPlanner(
		platformclock.Real{},
		uuid.NewString,
		filepath.Join,
	)
}

func provideCLIRunDefaults(
	recordingTargets recordings.LiveRecordingTargetPlanner,
) runcli.RunConfig {
	return runcli.RunConfig{
		RuntimeLogConfig:       logging.DefaultRuntimeLogConfig(),
		RuntimeMetricsConfig:   platformmetrics.DefaultRuntimeMetricsConfig(),
		RecordingTargetPlanner: recordingTargets,
		Clock:                  platformclock.Real{},
	}
}

func provideRunDirectoryCreator() platformfilesystem.DirectoryCreator {
	return platformfilesystem.Local{}
}

func provideBrowserOpener() platformbrowser.Opener {
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
) factorysessions.ExecutionOpeningFileSystem {
	if edges.FactorySessionExecutionOpeningFileSystem != nil {
		return edges.FactorySessionExecutionOpeningFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactorySessionDirectoryInspection(
	edges serviceedges.Edges,
) factorysessions.DirectoryInspection {
	if edges.FactorySessionDirectoryInspection != nil {
		return edges.FactorySessionDirectoryInspection
	}
	return platformfilesystem.Local{}
}

func provideFactorySessionContractFixtureReader(edges serviceedges.Edges) fileeffects.ContractFixtureReader {
	if edges.FactorySessionContractFixtureReader != nil {
		return edges.FactorySessionContractFixtureReader
	}
	return fileeffects.ContractFixtureReader(platformfilesystem.Local{}.ReadFile)
}

func provideFactorySessionInvocationInputReader(edges serviceedges.Edges) fileeffects.InvocationInputReader {
	if edges.FactorySessionInvocationInputReader != nil {
		return edges.FactorySessionInvocationInputReader
	}
	return fileeffects.InvocationInputReader(platformfilesystem.Local{}.ReadFile)
}

func provideFactorySessionReplayRecordingReader(edges serviceedges.Edges) fileeffects.ReplayRecordingReader {
	if edges.FactorySessionReplayRecordingReader != nil {
		return edges.FactorySessionReplayRecordingReader
	}
	return fileeffects.ReplayRecordingReader(platformfilesystem.Local{}.ReadFile)
}

func provideFactorySessionInitialWorkReader(edges serviceedges.Edges) fileeffects.InitialWorkReader {
	if edges.FactorySessionInitialWorkReader != nil {
		return edges.FactorySessionInitialWorkReader
	}
	return fileeffects.InitialWorkReader(platformfilesystem.Local{}.ReadFile)
}

func provideFactorySessionResolveHomeDirectory(
	edges serviceedges.Edges,
) factorysessions.HomeDirectoryResolver {
	if edges.FactorySessionResolveHomeDirectory != nil {
		return edges.FactorySessionResolveHomeDirectory
	}
	return os.UserHomeDir
}

func provideFactorySessionCursorPersistenceFileSystem(
	edges serviceedges.Edges,
) factorysessions.CursorPersistenceFileSystem {
	if edges.FactorySessionCursorPersistenceFileSystem != nil {
		return edges.FactorySessionCursorPersistenceFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactorySessionCursorCreateTemporaryFile(
	edges serviceedges.Edges,
) factorysessions.CursorPersistenceCreateTemporaryFile {
	if edges.FactorySessionCursorCreateTemporaryFile != nil {
		return edges.FactorySessionCursorCreateTemporaryFile
	}
	return func(dir, pattern string) (factorysessions.CursorPersistenceTemporaryFile, error) {
		return os.CreateTemp(dir, pattern)
	}
}

func provideFactorySessionCursorStoreFactory(
	files factorysessions.CursorPersistenceFileSystem,
	createTemporaryFile factorysessions.CursorPersistenceCreateTemporaryFile,
) factorysessions.CursorStoreFactory {
	return func(dir string) (factorysessions.CursorStore, error) {
		return factorysessioncursorpersistence.NewFileStore(dir, files, createTemporaryFile)
	}
}

func provideFactorySessionRuntimePersistenceFileSystem(
	edges serviceedges.Edges,
) factorysessions.RuntimePersistenceFileSystem {
	if edges.FactorySessionRuntimePersistenceFileSystem != nil {
		return edges.FactorySessionRuntimePersistenceFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactorySessionRuntimePersistenceStoreFactory(
	files factorysessions.RuntimePersistenceFileSystem,
) factorysessions.RuntimePersistenceStoreFactory {
	return func(projectRoot string) (factorysessions.RuntimePersistenceStore, error) {
		return factorysessionruntimepersist.NewProjectStore(projectRoot, files)
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
	operation factorysessions.InvocationOperation,
) factorysessions.ModelInvocationOperation {
	return operation
}

func provideSystemInitializationService(
	persistence factorydefinitions.Persistence,
	packagedInstallationFileSystem factorydefinitions.PackagedInstallationFileSystem,
	loadOperatorConfig operatorsettings.ConfigLoader,
	ensureOperatorBackendScope operatorsettings.BackendScopeEnsurer,
	inspectPath systeminitialization.InspectPath,
	migrationFiles systeminitialization.LegacyFactoryMigrationFileSystem,
) (systeminitialization.Service, error) {
	return systeminitialization.New(
		systeminitialization.OperatorSettingsFunctions{
			Load:   loadOperatorConfig,
			Ensure: ensureOperatorBackendScope,
		},
		packagedinstallation.New(persistence, packagedInstallationFileSystem),
		factorypackages.All(),
		inspectPath,
		migrationFiles,
	)
}

func provideDurableExecutionFactory(loadOperatorConfig operatorsettings.ConfigLoader) runtimeopening.DurableExecutionFactory {
	return func(
		definition factorydefinitions.RuntimeOpeningRequest,
		session factorysessions.SessionRuntimeOpeningRequest,
		root runtimeopening.RuntimeRoot,
		clock factoryruntime.Clock,
		provider workerprovider.Provider,
		factory runtimeopening.FactorySessionExecutionFactory,
	) (factorysessions.ExecutionService, error) {
		return runtimeopening.NewDurableExecution(loadOperatorConfig, definition, session, root, clock, provider, factory)
	}
}

func provideWorkerExecutionFactory() runtimeopening.WorkerExecutionFactory {
	return runtimeopening.NewWorkerExecution
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
) factorysessions.RuntimeHostOperation {
	return runtimehosting.New(starter)
}

func provideProcessRuntimeFactory(
	host factorysessions.RuntimeHostOperation,
) (factorysessions.ProcessRuntimeFactory, error) {
	return processlifecycle.NewFactory(host)
}

func provideFactoryVisualizationFactory() factoryvisualization.RuntimeFactory {
	return func(
		reader factorysessions.RuntimeReader,
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
	return work.NewContentStagingService(filesystem, random, clock, 0)
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
) (applicationopening.RuntimeAdapter, error) {
	if visualizationFactory == nil || httpHandler == nil || newRunner == nil {
		return nil, errors.New("Factory visualization, HTTP handler, and lifecycle component operations are required")
	}
	return func(
		opened factorysessions.OpenedApplicationRuntime,
		edges serviceedges.Edges,
		sink factoryvisualization.Sink,
	) (factorysessions.BoundProcessComponents, error) {
		var visualization lifecycle.Component
		var err error
		if sink != nil {
			logger := opened.Resources.Logger
			if logger == nil {
				logger = zap.NewNop()
			}
			visualization, err = visualizationFactory(
				opened.Visualization.Reader, opened.Visualization.Projections, edges.Clock, sink,
				func(err error) {
					logger.Error("Factory visualization failed", zap.Error(err))
				},
			)
			if err != nil {
				return factorysessions.BoundProcessComponents{}, err
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
	prepare factorysessions.RequestPreparation,
	workflowPreview factoryruntime.WorkflowPreviewOperation,
) factorysessions.FixtureStdioApplicationBuilder {
	return func(
		ctx context.Context,
		execution factorysessions.ExecutionService,
		input io.Reader,
		output io.Writer,
	) (factorysessions.StdioApplication, error) {
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
	prepare factorysessions.RequestPreparation,
) factorysessions.RuntimeStdioApplicationBuilder {
	return func(
		ctx context.Context,
		opened factorysessions.OpenedExecutionRuntime,
		input io.Reader,
		output io.Writer,
	) (factorysessions.StdioApplication, error) {
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
	open factorysessions.StdioOpeningOperation
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
	open factorysessions.StdioOpeningOperation,
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
		invocation factorysessions.InvocationOperation,
		presentation factoryvisualization.ResponsePresentation,
	) (*runcli.Operation, error) {
		return runcli.Open(
			ctx, cfg, buildRunner, invocation, presentation,
			prepareWorkTarget, loadMockWorkers, buildRuntimeRequest,
		)
	}
}

func provideRunSelectionFactory(
	open runcli.Opener,
	buildRunner runcli.RuntimeRunnerBuilder,
	invocation factorysessions.InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
	directJavaScript factorysessions.DirectJavaScriptRunOperation,
) (runcli.SelectionFactory, error) {
	return runcli.NewSelectionFactory(
		open, buildRunner, invocation, presentation, directJavaScript,
	)
}

func provideRunRuntimeRunnerBuilder(
	build initializer.RuntimeRunnerBuilder,
	open *applicationopening.Service,
) (runcli.RuntimeRunnerBuilder, error) {
	if build == nil || open == nil {
		return nil, errors.New("run application lifecycle builder and Factory Session opener are required")
	}
	return func(
		ctx context.Context,
		request factorysessions.ApplicationOpeningRequest,
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

func provideWorkStopSummaryProjector() factorysessions.WorkStopSummaryProjector {
	return func(request factorysessions.WorkStopSummaryRequest) *factorysessions.StopSummary {
		return factorysessions.ProjectWorkStopSummary(
			request.SessionID,
			request.Snapshot,
			request.Token,
			request.SessionStopSummary,
		)
	}
}

func provideResponsePresentation() factoryvisualization.ResponsePresentation {
	return factoryvisualization.NewResponsePresentation()
}

func provideRuntimeOpener(factory *runtimeopening.Factory) applicationopening.RuntimeOpener {
	return factory
}

func provideDirectJavaScriptSyncRunner() factorysessions.DirectJavaScriptSyncRunner {
	return sessionexecutioncli.RunNormalizedSync
}
