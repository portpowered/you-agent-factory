package wire

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	platformruntimeartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"go.uber.org/zap"
)

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
	clock runtimeArtifactClock,
	newID runtimeArtifactIDGenerator,
	paths platformruntimeartifact.Reserver,
) (factoryruntime.RuntimeMetricsOwner, error) {
	opener, err := platformmetrics.NewRuntimeMetricsOpener(paths)
	if err != nil {
		return nil, err
	}
	return runtimeMetricsOwner{opener: opener, clock: clock, newID: newID}, nil
}

type runtimeMetricsOwner struct {
	opener *platformmetrics.RuntimeMetricsOpener
	clock  runtimeArtifactClock
	newID  runtimeArtifactIDGenerator
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
