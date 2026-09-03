package host

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	runtimemetrics "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/metrics"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// InputFileSystem is the host bundle's private view of the runtime input-tree
// effect. It is intentionally not part of the Factory Runtime service root.
type InputFileSystem interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
}

// Bundle is the runtime wiring produced by Build and referenced from live handles.
type Bundle struct {
	Dir                  string
	FolderPath           string
	RuntimeInstanceID    string
	FactorySessionID     string
	BackendScopeID       string
	StartedAtUTC         time.Time
	EventHistory         recordings.RuntimeLedger
	Factory              Engine
	InputFiles           InputFileSystem
	InputDirectoryWalker factory.InputDirectoryWalker
	WorkRequestIDs       work.RequestIDGenerator
	Net                  *state.Net
	RuntimeCfg           factory.LoadedConfig
	Logger               *zap.Logger
	LogSink              factory.RuntimeLogSink
	MetricsSink          factory.RuntimeMetricsSink
	Recording            recordings.RuntimeRecorder
	RecordPath           string
	dispatchMetricFields sync.Map
	dispatchCompleted    func(string)
	metricsMu            sync.Mutex
	metricsClosed        bool
	metricsErrMu         sync.Mutex
	metricsErr           error
}

// NewBundle constructs one inert runtime host from direct collaborators.
func NewBundle(
	dir string,
	folderPath string,
	runtimeInstanceID string,
	factorySessionID string,
	backendScopeID string,
	startedAtUTC time.Time,
	eventHistory recordings.RuntimeLedger,
	net *state.Net,
	runtimeCfg factory.LoadedConfig,
	logger *zap.Logger,
	logSink factory.RuntimeLogSink,
	metricsSink factory.RuntimeMetricsSink,
	recording recordings.RuntimeRecorder,
	recordPath string,
	dispatchCompleted func(string),
) *Bundle {
	return &Bundle{
		Dir: dir, FolderPath: folderPath, RuntimeInstanceID: runtimeInstanceID,
		FactorySessionID: factorySessionID,
		BackendScopeID:   backendScopeID, StartedAtUTC: startedAtUTC,
		EventHistory: eventHistory, Net: net, RuntimeCfg: runtimeCfg,
		Logger: logger, LogSink: logSink, MetricsSink: metricsSink,
		Recording: recording, RecordPath: recordPath, dispatchCompleted: dispatchCompleted,
	}
}

// RuntimeLogger returns the bundle logger or a nop logger when unset.
func (r *Bundle) RuntimeLogger() *zap.Logger {
	if r == nil || r.Logger == nil {
		return zap.NewNop()
	}
	return r.Logger
}

func (r *Bundle) RuntimeService() factory.Service {
	if r == nil {
		return nil
	}
	service, _ := r.Factory.(factory.Service)
	return service
}

// BindModelsRuntimeScope forwards the opened Models capability to the
// concrete Factory implementation held by this runtime record. The public
// RuntimeRecord remains opaque; this narrow composition hook keeps the
// request-owned Models scope out of the public Factory service contract.
func (r *Bundle) BindModelsRuntimeScope(scope modelprovider.RuntimeScopeRef) error {
	if r == nil || r.Factory == nil {
		return fmt.Errorf("Factory Runtime is unavailable")
	}
	binder, ok := r.Factory.(interface {
		BindModelsRuntimeScope(modelprovider.RuntimeScopeRef) error
	})
	if !ok {
		return fmt.Errorf("Factory Runtime does not support Models runtime scope binding")
	}
	return binder.BindModelsRuntimeScope(scope)
}

// RuntimeProgressPublisher returns the runtime-scoped Workers observation
// bridge when the assembled Factory implementation exposes it. The bundle
// keeps this optional so inert and historical runtime records remain valid.
func (r *Bundle) RuntimeProgressPublisher() workers.ProgressPublisher {
	if r == nil || r.Factory == nil {
		return nil
	}
	provider, _ := r.Factory.(interface {
		RuntimeProgressPublisher() workers.ProgressPublisher
	})
	if provider == nil {
		return nil
	}
	return provider.RuntimeProgressPublisher()
}

// BeginWorkerAttempt forwards the optional Runtime-owned Worker Session
// opening capability through the hosted runtime record.
func (r *Bundle) BeginWorkerAttempt(
	ctx context.Context,
	request workers.ExecuteRequest,
) (func(context.Context, workers.ExecuteResult, error) error, error) {
	if r == nil || r.Factory == nil {
		return nil, factory.ErrNotRunning
	}
	provider, _ := r.Factory.(interface {
		BeginWorkerAttempt(
			context.Context,
			workers.ExecuteRequest,
		) (func(context.Context, workers.ExecuteResult, error) error, error)
	})
	if provider == nil {
		return nil, factory.ErrNotRunning
	}
	return provider.BeginWorkerAttempt(ctx, request)
}

func (r *Bundle) Directory() string {
	if r == nil {
		return ""
	}
	return r.Dir
}

func (r *Bundle) FolderDirectory() string {
	if r == nil {
		return ""
	}
	return r.FolderPath
}

func (r *Bundle) BackendScope() string {
	if r == nil {
		return ""
	}
	return r.BackendScopeID
}

func (r *Bundle) StartTime() time.Time {
	if r == nil {
		return time.Time{}
	}
	return r.StartedAtUTC
}

func (r *Bundle) LoadedRuntimeConfig() factory.LoadedConfig {
	if r == nil {
		return nil
	}
	return r.RuntimeCfg
}

func (r *Bundle) CanonicalEvents() []interfaces.FactoryEvent {
	if r == nil || r.EventHistory == nil {
		return nil
	}
	return r.EventHistory.CanonicalEvents()
}

func (r *Bundle) AddEventTypeRecorder(recorder func(interfaces.FactoryEventType)) {
	if r != nil && r.EventHistory != nil {
		r.EventHistory.AddEventTypeRecorder(recorder)
	}
}

func (r *Bundle) StreamGeneration() string {
	if r == nil || r.EventHistory == nil {
		return ""
	}
	return r.EventHistory.StreamGenerationID()
}

func (r *Bundle) RuntimeMetrics() runtimemetrics.MetricsEmitter {
	if r == nil {
		return runtimemetrics.NoopEmitter{}
	}
	return r.MetricsEmitter()
}

func (r *Bundle) RuntimeDiagnostics() factory.RuntimeLogDiagnostics {
	if r == nil || r.LogSink == nil {
		return factory.RuntimeLogDiagnostics{}
	}
	artifact := r.LogSink.Artifact()
	config := artifact.Config
	diagnostics := factory.RuntimeLogDiagnostics{
		Path: artifact.Path, RootDir: artifact.RootDir, StartTimeUTC: artifact.StartTimeUTC,
		MaxSizeMB: config.MaxSize, MaxBackups: config.MaxBackups,
		MaxAgeDays: config.MaxAge, Compress: config.Compress,
	}
	if r.MetricsSink != nil {
		artifact := r.MetricsSink.Artifact()
		diagnostics.MetricsPath = artifact.Path
		diagnostics.MetricsRootDir = artifact.RootDir
		diagnostics.MetricsStartTimeUTC = artifact.StartTimeUTC
	}
	return diagnostics
}

func (r *Bundle) RecordingLedger() recordings.Ledger {
	if r == nil {
		return nil
	}
	return r.EventHistory
}

// FinalizeRecording closes the runtime-owned Recordings scope during partial
// Factory Session unwind. Normal runtime shutdown reaches the same idempotent
// recorder through host.FinalizeArtifacts.
func (r *Bundle) FinalizeRecording(finishedAt time.Time) error {
	if r == nil || r.Recording == nil {
		return nil
	}
	return r.Recording.Finalize(finishedAt)
}

func (r *Bundle) CloseArtifacts() error {
	if r == nil {
		return nil
	}
	var errs []error
	if r.LogSink != nil {
		if err := r.LogSink.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := r.closeMetricsSink(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
