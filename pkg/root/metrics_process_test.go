package root

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryvisualizationcli "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
	factoryvisualizationhttp "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http"
	factoryvisualizationwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/wire"
	transporthttp "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestMetricsProductionRouteAndCLIThroughBuildProcess(t *testing.T) {
	t.Parallel()

	var (
		requestsMu sync.Mutex
		requests   []factoryvisualization.RuntimeMetricsQueryRequest
	)
	query := factoryvisualization.RuntimeMetricsQuery(func(
		_ context.Context,
		request factoryvisualization.RuntimeMetricsQueryRequest,
	) (factoryvisualization.RuntimeMetricsQueryResult, error) {
		requestsMu.Lock()
		requests = append(requests, request)
		requestsMu.Unlock()
		switch request.SessionID {
		case "":
			return rootMetricsResult(12, 8, 3), nil
		case "retained-live-id":
			return rootMetricsResult(5, 3, 1), nil
		case "retained-empty-id":
			return factoryvisualization.RuntimeMetricsQueryResult{
				Cost: factoryvisualization.RuntimeMetricsCost{
					Availability: factoryvisualization.RuntimeMetricsCostUnavailable,
				},
			}, nil
		default:
			return factoryvisualization.RuntimeMetricsQueryResult{}, nil
		}
	})
	resolver := rootMetricsScopeResolver(func(
		_ context.Context,
		sessionID string,
	) (factoryvisualizationhttp.MetricsSessionScope, error) {
		switch sessionID {
		case "live-public-id":
			return factoryvisualizationhttp.MetricsSessionScope{
				RequestedID: sessionID,
				RetainedIDs: []string{"retained-live-id"},
			}, nil
		case "empty-public-id":
			return factoryvisualizationhttp.MetricsSessionScope{
				RequestedID: sessionID,
				RetainedIDs: []string{"retained-empty-id"},
			}, nil
		case "unmappable-live-id":
			return factoryvisualizationhttp.MetricsSessionScope{},
				factoryvisualizationhttp.NewMetricsScopeUnavailableError(sessionID, nil)
		default:
			return factoryvisualizationhttp.MetricsSessionScope{},
				factoryvisualizationhttp.NewMetricsSessionNotFoundError(sessionID, nil)
		}
	})
	metricsHandler := factoryvisualizationhttp.NewMetricsHandler(
		factoryvisualizationhttp.NewMetricsAdapter(query, resolver, t.TempDir()),
		zap.NewNop(),
	)
	apiServer := transporthttp.NewServerWithRecordingsAndMetricsAndCosts(
		nil, nil, nil, nil, nil, nil, zap.NewNop(), metricsHandler, nil,
	)
	server := httptest.NewServer(apiServer.Handler())
	t.Cleanup(server.Close)

	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	t.Cleanup(func() {
		_ = process.Close(context.Background())
	})
	home := t.TempDir()
	workingDirectory := t.TempDir()

	response, err := http.Get(server.URL + "/metrics?session_id=live-public-id")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want 200", response.StatusCode)
	}
	var routeReport factoryapi.MetricsReport
	if err := json.NewDecoder(response.Body).Decode(&routeReport); err != nil {
		t.Fatalf("decode mapped route report: %v", err)
	}
	if routeReport.Scope.FactorySessionId == nil || *routeReport.Scope.FactorySessionId != "live-public-id" {
		t.Fatalf("route report scope = %#v, want public live ID", routeReport.Scope)
	}
	if routeReport.Totals.CompletedDispatches != 1 || routeReport.Totals.InputTokens != 5 {
		t.Fatalf("route report totals = %#v, want mapped non-zero totals", routeReport.Totals)
	}

	var unscopedJSON bytes.Buffer
	if err := process.Execute(rootMetricsProcessInput(
		home, workingDirectory, server.URL, &unscopedJSON,
		"--json", "metrics",
	)); err != nil {
		t.Fatalf("BuildProcess CLI unscoped metrics: %v", err)
	}
	if !strings.Contains(unscopedJSON.String(), `"completed_dispatches":3`) ||
		!strings.Contains(unscopedJSON.String(), `"kind":"all_factory_sessions"`) {
		t.Fatalf("unscoped CLI JSON = %q, want all-session report", unscopedJSON.String())
	}

	var scopedJSON bytes.Buffer
	if err := process.Execute(rootMetricsProcessInput(
		home, workingDirectory, server.URL, &scopedJSON,
		"--json", "metrics", "--session", "live-public-id",
	)); err != nil {
		t.Fatalf("BuildProcess CLI mapped metrics: %v", err)
	}
	if !strings.Contains(scopedJSON.String(), `"completed_dispatches":1`) ||
		!strings.Contains(scopedJSON.String(), `"factory_session_id":"live-public-id"`) {
		t.Fatalf("mapped CLI JSON = %q, want scoped non-zero report", scopedJSON.String())
	}

	var human bytes.Buffer
	if err := process.Execute(rootMetricsProcessInput(
		home, workingDirectory, server.URL, &human,
		"metrics", "--group-by", "provider", "--session", "live-public-id",
	)); err != nil {
		t.Fatalf("BuildProcess CLI human metrics: %v", err)
	}
	for _, want := range []string{
		"Scope: Factory Session live-public-id",
		"Group by: provider",
		"Completed dispatches: 1",
		"provider-a:",
	} {
		if !strings.Contains(human.String(), want) {
			t.Fatalf("human CLI output missing %q:\n%s", want, human.String())
		}
	}

	var emptyJSON bytes.Buffer
	if err := process.Execute(rootMetricsProcessInput(
		home, workingDirectory, server.URL, &emptyJSON,
		"--json", "metrics", "--session", "empty-public-id",
	)); err != nil {
		t.Fatalf("BuildProcess CLI verified-empty metrics: %v", err)
	}
	if !strings.Contains(emptyJSON.String(), `"completed_dispatches":0`) ||
		!strings.Contains(emptyJSON.String(), `"factory_session_id":"empty-public-id"`) {
		t.Fatalf("verified-empty CLI JSON = %q, want empty scoped report", emptyJSON.String())
	}

	for _, test := range []struct {
		name           string
		sessionID      string
		wantHTTPStatus int
		wantCode       string
	}{
		{name: "unknown", sessionID: "missing-live-id", wantHTTPStatus: http.StatusNotFound, wantCode: factoryvisualizationcli.MetricsSessionNotFoundCode},
		{name: "unmappable", sessionID: "unmappable-live-id", wantHTTPStatus: http.StatusServiceUnavailable, wantCode: factoryvisualizationcli.MetricsScopeUnavailableCode},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := process.Execute(rootMetricsProcessInput(
				home, workingDirectory, server.URL, &stdout,
				"--json", "metrics", "--session", test.sessionID,
			))
			if err == nil || !strings.Contains(err.Error(), test.wantCode) ||
				!strings.Contains(err.Error(), "you session list --scope live") {
				t.Fatalf("CLI error = %v, want %s with live-session guidance", err, test.wantCode)
			}
			if stdout.Len() != 0 {
				t.Fatalf("failed CLI stdout = %q, want empty", stdout.String())
			}

			response, requestErr := http.Get(server.URL + "/metrics?session_id=" + test.sessionID)
			if requestErr != nil {
				t.Fatalf("GET /metrics %s: %v", test.sessionID, requestErr)
			}
			response.Body.Close()
			if response.StatusCode != test.wantHTTPStatus {
				t.Fatalf("GET /metrics %s status = %d, want %d", test.sessionID, response.StatusCode, test.wantHTTPStatus)
			}
			if response.Header.Get("Content-Type") == "" {
				t.Fatalf("GET /metrics %s did not return JSON content type", test.sessionID)
			}
		})
	}

	requestsMu.Lock()
	defer requestsMu.Unlock()
	if len(requests) < 5 {
		t.Fatalf("metrics query requests = %d, want unscoped, mapped, empty, and route coverage", len(requests))
	}
}

func TestMetricsProductionRouteAndCLIReportReconciledProviderGroups(t *testing.T) {
	t.Parallel()

	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(
		rootMetricsReader{records: []factoryvisualization.RuntimeMetricRecord{
			{"metric_name": "dispatch.completed", "value": 1.0, "session_id": "retained-live-id", "dispatch_id": "dispatch-codex"},
			{"metric_name": "provider.input_tokens", "value": 4.0, "session_id": "retained-live-id", "dispatch_id": "dispatch-codex", "provider": "codex", "unit": "tokens"},
			{"metric_name": "dispatch.duration", "value": 10.0, "session_id": "retained-live-id", "dispatch_id": "dispatch-codex", "unit": "ms"},
			{"metric_name": "dispatch.completed", "value": 1.0, "session_id": "retained-live-id", "dispatch_id": "dispatch-claude"},
			{"metric_name": "provider.completed", "value": 1.0, "session_id": "retained-live-id", "dispatch_id": "dispatch-claude", "provider": "claude"},
			{"metric_name": "provider.input_tokens", "value": 3.0, "session_id": "retained-live-id", "dispatch_id": "dispatch-claude", "provider": "${workerProvider}", "unit": "tokens"},
			{"metric_name": "provider.duration", "value": 6.0, "session_id": "retained-live-id", "dispatch_id": "dispatch-claude", "unit": "ms"},
		}},
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}
	resolver := rootMetricsScopeResolver(func(
		_ context.Context,
		sessionID string,
	) (factoryvisualizationhttp.MetricsSessionScope, error) {
		if sessionID != "live-public-id" {
			return factoryvisualizationhttp.MetricsSessionScope{},
				factoryvisualizationhttp.NewMetricsSessionNotFoundError(sessionID, nil)
		}
		return factoryvisualizationhttp.MetricsSessionScope{
			RequestedID: sessionID,
			RetainedIDs: []string{"retained-live-id"},
		}, nil
	})
	metricsHandler := factoryvisualizationhttp.NewMetricsHandler(
		factoryvisualizationhttp.NewMetricsAdapter(query, resolver, t.TempDir()),
		zap.NewNop(),
	)
	apiServer := transporthttp.NewServerWithRecordingsAndMetricsAndCosts(
		nil, nil, nil, nil, nil, nil, zap.NewNop(), metricsHandler, nil,
	)
	server := httptest.NewServer(apiServer.Handler())
	t.Cleanup(server.Close)

	response, err := http.Get(server.URL + "/metrics?session_id=live-public-id")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status = %d", response.StatusCode)
	}
	var routeReport factoryapi.MetricsReport
	if err := json.NewDecoder(response.Body).Decode(&routeReport); err != nil {
		t.Fatalf("decode route report: %v", err)
	}
	assertReconciledMetricsReport(t, routeReport)

	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	t.Cleanup(func() { _ = process.Close(context.Background()) })
	var cliJSON bytes.Buffer
	if err := process.Execute(rootMetricsProcessInput(
		t.TempDir(), t.TempDir(), server.URL, &cliJSON,
		"--json", "metrics", "--group-by", "provider", "--session", "live-public-id",
	)); err != nil {
		t.Fatalf("BuildProcess CLI metrics: %v", err)
	}
	var cliReport struct {
		GroupBy string `json:"group_by"`
		Totals  struct {
			InputTokens         float64 `json:"input_tokens"`
			CompletedDispatches float64 `json:"completed_dispatches"`
		} `json:"totals"`
		Groups []struct {
			Key       string `json:"key"`
			Aggregate struct {
				InputTokens         float64 `json:"input_tokens"`
				CompletedDispatches float64 `json:"completed_dispatches"`
			} `json:"aggregate"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(cliJSON.Bytes(), &cliReport); err != nil {
		t.Fatalf("decode CLI report: %v\n%s", err, cliJSON.String())
	}
	if cliReport.GroupBy != "provider" || cliReport.Totals.InputTokens != 7 || cliReport.Totals.CompletedDispatches != 2 {
		t.Fatalf("CLI totals = %#v, want provider grouping with seven input tokens and two completions", cliReport)
	}
	if len(cliReport.Groups) != 2 {
		t.Fatalf("CLI provider groups = %#v, want codex and claude", cliReport.Groups)
	}
	for _, group := range cliReport.Groups {
		if group.Aggregate.CompletedDispatches != 1 {
			t.Fatalf("CLI provider group = %#v, want one completion", group)
		}
		if (group.Key == "codex" && group.Aggregate.InputTokens != 4) ||
			(group.Key == "claude" && group.Aggregate.InputTokens != 3) {
			t.Fatalf("CLI provider group = %#v, want codex=4 and claude=3 input tokens", group)
		}
	}
}

func assertReconciledMetricsReport(t *testing.T, report factoryapi.MetricsReport) {
	t.Helper()
	if report.Totals.CompletedDispatches != 2 || report.Totals.InputTokens != 7 {
		t.Fatalf("metrics totals = %#v, want two completions and seven input tokens", report.Totals)
	}
	if len(report.Providers) != 2 {
		t.Fatalf("provider groups = %#v, want codex and claude", report.Providers)
	}
	for _, breakdown := range report.Providers {
		if breakdown.Aggregate.CompletedDispatches != 1 {
			t.Fatalf("provider breakdown = %#v, want one completion per provider", breakdown)
		}
		if breakdown.Key == "codex" && breakdown.Aggregate.InputTokens != 4 {
			t.Fatalf("codex breakdown = %#v, want four input tokens", breakdown)
		}
		if breakdown.Key == "claude" && breakdown.Aggregate.InputTokens != 3 {
			t.Fatalf("claude breakdown = %#v, want three input tokens", breakdown)
		}
		if strings.Contains(breakdown.Key, "${") {
			t.Fatalf("provider breakdown leaked template key: %#v", breakdown)
		}
	}
}

type rootMetricsReader struct {
	records []factoryvisualization.RuntimeMetricRecord
}

func (reader rootMetricsReader) Read(context.Context, string) ([]factoryvisualization.RuntimeMetricRecord, error) {
	return append([]factoryvisualization.RuntimeMetricRecord(nil), reader.records...), nil
}

type rootMetricsScopeResolver func(context.Context, string) (factoryvisualizationhttp.MetricsSessionScope, error)

func (resolver rootMetricsScopeResolver) ResolveMetricsSessionScope(
	ctx context.Context,
	sessionID string,
) (factoryvisualizationhttp.MetricsSessionScope, error) {
	return resolver(ctx, sessionID)
}

func rootMetricsProcessInput(
	home, workingDirectory, server string,
	stdout *bytes.Buffer,
	args ...string,
) Input {
	return Input{
		Args:             append([]string{"you", "--server", server}, args...),
		Env:              homeEnvironment(home),
		Stdout:           stdout,
		Stderr:           bytes.NewBuffer(nil),
		Context:          context.Background(),
		WorkingDirectory: workingDirectory,
	}
}

func rootMetricsResult(inputTokens, outputTokens, completed float64) factoryvisualization.RuntimeMetricsQueryResult {
	return factoryvisualization.RuntimeMetricsQueryResult{
		Cost: factoryvisualization.RuntimeMetricsCost{
			Availability: factoryvisualization.RuntimeMetricsCostUnavailable,
		},
		Totals: factoryvisualization.RuntimeMetricsAggregate{
			InputTokens:         inputTokens,
			OutputTokens:        outputTokens,
			CompletedDispatches: completed,
		},
		Workstations: []factoryvisualization.RuntimeMetricsBreakdown{{
			Key: "workstation-a",
			Aggregate: factoryvisualization.RuntimeMetricsAggregate{
				InputTokens:         inputTokens,
				OutputTokens:        outputTokens,
				CompletedDispatches: completed,
			},
		}},
		WorkerTypes: []factoryvisualization.RuntimeMetricsBreakdown{{
			Key: "worker-a",
			Aggregate: factoryvisualization.RuntimeMetricsAggregate{
				InputTokens:         inputTokens,
				OutputTokens:        outputTokens,
				CompletedDispatches: completed,
			},
		}},
		Providers: []factoryvisualization.RuntimeMetricsBreakdown{{
			Key: "provider-a",
			Aggregate: factoryvisualization.RuntimeMetricsAggregate{
				InputTokens:         inputTokens,
				OutputTokens:        outputTokens,
				CompletedDispatches: completed,
			},
		}},
	}
}
