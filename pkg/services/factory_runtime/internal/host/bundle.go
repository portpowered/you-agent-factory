package host

import (
	"sync"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	runtimemetrics "github.com/portpowered/infinite-you/pkg/services/factory_runtime/metrics"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

// Bundle is the runtime wiring produced by Build and referenced from live handles.
type Bundle struct {
	Dir                  string
	FolderPath           string
	RuntimeInstanceID    string
	BackendScopeID       string
	StartedAtUTC         time.Time
	EventHistory         factory.HostedLedger
	Factory              factory.Factory
	InputFiles           factory.InputFileSystem
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
}

// NewBundle constructs one inert runtime host from direct collaborators.
func NewBundle(
	dir string,
	folderPath string,
	runtimeInstanceID string,
	backendScopeID string,
	startedAtUTC time.Time,
	eventHistory factory.HostedLedger,
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
		BackendScopeID: backendScopeID, StartedAtUTC: startedAtUTC,
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

func (r *Bundle) CloseArtifacts() error {
	if r == nil {
		return nil
	}
	return CloseBundleSinks(r.LogSink, r.MetricsSink)
}
