package runtime_metrics_test

import (
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	platformartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

// TestRuntimeMetricsPublicArtifactsRetainAndStreamThroughPlatformContracts
// proves the operator-facing metrics artifact lifecycle: stale artifacts are
// pruned before a live sink is reserved, multiple sinks share one retention
// loop, and active plus compressed artifacts can be read through the public
// streaming boundary with envelope selection.
func TestRuntimeMetricsPublicArtifactsRetainAndStreamThroughPlatformContracts(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, time.August, 24, 12, 30, 0, 0, time.UTC)
	stalePath := filepath.Join(
		root, "2026", "08", "01",
		"120000.000000000-runtime-metrics-stale-runtime.log",
	)
	if err := os.MkdirAll(filepath.Dir(stalePath), 0o755); err != nil {
		t.Fatalf("create stale metrics directory: %v", err)
	}
	if err := os.WriteFile(stalePath, []byte(`{"metric_name":"stale"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write stale metrics artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "operator-notes.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatalf("write unrelated metrics-root file: %v", err)
	}

	retention, err := platformmetrics.NewRuntimeMetricsRetention(
		platformfilesystem.Local{},
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("construct runtime metrics retention: %v", err)
	}
	var reports []platformmetrics.RuntimeMetricsRetentionReport
	scheduler, err := platformmetrics.NewRuntimeMetricsRetentionScheduler(
		retention,
		nil,
		func(report platformmetrics.RuntimeMetricsRetentionReport, sweepErr error) {
			if sweepErr != nil {
				t.Errorf("runtime metrics startup sweep: %v", sweepErr)
			}
			reports = append(reports, report)
		},
	)
	if err != nil {
		t.Fatalf("construct runtime metrics retention scheduler: %v", err)
	}
	paths, err := platformartifact.NewReserver(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("construct runtime artifact reserver: %v", err)
	}
	opener, err := platformmetrics.NewRuntimeMetricsOpener(
		paths,
		scheduler,
	)
	if err != nil {
		t.Fatalf("construct runtime metrics opener: %v", err)
	}

	config := platformmetrics.RuntimeMetricsConfig{MaxAge: 1, MaxSize: 1, MaxBackups: 2}
	first, err := opener.Open(platformmetrics.RuntimeMetricsOpeningRequest{
		SessionID:         "metrics-functional-session",
		RuntimeInstanceID: "metrics-functional-runtime",
		RootDirectory:     root,
		StartTimeUTC:      now,
		CollisionID:       "first",
		Config:            config,
	})
	if err != nil {
		t.Fatalf("open first runtime metrics sink: %v", err)
	}
	second, err := opener.Open(platformmetrics.RuntimeMetricsOpeningRequest{
		SessionID:         "metrics-functional-session",
		RuntimeInstanceID: "metrics-functional-runtime",
		RootDirectory:     root,
		StartTimeUTC:      now,
		CollisionID:       "second",
		Config:            config,
	})
	if err != nil {
		_ = first.Close()
		t.Fatalf("open shared-root runtime metrics sink: %v", err)
	}

	if first.RootDir() != root || first.StartTimeUTC() != now || first.Config().MaxAge != 1 {
		t.Fatalf("first sink metadata = root:%q start:%s config:%#v, want selected public values", first.RootDir(), first.StartTimeUTC(), first.Config())
	}
	if err := first.WriteMetric(context.Background(), map[string]any{
		"metric_name": "dispatch.completed",
		"value":       1,
	}); err != nil {
		t.Fatalf("write first runtime metric: %v", err)
	}
	if err := second.WriteMetric(context.Background(), map[string]any{
		"metric_name": "provider.completed",
		"value":       2,
	}); err != nil {
		t.Fatalf("write second runtime metric: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close second runtime metrics sink: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first runtime metrics sink: %v", err)
	}
	if err := opener.Close(context.Background()); err != nil {
		t.Fatalf("close runtime metrics opener: %v", err)
	}

	if len(reports) != 1 || reports[0].Removed.Files != 1 || reports[0].Skipped {
		t.Fatalf("runtime metrics startup reports = %#v, want one non-skipped sweep removing the stale artifact", reports)
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("stale metrics artifact stat error = %v, want removed", err)
	}
	if _, err := os.Stat(filepath.Join(root, "operator-notes.txt")); err != nil {
		t.Fatalf("unrecognized root file was not preserved: %v", err)
	}

	backupPath := filepath.Join(
		root, "2026", "08", "24",
		"123000.000000000-runtime-metrics-backup-runtime-2026-08-24T12-30-00.000.log.gz",
	)
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		t.Fatalf("create compressed metrics directory: %v", err)
	}
	backupFile, err := os.Create(backupPath)
	if err != nil {
		t.Fatalf("create compressed metrics artifact: %v", err)
	}
	compressor := gzip.NewWriter(backupFile)
	if _, err := compressor.Write([]byte(`{"metric_name":"provider.completed","value":3}` + "\n")); err != nil {
		_ = compressor.Close()
		_ = backupFile.Close()
		t.Fatalf("write compressed metrics artifact: %v", err)
	}
	if err := compressor.Close(); err != nil {
		_ = backupFile.Close()
		t.Fatalf("close compressed metrics writer: %v", err)
	}
	if err := backupFile.Close(); err != nil {
		t.Fatalf("close compressed metrics artifact: %v", err)
	}

	reader, err := platformmetrics.NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("construct runtime metrics reader: %v", err)
	}
	records, err := reader.Read(context.Background(), root)
	if err != nil {
		t.Fatalf("read public runtime metrics artifacts: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("read public runtime metrics records = %d, want active two plus compressed one", len(records))
	}

	stats := &platformmetrics.RuntimeMetricsReadStats{}
	selected := make([]platformmetrics.RuntimeMetricRecord, 0, 2)
	err = reader.StreamSelected(context.Background(), root, platformmetrics.StreamSelection{
		EnvelopeFields: []string{"metric_name"},
		IncludeEnvelope: func(envelope platformmetrics.RuntimeMetricRecordEnvelope) bool {
			return envelope.Fields["metric_name"] == "provider.completed"
		},
		Stats: stats,
	}, func(record platformmetrics.RuntimeMetricRecord) error {
		selected = append(selected, record)
		return nil
	})
	if err != nil {
		t.Fatalf("stream selected public runtime metrics: %v", err)
	}
	if len(selected) != 2 || stats.ArtifactsOpened != 3 || stats.RecordsDecoded != 2 || stats.BytesRead == 0 {
		t.Fatalf("selected metrics = %d, stats = %#v, want two selected records from three artifacts", len(selected), stats)
	}

}
