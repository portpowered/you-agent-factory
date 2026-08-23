package factory

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	RuntimeMetricTypeCounter = "counter"
	RuntimeMetricTypeGauge   = "gauge"
	RuntimeMetricTypeSample  = "sample"
)

// RuntimeMetricRecord is the Factory Runtime-owned durable metric vocabulary.
type RuntimeMetricRecord struct {
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
	WorkerSessionID   string  `json:"worker_session_id,omitempty"`
	Provider          string  `json:"provider,omitempty"`
	Model             string  `json:"model,omitempty"`
	Outcome           string  `json:"outcome,omitempty"`
	Reason            string  `json:"reason,omitempty"`
}

// RuntimeMetricRecordWriter is the exact policy-free serialization effect
// required by Factory Runtime metrics projection.
type RuntimeMetricRecordWriter interface {
	WriteMetric(context.Context, RuntimeMetricRecord) error
	io.Closer
}

type RuntimeMetricsArtifact struct {
	Path         string
	RootDir      string
	StartTimeUTC time.Time
}

// RuntimeMetricsSink is one runtime-scoped emitter and owned artifact.
type RuntimeMetricsSink interface {
	MetricsEmitter
	io.Closer
	Path() string
	Artifact() RuntimeMetricsArtifact
}

// RuntimeMetricsScope supplies stable correlation attached to every record.
type RuntimeMetricsScope struct {
	SessionID         string
	RuntimeInstanceID string
	FolderPath        string
	FactoryDir        string
}

// RuntimeMetricsStorageConfig is the bounded rolling-file request passed to
// the Wire-selected storage adapter.
type RuntimeMetricsStorageConfig struct {
	MaxSize    int
	MaxBackups int
	MaxAge     int
	Compress   bool
}

// RuntimeMetricsScopeRequest contains the value selections for one private
// metrics scope. The owner retains the clock, ID, path, and filesystem
// effects; callers provide only correlation values and destination policy.
type RuntimeMetricsScopeRequest struct {
	Scope         RuntimeMetricsScope
	RootDirectory string
	Policy        RuntimeMetricsPolicy
	Config        RuntimeMetricsStorageConfig
}

// RuntimeMetricsOwner is the process-scoped observability root for runtime
// metrics. Open returns one operation-private sink; the owner remains alive
// across session finishes and is closed with the owning application process.
type RuntimeMetricsOwner interface {
	Open(RuntimeMetricsScopeRequest) (RuntimeMetricsSink, error)
}

type projectedRuntimeMetricsSink struct {
	writer   RuntimeMetricRecordWriter
	scope    RuntimeMetricsScope
	now      func() time.Time
	artifact RuntimeMetricsArtifact
}

// NewRuntimeMetricsSink owns projection from logical runtime measurements to
// the durable Factory Runtime metric record.
func NewRuntimeMetricsSink(
	writer RuntimeMetricRecordWriter,
	scope RuntimeMetricsScope,
	now func() time.Time,
	artifact RuntimeMetricsArtifact,
) (RuntimeMetricsSink, error) {
	if writer == nil {
		return nil, fmt.Errorf("runtime metric record writer is required")
	}
	if now == nil {
		return nil, fmt.Errorf("runtime metrics clock is required")
	}
	return &projectedRuntimeMetricsSink{
		writer: writer, scope: scope, now: now, artifact: artifact,
	}, nil
}

func (s *projectedRuntimeMetricsSink) Counter(
	ctx context.Context, name string, value float64, fields Fields,
) error {
	return s.emit(ctx, name, RuntimeMetricTypeCounter, value, "", fields)
}

func (s *projectedRuntimeMetricsSink) Gauge(
	ctx context.Context, name string, value float64, fields Fields,
) error {
	return s.emit(ctx, name, RuntimeMetricTypeGauge, value, "", fields)
}

func (s *projectedRuntimeMetricsSink) Sample(
	ctx context.Context, name string, value float64, unit string, fields Fields,
) error {
	return s.emit(ctx, name, RuntimeMetricTypeSample, value, unit, fields)
}

func (s *projectedRuntimeMetricsSink) emit(
	ctx context.Context,
	name string,
	metricType string,
	value float64,
	unit string,
	fields Fields,
) error {
	return s.writer.WriteMetric(ctx, RuntimeMetricRecord{
		Timestamp:         s.now().UTC().Format(time.RFC3339Nano),
		MetricName:        name,
		MetricType:        metricType,
		Value:             value,
		Unit:              unit,
		SessionID:         s.scope.SessionID,
		RuntimeInstanceID: s.scope.RuntimeInstanceID,
		FolderPath:        s.scope.FolderPath,
		FactoryDir:        s.scope.FactoryDir,
		DispatchID:        fields.DispatchID,
		WorkID:            fields.WorkID,
		TraceID:           fields.TraceID,
		Workstation:       fields.Workstation,
		WorkerType:        fields.WorkerType,
		WorkerSessionID:   fields.WorkerSessionID,
		Provider:          normalizedRuntimeMetricProvider(fields.Provider),
		Model:             fields.Model,
		Outcome:           fields.Outcome,
		Reason:            fields.Reason,
	})
}

func normalizedRuntimeMetricProvider(provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" || strings.Contains(provider, "${") {
		return ""
	}
	return provider
}

func (s *projectedRuntimeMetricsSink) Path() string                     { return s.artifact.Path }
func (s *projectedRuntimeMetricsSink) Artifact() RuntimeMetricsArtifact { return s.artifact }
func (s *projectedRuntimeMetricsSink) Close() error                     { return s.writer.Close() }
