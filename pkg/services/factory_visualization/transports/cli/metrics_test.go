package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	visualizationcli "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
)

func TestMetricsCommand_DefaultsToWorkstationAndRendersHumanMetrics(t *testing.T) {
	var gotRequest factoryvisualization.RuntimeMetricsQueryRequest
	query := metricsQueryStub(func(_ context.Context, request factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
		gotRequest = request
		return metricsResult(), nil
	})

	output, err := executeMetricsCommand(t, query, nil)
	if err != nil {
		t.Fatalf("execute metrics: %v", err)
	}
	if gotRequest.MetricsRoot != filepath.Join("/home/operator", ".you-agent-factory", "metrics") {
		t.Fatalf("metrics root = %q, want default home metrics root", gotRequest.MetricsRoot)
	}
	for _, want := range []string{
		"Scope: all Factory Sessions",
		"Group by: workstation",
		"Cost: unavailable",
		"Input tokens: 12",
		"Output tokens: 8",
		"Completed dispatches: 3",
		"Failures by reason:",
		"timeout: 2",
		"Dispatch latency (milliseconds): p50=30, p95=95, samples=5",
		"Provider latency (milliseconds): p50=25, p95=75, samples=3",
		"Breakdown by workstation: 1 rows",
		"workstation-a:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "worker-a:") || strings.Contains(output, "provider-a:") {
		t.Fatalf("default workstation output included another grouping:\n%s", output)
	}
}

func TestMetricsCommand_SelectsEverySupportedGrouping(t *testing.T) {
	tests := []struct {
		name       string
		groupBy    string
		wantHeader string
		wantKey    string
	}{
		{name: "workstation", groupBy: "workstation", wantHeader: "Breakdown by workstation: 1 rows", wantKey: "workstation-a:"},
		{name: "worker", groupBy: "worker", wantHeader: "Breakdown by worker: 1 rows", wantKey: "worker-a:"},
		{name: "provider", groupBy: "provider", wantHeader: "Breakdown by provider: 1 rows", wantKey: "provider-a:"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, err := executeMetricsCommand(t, metricsQueryStub(func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
				return metricsResult(), nil
			}), []string{"--group-by", test.groupBy})
			if err != nil {
				t.Fatalf("execute metrics: %v", err)
			}
			if !strings.Contains(output, test.wantHeader) || !strings.Contains(output, test.wantKey) {
				t.Fatalf("output = %q, want %q and %q", output, test.wantHeader, test.wantKey)
			}
		})
	}
}

func TestMetricsCommand_EmptyResultShowsZeroCountsAndNoSamples(t *testing.T) {
	output, err := executeMetricsCommand(t, metricsQueryStub(func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
		return factoryvisualization.RuntimeMetricsQueryResult{
			Cost: factoryvisualization.RuntimeMetricsCost{Availability: factoryvisualization.RuntimeMetricsCostUnavailable},
		}, nil
	}), nil)
	if err != nil {
		t.Fatalf("execute metrics: %v", err)
	}
	for _, want := range []string{
		"Input tokens: 0",
		"Output tokens: 0",
		"Completed dispatches: 0",
		"Failures by reason: none",
		"Dispatch latency (milliseconds): no samples",
		"Provider latency (milliseconds): no samples",
		"Breakdown by workstation: 0 rows",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("empty output missing %q:\n%s", want, output)
		}
	}
}

func TestMetricsCommand_RejectsUnsupportedGroupingBeforeQuery(t *testing.T) {
	called := false
	output, err := executeMetricsCommand(t, metricsQueryStub(func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
		called = true
		return factoryvisualization.RuntimeMetricsQueryResult{}, nil
	}), []string{"--group-by", "region"})
	if err == nil || !strings.Contains(err.Error(), `invalid --group-by "region"`) || !strings.Contains(err.Error(), "workstation, worker, or provider") {
		t.Fatalf("error = %v, want actionable group validation error", err)
	}
	if called {
		t.Fatal("invalid grouping invoked the metrics query")
	}
	if output != "" {
		t.Fatalf("invalid grouping wrote partial output %q", output)
	}
}

func TestMetricsCommand_QueryFailureDoesNotWritePartialOutput(t *testing.T) {
	wantErr := errors.New("metrics artifact unavailable")
	output, err := executeMetricsCommand(t, metricsQueryStub(func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
		return factoryvisualization.RuntimeMetricsQueryResult{}, wantErr
	}), nil)
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("error = %v, want query failure", err)
	}
	if output != "" {
		t.Fatalf("query failure wrote partial output %q", output)
	}
}

func executeMetricsCommand(
	t *testing.T,
	query factoryvisualization.RuntimeMetricsQuery,
	args []string,
) (string, error) {
	t.Helper()
	command := visualizationcli.NewMetricsCommand(visualizationcli.MetricsCommandConfig{
		Query:   query,
		HomeDir: func() (string, error) { return "/home/operator", nil },
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(io.Discard)
	command.SetArgs(args)
	err := command.Execute()
	return output.String(), err
}

func metricsQueryStub(
	fn func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error),
) factoryvisualization.RuntimeMetricsQuery {
	return factoryvisualization.RuntimeMetricsQuery(fn)
}

func metricsResult() factoryvisualization.RuntimeMetricsQueryResult {
	return factoryvisualization.RuntimeMetricsQueryResult{
		Cost: factoryvisualization.RuntimeMetricsCost{Availability: factoryvisualization.RuntimeMetricsCostUnavailable},
		Totals: factoryvisualization.RuntimeMetricsAggregate{
			InputTokens: 12, OutputTokens: 8, CompletedDispatches: 3,
			FailuresByReason: map[string]float64{"timeout": 2},
			DispatchDuration: metricsDuration(30, 95, 5),
			ProviderDuration: metricsDuration(25, 75, 3),
		},
		Workstations: []factoryvisualization.RuntimeMetricsBreakdown{{
			Key: "workstation-a", Aggregate: factoryvisualization.RuntimeMetricsAggregate{InputTokens: 12},
		}},
		WorkerTypes: []factoryvisualization.RuntimeMetricsBreakdown{{
			Key: "worker-a", Aggregate: factoryvisualization.RuntimeMetricsAggregate{InputTokens: 12},
		}},
		Providers: []factoryvisualization.RuntimeMetricsBreakdown{{
			Key: "provider-a", Aggregate: factoryvisualization.RuntimeMetricsAggregate{InputTokens: 12},
		}},
	}
}

func metricsDuration(p50, p95 float64, samples int) *factoryvisualization.RuntimeMetricsDuration {
	return &factoryvisualization.RuntimeMetricsDuration{Unit: "ms", Samples: samples, P50: &p50, P95: &p95}
}
