package runtime_metrics_test

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

func TestRuntimeMetricsPublicReaderReportsMalformedAndCancelledReads(t *testing.T) {
	reader, err := platformmetrics.NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("construct runtime metrics reader: %v", err)
	}
	if _, err := platformmetrics.NewRuntimeMetricsReader(nil); err == nil {
		t.Fatal("NewRuntimeMetricsReader(nil) error = nil, want required-filesystem error")
	}

	t.Run("invalidRequests", func(t *testing.T) {
		var nilReader *platformmetrics.RuntimeMetricsReader
		if err := nilReader.Stream(context.Background(), t.TempDir(), func(platformmetrics.RuntimeMetricRecord) error { return nil }); err == nil {
			t.Fatal("nil reader Stream error = nil, want typed read error")
		}
		if err := reader.Stream(context.Background(), t.TempDir(), nil); err == nil {
			t.Fatal("nil visitor Stream error = nil, want typed read error")
		}
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		if err := reader.Stream(cancelled, t.TempDir(), func(platformmetrics.RuntimeMetricRecord) error { return nil }); err == nil {
			t.Fatal("cancelled Stream error = nil, want context cancellation")
		} else if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled Stream error = %v, want context.Canceled", err)
		}
		if err := reader.Stream(context.Background(), "", func(platformmetrics.RuntimeMetricRecord) error { return nil }); err == nil {
			t.Fatal("empty-root Stream error = nil, want root validation error")
		}
		file := filepath.Join(t.TempDir(), "not-a-directory")
		if err := os.WriteFile(file, []byte("file"), 0o600); err != nil {
			t.Fatalf("write non-directory root: %v", err)
		}
		if err := reader.Stream(context.Background(), file, func(platformmetrics.RuntimeMetricRecord) error { return nil }); err == nil {
			t.Fatal("file-root Stream error = nil, want not-a-directory error")
		}
	})

	t.Run("malformedArtifact", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "2026", "08", "24", "123000.000000000-runtime-metrics-malformed-runtime.log")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create malformed artifact directory: %v", err)
		}
		if err := os.WriteFile(path, []byte("not-json\n"), 0o600); err != nil {
			t.Fatalf("write malformed artifact: %v", err)
		}
		var typedErr *platformmetrics.RuntimeMetricsReadError
		if err := reader.Stream(context.Background(), root, func(platformmetrics.RuntimeMetricRecord) error { return nil }); err == nil {
			t.Fatal("malformed artifact Stream error = nil, want decode error")
		} else if !errors.As(err, &typedErr) || typedErr.Path != path {
			t.Fatalf("malformed artifact error = %v, want typed error at %q", err, path)
		}
	})

	t.Run("nullAndGzipArtifacts", func(t *testing.T) {
		root := t.TempDir()
		dateDir := filepath.Join(root, "2026", "08", "24")
		if err := os.MkdirAll(dateDir, 0o755); err != nil {
			t.Fatalf("create null/gzip artifact directory: %v", err)
		}
		nullPath := filepath.Join(dateDir, "123000.000000000-runtime-metrics-null-runtime.log")
		if err := os.WriteFile(nullPath, []byte("null\n"), 0o600); err != nil {
			t.Fatalf("write null artifact: %v", err)
		}
		if err := reader.Stream(context.Background(), root, func(platformmetrics.RuntimeMetricRecord) error { return nil }); err == nil {
			t.Fatal("null artifact Stream error = nil, want object-shape error")
		}
		gzipPath := filepath.Join(dateDir, "123001.000000000-runtime-metrics-invalid-runtime.log.gz")
		if err := os.WriteFile(gzipPath, []byte("not-gzip"), 0o600); err != nil {
			t.Fatalf("write invalid gzip artifact: %v", err)
		}
		if err := reader.Stream(context.Background(), root, func(platformmetrics.RuntimeMetricRecord) error { return nil }); err == nil {
			t.Fatal("invalid gzip Stream error = nil, want gzip decoder error")
		}
	})

	t.Run("selectionAndVisitorErrors", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "2026", "08", "24", "123000.000000000-runtime-metrics-selected-runtime.log")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create selected artifact directory: %v", err)
		}
		if err := os.WriteFile(path, []byte("{\"metric_name\":\"keep\",\"count\":1}\n{\"metric_name\":7}\n"), 0o600); err != nil {
			t.Fatalf("write selected artifact: %v", err)
		}
		stats := &platformmetrics.RuntimeMetricsReadStats{}
		var envelopes []platformmetrics.RuntimeMetricRecordEnvelope
		err = reader.StreamSelected(context.Background(), root, platformmetrics.StreamSelection{
			EnvelopeFields: []string{"metric_name", "missing"},
			IncludeEnvelope: func(envelope platformmetrics.RuntimeMetricRecordEnvelope) bool {
				envelopes = append(envelopes, envelope)
				return envelope.Fields["metric_name"] == "keep"
			},
			Stats: stats,
		}, func(platformmetrics.RuntimeMetricRecord) error { return nil })
		if err != nil {
			t.Fatalf("selected Stream error: %v", err)
		}
		if len(envelopes) != 2 || envelopes[0].Fields["metric_name"] != "keep" || len(envelopes[1].Fields) != 0 {
			t.Fatalf("selected envelopes = %#v, want string field then omitted non-string field", envelopes)
		}
		if stats.RecordsDecoded != 1 || stats.BytesRead == 0 || stats.ArtifactsOpened != 1 {
			t.Fatalf("selected stats = %#v, want one decoded record and one opened artifact", stats)
		}
		sentinel := errors.New("visitor stopped metrics read")
		if err := reader.Stream(context.Background(), root, func(platformmetrics.RuntimeMetricRecord) error { return sentinel }); !errors.Is(err, sentinel) {
			t.Fatalf("visitor error = %v, want %v", err, sentinel)
		}
	})
}

func TestRuntimeMetricsPublicOpenerValidatesRequestsAndSinkLifecycle(t *testing.T) {
	paths, err := platformartifact.NewReserver(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("construct runtime artifact reserver: %v", err)
	}
	retention, err := platformmetrics.NewRuntimeMetricsRetention(
		platformfilesystem.Local{}, func() time.Time { return time.Now().UTC() },
	)
	if err != nil {
		t.Fatalf("construct runtime metrics retention: %v", err)
	}
	scheduler, err := platformmetrics.NewRuntimeMetricsRetentionScheduler(retention, nil, nil)
	if err != nil {
		t.Fatalf("construct runtime metrics scheduler: %v", err)
	}
	opener, err := platformmetrics.NewRuntimeMetricsOpener(paths, scheduler)
	if err != nil {
		t.Fatalf("construct runtime metrics opener: %v", err)
	}
	if _, err := platformmetrics.NewRuntimeMetricsRetention(nil, func() time.Time { return time.Now() }); err == nil {
		t.Fatal("NewRuntimeMetricsRetention(nil, clock) error = nil, want required-filesystem error")
	}
	if _, err := platformmetrics.NewRuntimeMetricsRetention(platformfilesystem.Local{}, nil); err == nil {
		t.Fatal("NewRuntimeMetricsRetention(filesystem, nil) error = nil, want required-clock error")
	}
	coordination, err := platformmetrics.NewRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("construct opener coordination: %v", err)
	}
	if _, err := platformmetrics.NewRuntimeMetricsRetention(platformfilesystem.Local{}, func() time.Time { return time.Now() }, nil); err == nil {
		t.Fatal("NewRuntimeMetricsRetention(nil coordination) error = nil, want coordination error")
	}
	if _, err := platformmetrics.NewRuntimeMetricsRetention(platformfilesystem.Local{}, func() time.Time { return time.Now() }, coordination, coordination); err == nil {
		t.Fatal("NewRuntimeMetricsRetention(two coordination implementations) error = nil, want cardinality error")
	}
	if _, err := platformmetrics.NewRuntimeMetricsRetentionScheduler(nil, nil, nil); err == nil {
		t.Fatal("NewRuntimeMetricsRetentionScheduler(nil) error = nil, want required-retention error")
	}
	if _, err := platformmetrics.NewRuntimeMetricsOpener(nil, scheduler); err == nil {
		t.Fatal("NewRuntimeMetricsOpener(nil, scheduler) error = nil, want required-reserver error")
	}
	if _, err := platformmetrics.NewRuntimeMetricsOpener(paths, scheduler, nil); err == nil {
		t.Fatal("NewRuntimeMetricsOpener(nil coordination) error = nil, want coordination error")
	}
	if _, err := platformmetrics.NewRuntimeMetricsOpener(paths, scheduler, coordination, coordination); err == nil {
		t.Fatal("NewRuntimeMetricsOpener(two coordination implementations) error = nil, want cardinality error")
	}

	root := t.TempDir()
	at := time.Date(2026, time.August, 24, 12, 30, 0, 0, time.FixedZone("PDT", -7*60*60))
	base := platformmetrics.RuntimeMetricsOpeningRequest{
		SessionID: "session", RuntimeInstanceID: "runtime", RootDirectory: root,
		StartTimeUTC: at, CollisionID: "lifecycle",
	}
	cases := []struct {
		name string
		edit func(*platformmetrics.RuntimeMetricsOpeningRequest)
	}{
		{name: "runtime instance", edit: func(request *platformmetrics.RuntimeMetricsOpeningRequest) { request.RuntimeInstanceID = "" }},
		{name: "root", edit: func(request *platformmetrics.RuntimeMetricsOpeningRequest) { request.RootDirectory = "" }},
		{name: "start time", edit: func(request *platformmetrics.RuntimeMetricsOpeningRequest) { request.StartTimeUTC = time.Time{} }},
		{name: "collision", edit: func(request *platformmetrics.RuntimeMetricsOpeningRequest) { request.CollisionID = "" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.edit(&request)
			if _, err := opener.Open(request); err == nil {
				t.Fatalf("Open(%s) error = nil, want validation error", test.name)
			}
		})
	}

	sink, err := opener.Open(base)
	if err != nil {
		t.Fatalf("open lifecycle sink: %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sink.WriteMetric(cancelled, map[string]any{"metric_name": "cancelled"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled WriteMetric error = %v, want context.Canceled", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("close lifecycle sink: %v", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("second close lifecycle sink: %v", err)
	}
	if err := sink.WriteMetric(context.Background(), map[string]any{"metric_name": "closed"}); err == nil {
		t.Fatal("closed WriteMetric error = nil, want closed-sink error")
	}
	if err := opener.Close(context.Background()); err != nil {
		t.Fatalf("close lifecycle opener: %v", err)
	}
}

func TestRuntimeMetricsPublicRetentionSchedulerSharesRootAndSweepsOnTick(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, time.August, 24, 12, 30, 0, 0, time.UTC)
	retention, err := platformmetrics.NewRuntimeMetricsRetention(
		platformfilesystem.Local{}, func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("construct runtime metrics retention: %v", err)
	}
	ticker := &functionalMetricsTicker{ticks: make(chan time.Time, 1)}
	var reports []platformmetrics.RuntimeMetricsRetentionReport
	reportsReady := make(chan struct{}, 4)
	scheduler, err := platformmetrics.NewRuntimeMetricsRetentionScheduler(
		retention,
		func(time.Duration) platformmetrics.RuntimeMetricsRetentionTicker { return ticker },
		func(report platformmetrics.RuntimeMetricsRetentionReport, sweepErr error) {
			if sweepErr != nil {
				t.Errorf("retention report error: %v", sweepErr)
			}
			reports = append(reports, report)
			reportsReady <- struct{}{}
		},
	)
	if err != nil {
		t.Fatalf("construct deterministic retention scheduler: %v", err)
	}
	config := platformmetrics.RuntimeMetricsConfig{MaxAge: 1, MaxSize: 1}
	first, err := scheduler.Start(context.Background(), platformmetrics.RuntimeMetricsRetentionRequest{RootDirectory: root, Config: config})
	if err != nil {
		t.Fatalf("start first retention lease: %v", err)
	}
	select {
	case <-reportsReady:
	case <-time.After(2 * time.Second):
		t.Fatal("startup retention report not published")
	}
	second, err := scheduler.Start(context.Background(), platformmetrics.RuntimeMetricsRetentionRequest{RootDirectory: root, Config: config})
	if err != nil {
		t.Fatalf("start shared retention lease: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first shared lease: %v", err)
	}

	stalePath := filepath.Join(root, "2026", "08", "01", "120000.000000000-runtime-metrics-ticked-runtime.log")
	if err := os.MkdirAll(filepath.Dir(stalePath), 0o755); err != nil {
		t.Fatalf("create ticked artifact directory: %v", err)
	}
	if err := os.WriteFile(stalePath, []byte(`{"metric_name":"ticked"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write ticked artifact: %v", err)
	}
	ticker.Tick(now)
	select {
	case <-reportsReady:
	case <-time.After(2 * time.Second):
		t.Fatal("periodic retention report not published")
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("ticked stale artifact stat error = %v, want removed", err)
	}
	if len(reports) != 2 || reports[1].Removed.Files != 1 {
		t.Fatalf("retention reports = %#v, want startup plus one removal report", reports)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close second shared lease: %v", err)
	}
	if err := scheduler.Close(context.Background()); err != nil {
		t.Fatalf("close retention scheduler: %v", err)
	}
	if err := scheduler.Close(context.Background()); err != nil {
		t.Fatalf("second close retention scheduler: %v", err)
	}
	if _, err := scheduler.Start(context.Background(), platformmetrics.RuntimeMetricsRetentionRequest{RootDirectory: root, Config: config}); err == nil {
		t.Fatal("start after scheduler close error = nil, want closed-scheduler error")
	}
	if _, err := scheduler.Start(context.Background(), platformmetrics.RuntimeMetricsRetentionRequest{RootDirectory: "", Config: config}); err == nil {
		t.Fatal("start empty root error = nil, want root validation error")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := scheduler.Start(cancelled, platformmetrics.RuntimeMetricsRetentionRequest{RootDirectory: root, Config: config}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled start error = %v, want context.Canceled", err)
	}
}

func TestRuntimeArtifactPublicNamedReservationsExposeCollisionPolicy(t *testing.T) {
	reserver, err := platformartifact.NewReserver(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("construct runtime artifact reserver: %v", err)
	}
	if _, err := platformartifact.NewReserver(nil); err == nil {
		t.Fatal("NewReserver(nil) error = nil, want required-filesystem error")
	}
	root := t.TempDir()
	at := time.Date(2026, time.August, 24, 12, 30, 0, 0, time.FixedZone("PDT", -7*60*60))
	path, err := reserver.ReserveNamed(root, at, "session", ".jsonl")
	if err != nil {
		t.Fatalf("ReserveNamed: %v", err)
	}
	if !filepath.IsAbs(path) || filepath.Ext(path) != ".jsonl" {
		t.Fatalf("named reservation path = %q, want absolute .jsonl path", path)
	}
	if _, err := reserver.ReserveNamed(root, at, "session", ".jsonl"); !errors.Is(err, platformartifact.ErrNamedReservationExists) {
		t.Fatalf("duplicate ReserveNamed error = %v, want ErrNamedReservationExists", err)
	}
	collisionPath, err := reserver.ReserveNamedWithCollision(root, at, "session", ".jsonl")
	if err != nil {
		t.Fatalf("ReserveNamedWithCollision: %v", err)
	}
	if collisionPath == path {
		t.Fatalf("collision reservation path = %q, want distinct path from %q", collisionPath, path)
	}
	if got := platformartifact.RuntimeArtifactPathComponents("session id", "runtime/one", "secret=value"); got == "" || got == fmt.Sprint("session id", "runtime/one", "secret=value") {
		t.Fatalf("sanitized runtime artifact components = %q, want policy-safe transformed suffix", got)
	}
}

func TestRuntimeMetricsPublicCoordinationRejectsInvalidAndBusyClaims(t *testing.T) {
	coordination, err := platformmetrics.NewRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("construct runtime metrics coordination: %v", err)
	}
	if _, err := platformmetrics.NewRuntimeMetricsCoordination(); err != nil {
		t.Fatalf("construct second runtime metrics coordination: %v", err)
	}
	root := t.TempDir()
	if _, err := coordination.LockRoot(context.Background(), ""); err == nil {
		t.Fatal("LockRoot(empty) error = nil, want path validation error")
	}
	if _, err := coordination.TryLockRoot(""); err == nil {
		t.Fatal("TryLockRoot(empty) error = nil, want path validation error")
	}
	if _, err := coordination.TryClaim(""); err == nil {
		t.Fatal("TryClaim(empty) error = nil, want path validation error")
	}
	if _, err := coordination.TryClaimMarker(""); err == nil {
		t.Fatal("TryClaimMarker(empty) error = nil, want path validation error")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := coordination.LockRoot(cancelled, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled LockRoot error = %v, want context.Canceled", err)
	}
	rootLock, err := coordination.LockRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("lock coordination root: %v", err)
	}
	if _, err := coordination.TryLockRoot(root); !errors.Is(err, platformmetrics.ErrRuntimeMetricsRootBusy) {
		t.Fatalf("busy TryLockRoot error = %v, want ErrRuntimeMetricsRootBusy", err)
	}
	if err := rootLock.Close(); err != nil {
		t.Fatalf("close coordination root lock: %v", err)
	}
	claimPath := filepath.Join(root, "2026", "08", "24", "123000.000000000-runtime-metrics-claimed-runtime.log")
	claim, err := coordination.Claim(claimPath)
	if err != nil {
		t.Fatalf("claim metrics artifact: %v", err)
	}
	if _, err := coordination.TryClaim(claimPath); !errors.Is(err, platformmetrics.ErrRuntimeMetricsArtifactBusy) {
		t.Fatalf("busy TryClaim error = %v, want ErrRuntimeMetricsArtifactBusy", err)
	}
	if err := claim.Close(); err != nil {
		t.Fatalf("close metrics artifact claim: %v", err)
	}
	markerPath := filepath.Join(root, "claims", "orphan.marker")
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
		t.Fatalf("create marker directory: %v", err)
	}
	if err := os.WriteFile(markerPath, nil, 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	marker, err := coordination.TryClaimMarker(markerPath)
	if err != nil {
		t.Fatalf("claim existing marker: %v", err)
	}
	if err := marker.Close(); err != nil {
		t.Fatalf("close existing marker claim: %v", err)
	}
}

func TestRuntimeMetricsPublicRetentionReportsFailedInventoryAndReapsOrphanMarkers(t *testing.T) {
	failureRoot := t.TempDir()
	now := time.Date(2026, time.August, 24, 12, 30, 0, 0, time.UTC)
	failedPath := filepath.Join(failureRoot, "2026", "08", "01", "120000.000000000-runtime-metrics-failed-runtime.log")
	if err := os.MkdirAll(filepath.Dir(failedPath), 0o755); err != nil {
		t.Fatalf("create failed inventory directory: %v", err)
	}
	if err := os.WriteFile(failedPath, []byte(`{"metric_name":"failed"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write failed inventory artifact: %v", err)
	}
	retention, err := platformmetrics.NewRuntimeMetricsRetention(
		failingRetentionFileSystem{Local: platformfilesystem.Local{}, FailedPath: failedPath},
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("construct failing retention: %v", err)
	}
	report, err := retention.Sweep(context.Background(), platformmetrics.RuntimeMetricsRetentionRequest{
		RootDirectory: failureRoot,
		Config:        platformmetrics.RuntimeMetricsConfig{MaxAge: 1, MaxSize: 1},
	})
	if err != nil {
		t.Fatalf("sweep failing retention: %v", err)
	}
	if report.Failed.Files != 1 || report.Protected.Files != 0 || len(report.Failures) != 2 {
		t.Fatalf("retention failure report = %#v, want failed artifact plus skipped-marker cleanup failure", report)
	}

	root := t.TempDir()
	claimsDirectory := filepath.Join(root, ".runtime-metrics-retention-claims")
	if err := os.MkdirAll(claimsDirectory, 0o755); err != nil {
		t.Fatalf("create claim marker directory: %v", err)
	}
	orphanMarker := filepath.Join(claimsDirectory, strings.Repeat("a", 64)+".active")
	if err := os.WriteFile(orphanMarker, nil, 0o600); err != nil {
		t.Fatalf("write orphan marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claimsDirectory, "not-a-marker"), nil, 0o600); err != nil {
		t.Fatalf("write malformed marker: %v", err)
	}
	retention, err = platformmetrics.NewRuntimeMetricsRetention(
		platformfilesystem.Local{},
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("construct failing retention: %v", err)
	}
	report, err = retention.Sweep(context.Background(), platformmetrics.RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        platformmetrics.RuntimeMetricsConfig{MaxAge: 1, MaxSize: 1},
	})
	if err != nil {
		t.Fatalf("sweep failing retention: %v", err)
	}
	if report.Failed.Files != 0 || len(report.Failures) != 1 {
		t.Fatalf("retention marker report = %#v, want one malformed-marker failure", report)
	}
	if _, err := os.Stat(orphanMarker); !os.IsNotExist(err) {
		t.Fatalf("orphan marker stat error = %v, want marker reaped", err)
	}
}

type failingRetentionFileSystem struct {
	platformfilesystem.Local
	FailedPath string
}

func (filesystem failingRetentionFileSystem) Lstat(path string) (fs.FileInfo, error) {
	if path == filesystem.FailedPath {
		return nil, errors.New("simulated artifact inspection failure")
	}
	return filesystem.Local.Lstat(path)
}

type functionalMetricsTicker struct {
	ticks   chan time.Time
	stopped bool
}

func (ticker *functionalMetricsTicker) C() <-chan time.Time { return ticker.ticks }

func (ticker *functionalMetricsTicker) Stop() { ticker.stopped = true }

func (ticker *functionalMetricsTicker) Tick(at time.Time) { ticker.ticks <- at }
