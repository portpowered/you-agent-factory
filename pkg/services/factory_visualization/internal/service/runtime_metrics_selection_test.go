package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
)

func TestRuntimeMetricsSelectionReducesSessionProjectionAndDateWork(t *testing.T) {
	root := t.TempDir()
	writeSelectionArtifact(t, filepath.Join(root, "2026", "08", "20", "120000.000000000-runtime-metrics-session-a-runtime-a.log"), "session-a", "runtime-a")
	writeSelectionArtifact(t, filepath.Join(root, "2026", "08", "21", "120000.000000000-runtime-metrics-session-b-runtime-b.log"), "session-b", "runtime-b")

	reader, err := platformmetrics.NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader() error = %v", err)
	}

	allStats := collectSelectionStats(t, reader, root, "", "", time.Time{}, time.Time{}, "")
	sessionStats := collectSelectionStats(t, reader, root, "session-a", "", time.Time{}, time.Time{}, "")
	providerStats := collectSelectionStats(t, reader, root, "", "", time.Time{}, time.Time{}, "provider")
	windowStats := collectSelectionStats(
		t,
		reader,
		root,
		"",
		"",
		time.Date(2026, time.August, 21, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.August, 22, 0, 0, 0, 0, time.UTC),
		"",
	)
	assertSelectionWorkReduction(t, allStats, sessionStats, providerStats, windowStats)
	runRuntimeMetricsQueryBoundsDecodedRecordLifetimeAcrossArtifactScale(t)
}

func TestRuntimeMetricsSelectionKeepsLegacyArtifactForEnvelopeFiltering(t *testing.T) {
	root := t.TempDir()
	writeSelectionArtifact(t, filepath.Join(root, "2026", "08", "20", "120000.000000000-runtime-metrics-legacy.log"), "session-a", "runtime-a")

	reader, err := platformmetrics.NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader() error = %v", err)
	}
	projection, err := newMetricsProjection("")
	if err != nil {
		t.Fatalf("newMetricsProjection() error = %v", err)
	}
	selection, err := runtimeMetricsStreamSelection(root, "session-a", "", time.Time{}, time.Time{}, projection)
	if err != nil {
		t.Fatalf("runtimeMetricsStreamSelection() error = %v", err)
	}
	stats := &platformmetrics.RuntimeMetricsReadStats{}
	selection.Stats = stats
	decoded := 0
	if err := reader.StreamSelected(context.Background(), root, selection, func(platformmetrics.RuntimeMetricRecord) error {
		decoded++
		return nil
	}); err != nil {
		t.Fatalf("StreamSelected() error = %v", err)
	}
	if decoded != 3 || stats.ArtifactsOpened != 1 || stats.RecordsDecoded != 3 {
		t.Fatalf("legacy selection work = decoded %d, stats %#v; want three records from one opened artifact", decoded, stats)
	}
	runRuntimeMetricsQueryCharacterizesFixedArtifactCorpus(t)
	runRuntimeMetricsQueryCharacterizationRejectsMalformedCompleteLine(t)
}

func collectSelectionStats(
	t *testing.T,
	reader platformmetrics.SelectedReader,
	root string,
	sessionID string,
	runtimeID string,
	startTimeUTC time.Time,
	endTimeUTC time.Time,
	groupBy string,
) *platformmetrics.RuntimeMetricsReadStats {
	t.Helper()
	projection, err := newMetricsProjection(groupBy)
	if err != nil {
		t.Fatalf("newMetricsProjection(%q) error = %v", groupBy, err)
	}
	selection, err := runtimeMetricsStreamSelection(root, sessionID, runtimeID, startTimeUTC, endTimeUTC, projection)
	if err != nil {
		t.Fatalf("runtimeMetricsStreamSelection(%q) error = %v", groupBy, err)
	}
	stats := &platformmetrics.RuntimeMetricsReadStats{}
	selection.Stats = stats
	if err := reader.StreamSelected(context.Background(), root, selection, func(platformmetrics.RuntimeMetricRecord) error { return nil }); err != nil {
		t.Fatalf("StreamSelected(%q) error = %v", groupBy, err)
	}
	return stats
}

func assertSelectionWorkReduction(
	t *testing.T,
	allStats *platformmetrics.RuntimeMetricsReadStats,
	sessionStats *platformmetrics.RuntimeMetricsReadStats,
	providerStats *platformmetrics.RuntimeMetricsReadStats,
	windowStats *platformmetrics.RuntimeMetricsReadStats,
) {
	t.Helper()
	if sessionStats.ArtifactsOpened >= allStats.ArtifactsOpened || sessionStats.RecordsDecoded >= allStats.RecordsDecoded {
		t.Fatalf("session selection work = %#v, all work = %#v; want fewer opened artifacts and decoded records", sessionStats, allStats)
	}
	if providerStats.RecordsDecoded >= allStats.RecordsDecoded {
		t.Fatalf("provider projection work = %#v, all work = %#v; want fewer decoded metric families", providerStats, allStats)
	}
	if windowStats.ArtifactsOpened >= allStats.ArtifactsOpened {
		t.Fatalf("date selection work = %#v, all work = %#v; want date partition pruning before artifact open", windowStats, allStats)
	}
}

func writeSelectionArtifact(t *testing.T, path, sessionID, runtimeID string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	payload := []string{
		fmt.Sprintf(`{"metric_name":"provider.input_tokens","value":1,"session_id":%q,"runtime_instance_id":%q,"provider":"provider-%s"}`, sessionID, runtimeID, sessionID),
		fmt.Sprintf(`{"metric_name":"provider.cached_input_tokens","value":1,"session_id":%q,"runtime_instance_id":%q,"provider":"provider-%s"}`, sessionID, runtimeID, sessionID),
		fmt.Sprintf(`{"metric_name":"dispatch.completed","value":1,"session_id":%q,"runtime_instance_id":%q,"provider":"provider-%s"}`, sessionID, runtimeID, sessionID),
	}
	if err := os.WriteFile(path, []byte(payload[0]+"\n"+payload[1]+"\n"+payload[2]+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
