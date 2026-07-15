package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	factorymetrics "github.com/portpowered/infinite-you/pkg/factory/metrics"
	"github.com/portpowered/infinite-you/pkg/platform/internal/runtimeartifact"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	metricsMetricTypeCounter     = "counter"
	metricsMetricTypeGauge       = "gauge"
	metricsMetricTypeSample      = "sample"
	defaultRuntimeMetricsMaxSize = 100
	defaultRuntimeMetricsBackups = 20
	defaultRuntimeMetricsMaxAge  = 30
)

// RuntimeMetricsConfig controls rolling-file policy for runtime metrics.
// Values are in megabytes (MaxSize) and days (MaxAge).
type RuntimeMetricsConfig struct {
	// MaxSize sets the maximum size in megabytes before rotation.
	MaxSize int
	// MaxBackups controls how many backup files are retained.
	MaxBackups int
	// MaxAge controls how many days backup files are retained.
	MaxAge int
	// Compress enables gzip compression for rotated files.
	Compress bool
}

// RuntimeMetricsSink owns the file-backed JSONL metrics emitter and rolling
// writer for one live runtime/session bundle.
type RuntimeMetricsSink struct {
	mu                sync.Mutex
	writer            io.Closer
	encoder           *json.Encoder
	path              string
	rootDir           string
	startTimeUTC      time.Time
	config            RuntimeMetricsConfig
	sessionID         string
	runtimeInstanceID string
	folderPath        string
	factoryDir        string
	closed            bool
}

var errRuntimeMetricsSinkClosed = errors.New("runtime metrics sink closed")

// DefaultRuntimeMetricsConfig returns the production rolling-file policy used
// when callers do not set explicit runtime metrics limits.
func DefaultRuntimeMetricsConfig() RuntimeMetricsConfig {
	return RuntimeMetricsConfig{
		MaxSize:    defaultRuntimeMetricsMaxSize,
		MaxBackups: defaultRuntimeMetricsBackups,
		MaxAge:     defaultRuntimeMetricsMaxAge,
	}
}

// BuildRuntimeMetricsSink creates a bounded rolling JSONL metrics sink.
func BuildRuntimeMetricsSink(
	sessionID string,
	runtimeInstanceID string,
	folderPath string,
	factoryDir string,
	metricsDir string,
	config RuntimeMetricsConfig,
) (*RuntimeMetricsSink, error) {
	if runtimeInstanceID == "" {
		return nil, fmt.Errorf("runtime instance ID is required")
	}
	if metricsDir == "" {
		dir, err := defaultRuntimeMetricsDir()
		if err != nil {
			return nil, err
		}
		metricsDir = dir
	}

	startTimeUTC := time.Now().UTC()
	path, err := runtimeartifact.ReserveAvailablePath(
		metricsDir,
		startTimeUTC,
		defaultpaths.RuntimeArtifactKindMetrics,
		defaultpaths.RuntimeArtifactPathComponents(sessionID, runtimeInstanceID, uuid.NewString()),
	)
	if err != nil {
		return nil, err
	}

	metricsConfig := normalizeRuntimeMetricsConfig(config)
	writer := newRuntimeMetricsWriter(&lumberjack.Logger{
		Filename:   path,
		MaxSize:    metricsConfig.MaxSize,
		MaxBackups: metricsConfig.MaxBackups,
		MaxAge:     metricsConfig.MaxAge,
		Compress:   metricsConfig.Compress,
	})

	return &RuntimeMetricsSink{
		writer:            writer,
		encoder:           json.NewEncoder(writer),
		path:              path,
		rootDir:           metricsDir,
		startTimeUTC:      startTimeUTC,
		config:            metricsConfig,
		sessionID:         sessionID,
		runtimeInstanceID: runtimeInstanceID,
		folderPath:        folderPath,
		factoryDir:        factoryDir,
	}, nil
}

// Counter records a monotonic increment.
func (s *RuntimeMetricsSink) Counter(ctx context.Context, name string, delta float64, fields factorymetrics.Fields) error {
	return s.emit(ctx, name, metricsMetricTypeCounter, delta, "", fields)
}

// Gauge records a point-in-time level.
func (s *RuntimeMetricsSink) Gauge(ctx context.Context, name string, value float64, fields factorymetrics.Fields) error {
	return s.emit(ctx, name, metricsMetricTypeGauge, value, "", fields)
}

// Sample records a measured value.
func (s *RuntimeMetricsSink) Sample(ctx context.Context, name string, value float64, unit string, fields factorymetrics.Fields) error {
	return s.emit(ctx, name, metricsMetricTypeSample, value, unit, fields)
}

// Path returns the active runtime metrics path.
func (s *RuntimeMetricsSink) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// RootDir returns the runtime metrics root selected by caller configuration or
// the default home-directory policy.
func (s *RuntimeMetricsSink) RootDir() string {
	if s == nil {
		return ""
	}
	return s.rootDir
}

// StartTimeUTC returns the UTC timestamp used to organize the active metrics path.
func (s *RuntimeMetricsSink) StartTimeUTC() time.Time {
	if s == nil {
		return time.Time{}
	}
	return s.startTimeUTC
}

// Config returns the normalized rolling-file policy applied to the sink.
func (s *RuntimeMetricsSink) Config() RuntimeMetricsConfig {
	if s == nil {
		return RuntimeMetricsConfig{}
	}
	return s.config
}

// Close releases the runtime metrics writer.
func (s *RuntimeMetricsSink) Close() error {
	if s == nil || s.writer == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.writer.Close()
}

func (s *RuntimeMetricsSink) emit(ctx context.Context, name string, metricType string, value float64, unit string, fields factorymetrics.Fields) error {
	if s == nil {
		return nil
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errRuntimeMetricsSinkClosed
	}
	return s.encoder.Encode(s.newRecord(name, metricType, value, unit, fields))
}

func (s *RuntimeMetricsSink) newRecord(name string, metricType string, value float64, unit string, fields factorymetrics.Fields) runtimeMetricsRecord {
	return runtimeMetricsRecord{
		Timestamp:         time.Now().UTC().Format(time.RFC3339Nano),
		MetricName:        name,
		MetricType:        metricType,
		Value:             value,
		Unit:              unit,
		SessionID:         s.sessionID,
		RuntimeInstanceID: s.runtimeInstanceID,
		FolderPath:        s.folderPath,
		FactoryDir:        s.factoryDir,
		DispatchID:        fields.DispatchID,
		WorkID:            fields.WorkID,
		TraceID:           fields.TraceID,
		Workstation:       fields.Workstation,
		WorkerType:        fields.WorkerType,
		Provider:          fields.Provider,
		Outcome:           fields.Outcome,
		Reason:            fields.Reason,
	}
}

type runtimeMetricsRecord struct {
	Timestamp         string  `json:"ts"`
	MetricName        string  `json:"metric_name"`
	MetricType        string  `json:"metric_type"`
	Value             float64 `json:"value"`
	Unit              string  `json:"unit"`
	SessionID         string  `json:"session_id"`
	RuntimeInstanceID string  `json:"runtime_instance_id"`
	FolderPath        string  `json:"folder_path"`
	FactoryDir        string  `json:"factory_dir"`
	DispatchID        string  `json:"dispatch_id,omitempty"`
	WorkID            string  `json:"work_id,omitempty"`
	TraceID           string  `json:"trace_id,omitempty"`
	Workstation       string  `json:"workstation,omitempty"`
	WorkerType        string  `json:"worker_type,omitempty"`
	Provider          string  `json:"provider,omitempty"`
	Outcome           string  `json:"outcome,omitempty"`
	Reason            string  `json:"reason,omitempty"`
}

type runtimeMetricsWriter struct {
	mu     sync.Mutex
	writer *lumberjack.Logger
	closed bool
}

func newRuntimeMetricsWriter(writer *lumberjack.Logger) *runtimeMetricsWriter {
	return &runtimeMetricsWriter{writer: writer}
}

func (w *runtimeMetricsWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, errRuntimeMetricsSinkClosed
	}
	return w.writer.Write(p)
}

func (w *runtimeMetricsWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	return w.writer.Close()
}
func normalizeRuntimeMetricsConfig(config RuntimeMetricsConfig) RuntimeMetricsConfig {
	if config.MaxSize <= 0 {
		config.MaxSize = DefaultRuntimeMetricsConfig().MaxSize
	}
	if config.MaxBackups < 0 {
		config.MaxBackups = 0
	}
	if config.MaxAge < 0 {
		config.MaxAge = 0
	}
	if config.MaxBackups == 0 && config.MaxAge == 0 {
		defaults := DefaultRuntimeMetricsConfig()
		config.MaxBackups = defaults.MaxBackups
		config.MaxAge = defaults.MaxAge
	}
	return config
}

func defaultRuntimeMetricsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home for runtime metrics: %w", err)
	}
	return defaultpaths.RuntimeMetricsRoot(home), nil
}

var _ factorymetrics.MetricsEmitter = (*RuntimeMetricsSink)(nil)
