package factory

import (
	"context"
	"reflect"
	"testing"
	"time"
)

type runtimeMetricWriterFake struct {
	records []any
	closed  bool
}

func (f *runtimeMetricWriterFake) WriteMetric(_ context.Context, record RuntimeMetricRecord) error {
	f.records = append(f.records, record)
	return nil
}
func (f *runtimeMetricWriterFake) Close() error {
	f.closed = true
	return nil
}

func TestRuntimeMetricsSinkOwnsVocabularyAndCorrelationProjection(t *testing.T) {
	t.Parallel()
	writer := &runtimeMetricWriterFake{}
	now := time.Date(2026, 7, 20, 12, 30, 0, 123, time.UTC)
	artifact := RuntimeMetricsArtifact{
		Path: "metrics.jsonl", RootDir: "metrics", StartTimeUTC: now,
	}
	sink, err := NewRuntimeMetricsSink(
		writer,
		RuntimeMetricsScope{
			SessionID: "~default", RuntimeInstanceID: "runtime-1",
			FolderPath: "/factory/folder", FactoryDir: "/factory",
		},
		func() time.Time { return now },
		artifact,
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsSink: %v", err)
	}
	fields := Fields{
		DispatchID: "dispatch-1", WorkID: "work-1", TraceID: "trace-1",
		Workstation: "review", WorkerType: "agent", Provider: "codex",
		Outcome: "complete", Reason: "ok",
	}
	if err := sink.Sample(context.Background(), RuntimeDispatchDuration, 42.5, "ms", fields); err != nil {
		t.Fatalf("Sample: %v", err)
	}
	if len(writer.records) != 1 {
		t.Fatalf("records = %#v, want one", writer.records)
	}
	record, ok := writer.records[0].(RuntimeMetricRecord)
	if !ok {
		t.Fatalf("record type = %T, want RuntimeMetricRecord", writer.records[0])
	}
	want := RuntimeMetricRecord{
		Timestamp: now.Format(time.RFC3339Nano), MetricName: RuntimeDispatchDuration,
		MetricType: RuntimeMetricTypeSample, Value: 42.5, Unit: "ms",
		SessionID: "~default", RuntimeInstanceID: "runtime-1",
		FolderPath: "/factory/folder", FactoryDir: "/factory",
		DispatchID: "dispatch-1", WorkID: "work-1", TraceID: "trace-1",
		Workstation: "review", WorkerType: "agent", Provider: "codex",
		Outcome: "complete", Reason: "ok",
	}
	if !reflect.DeepEqual(record, want) {
		t.Fatalf("record = %#v, want %#v", record, want)
	}
	if sink.Path() != artifact.Path || sink.Artifact() != artifact {
		t.Fatalf("artifact = %#v, want %#v", sink.Artifact(), artifact)
	}
	if err := sink.Close(); err != nil || !writer.closed {
		t.Fatalf("Close = %v, closed=%v", err, writer.closed)
	}
}

func TestRuntimeMetricsSinkProjectsCounterAndGaugeKinds(t *testing.T) {
	t.Parallel()
	writer := &runtimeMetricWriterFake{}
	sink, err := NewRuntimeMetricsSink(
		writer, RuntimeMetricsScope{}, func() time.Time { return time.Unix(0, 0) },
		RuntimeMetricsArtifact{},
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsSink: %v", err)
	}
	if err := sink.Counter(context.Background(), "counter", 1, Fields{}); err != nil {
		t.Fatalf("Counter: %v", err)
	}
	if err := sink.Gauge(context.Background(), "gauge", 2, Fields{}); err != nil {
		t.Fatalf("Gauge: %v", err)
	}
	if writer.records[0].(RuntimeMetricRecord).MetricType != RuntimeMetricTypeCounter ||
		writer.records[1].(RuntimeMetricRecord).MetricType != RuntimeMetricTypeGauge {
		t.Fatalf("records = %#v, want counter then gauge", writer.records)
	}
}
