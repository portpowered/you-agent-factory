package wire

import (
	"context"
	"errors"
	"reflect"
	"strings"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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
					Port:        cfg.Port,
					AutoPort:    cfg.AutoPort,
				},
			},
			Workers: workers.RuntimeOpeningRequest{
				RunnerID:                          cfg.RunnerID,
				MockWorkers:                       mockWorkers,
				InvocationSkipPermissionsOverride: cfg.InvocationSkipPermissionsOverride,
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
