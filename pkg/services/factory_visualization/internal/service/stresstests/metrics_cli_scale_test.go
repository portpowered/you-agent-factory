package stresstests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	visualizationservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/service"
	visualizationcli "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunMetricsCompletesDeterministicallyOnLargePartitionedStore(t *testing.T) {
	if testing.Short() {
		t.Skip("2000-artifact partitioned metrics scale profile")
	}
	const artifactCount = 2_000
	const readBudget = 30 * time.Second

	homeDir := t.TempDir()
	metricsRoot := filepath.Join(homeDir, ".you-agent-factory", "metrics")
	writeLargePartitionedMetricsStore(t, metricsRoot, artifactCount)

	reader, err := platformmetrics.NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader() error = %v", err)
	}
	query, err := visualizationservice.NewRuntimeMetricsQuery(reader, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}

	human, humanDuration := runLargeMetricsCommand(t, query, homeDir, false, readBudget)
	if !strings.Contains(human, fmt.Sprintf("Completed dispatches: %d", artifactCount)) {
		t.Fatalf("human output = %q, want %d completed dispatches", human, artifactCount)
	}
	if !strings.Contains(human, "Breakdown by provider: 1 rows") || !strings.Contains(human, "provider-large:") {
		t.Fatalf("human output = %q, want deterministic provider grouping", human)
	}

	jsonOutput, jsonDuration := runLargeMetricsCommand(t, query, homeDir, true, readBudget)
	var document struct {
		GroupBy string `json:"group_by"`
		Totals  struct {
			CompletedDispatches float64 `json:"completed_dispatches"`
		} `json:"totals"`
		Groups []struct {
			Key string `json:"key"`
		} `json:"groups"`
	}
	if err := json.Unmarshal([]byte(jsonOutput), &document); err != nil {
		t.Fatalf("decode large-store JSON: %v\n%s", err, jsonOutput)
	}
	if document.GroupBy != "provider" || document.Totals.CompletedDispatches != artifactCount ||
		len(document.Groups) != 1 || document.Groups[0].Key != "provider-large" {
		t.Fatalf("large-store JSON = %#v, want provider grouping and %d dispatches", document, artifactCount)
	}

	repeatedJSON, repeatedDuration := runLargeMetricsCommand(t, query, homeDir, true, readBudget)
	if repeatedJSON != jsonOutput {
		t.Fatalf("large-store JSON is not deterministic:\nfirst: %s\nsecond: %s", jsonOutput, repeatedJSON)
	}
	t.Logf("large metrics store: files=%d human=%s json=%s repeat-json=%s", artifactCount, humanDuration, jsonDuration, repeatedDuration)
}

func runLargeMetricsCommand(
	t *testing.T,
	query factoryvisualization.RuntimeMetricsQuery,
	homeDir string,
	jsonOutput bool,
	readBudget time.Duration,
) (string, time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), readBudget)
	defer cancel()
	var output bytes.Buffer
	started := time.Now()
	err := visualizationcli.RunMetrics(ctx, visualizationcli.MetricsConfig{
		GroupBy: "provider",
		JSON:    jsonOutput,
		Output:  &output,
		Query:   query,
		HomeDir: func() (string, error) { return homeDir, nil },
	})
	duration := time.Since(started)
	if err != nil {
		t.Fatalf("RunMetrics(large store, json=%t) error after %s: %v", jsonOutput, duration, err)
	}
	return output.String(), duration
}

func writeLargePartitionedMetricsStore(t *testing.T, root string, artifactCount int) {
	t.Helper()
	const shardCount = 128
	for shard := 0; shard < shardCount; shard++ {
		shardPath := filepath.Join(root, "2026", "08", "20", fmt.Sprintf("shard-%03d", shard))
		if err := os.MkdirAll(shardPath, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", shardPath, err)
		}
	}
	const record = `{"metric_name":"dispatch.completed","value":1,"session_id":"session-large","runtime_instance_id":"runtime-large","workstation":"workstation-large","worker_type":"worker-large","provider":"provider-large"}` + "\n"
	for index := 0; index < artifactCount; index++ {
		shardPath := filepath.Join(root, "2026", "08", "20", fmt.Sprintf("shard-%03d", index%shardCount))
		name := fmt.Sprintf("%06d.000000000-runtime-metrics-session-large-runtime-large-%06d.log", index, index)
		if err := os.WriteFile(filepath.Join(shardPath, name), []byte(record), 0o600); err != nil {
			t.Fatalf("WriteFile(%q): %v", name, err)
		}
	}
}
