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

	allProjection, err := newMetricsProjection("")
	if err != nil {
		t.Fatalf("newMetricsProjection(all) error = %v", err)
	}
	allSelection, err := runtimeMetricsStreamSelection(root, "", "", time.Time{}, time.Time{}, allProjection)
	if err != nil {
		t.Fatalf("runtimeMetricsStreamSelection(all) error = %v", err)
	}
	allStats := &platformmetrics.RuntimeMetricsReadStats{}
	allSelection.Stats = allStats
	if err := reader.StreamSelected(context.Background(), root, allSelection, func(platformmetrics.RuntimeMetricRecord) error { return nil }); err != nil {
		t.Fatalf("StreamSelected(all) error = %v", err)
	}

	sessionSelection, err := runtimeMetricsStreamSelection(root, "session-a", "", time.Time{}, time.Time{}, allProjection)
	if err != nil {
		t.Fatalf("runtimeMetricsStreamSelection(session) error = %v", err)
	}
	sessionStats := &platformmetrics.RuntimeMetricsReadStats{}
	sessionSelection.Stats = sessionStats
	if err := reader.StreamSelected(context.Background(), root, sessionSelection, func(platformmetrics.RuntimeMetricRecord) error { return nil }); err != nil {
		t.Fatalf("StreamSelected(session) error = %v", err)
	}

	providerProjection, err := newMetricsProjection("provider")
	if err != nil {
		t.Fatalf("newMetricsProjection(provider) error = %v", err)
	}
	providerSelection, err := runtimeMetricsStreamSelection(root, "", "", time.Time{}, time.Time{}, providerProjection)
	if err != nil {
		t.Fatalf("runtimeMetricsStreamSelection(provider) error = %v", err)
	}
	providerStats := &platformmetrics.RuntimeMetricsReadStats{}
	providerSelection.Stats = providerStats
	if err := reader.StreamSelected(context.Background(), root, providerSelection, func(platformmetrics.RuntimeMetricRecord) error { return nil }); err != nil {
		t.Fatalf("StreamSelected(provider) error = %v", err)
	}

	windowSelection, err := runtimeMetricsStreamSelection(
		root,
		"",
		"",
		time.Date(2026, time.August, 21, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.August, 22, 0, 0, 0, 0, time.UTC),
		allProjection,
	)
	if err != nil {
		t.Fatalf("runtimeMetricsStreamSelection(window) error = %v", err)
	}
	windowStats := &platformmetrics.RuntimeMetricsReadStats{}
	windowSelection.Stats = windowStats
	if err := reader.StreamSelected(context.Background(), root, windowSelection, func(platformmetrics.RuntimeMetricRecord) error { return nil }); err != nil {
		t.Fatalf("StreamSelected(window) error = %v", err)
	}

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
