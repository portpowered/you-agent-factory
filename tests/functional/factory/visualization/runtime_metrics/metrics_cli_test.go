package runtime_metrics_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryvisualizationhttp "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http"
	transporthttp "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
)

// TestMetricsInvalidGroupThroughRootProcessPreservesCodedDiagnostic proves
// the customer CLI process keeps the metrics-owned code and safe message at
// the production central-diagnostics boundary.
func TestMetricsInvalidGroupThroughRootProcessPreservesCodedDiagnostic(t *testing.T) {
	t.Parallel()

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "metrics", "--group-by", "region",
	})
	inputs.Input.Env = []string{"HOME=" + t.TempDir(), "USERPROFILE=" + t.TempDir()}

	err := process.Execute(inputs.Input)
	if err == nil {
		t.Fatal("Process.Execute(metrics invalid group) error = nil, want coded failure")
	}
	if inputs.Stdout() != "" {
		t.Fatalf("metrics stdout = %q, want empty", inputs.Stdout())
	}
	assertMetricsDiagnostic(t, inputs.Stderr(), "METRICS_INVALID_GROUP_BY", `invalid --group-by "region": choose workstation, worker, or provider`)
}

// TestMetricsSuccessThroughRootProcessRendersQueryCostAvailability proves
// both public presenters consume the query result returned by the canonical
// process rather than relying on a presenter-local cost constant.
func TestMetricsSuccessThroughRootProcessRendersQueryCostAvailability(t *testing.T) {
	t.Parallel()

	query := factoryvisualization.RuntimeMetricsQuery(func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
		return factoryvisualization.RuntimeMetricsQueryResult{
			Cost: factoryvisualization.RuntimeMetricsCost{
				Availability: factoryvisualization.RuntimeMetricsCostUnavailable,
			},
		}, nil
	})
	metricsHandler := factoryvisualizationhttp.NewMetricsHandler(
		factoryvisualizationhttp.NewMetricsAdapter(query, nil, t.TempDir()),
		zap.NewNop(),
	)
	apiServer := transporthttp.NewServerWithRecordingsAndMetricsAndCosts(
		nil, nil, nil, nil, nil, nil, zap.NewNop(), metricsHandler, nil,
	)
	server := httptest.NewServer(apiServer.Handler())
	t.Cleanup(server.Close)

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	env := []string{"HOME=" + t.TempDir(), "USERPROFILE=" + t.TempDir()}

	human := support.FakeInputs(t.Context(), []string{"you", "--server", server.URL, "metrics"})
	human.Input.Env = env
	human.Input.WorkingDirectory = t.TempDir()
	if err := process.Execute(human.Input); err != nil {
		t.Fatalf("Process.Execute(metrics human) error = %v\nstdout:\n%s\nstderr:\n%s", err, human.Stdout(), human.Stderr())
	}
	if !strings.Contains(human.Stdout(), "Cost: unavailable\n") || human.Stderr() != "" {
		t.Fatalf("human metrics output = %q, stderr = %q", human.Stdout(), human.Stderr())
	}

	machine := support.FakeInputs(t.Context(), []string{"you", "--json", "--server", server.URL, "metrics"})
	machine.Input.Env = env
	machine.Input.WorkingDirectory = t.TempDir()
	if err := process.Execute(machine.Input); err != nil {
		t.Fatalf("Process.Execute(metrics JSON) error = %v\nstdout:\n%s\nstderr:\n%s", err, machine.Stdout(), machine.Stderr())
	}
	var document struct {
		Cost struct {
			Availability string `json:"availability"`
		} `json:"cost"`
	}
	if err := json.Unmarshal([]byte(machine.Stdout()), &document); err != nil {
		t.Fatalf("decode metrics JSON: %v\n%s", err, machine.Stdout())
	}
	if document.Cost.Availability != "unavailable" || machine.Stderr() != "" {
		t.Fatalf("JSON metrics cost = %#v, stderr = %q", document.Cost, machine.Stderr())
	}
}

func assertMetricsDiagnostic(t *testing.T, output, wantCode, wantMessage string) {
	t.Helper()
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || strings.Contains(trimmed, "\n") {
		t.Fatalf("metrics diagnostic = %q, want one JSON line", output)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
		t.Fatalf("decode metrics diagnostic: %v\n%s", err, output)
	}
	if response.Code != factoryapi.ErrorResponseCode(wantCode) || response.Message != wantMessage {
		t.Fatalf("metrics diagnostic = %#v, want code %q and message %q", response, wantCode, wantMessage)
	}
}
