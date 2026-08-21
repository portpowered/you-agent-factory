package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestMetricsCommand_ScopesEveryGroupingInHumanAndJSON(t *testing.T) {
	formats := []struct {
		name string
		json bool
	}{
		{name: "human", json: false},
		{name: "json", json: true},
	}
	scopes := []struct {
		name string
		id   string
	}{
		{name: "unscoped", id: ""},
		{name: "factory-session", id: "session-a"},
	}
	groups := []struct {
		name string
		key  string
	}{
		{name: "workstation", key: "workstation-a"},
		{name: "worker", key: "worker-a"},
		{name: "provider", key: "provider-a"},
	}

	for _, format := range formats {
		for _, scope := range scopes {
			for _, group := range groups {
				t.Run(fmt.Sprintf("%s/%s/%s", format.name, scope.name, group.name), func(t *testing.T) {
					var gotRequest factoryvisualization.RuntimeMetricsQueryRequest
					query := metricsQueryStub(func(_ context.Context, request factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
						gotRequest = request
						return metricsResult(), nil
					})
					args := []string{"--group-by", group.name}
					if scope.id != "" {
						args = append(args, "--session", scope.id)
					}
					output, err := executeMetricsCommandWithJSON(t, query, args, format.json)
					if err != nil {
						t.Fatalf("execute metrics: %v", err)
					}
					if gotRequest.SessionID != scope.id {
						t.Fatalf("query session ID = %q, want %q", gotRequest.SessionID, scope.id)
					}
					if format.json {
						assertMetricsJSONOutput(t, output, group.name, group.key, scope.id)
						return
					}
					wantScope := "Scope: all Factory Sessions"
					if scope.id != "" {
						wantScope = "Scope: Factory Session " + scope.id
					}
					for _, want := range []string{
						wantScope,
						"Group by: " + group.name,
						"Input tokens: 12",
						"Output tokens: 8",
						"Completed dispatches: 3",
						"Failures by reason:",
						"Dispatch latency (milliseconds): p50=30, p95=95, samples=5",
						"Provider latency (milliseconds): p50=25, p95=75, samples=3",
						"Breakdown by " + group.name + ": 1 rows",
						group.key + ":",
					} {
						if !strings.Contains(output, want) {
							t.Fatalf("human output missing %q:\n%s", want, output)
						}
					}
				})
			}
		}
	}
}

func TestMetricsCommand_JSONIsDeterministicAndPreservesMissingLatency(t *testing.T) {
	result := metricsResult()
	result.Totals.DispatchDuration = nil
	result.Totals.ProviderDuration = nil
	result.Totals.FailuresByReason = map[string]float64{"zeta": 2, "alpha": 1}
	result.Workstations = []factoryvisualization.RuntimeMetricsBreakdown{
		{Key: "zeta", Aggregate: factoryvisualization.RuntimeMetricsAggregate{}},
		{Key: "alpha", Aggregate: factoryvisualization.RuntimeMetricsAggregate{}},
	}
	query := metricsQueryStub(func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
		return result, nil
	})

	first, err := executeMetricsCommandWithJSON(t, query, nil, true)
	if err != nil {
		t.Fatalf("execute first JSON metrics: %v", err)
	}
	second, err := executeMetricsCommandWithJSON(t, query, nil, true)
	if err != nil {
		t.Fatalf("execute second JSON metrics: %v", err)
	}
	if first != second {
		t.Fatalf("JSON output is not deterministic:\nfirst:  %s\nsecond: %s", first, second)
	}
	if !strings.Contains(first, `"failures_by_reason":{"alpha":1,"zeta":2}`) {
		t.Fatalf("JSON failure reasons are not deterministic:\n%s", first)
	}
	if !strings.Contains(first, `"dispatch_latency":{"unit":"milliseconds","samples":0,"p50":null,"p95":null}`) ||
		!strings.Contains(first, `"provider_latency":{"unit":"milliseconds","samples":0,"p50":null,"p95":null}`) {
		t.Fatalf("JSON missing latency samples were not represented as null percentiles:\n%s", first)
	}
	if strings.Contains(first, `"dispatch_latency":{"unit":"milliseconds","samples":1`) ||
		strings.Contains(first, `"provider_latency":{"unit":"milliseconds","samples":1`) {
		t.Fatalf("JSON invented a latency observation:\n%s", first)
	}
}

func TestMetricsCommand_PresentsQueryCostAvailabilityInHumanAndJSON(t *testing.T) {
	tests := []struct {
		name         string
		availability factoryvisualization.RuntimeMetricsCostAvailability
		want         string
	}{
		{name: "unavailable", availability: factoryvisualization.RuntimeMetricsCostUnavailable, want: "unavailable"},
		{name: "query-provided state", availability: "ESTIMATED", want: "estimated"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := metricsQueryStub(func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
				result := metricsResult()
				result.Cost.Availability = test.availability
				return result, nil
			})

			human, err := executeMetricsCommand(t, query, nil)
			if err != nil {
				t.Fatalf("execute human metrics: %v", err)
			}
			if !strings.Contains(human, "Cost: "+test.want+"\n") {
				t.Fatalf("human cost = %q, want %q", human, "Cost: "+test.want)
			}

			jsonOutput, err := executeMetricsCommandWithJSON(t, query, nil, true)
			if err != nil {
				t.Fatalf("execute JSON metrics: %v", err)
			}
			var document metricsJSONDocument
			if err := json.Unmarshal([]byte(jsonOutput), &document); err != nil {
				t.Fatalf("decode JSON output: %v\n%s", err, jsonOutput)
			}
			if document.Cost.Availability != test.want {
				t.Fatalf("JSON cost availability = %q, want %q", document.Cost.Availability, test.want)
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
	var metricsErr *visualizationcli.MetricsError
	if !errors.As(err, &metricsErr) || metricsErr.CLIErrorCode() != visualizationcli.MetricsInvalidGroupByCode ||
		metricsErr.CLIErrorMessage() != `invalid --group-by "region": choose workstation, worker, or provider` {
		t.Fatalf("error = %#v, want coded invalid-group failure", err)
	}
	if called {
		t.Fatal("invalid grouping invoked the metrics query")
	}
	if output != "" {
		t.Fatalf("invalid grouping wrote partial output %q", output)
	}
}

func TestMetricsCommand_QueryFailureDoesNotWritePartialOutput(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		t.Run(fmt.Sprintf("json=%t", jsonOutput), func(t *testing.T) {
			wantErr := errors.New("metrics artifact unavailable: credential=do-not-leak")
			output, err := executeMetricsCommandWithJSON(t, metricsQueryStub(func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
				return factoryvisualization.RuntimeMetricsQueryResult{}, wantErr
			}), nil, jsonOutput)
			if err == nil || !errors.Is(err, wantErr) || !strings.HasPrefix(err.Error(), "METRICS_QUERY_FAILED: query Factory Runtime metrics:") {
				t.Fatalf("error = %v, want safe coded query failure", err)
			}
			if strings.Contains(err.Error(), "credential=do-not-leak") {
				t.Fatalf("query failure exposed its underlying payload: %v", err)
			}
			if output != "" {
				t.Fatalf("query failure wrote partial output %q", output)
			}
		})
	}
}

func TestMetricsCommand_HomeResolutionFailuresAreCodedAndPrecedeQuery(t *testing.T) {
	resolverErr := errors.New("home lookup credential=do-not-leak")
	tests := []struct {
		name      string
		home      func() (string, error)
		wantCause error
	}{
		{
			name: "resolver failure",
			home: func() (string, error) { return "", resolverErr }, wantCause: resolverErr,
		},
		{
			name: "empty path",
			home: func() (string, error) { return "  ", nil },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			queryCalled := false
			var output bytes.Buffer
			err := visualizationcli.RunMetrics(context.Background(), visualizationcli.MetricsConfig{
				GroupBy: "workstation", Output: &output, HomeDir: test.home,
				Query: metricsQueryStub(func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
					queryCalled = true
					return factoryvisualization.RuntimeMetricsQueryResult{}, nil
				}),
			})
			if err == nil {
				t.Fatal("RunMetrics() error = nil, want home-resolution failure")
			}
			var metricsErr *visualizationcli.MetricsError
			if !errors.As(err, &metricsErr) || metricsErr.CLIErrorCode() != visualizationcli.MetricsHomeDirectoryFailedCode ||
				!strings.Contains(metricsErr.CLIErrorMessage(), "resolve metrics home directory") {
				t.Fatalf("error = %#v, want coded home-resolution failure", err)
			}
			if test.wantCause != nil && !errors.Is(err, test.wantCause) {
				t.Fatalf("error = %v, want resolver cause", err)
			}
			if strings.Contains(err.Error(), "credential=do-not-leak") {
				t.Fatalf("home failure exposed its underlying payload: %v", err)
			}
			if queryCalled {
				t.Fatal("home-resolution failure invoked the metrics query")
			}
			if output.Len() != 0 {
				t.Fatalf("home-resolution failure wrote output %q", output.String())
			}
		})
	}
}

type metricsJSONDocument struct {
	Scope struct {
		Kind             string  `json:"kind"`
		FactorySessionID *string `json:"factory_session_id"`
	} `json:"scope"`
	GroupBy string `json:"group_by"`
	Units   struct {
		Tokens  string `json:"tokens"`
		Counts  string `json:"counts"`
		Latency string `json:"latency"`
	} `json:"units"`
	Cost struct {
		Availability string `json:"availability"`
	} `json:"cost"`
	Totals struct {
		InputTokens         float64            `json:"input_tokens"`
		OutputTokens        float64            `json:"output_tokens"`
		CompletedDispatches float64            `json:"completed_dispatches"`
		FailuresByReason    map[string]float64 `json:"failures_by_reason"`
	} `json:"totals"`
	Groups []struct {
		Key string `json:"key"`
	} `json:"groups"`
}

func assertMetricsJSONOutput(t *testing.T, output, groupBy, groupKey, sessionID string) {
	t.Helper()
	var document metricsJSONDocument
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		t.Fatalf("decode JSON output: %v\n%s", err, output)
	}
	assertMetricsJSONGrouping(t, document, groupBy, groupKey)
	assertMetricsJSONTotals(t, document)
	assertMetricsJSONMetadata(t, output, document)
	assertMetricsJSONScope(t, document, sessionID)
	assertMetricsJSONBreakdown(t, output)
}

func assertMetricsJSONGrouping(t *testing.T, document metricsJSONDocument, groupBy, groupKey string) {
	t.Helper()
	if document.GroupBy != groupBy || len(document.Groups) != 1 || document.Groups[0].Key != groupKey {
		t.Fatalf("JSON grouping = %q with groups %#v, want %q and only %q", document.GroupBy, document.Groups, groupBy, groupKey)
	}
}

func assertMetricsJSONTotals(t *testing.T, document metricsJSONDocument) {
	t.Helper()
	if document.Totals.InputTokens != 12 || document.Totals.OutputTokens != 8 || document.Totals.CompletedDispatches != 3 {
		t.Fatalf("JSON totals = %#v, want input 12, output 8, dispatches 3", document.Totals)
	}
	if document.Totals.FailuresByReason["timeout"] != 2 {
		t.Fatalf("JSON failures = %#v, want timeout 2", document.Totals.FailuresByReason)
	}
}

func assertMetricsJSONMetadata(t *testing.T, output string, document metricsJSONDocument) {
	t.Helper()
	if document.Units.Tokens != "tokens" || document.Units.Counts != "count" || document.Units.Latency != "milliseconds" {
		t.Fatalf("JSON units = %#v, want token/count/millisecond units", document.Units)
	}
	if document.Cost.Availability != "unavailable" || strings.Contains(output, "price") {
		t.Fatalf("JSON cost = %#v or contains a numeric price:\n%s", document.Cost, output)
	}
}

func assertMetricsJSONScope(t *testing.T, document metricsJSONDocument, sessionID string) {
	t.Helper()
	if sessionID == "" {
		if document.Scope.Kind != "all_factory_sessions" || document.Scope.FactorySessionID != nil {
			t.Fatalf("JSON unscoped value = %#v, want all_factory_sessions with null session", document.Scope)
		}
	} else if document.Scope.Kind != "factory_session" || document.Scope.FactorySessionID == nil || *document.Scope.FactorySessionID != sessionID {
		t.Fatalf("JSON session scope = %#v, want Factory Session %q", document.Scope, sessionID)
	}
}

func assertMetricsJSONBreakdown(t *testing.T, output string) {
	t.Helper()
	for _, omitted := range []string{"workstations", "worker_types", "providers"} {
		if strings.Contains(output, `"`+omitted+`"`) {
			t.Fatalf("JSON output included unrequested breakdown field %q:\n%s", omitted, output)
		}
	}
}

func executeMetricsCommand(
	t *testing.T,
	query factoryvisualization.RuntimeMetricsQuery,
	args []string,
) (string, error) {
	return executeMetricsCommandWithJSON(t, query, args, false)
}

func executeMetricsCommandWithJSON(
	t *testing.T,
	query factoryvisualization.RuntimeMetricsQuery,
	args []string,
	jsonOutput bool,
) (string, error) {
	t.Helper()
	command := visualizationcli.NewMetricsCommand(visualizationcli.MetricsCommandConfig{
		Query:   query,
		HomeDir: func() (string, error) { return "/home/operator", nil },
		JSON:    func() bool { return jsonOutput },
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
