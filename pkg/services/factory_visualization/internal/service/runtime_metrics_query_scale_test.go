package service_test

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryvisualizationwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/wire"
)

const (
	scaleSmallArtifactCount = 500
	scaleLargeArtifactCount = 2000
	scaleProbeCheckpoint    = 128
	scaleProbeField         = "__runtime_metrics_scale_probe"
)

func TestRuntimeMetricsQueryBoundsDecodedRecordLifetimeAcrossArtifactScale(t *testing.T) {
	smallRoot := installScaleMetricsTree(t, scaleSmallArtifactCount)
	largeRoot := installScaleMetricsTree(t, scaleLargeArtifactCount)

	smallQuery, smallReader := newScaleMetricsQuery(t)
	largeQuery, largeReader := newScaleMetricsQuery(t)

	small, err := smallQuery.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot: smallRoot,
	})
	if err != nil {
		t.Fatalf("small QueryRuntimeMetrics() error = %v", err)
	}
	large, err := largeQuery.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot: largeRoot,
	})
	if err != nil {
		t.Fatalf("large QueryRuntimeMetrics() error = %v", err)
	}

	if !reflect.DeepEqual(large, small) {
		t.Fatalf("large result = %#v, want exact small-tree result %#v", large, small)
	}
	smallObservation := assertScaleObservation(t, smallReader, scaleSmallArtifactCount, small)
	largeObservation := assertScaleObservation(t, largeReader, scaleLargeArtifactCount, large)
	if smallObservation.outputRows != largeObservation.outputRows {
		t.Fatalf("fixed output cardinality = small %d, large %d; want equal cardinality", smallObservation.outputRows, largeObservation.outputRows)
	}
	if smallObservation.peakLiveRecords == 0 || largeObservation.peakLiveRecords == 0 {
		t.Fatalf("peak live decoded records = small %d, large %d; want measured non-zero proxy", smallObservation.peakLiveRecords, largeObservation.peakLiveRecords)
	}

	proxyRatio := float64(largeObservation.peakLiveRecords) / float64(smallObservation.peakLiveRecords)
	t.Logf(
		"runtime metrics scale proof: small artifacts=%d decoded=%d output_rows=%d peak_live_decoded_records=%d max_retained_probes=%d; large artifacts=%d decoded=%d output_rows=%d peak_live_decoded_records=%d max_retained_probes=%d; measurement=maximum decoded records simultaneously crossing the production reader/query callback boundary, with finalizer checkpoints for prior payload reachability; larger/smaller peak-live proxy ratio=%.2f",
		smallObservation.artifacts,
		smallObservation.decodedRecords,
		smallObservation.outputRows,
		smallObservation.peakLiveRecords,
		smallObservation.maxRetainedProbes,
		largeObservation.artifacts,
		largeObservation.decodedRecords,
		largeObservation.outputRows,
		largeObservation.peakLiveRecords,
		largeObservation.maxRetainedProbes,
		proxyRatio,
	)
	if proxyRatio > 1.5 {
		t.Fatalf("larger/smaller peak-live decoded-record proxy ratio = %.2f (small artifacts=%d peak=%d, large artifacts=%d peak=%d), want <= 1.5", proxyRatio, smallObservation.artifacts, smallObservation.peakLiveRecords, largeObservation.artifacts, largeObservation.peakLiveRecords)
	}
}

type scaleObservation struct {
	artifacts           int
	decodedRecords      int
	outputRows          int
	peakLiveRecords     int
	maxRetainedProbes   int
	streamSelectedCalls int
}

func assertScaleObservation(
	t *testing.T,
	reader *scaleMetricsReader,
	wantArtifacts int,
	result factoryvisualization.RuntimeMetricsQueryResult,
) scaleObservation {
	t.Helper()
	if reader.streamSelectedCalls != 1 {
		t.Fatalf("production reader capability calls = %d, want one SelectedReader stream", reader.streamSelectedCalls)
	}
	if reader.liveRecords != 0 {
		t.Fatalf("decoded records still crossing reader/query boundary after query = %d, want zero", reader.liveRecords)
	}
	if !reader.waitForFinalizers(reader.delivered) {
		t.Fatalf("decoded record payload finalizers = %d, want all %d delivered payloads released after query", reader.finalized.Load(), reader.delivered)
	}
	if reader.stats.ArtifactsVisited != wantArtifacts || reader.stats.ArtifactsOpened != wantArtifacts {
		t.Fatalf("artifact traversal stats = visited %d opened %d, want %d valid artifacts visited and opened", reader.stats.ArtifactsVisited, reader.stats.ArtifactsOpened, wantArtifacts)
	}
	if reader.stats.RecordsDecoded != wantArtifacts || int(reader.delivered) != wantArtifacts {
		t.Fatalf("decoded-record stats = reader %d delivered %d, want %d records", reader.stats.RecordsDecoded, reader.delivered, wantArtifacts)
	}
	if reader.maxRetainedProbes > 1 {
		t.Fatalf("decoded payloads retained at a checkpoint = %d, want at most the current callback record", reader.maxRetainedProbes)
	}
	return scaleObservation{
		artifacts:           reader.stats.ArtifactsVisited,
		decodedRecords:      reader.stats.RecordsDecoded,
		outputRows:          runtimeMetricsOutputRowCardinality(result),
		peakLiveRecords:     reader.peakLiveRecords,
		maxRetainedProbes:   reader.maxRetainedProbes,
		streamSelectedCalls: reader.streamSelectedCalls,
	}
}

func runtimeMetricsOutputRowCardinality(result factoryvisualization.RuntimeMetricsQueryResult) int {
	return len(result.Workstations) + len(result.WorkerTypes) + len(result.Providers) + len(result.UsageRows)
}

func installScaleMetricsTree(t *testing.T, artifactCount int) string {
	t.Helper()
	root := t.TempDir()
	writeRuntimeMetricsJSONL(
		t,
		filepath.Join(root, "120000.000000000-runtime-metrics-scale-session-scale-runtime.log"),
		[]factoryvisualization.RuntimeMetricRecord{
			metricRecord("provider.input_tokens", 7, "scale-session", "scale-runtime", "scale-workstation", "scale-worker", "scale-provider", "", "tokens"),
		},
		"",
	)
	for index := 1; index < artifactCount; index++ {
		writeRuntimeMetricsJSONL(
			t,
			filepath.Join(root, fmt.Sprintf("120001.000000000-runtime-metrics-scale-session-scale-runtime-2026-08-20T12-01-00.000-%d.log", index)),
			[]factoryvisualization.RuntimeMetricRecord{{
				"metric_name": "unknown.scale.filler",
				"value":       1,
			}},
			"",
		)
	}
	return root
}

func newScaleMetricsQuery(t *testing.T) (factoryvisualization.RuntimeMetricsQuery, *scaleMetricsReader) {
	t.Helper()
	reader, err := platformmetrics.NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader() error = %v", err)
	}
	tracked := &scaleMetricsReader{delegate: reader}
	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(tracked, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}
	return query, tracked
}

type scaleMetricsReader struct {
	delegate            *platformmetrics.RuntimeMetricsReader
	stats               platformmetrics.RuntimeMetricsReadStats
	delivered           int64
	finalized           atomic.Int64
	liveRecords         int
	peakLiveRecords     int
	maxRetainedProbes   int
	streamSelectedCalls int
}

func (reader *scaleMetricsReader) Read(ctx context.Context, root string) ([]platformmetrics.RuntimeMetricRecord, error) {
	return reader.delegate.Read(ctx, root)
}

func (reader *scaleMetricsReader) StreamSelected(
	ctx context.Context,
	root string,
	selection platformmetrics.StreamSelection,
	visit func(platformmetrics.RuntimeMetricRecord) error,
) error {
	reader.streamSelectedCalls++
	selection.Stats = &reader.stats
	return reader.delegate.StreamSelected(ctx, root, selection, func(record platformmetrics.RuntimeMetricRecord) error {
		probe := &scaleDecodedRecordProbe{}
		runtime.SetFinalizer(probe, func(*scaleDecodedRecordProbe) {
			reader.finalized.Add(1)
		})
		record[scaleProbeField] = probe
		reader.delivered++
		reader.liveRecords++
		if reader.liveRecords > reader.peakLiveRecords {
			reader.peakLiveRecords = reader.liveRecords
		}
		err := visit(record)
		reader.liveRecords--
		if err != nil {
			return err
		}
		if reader.delivered%scaleProbeCheckpoint == 0 {
			if err := reader.assertPriorRecordsReleased(); err != nil {
				return err
			}
		}
		runtime.KeepAlive(record)
		return nil
	})
}

func (reader *scaleMetricsReader) assertPriorRecordsReleased() error {
	wantFinalized := reader.delivered - 1
	if !reader.waitForFinalizers(wantFinalized) {
		return fmt.Errorf("decoded record payload retention checkpoint: delivered=%d finalized=%d want at least %d before the current callback returns", reader.delivered, reader.finalized.Load(), wantFinalized)
	}
	retained := int(reader.delivered - reader.finalized.Load())
	if retained > reader.maxRetainedProbes {
		reader.maxRetainedProbes = retained
	}
	return nil
}

func (reader *scaleMetricsReader) waitForFinalizers(want int64) bool {
	if want <= 0 {
		return true
	}
	deadline := time.Now().Add(3 * time.Second)
	for reader.finalized.Load() < want && time.Now().Before(deadline) {
		// Finalizer reachability is the direct test-only observation of whether
		// a decoded record map remains reachable while the stream continues.
		// The bounded retry is necessary because finalizers run asynchronously;
		// no production synchronization can replace this GC checkpoint.
		runtime.GC()
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	return reader.finalized.Load() >= want
}

type scaleDecodedRecordProbe struct {
	marker byte
}
