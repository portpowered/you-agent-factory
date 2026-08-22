package runtime_metrics_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	cligenerated "github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

func TestMetricsCostsCLITraversesProductionHTTPRoute(t *testing.T) {
	homeDir := t.TempDir()
	factoryDir := support.ScaffoldSingleStepFactory(t, "costs-cli-functional")
	const sessionID = "costs-session-functional"
	writeCostsFunctionalMetrics(t, homeDir, sessionID)

	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       environment,
	})
	defer server.Stop(t)

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	if root.CostsQueryFromProcess(process) == nil ||
		root.RuntimeMetricsQueryFromProcess(process) == nil ||
		root.DetachedOperationsFromProcess(process) == nil ||
		root.ExecutionRuntimeOpeningFromProcess(process) == nil {
		t.Fatal("root process did not expose all canonical metrics, costs, detached, and execution capabilities")
	}
	manifest, err := cligenerated.MetricsFamilyManifest()
	if err != nil {
		t.Fatalf("load generated metrics command manifest: %v", err)
	}
	command, err := manifest.CommandByID("you.metrics.costs")
	if err != nil || command.Path != "you metrics costs" {
		t.Fatalf("generated costs command = %#v, error = %v", command, err)
	}
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "--json", "--server", server.URL(), "metrics", "costs", "--session", sessionID,
	})
	inputs.Env = environment
	inputs.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("execute production metrics costs CLI: %v\nstderr=%s", err, inputs.Stderr())
	}

	var report generatedclient.CostsReport
	if err := json.Unmarshal([]byte(inputs.Stdout()), &report); err != nil {
		t.Fatalf("decode production metrics costs JSON: %v\nstdout=%s\nstderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if report.Status != generatedclient.CostsReportStatus("PARTIAL") {
		t.Fatalf("report status = %q, want PARTIAL", report.Status)
	}
	if report.PricedSubtotal == nil || *report.PricedSubtotal != "21.1375" {
		t.Fatalf("priced subtotal = %v, want exact 21.1375 USD", report.PricedSubtotal)
	}
	if report.Coverage.EncounteredRows != 2 || report.Coverage.PricedRows != 1 ||
		report.Coverage.UnpricedRows != 1 || report.Coverage.EncounteredProviderModels != 2 ||
		report.Coverage.PricedProviderModels != 1 || report.Coverage.UnpricedProviderModels != 1 {
		t.Fatalf("coverage = %#v, want one priced and one unpriced row/model", report.Coverage)
	}
	if len(report.LineItems) != 2 || len(report.WorkItems) != 2 || len(report.WorkerSessions) != 2 ||
		len(report.ProviderModels) != 2 || len(report.FactorySessions) != 1 {
		t.Fatalf("report dimensions have incomplete rows: line_items=%d work=%d workers=%d providers=%d sessions=%d",
			len(report.LineItems), len(report.WorkItems), len(report.WorkerSessions), len(report.ProviderModels), len(report.FactorySessions))
	}
	if !strings.Contains(inputs.Stdout(), `"currency":"USD"`) ||
		!strings.Contains(inputs.Stdout(), `"status":"UNPRICED"`) {
		t.Fatalf("API-shaped output omitted currency or retained unpriced status: %s", inputs.Stdout())
	}
	functionalevidence.Covers(t, "cli/you.metrics.costs")
}

func writeCostsFunctionalMetrics(t *testing.T, homeDir, sessionID string) {
	t.Helper()
	root := platformmetrics.RuntimeMetricsRoot(homeDir)
	known := func(metricName string, value float64) map[string]any {
		return map[string]any{
			"metric_name": metricName, "value": value, "unit": "tokens",
			"session_id": sessionID, "runtime_instance_id": "runtime-costs-functional",
			"work_id": "work-known", "dispatch_id": "dispatch-known",
			"worker_session_id": "worker-known", "provider": "CODEX", "model": "gpt-5-codex",
			"workstation": "build", "worker_type": "processor",
		}
	}
	unknown := func(metricName string, value float64) map[string]any {
		return map[string]any{
			"metric_name": metricName, "value": value, "unit": "tokens",
			"session_id": sessionID, "runtime_instance_id": "runtime-costs-functional",
			"work_id": "work-unknown", "dispatch_id": "dispatch-unknown",
			"worker_session_id": "worker-unknown", "provider": "CODEX", "model": "unknown-model",
			"workstation": "build", "worker_type": "processor",
		}
	}
	records := []map[string]any{
		known("provider.input_tokens", 1_000_000),
		known("provider.cached_input_tokens", 100_000),
		known("provider.output_tokens", 2_000_000),
		known("provider.reasoning_output_tokens", 500_000),
		unknown("provider.input_tokens", 10),
		unknown("provider.output_tokens", 20),
	}
	path := filepath.Join(root, "120000.000000000-runtime-metrics-"+sessionID+"-runtime-costs-functional.log")
	writeRuntimeMetricsArtifact(t, path, false, records)
}
