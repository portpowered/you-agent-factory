package runtime_metrics_test

import (
	"context"
	"encoding/json"
	"net/http"
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

// TestMetricsDocumentedWorkflowThroughRootProcess follows the packaged guide
// from live-session discovery through scoped human/JSON reports. The server
// returns deterministic runtime facts, but every command crosses the same
// root-built generated-client and authored HTTP route used by customers.
func TestMetricsDocumentedWorkflowThroughRootProcess(t *testing.T) {
	t.Parallel()

	query := factoryvisualization.RuntimeMetricsQuery(func(_ context.Context, request factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
		switch request.SessionID {
		case "":
			return documentedMetricsResult(false), nil
		case "retained-live-id":
			return documentedMetricsResult(true), nil
		default:
			t.Fatalf("metrics query session ID = %q, want unscoped or resolved retained scope", request.SessionID)
			return factoryvisualization.RuntimeMetricsQueryResult{}, nil
		}
	})
	resolver := documentedMetricsScopeResolver(func(_ context.Context, sessionID string) (factoryvisualizationhttp.MetricsSessionScope, error) {
		switch sessionID {
		case "live-public-id":
			return factoryvisualizationhttp.MetricsSessionScope{
				RequestedID: sessionID,
				RetainedIDs: []string{"retained-live-id"},
			}, nil
		case "unmappable-live-id":
			return factoryvisualizationhttp.MetricsSessionScope{}, factoryvisualizationhttp.NewMetricsScopeUnavailableError(sessionID, nil)
		default:
			return factoryvisualizationhttp.MetricsSessionScope{}, factoryvisualizationhttp.NewMetricsSessionNotFoundError(sessionID, nil)
		}
	})
	metricsHandler := factoryvisualizationhttp.NewMetricsHandler(
		factoryvisualizationhttp.NewMetricsAdapter(query, resolver, t.TempDir()),
		zap.NewNop(),
	)
	apiServer := transporthttp.NewServerWithRecordingsAndMetricsAndCosts(
		nil, nil, nil, nil, nil, nil, zap.NewNop(), metricsHandler, nil,
	)
	mux := http.NewServeMux()
	mux.Handle("/metrics", apiServer.Handler())
	mux.HandleFunc("/factory-sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{{
				Id:      "live-public-id",
				Project: "metrics-workflow",
			}},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	home := t.TempDir()
	workingDirectory := t.TempDir()
	env := []string{"HOME=" + home, "USERPROFILE=" + home}

	list := runDocumentedMetricsCommand(t, process, env, workingDirectory,
		"you", "--json", "--server", server.URL, "session", "list", "--scope", "live")
	if list.err != nil {
		t.Fatalf("session list: %v\nstderr:\n%s", list.err, list.inputs.Stderr())
	}
	var sessions factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal([]byte(list.inputs.Stdout()), &sessions); err != nil {
		t.Fatalf("decode live session list: %v\n%s", err, list.inputs.Stdout())
	}
	if len(sessions.Sessions) != 1 || sessions.Sessions[0].Id == "" {
		t.Fatalf("live sessions = %#v, want one public ID", sessions.Sessions)
	}
	publicSessionID := sessions.Sessions[0].Id

	unscoped := runDocumentedMetricsCommand(t, process, env, workingDirectory,
		"you", "--json", "--server", server.URL, "metrics")
	scoped := runDocumentedMetricsCommand(t, process, env, workingDirectory,
		"you", "--json", "--server", server.URL, "metrics", "--session", publicSessionID)
	if unscoped.err != nil || scoped.err != nil {
		t.Fatalf("metrics workflow failed: unscoped=%v scoped=%v\nunscoped stderr=%s\nscoped stderr=%s", unscoped.err, scoped.err, unscoped.inputs.Stderr(), scoped.inputs.Stderr())
	}
	var unscopedReport, scopedReport factoryapi.MetricsReport
	decodeDocumentedMetricsReport(t, unscoped.inputs.Stdout(), &unscopedReport)
	decodeDocumentedMetricsReport(t, scoped.inputs.Stdout(), &scopedReport)
	if scopedReport.Totals.CompletedDispatches <= 0 || scopedReport.Totals.CompletedDispatches > unscopedReport.Totals.CompletedDispatches {
		t.Fatalf("scoped completion total = %v, unscoped = %v, want non-zero proper subset", scopedReport.Totals.CompletedDispatches, unscopedReport.Totals.CompletedDispatches)
	}
	if scopedReport.Totals.InputTokens > unscopedReport.Totals.InputTokens || scopedReport.Totals.OutputTokens > unscopedReport.Totals.OutputTokens {
		t.Fatalf("scoped token totals = (%v, %v), unscoped = (%v, %v), want component-wise subset", scopedReport.Totals.InputTokens, scopedReport.Totals.OutputTokens, unscopedReport.Totals.InputTokens, unscopedReport.Totals.OutputTokens)
	}

	for _, grouping := range []string{"workstation", "worker", "provider"} {
		human := runDocumentedMetricsCommand(t, process, env, workingDirectory,
			"you", "--server", server.URL, "metrics", "--group-by", grouping, "--session", publicSessionID)
		if human.err != nil {
			t.Fatalf("human metrics group=%s: %v\nstderr:\n%s", grouping, human.err, human.inputs.Stderr())
		}
		if !strings.Contains(human.inputs.Stdout(), "Breakdown by "+grouping) || human.inputs.Stderr() != "" {
			t.Fatalf("human metrics group=%s output=%q stderr=%q", grouping, human.inputs.Stdout(), human.inputs.Stderr())
		}
	}

	providerJSON := runDocumentedMetricsCommand(t, process, env, workingDirectory,
		"you", "--json", "--server", server.URL, "metrics", "--group-by", "provider", "--session", publicSessionID)
	if providerJSON.err != nil {
		t.Fatalf("provider JSON metrics: %v", providerJSON.err)
	}
	var providerReport struct {
		Totals factoryapi.MetricsAggregate `json:"totals"`
		Groups []struct {
			Key       string                      `json:"key"`
			Aggregate factoryapi.MetricsAggregate `json:"aggregate"`
		} `json:"groups"`
	}
	if err := json.Unmarshal([]byte(providerJSON.inputs.Stdout()), &providerReport); err != nil {
		t.Fatalf("decode provider metrics JSON: %v\n%s", err, providerJSON.inputs.Stdout())
	}
	completedByProvider := 0.0
	for _, group := range providerReport.Groups {
		if strings.Contains(group.Key, "${") {
			t.Fatalf("provider JSON exposed template key %q", group.Key)
		}
		completedByProvider += group.Aggregate.CompletedDispatches
	}
	if completedByProvider != providerReport.Totals.CompletedDispatches {
		t.Fatalf("provider completed dispatches = %v, want total %v", completedByProvider, providerReport.Totals.CompletedDispatches)
	}

	routeResponse, err := http.Get(server.URL + "/metrics?session_id=" + publicSessionID)
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer routeResponse.Body.Close()
	if routeResponse.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want 200", routeResponse.StatusCode)
	}
	var routeReport factoryapi.MetricsReport
	if err := json.NewDecoder(routeResponse.Body).Decode(&routeReport); err != nil {
		t.Fatalf("decode GET /metrics: %v", err)
	}
	if routeReport.Totals.CompletedDispatches != scopedReport.Totals.CompletedDispatches {
		t.Fatalf("route completed dispatches = %v, CLI = %v, want parity", routeReport.Totals.CompletedDispatches, scopedReport.Totals.CompletedDispatches)
	}

	for _, failure := range []struct {
		name       string
		sessionID  string
		wantCode   string
		wantPhrase string
	}{
		{name: "unknown", sessionID: "missing-live-id", wantCode: "METRICS_SESSION_NOT_FOUND", wantPhrase: "you session list --scope live"},
		{name: "unmappable", sessionID: "unmappable-live-id", wantCode: "METRICS_SESSION_SCOPE_UNAVAILABLE", wantPhrase: "you session list --scope live"},
	} {
		t.Run(failure.name, func(t *testing.T) {
			result := runDocumentedMetricsCommand(t, process, env, workingDirectory,
				"you", "--json", "--server", server.URL, "metrics", "--session", failure.sessionID)
			if result.err == nil {
				t.Fatal("metrics failure returned nil error")
			}
			if result.inputs.Stdout() != "" {
				t.Fatalf("metrics failure stdout = %q, want empty", result.inputs.Stdout())
			}
			assertMetricsDiagnostic(t, result.inputs.Stderr(), failure.wantCode,
				"Factory Session \""+failure.sessionID+"\" "+map[string]string{
					"METRICS_SESSION_NOT_FOUND":         "was not found; use `you session list --scope live` to choose a live ID",
					"METRICS_SESSION_SCOPE_UNAVAILABLE": "has no retained metrics scope; use `you session list --scope live` to choose a live ID",
				}[failure.wantCode])
			if !strings.Contains(result.inputs.Stderr(), failure.wantPhrase) {
				t.Fatalf("metrics failure stderr = %q, want recovery phrase %q", result.inputs.Stderr(), failure.wantPhrase)
			}
		})
	}
}

type documentedMetricsScopeResolver func(context.Context, string) (factoryvisualizationhttp.MetricsSessionScope, error)

func (resolver documentedMetricsScopeResolver) ResolveMetricsSessionScope(
	ctx context.Context,
	sessionID string,
) (factoryvisualizationhttp.MetricsSessionScope, error) {
	return resolver(ctx, sessionID)
}

type documentedMetricsCommandResult struct {
	inputs *support.CapturedInputs
	err    error
}

func runDocumentedMetricsCommand(
	t *testing.T,
	process support.Process,
	env []string,
	workingDirectory string,
	args ...string,
) documentedMetricsCommandResult {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = workingDirectory
	return documentedMetricsCommandResult{inputs: inputs, err: process.Execute(inputs.Input)}
}

func decodeDocumentedMetricsReport(t *testing.T, output string, report *factoryapi.MetricsReport) {
	t.Helper()
	if err := json.Unmarshal([]byte(output), report); err != nil {
		t.Fatalf("decode metrics report: %v\n%s", err, output)
	}
}

func documentedMetricsResult(scoped bool) factoryvisualization.RuntimeMetricsQueryResult {
	inputTokens := 12.0
	outputTokens := 8.0
	completedDispatches := 5.0
	if scoped {
		inputTokens = 7
		outputTokens = 4
		completedDispatches = 2
	}
	aggregate := factoryvisualization.RuntimeMetricsAggregate{
		InputTokens:         inputTokens,
		OutputTokens:        outputTokens,
		CompletedDispatches: completedDispatches,
	}
	return factoryvisualization.RuntimeMetricsQueryResult{
		Cost:   factoryvisualization.RuntimeMetricsCost{Availability: factoryvisualization.RuntimeMetricsCostUnavailable},
		Totals: aggregate,
		Workstations: []factoryvisualization.RuntimeMetricsBreakdown{{
			Key:       "workstation-a",
			Aggregate: aggregate,
		}},
		WorkerTypes: []factoryvisualization.RuntimeMetricsBreakdown{{
			Key:       "worker-a",
			Aggregate: aggregate,
		}},
		Providers: []factoryvisualization.RuntimeMetricsBreakdown{
			{Key: "codex", Aggregate: factoryvisualization.RuntimeMetricsAggregate{
				InputTokens: 3, OutputTokens: 2, CompletedDispatches: 1,
			}},
			{Key: factoryvisualization.RuntimeMetricsUnavailableProviderKey, Aggregate: factoryvisualization.RuntimeMetricsAggregate{
				InputTokens: inputTokens - 3, OutputTokens: outputTokens - 2, CompletedDispatches: completedDispatches - 1,
			}},
		},
	}
}
