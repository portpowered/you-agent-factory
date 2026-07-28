package wire

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"

	"github.com/google/uuid"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
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

// provideRuntimeOpeningRequestFactory is the sole mapping from transport
// selections into the bounded owner requests consumed by Factory Sessions.
func provideRuntimeOpeningRequestFactory() runcli.RuntimeOpeningRequestFactory {
	return func(
		cfg runcli.RunConfig,
		mockWorkers *workers.MockWorkersConfig,
		observer factorysessions.RuntimeHostObserver,
	) factorysessionwire.ApplicationOpeningRequest {
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
				SystemConfigHome: cfg.HomeDir,
				WorkFile:         cfg.WorkFile,
				Host: factorysessions.RuntimeHostRequest{
					Directory:   cfg.Dir,
					RuntimeMode: mode,
					WorkFile:    cfg.WorkFile,
					MockWorkers: mockWorkers != nil,
					Host:        cfg.BindHost,
					Port:        cfg.Port,
					AutoPort:    cfg.AutoPort,
				},
			},
			Workers: workers.RuntimeOpeningRequest{
				RunnerID:                          cfg.RunnerID,
				MockWorkers:                       mockWorkers,
				InvocationSkipPermissionsOverride: cfg.InvocationSkipPermissionsOverride,
				ProviderIntegrations:              mapACPProviderIntegrations(cfg.ACPIntegrations),
			},
			Recordings: recordings.RuntimeOpeningRequest{
				RecordPath: cfg.RecordPath,
				ReplayPath: cfg.ReplayPath,
				WorkflowID: cfg.Workflow,
			},
			Models: models.RuntimeOpeningRequest{
				CacheDirectory: cfg.ModelCacheDir,
			},
			OperatorDefaults: cfg.OperatorDefaults,
		}
		return factorysessionwire.ApplicationOpeningRequest{
			Runtime: request,
			Ports: factorysessionwire.ApplicationOpeningPorts{
				InvocationMetricsRecorder: cfg.InvocationMetricsRecorder,
				RuntimeHostObserver:       observer,
			},
		}
	}
}

func mapACPProviderIntegrations(values []operatorsettings.ACPIntegration) []providers.Integration {
	if len(values) == 0 {
		return nil
	}
	result := make([]providers.Integration, len(values))
	for index, value := range values {
		result[index] = providers.Integration{
			ID: value.ID, Name: providers.ID(value.Name), Transport: value.Transport, Command: value.Command,
		}
	}
	return result
}

// provideRuntimeInputResolver merges process edges into the exact opening
// effect ports. Per-operation selections are already owner-bounded by the
// canonical injector mapper.
func provideRuntimeInputResolver(
	defaultEdges serviceedges.Edges,
	resolveClock factoryruntime.ClockResolver,
) factorysessionwire.ApplicationRuntimeInputResolver {
	return func(
		ctx context.Context,
		request *factorysessions.RuntimeOpeningRequest,
		ports factorysessionwire.ApplicationOpeningPorts,
		logger *zap.Logger,
	) (factorysessionwire.ApplicationRuntimeInputs, error) {
		edges := defaultEdges
		if ports.InvocationMetricsRecorder != nil {
			edges.InvocationMetricsRecorder = ports.InvocationMetricsRecorder
		}
		if ports.RuntimeHostObserver != nil {
			edges.RuntimeHostObserver = ports.RuntimeHostObserver
		}
		if resolveClock != nil {
			edges.Clock = resolveClock(edges.Clock)
		}
		effects := projectRuntimeOpeningExternalEffects(edges)
		if err := validateResolvedRuntimeInputs(ctx, request, effects, logger); err != nil {
			return factorysessionwire.ApplicationRuntimeInputs{}, err
		}
		configured := *request
		return factorysessionwire.ApplicationRuntimeInputs{
			Request: &configured,
			Effects: effects,
			Logger:  logger,
		}, nil
	}
}

func validateResolvedRuntimeInputs(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	effects factorysessionwire.RuntimeOpeningExternalEffects,
	logger *zap.Logger,
) error {
	switch {
	case ctx == nil:
		return errors.New("context is required")
	case ctx.Err() != nil:
		return ctx.Err()
	case request == nil:
		return errors.New("runtime opening request is required")
	case logger == nil:
		return errors.New("runtime logger is required")
	case isNilRuntimeInput(effects.Clock):
		return errors.New("runtime clock edge is required")
	default:
		return nil
	}
}

func isNilRuntimeInput(value any) bool {
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

func provideSessionExecutionOpeningFactory(
	runtimes *factorysessionwire.RuntimeOpeningFactory,
	edges serviceedges.Edges,
	build factorysessionwire.StandaloneSessionExecutionFactory,
	invocation factorysessionwire.WorkerInvocationFactory,
	resolveClock factoryruntime.ClockResolver,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	adaptRunner factorysessionwire.WorkerCommandRunnerAdapter,
	paths factorysessionwire.ExecutionOpeningFileSystem,
	allocator agypty.PTYAllocator,
	logger *zap.Logger,
) (*factorysessionwire.ExecutionOpeningFactory, error) {
	workerEdges, err := withStandaloneWorkerProductionEdges(edges)
	if err != nil {
		return nil, err
	}
	return factorysessionwire.NewExecutionOpeningFactory(
		runtimes, projectRuntimeOpeningExternalEffects(workerEdges), adaptRunner(workerEdges.ProviderCommandRunner), allocator,
		build, invocation, resolveClock, artifactRoots, adaptRunner, paths, logger,
	)
}

func provideInvocationOperation(
	openRuntime *factorysessionwire.RuntimeOpeningFactory,
	edges serviceedges.Edges,
	workingDirectory platformfilesystem.WorkingDirectory,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	artifactExporter models.InvocationArtifactExporter,
	modelTimeout factorysessions.ModelInvocationTimeout,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	generateSessionID factorysessions.SessionIDGenerator,
) (factorysessionwire.InvocationOperation, error) {
	return factorysessionwire.NewInvocationOperation(
		openRuntime,
		projectRuntimeOpeningExternalEffects(edges),
		workingDirectory,
		resolveCurrentDir,
		artifactExporter,
		modelTimeout,
		artifactRoots,
		generateSessionID,
	)
}

// projectRuntimeOpeningExternalEffects is the sole selection from the process
// edge aggregate into the effects consumed by Factory Session runtime opening.
func projectRuntimeOpeningExternalEffects(edges serviceedges.Edges) factorysessionwire.RuntimeOpeningExternalEffects {
	providerRunner := edges.ProviderCommandRunner
	scriptRunner := edges.ScriptCommandRunner
	if providerRunner == nil || scriptRunner == nil {
		if defaultRunner, err := providePlatformProcessCommandRunner(edges); err == nil && defaultRunner != nil {
			if providerRunner == nil {
				providerRunner = defaultRunner
			}
			if scriptRunner == nil {
				scriptRunner = defaultRunner
			}
		}
	}
	return factorysessionwire.RuntimeOpeningExternalEffects{
		Clock:                     edges.Clock,
		ProviderOverride:          edges.ProviderOverride,
		ModelPullMetricsRecorder:  edges.ModelPullMetricsRecorder,
		InvocationMetricsRecorder: edges.InvocationMetricsRecorder,
		ProviderCommandRunner:     providerRunner,
		ScriptCommandRunner:       scriptRunner,
		SubmissionRecorder:        edges.SubmissionRecorder,
		DispatchRecorder:          edges.DispatchRecorder,
		RuntimeHostObserver:              edges.RuntimeHostObserver,
		FactoryVisualizationSink:         edges.FactoryVisualizationSink,
		FactoryVisualizationRootObserver: edges.FactoryVisualizationRootObserver,
		HostedClock:               edges.HostedClock,
		HostedHTTPClient:          edges.HostedHTTPClient,
		HostedSecretResolver:      edges.HostedSecretResolver,
		HostedLinearEndpoint:      edges.HostedLinearEndpoint,
	}
}

func withStandaloneWorkerProductionEdges(overrides serviceedges.Edges) (serviceedges.Edges, error) {
	commandRunner, err := providePlatformProcessCommandRunner(overrides)
	if err != nil {
		return serviceedges.Edges{}, err
	}
	return serviceedges.Merge(serviceedges.Edges{
		ProviderCommandRunner: commandRunner,
		ScriptCommandRunner:   commandRunner,
	}, overrides), nil
}

func provideAgyPTYAllocator(edges serviceedges.Edges) (agypty.PTYAllocator, error) {
	clock := edges.AgyPTYClock
	if clock == nil {
		clock = platformclock.Real{}
	}
	host := edges.AgyPTYHost
	if host == nil {
		host = platformpty.NewHost()
	}
	return agypty.NewAllocator(host, clock)
}

func provideWorkerCommandRunnerAdapter() factorysessionwire.WorkerCommandRunnerAdapter {
	return workerprocess.AdaptCommandRunner
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

func provideWorkFactory(
	readFile work.SubmittedFileReader,
	contentStaging work.ContentStagingService,
	contentMaterializer work.ContentMaterializer,
) factorysessionwire.WorkFactory {
	return func(runtimes work.RuntimeResolver) work.Service {
		return workwire.NewRuntimeService(runtimes, readFile, contentStaging, contentMaterializer)
	}
}
