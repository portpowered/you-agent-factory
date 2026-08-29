package wire

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	platformruntimeartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingshttp "github.com/portpowered/infinite-you/pkg/services/recordings/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionshttp "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	transporthttp "github.com/portpowered/infinite-you/pkg/transports/http"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"go.uber.org/zap"
)

func provideWorkersMockWorkersConfigFileSystem(
	edges serviceedges.Edges,
) workers.MockWorkersConfigFileSystem {
	if edges.WorkersMockWorkersConfigFileSystem != nil {
		return edges.WorkersMockWorkersConfigFileSystem
	}
	return platformfilesystem.Local{}
}

func provideWorkersMockWorkersConfigDiagnosticsLoader(
	files workers.MockWorkersConfigFileSystem,
) (workers.MockWorkersConfigDiagnosticsLoader, error) {
	return (workers.MockWorkersConfigCodec{}).NewDiagnosticsLoader(files)
}

func provideRunInputPathInspector() platformfilesystem.PathInspector {
	return platformfilesystem.Local{}
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

func provideRuntimeMetricsCoordination() (platformmetrics.RuntimeMetricsCoordination, error) {
	return platformmetrics.NewRuntimeMetricsCoordination()
}

func provideRuntimeMetricsRetentionFileSystem() platformmetrics.RuntimeMetricsRetentionFileSystem {
	return platformfilesystem.Local{}
}

func provideRuntimeLogOwner(
	baseLogger *zap.Logger,
	clock runtimeArtifactClock,
	newID runtimeArtifactIDGenerator,
	paths platformruntimeartifact.Reserver,
) (factoryruntime.RuntimeLogOwner, error) {
	opener, err := logging.NewRuntimeLogOpener(paths)
	if err != nil {
		return nil, err
	}
	if baseLogger == nil {
		return nil, errors.New("runtime log owner base logger is required")
	}
	return runtimeLogOwner{
		baseLogger: baseLogger, opener: opener, clock: clock, newID: newID,
	}, nil
}

type runtimeLogOwner struct {
	baseLogger *zap.Logger
	opener     *logging.RuntimeLogOpener
	clock      runtimeArtifactClock
	newID      runtimeArtifactIDGenerator
}

func (owner runtimeLogOwner) Open(request factoryruntime.RuntimeLogScopeRequest) (factoryruntime.RuntimeLogSink, error) {
	if request.Policy == factoryruntime.RuntimeFileLoggingPolicyDisabled {
		return nil, nil
	}
	if owner.opener == nil || owner.clock == nil || owner.newID == nil {
		return nil, errors.New("runtime log owner is not configured")
	}
	opened, err := owner.opener.Open(logging.RuntimeLogOpeningRequest{
		BaseLogger: owner.baseLogger, RuntimeInstanceID: request.RuntimeInstanceID,
		RootDirectory: request.RootDirectory, StartTimeUTC: owner.clock(), CollisionID: owner.newID(),
		Config: logging.RuntimeLogConfig{
			MaxSize: request.Config.MaxSize, MaxBackups: request.Config.MaxBackups,
			MaxAge: request.Config.MaxAge, Compress: request.Config.Compress,
		},
	})
	if err != nil {
		return nil, err
	}
	return runtimeLogSinkAdapter{sink: opened}, nil
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

func provideRuntimeMetricsOwner(
	baseLogger *zap.Logger,
	clock runtimeArtifactClock,
	newID runtimeArtifactIDGenerator,
	paths platformruntimeartifact.Reserver,
	retentionFileSystem platformmetrics.RuntimeMetricsRetentionFileSystem,
	coordination platformmetrics.RuntimeMetricsCoordination,
) (factoryruntime.RuntimeMetricsOwner, error) {
	retention, err := platformmetrics.NewRuntimeMetricsRetention(
		retentionFileSystem, clock, coordination,
	)
	if err != nil {
		return nil, err
	}
	scheduler, err := platformmetrics.NewRuntimeMetricsRetentionScheduler(
		retention, nil, runtimeMetricsRetentionReporter(baseLogger),
	)
	if err != nil {
		return nil, err
	}
	opener, err := platformmetrics.NewRuntimeMetricsOpenerWithRetention(
		paths, scheduler, coordination,
	)
	if err != nil {
		return nil, err
	}
	return runtimeMetricsOwner{opener: opener, clock: clock, newID: newID}, nil
}

func runtimeMetricsRetentionReporter(
	baseLogger *zap.Logger,
) platformmetrics.RuntimeMetricsRetentionReporter {
	return func(report platformmetrics.RuntimeMetricsRetentionReport, sweepErr error) {
		if baseLogger == nil {
			return
		}
		fields := []zap.Field{
			zap.String("root", report.RootDirectory),
			zap.Bool("skipped", report.Skipped),
			zap.Int("scanned_files", report.Scanned.Files),
			zap.Int64("scanned_bytes", report.Scanned.Bytes),
			zap.Int("before_files", report.Before.Files),
			zap.Int64("before_bytes", report.Before.Bytes),
			zap.Int("after_files", report.After.Files),
			zap.Int64("after_bytes", report.After.Bytes),
			zap.Int("removed_files", report.Removed.Files),
			zap.Int64("removed_bytes", report.Removed.Bytes),
			zap.Int("protected_files", report.Protected.Files),
			zap.Int64("protected_bytes", report.Protected.Bytes),
			zap.Int("failed_files", report.Failed.Files),
			zap.Int64("failed_bytes", report.Failed.Bytes),
		}
		if sweepErr != nil {
			baseLogger.Warn("runtime metrics retention sweep failed", append(fields, zap.Error(sweepErr))...)
			return
		}
		if report.Failed.Files > 0 {
			baseLogger.Warn("runtime metrics retention sweep completed with failures", fields...)
			return
		}
		baseLogger.Debug("runtime metrics retention sweep completed", fields...)
	}
}

type runtimeMetricsOwner struct {
	opener *platformmetrics.RuntimeMetricsOpener
	clock  runtimeArtifactClock
	newID  runtimeArtifactIDGenerator
}

func (owner runtimeMetricsOwner) Close(ctx context.Context) error {
	if owner.opener == nil {
		return nil
	}
	return owner.opener.Close(ctx)
}

func (owner runtimeMetricsOwner) Open(request factoryruntime.RuntimeMetricsScopeRequest) (factoryruntime.RuntimeMetricsSink, error) {
	if request.Policy == factoryruntime.RuntimeMetricsPolicyDisabled {
		return nil, nil
	}
	if owner.opener == nil || owner.clock == nil || owner.newID == nil {
		return nil, errors.New("runtime metrics owner is not configured")
	}
	writer, err := owner.opener.Open(platformmetrics.RuntimeMetricsOpeningRequest{
		SessionID: request.Scope.SessionID, RuntimeInstanceID: request.Scope.RuntimeInstanceID,
		FolderPath: request.Scope.FolderPath, FactoryDirectory: request.Scope.FactoryDir,
		RootDirectory: request.RootDirectory, StartTimeUTC: owner.clock(), CollisionID: owner.newID(),
		Config: platformmetrics.RuntimeMetricsConfig{
			MaxSize: request.Config.MaxSize, MaxBackups: request.Config.MaxBackups,
			MaxAge: request.Config.MaxAge, Compress: request.Config.Compress,
		},
	})
	if err != nil {
		return nil, err
	}
	return factoryruntime.NewRuntimeMetricsSink(
		runtimeMetricRecordWriterAdapter{writer: writer},
		request.Scope,
		owner.clock,
		factoryruntime.RuntimeMetricsArtifact{
			Path: writer.Path(), RootDir: writer.RootDir(),
			StartTimeUTC: writer.StartTimeUTC(),
		},
	)
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

// provideRuntimeOpeningRequestFactory is the sole mapping from transport
// selections into the bounded owner requests consumed by Factory Sessions.
func provideRuntimeOpeningRequestFactory() runcli.RuntimeOpeningRequestFactory {
	return func(
		cfg runcli.RunConfig,
		mockWorkers *workers.MockWorkersConfig,
	) *factorysessions.RuntimeOpeningRequest {
		logDirectory := cfg.RuntimeLogDir
		if strings.TrimSpace(logDirectory) == "" && strings.TrimSpace(cfg.HomeDir) != "" {
			logDirectory = logging.RuntimeLogsRoot(cfg.HomeDir)
		}
		metricsDirectory := cfg.RuntimeMetricsDir
		if strings.TrimSpace(metricsDirectory) == "" && strings.TrimSpace(cfg.HomeDir) != "" {
			metricsDirectory = platformmetrics.RuntimeMetricsRoot(cfg.HomeDir)
		}
		mode := factorydefinitions.RuntimeModeBatch
		if cfg.Continuously {
			mode = factorydefinitions.RuntimeModeService
		}
		request := &factorysessions.RuntimeOpeningRequest{
			FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{
				Directory:        cfg.Dir,
				SourcePath:       cfg.FactoryConfigPath,
				ExecutionBaseDir: cfg.ExecutionBaseDir,
			},
			FactoryRuntime: factoryruntime.RuntimeOpeningRequest{
				Mode:         mode,
				Verbose:      cfg.Verbose,
				LogDirectory: logDirectory,
				LogConfig: factoryruntime.RuntimeLogStorageConfig{
					MaxSize: cfg.RuntimeLogConfig.MaxSize, MaxBackups: cfg.RuntimeLogConfig.MaxBackups,
					MaxAge: cfg.RuntimeLogConfig.MaxAge, Compress: cfg.RuntimeLogConfig.Compress,
				},
				MetricsDirectory: metricsDirectory,
				MetricsConfig: factoryruntime.RuntimeMetricsStorageConfig{
					MaxSize: cfg.RuntimeMetricsConfig.MaxSize, MaxBackups: cfg.RuntimeMetricsConfig.MaxBackups,
					MaxAge: cfg.RuntimeMetricsConfig.MaxAge, Compress: cfg.RuntimeMetricsConfig.Compress,
				},
			},
			FactorySession: factorysessions.SessionRuntimeOpeningRequest{
				CanonicalSessionID: cfg.CanonicalSessionID,
				// Public run and service openings use the existing durable
				// snapshot path explicitly. Empty and disabled remain
				// memory-only choices for callers that opt into them.
				PersistencePolicy: factorysessions.PersistencePolicyEnabled,
				SystemConfigHome:  cfg.HomeDir,
				WorkFile:          cfg.WorkFile,
				Host: factorysessions.RuntimeHostRequest{
					Directory:   cfg.Dir,
					RuntimeMode: mode,
					WorkFile:    cfg.WorkFile,
					MockWorkers: mockWorkers != nil,
					Host:        cfg.BindHost,
					Port:        cfg.Port,
					AutoPort:    cfg.AutoPort,
					Pprof:       cfg.Pprof,
				},
			},
			Workers: workers.RuntimeOpeningRequest{
				RunnerID:                          cfg.RunnerID,
				Worktree:                          cfg.Worktree,
				WorkerReasoningEffort:             cfg.WorkerReasoningEffort,
				MockWorkers:                       mockWorkers,
				InvocationSkipPermissionsOverride: cfg.InvocationSkipPermissionsOverride,
			},
			Recordings: recordings.RuntimeOpeningRequest{
				RecordPath: cfg.RecordPath,
				ReplayPath: cfg.ReplayPath,
				ResumePath: cfg.ResumePath,
				WorkflowID: cfg.Workflow,
			},
			ModelCacheDirectory: cfg.ModelCacheDir,
			OperatorDefaults:    cfg.OperatorDefaults,
		}
		return request
	}
}

// provideRuntimeInputResolver copies transport-owned request values into the
// stable application-opening input. External effects are selected by the
// process graph and are not projected from this operation callback.
func provideRuntimeInputResolver() factorysessionwire.ApplicationRuntimeInputResolver {
	return func(
		ctx context.Context,
		request *factorysessions.RuntimeOpeningRequest,
	) (factorysessionwire.ApplicationRuntimeInputs, error) {
		if err := validateRuntimeOpeningInputs(ctx, request); err != nil {
			return factorysessionwire.ApplicationRuntimeInputs{}, err
		}
		configured := *request
		return factorysessionwire.ApplicationRuntimeInputs{
			Request: &configured,
		}, nil
	}
}

func validateRuntimeOpeningInputs(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) error {
	switch {
	case ctx == nil:
		return errors.New("context is required")
	case ctx.Err() != nil:
		return ctx.Err()
	case request == nil:
		return errors.New("runtime opening request is required")
	default:
		return nil
	}
}

// The following providers select the process-owned external effects once for
// the long-lived runtime-opening Factory. Operation calls receive only
// invocation data and observation fallbacks; they do not re-read Edges or
// manufacture replacement runners.
func provideFactoryRuntimeClock(edges serviceedges.Edges) factoryruntime.Clock {
	if edges.Clock != nil {
		return edges.Clock
	}
	return platformclock.Real{}
}

func provideFactoryRuntimeProviderOverride(edges serviceedges.Edges) factorysessionwire.ProviderOverrideService {
	return edges.ProviderOverride
}

func provideFactoryRuntimeSubmissionRecorder(edges serviceedges.Edges) recordings.SubmissionRecorder {
	return edges.SubmissionRecorder
}

func provideFactoryRuntimeDispatchRecorder(edges serviceedges.Edges) recordings.DispatchRecorder {
	return edges.DispatchRecorder
}

func provideFactorySessionInvocationMetricsRecorder(edges serviceedges.Edges) factorysessionwire.InvocationMetricsRecorder {
	return edges.InvocationMetricsRecorder
}

func provideFactoryRuntimeProviderCommandRunner(
	edges serviceedges.Edges,
) (factorysessionwire.ProviderCommandRunner, error) {
	if edges.ProviderCommandRunner != nil {
		return edges.ProviderCommandRunner, nil
	}
	runner, err := providePlatformProcessCommandRunner(edges)
	if err != nil {
		return nil, err
	}
	return runner, nil
}

func provideFactoryRuntimeScriptCommandRunner(
	edges serviceedges.Edges,
) (factorysessionwire.ScriptCommandRunner, error) {
	if edges.ScriptCommandRunner != nil {
		return edges.ScriptCommandRunner, nil
	}
	runner, err := providePlatformProcessCommandRunner(edges)
	if err != nil {
		return nil, err
	}
	return runner, nil
}

func provideSessionExecutionOpeningFactory(
	runtimes factorysessionwire.ExecutionRuntimeOpening,
	workerExecution workers.Service,
	build factorysessionwire.StandaloneSessionExecutionFactory,
	resolveClock factoryruntime.ClockResolver,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	paths factorysessionwire.ExecutionOpeningFileSystem,
	logger *zap.Logger,
) (*factorysessionwire.ExecutionOpeningFactory, error) {
	return factorysessionwire.NewExecutionOpeningFactory(
		runtimes, workerExecution, build, resolveClock, artifactRoots, paths, logger,
	)
}

func provideInvocationOperation(
	openRuntime factorysessionwire.InvocationRuntimeOpening,
	modelsRoot models.Service,
	workingDirectory platformfilesystem.WorkingDirectory,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	artifactExporter modelswire.InvocationArtifactExporter,
	modelTimeout factorysessions.ModelInvocationTimeout,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	generateSessionID factorysessions.SessionIDGenerator,
	logger *zap.Logger,
	presentations factorysessions.OpeningPresentationOwner,
) (factorysessionwire.InvocationOperation, error) {
	return factorysessionwire.NewInvocationOperation(
		openRuntime,
		modelsRoot,
		workingDirectory,
		resolveCurrentDir,
		artifactExporter,
		modelTimeout,
		artifactRoots,
		generateSessionID,
		logger,
		presentations,
	)
}

func provideAgyPTYAllocator(edges serviceedges.Edges) (providerswire.PTYAllocator, error) {
	return provideProvidersAgyPTYAllocator(edges)
}

func provideProvidersAgyPTYAllocator(edges serviceedges.Edges) (providerswire.PTYAllocator, error) {
	clock := edges.AgyPTYClock
	if clock == nil {
		clock = platformclock.Real{}
	}
	host := edges.AgyPTYHost
	if host == nil {
		host = platformpty.NewHost()
	}
	return providerswire.NewAgyPTYAllocator(host, clock)
}

func provideWorkerCommandRunnerAdapter() factorysessionwire.WorkerCommandRunnerAdapter {
	return func(runner platformprocess.CommandRunner) platformprocess.CommandRunner { return runner }
}

func provideWorkRequestIDGenerator(edges serviceedges.Edges) work.RequestIDGenerator {
	if edges.WorkRequestIDGenerator != nil {
		return edges.WorkRequestIDGenerator
	}
	return uuid.NewString
}

func provideFactorySessionRuntimeInstanceIDGenerator(edges serviceedges.Edges) factorysessions.RuntimeInstanceIDGenerator {
	if edges.FactorySessionRuntimeInstanceIDGenerator != nil {
		return edges.FactorySessionRuntimeInstanceIDGenerator
	}
	return uuid.NewString
}

func provideFactorySessionIDGenerator(edges serviceedges.Edges) factorysessions.SessionIDGenerator {
	if edges.FactorySessionIDGenerator != nil {
		return edges.FactorySessionIDGenerator
	}
	return uuid.NewString
}

func provideFactorySessionResponseEventIDGenerator(edges serviceedges.Edges) factorysessions.ResponseEventIDGenerator {
	if edges.FactorySessionResponseEventIDGenerator != nil {
		return edges.FactorySessionResponseEventIDGenerator
	}
	return uuid.NewString
}

func provideWorkSubmittedFileReader(edges serviceedges.Edges) work.SubmittedFileReader {
	if edges.WorkSubmittedFileReader != nil {
		return edges.WorkSubmittedFileReader
	}
	return os.ReadFile
}

func provideWorkSubmittedFilePathInspector(edges serviceedges.Edges) work.SubmittedFilePathInspector {
	if edges.WorkSubmittedFilePathInspector != nil {
		return edges.WorkSubmittedFilePathInspector
	}
	return os.Stat
}

func provideWorkService(
	runtimes factorysessionwire.RuntimeAssembly,
	readFile work.SubmittedFileReader,
	inspectPath work.SubmittedFilePathInspector,
	contentStaging work.ContentStagingService,
	contentMaterializer work.ContentMaterializer,
	durability work.CompletedFlushSequenceReader,
) work.Service {
	return workwire.NewRuntimeService(runtimes, readFile, inspectPath, contentStaging, contentMaterializer, durability)
}

// provideWorkDurabilityReader adapts the narrow Recordings lifecycle
// capability to Work's sequence-only consumer contract. The root graph still
// injects one capability; Work never receives the broad Recordings service.
func provideWorkDurabilityReader(service recordings.Service) work.CompletedFlushSequenceReader {
	reader, ok := service.(recordings.CompletedFlushWatermarkReader)
	if !ok || reader == nil {
		return nil
	}
	return recordingsWorkDurabilityReader{reader: reader}
}

type recordingsWorkDurabilityReader struct {
	reader recordings.CompletedFlushWatermarkReader
}

func (reader recordingsWorkDurabilityReader) CompletedFlushSequence(
	streamGenerationID string,
) (int64, bool) {
	if reader.reader == nil {
		return 0, false
	}
	cursor, ok := reader.reader.CompletedFlushWatermark(streamGenerationID)
	return int64(cursor.Sequence), ok
}

func provideDirectJavaScriptHostAdapter(
	validation factorydefinitions.SubmittedDefinitionValidationOperation,
	invocationWorkType factorydefinitions.InvocationWorkTypeService,
	sessionRequests factorysessionshttp.RequestPreparation,
	start platformhttpserver.Starter,
	newRunner lifecycle.RunnerFactory,
	logger *zap.Logger,
) (factorysessionwire.DirectJavaScriptHostAdapter, error) {
	if validation == nil || invocationWorkType == nil || sessionRequests == nil || start == nil || newRunner == nil || logger == nil {
		return nil, errors.New("direct JavaScript HTTP handler, starter, and lifecycle runner are required")
	}
	return func(
		execution factorysessionwire.OwnedExecutionService,
		host factorysessions.RuntimeHostRequest,
		cancellation initializer.InvocationCancellation,
		observer factorysessions.RuntimeHostObserver,
	) (lifecycle.Component, error) {
		handler, err := newDurableExecutionHTTPHandler(
			execution, validation, invocationWorkType, sessionRequests, logger, cancellation,
		)
		if err != nil {
			return nil, err
		}
		return newRunner(func(ctx context.Context) error {
			return start(ctx, platformhttpserver.StartRequest{
				Handler: handler, Host: host.Host, Port: host.Port,
				AutoPort: host.AutoPort, Pprof: host.Pprof, Logger: logger,
				OnBound: func(binding platformhttpserver.Binding) {
					if observer != nil {
						observer(factorysessions.RuntimeHostBinding{
							Host: binding.Host, Port: binding.Port,
						})
					}
				},
			})
		}), nil
	}, nil
}

func newDurableExecutionHTTPHandler(
	execution factorysessionwire.OwnedExecutionService,
	validation factorydefinitions.SubmittedDefinitionValidationOperation,
	invocationWorkType factorydefinitions.InvocationWorkTypeService,
	sessionRequests factorysessionshttp.RequestPreparation,
	logger *zap.Logger,
	cancellation initializer.InvocationCancellation,
) (http.Handler, error) {
	if execution == nil || validation == nil || invocationWorkType == nil || sessionRequests == nil || logger == nil {
		return nil, errors.New("construct durable execution HTTP handler: execution, policies, request preparation, and logger are required")
	}
	durable := factorysessionmapping.NewDurableAPI(execution)
	sessionsHandler := factorysessionshttp.NewHandler(factorysessionshttp.Dependencies{
		DurableExecution: durable, DurableLifecycle: durable,
		DurableListing: durable, DurableResponseEvents: durable,
		DurableLister: execution, FactoryValidation: validation,
		InvocationWorkType: invocationWorkType, SessionRequests: sessionRequests,
	}, logger)
	var shutdown transporthttp.ShutdownOperation
	if cancellation != nil {
		shutdown = cancellation.Cancel
	}
	return transporthttp.NewServerWithRecordingsAndShutdown(
		recordingshttp.NewLegacyAdapter(
			factorysessionmapping.NewDurableHistoryBridge(durable),
			factorysessionshttp.NewDurableRequestPreparation(sessionRequests),
		),
		sessionsHandler, nil, nil, nil, nil, logger, shutdown,
	).Handler(), nil
}

type workerSessionsFactorySessionScopeResolver struct {
	sessions factorysessions.Service
}

func newWorkerSessionsFactorySessionScopeResolver(
	sessions factorysessions.Service,
) workersessionshttp.SessionScopeResolver {
	if sessions == nil {
		return nil
	}
	return workerSessionsFactorySessionScopeResolver{sessions: sessions}
}

func (resolver workerSessionsFactorySessionScopeResolver) ResolveWorkerSessionScope(
	ctx context.Context,
	sessionID string,
) (workersessionshttp.SessionScope, error) {
	if fast, ok := resolver.sessions.(interface {
		ResolveFactorySessionRuntimeID(string) (string, error)
	}); ok {
		effectiveID, err := fast.ResolveFactorySessionRuntimeID(sessionID)
		if err != nil {
			if errors.Is(err, factorysessions.ErrSessionNotFound) || errors.Is(err, factorysessions.ErrNotFound) {
				return workersessionshttp.SessionScope{}, workersessions.ErrObservationSessionNotFound
			}
			return workersessionshttp.SessionScope{}, err
		}
		return workersessionshttp.SessionScope{
			EffectiveID: effectiveID,
			IsDefault:   strings.TrimSpace(sessionID) == factorysessions.DefaultSessionID,
		}, nil
	}
	projection, err := resolver.sessions.GetFactorySession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, factorysessions.ErrSessionNotFound) || errors.Is(err, factorysessions.ErrNotFound) {
			return workersessionshttp.SessionScope{}, workersessions.ErrObservationSessionNotFound
		}
		return workersessionshttp.SessionScope{}, err
	}
	return workersessionshttp.SessionScope{
		EffectiveID: projection.Context.FactorySessionID,
		IsDefault:   projection.Context.Session != nil && projection.Context.Session.IsDefault,
	}, nil
}

// WorkerSessionsObservationForSession forwards the optional decorated read
// capability through the composition-root adapter. The Worker Sessions HTTP
// transport keeps only its narrow resolver contract; the Factory Sessions
// service remains the owner of resolving an opened runtime's observation view.
func (resolver workerSessionsFactorySessionScopeResolver) WorkerSessionsObservationForSession(
	factorySessionID string,
) workersessions.ObservationService {
	if resolver.sessions == nil {
		return nil
	}
	provider, ok := resolver.sessions.(interface {
		WorkerSessionsObservationForSession(string) workersessions.ObservationService
	})
	if !ok {
		return nil
	}
	return provider.WorkerSessionsObservationForSession(factorySessionID)
}
