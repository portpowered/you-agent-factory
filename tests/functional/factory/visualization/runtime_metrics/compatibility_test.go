package runtime_metrics_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestExistingMetricsCommandsMatchSelectedPublicAPIs(t *testing.T) {
	t.Parallel()
	fixture := newBoundarySessionFixture(t, "compatibility", "compatibility-session", "0.0123")
	server := newBoundaryServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/metrics":
			writeBoundaryMetricsReport(writer, fixture)
		case "/metrics/costs":
			writeBoundaryCostsReport(writer, fixture)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})
	process := runtimeMetricsCLIProcess

	metricsJSON := boundaryInputs(t, t.Context(), "you", "--json", "--server", server.URL(), "metrics", "--session", fixture.sessionID)
	if err := process.Execute(metricsJSON.Input); err != nil {
		t.Fatalf("existing metrics JSON error = %v\nstdout:\n%s\nstderr:\n%s", err, metricsJSON.Stdout(), metricsJSON.Stderr())
	}
	assertCompatibleMetricsJSON(t, metricsJSON.Stdout(), fixture)

	metricsHuman := boundaryInputs(t, t.Context(), "you", "--server", server.URL(), "metrics", "--session", fixture.sessionID)
	if err := process.Execute(metricsHuman.Input); err != nil {
		t.Fatalf("existing metrics human error = %v\nstdout:\n%s\nstderr:\n%s", err, metricsHuman.Stdout(), metricsHuman.Stderr())
	}
	for _, marker := range []string{"Scope: Factory Session " + fixture.sessionID, "Input tokens: 10", "Cost: unavailable", "Breakdown by workstation"} {
		if !strings.Contains(metricsHuman.Stdout(), marker) {
			t.Fatalf("existing metrics human output missing %q:\n%s", marker, metricsHuman.Stdout())
		}
	}

	metricsAPI := getBoundaryMetricsReport(t, server.URL(), fixture.sessionID)
	var metricsDocument struct {
		Scope struct {
			Kind             string  `json:"kind"`
			FactorySessionID *string `json:"factory_session_id"`
		} `json:"scope"`
	}
	if err := json.Unmarshal([]byte(metricsJSON.Stdout()), &metricsDocument); err != nil {
		t.Fatalf("decode existing metrics JSON: %v", err)
	}
	if metricsAPI.Scope.FactorySessionId == nil || metricsDocument.Scope.Kind != "factory_session" || metricsDocument.Scope.FactorySessionID == nil || *metricsDocument.Scope.FactorySessionID != *metricsAPI.Scope.FactorySessionId {
		t.Fatalf("metrics scope = %#v, want selected API scope %#v", metricsDocument.Scope, metricsAPI.Scope)
	}

	costJSON := boundaryInputs(t, t.Context(), "you", "--json", "--server", server.URL(), "metrics", "costs", "--session", fixture.sessionID)
	if err := process.Execute(costJSON.Input); err != nil {
		t.Fatalf("existing costs JSON error = %v\nstdout:\n%s\nstderr:\n%s", err, costJSON.Stdout(), costJSON.Stderr())
	}
	var costDocument generatedclient.CostsReport
	if err := json.Unmarshal([]byte(costJSON.Stdout()), &costDocument); err != nil {
		t.Fatalf("decode existing costs JSON: %v\n%s", err, costJSON.Stdout())
	}
	costAPI := getBoundaryCostsReport(t, server.URL(), fixture.sessionID)
	if !reflect.DeepEqual(costDocument, costAPI) {
		t.Fatalf("existing costs JSON = %#v, want selected API report %#v", costDocument, costAPI)
	}

	costHuman := boundaryInputs(t, t.Context(), "you", "--server", server.URL(), "metrics", "costs", "--session", fixture.sessionID)
	if err := process.Execute(costHuman.Input); err != nil {
		t.Fatalf("existing costs human error = %v\nstdout:\n%s\nstderr:\n%s", err, costHuman.Stdout(), costHuman.Stderr())
	}
	for _, marker := range []string{"Scope: Factory Session " + fixture.sessionID, "Currency: USD", "Status: PRICED", "Cost (USD): $0.01", "Price source: BUILT_IN"} {
		if !strings.Contains(costHuman.Stdout(), marker) {
			t.Fatalf("existing costs human output missing %q:\n%s", marker, costHuman.Stdout())
		}
	}
	if metricsJSON.Stderr() != "" || metricsHuman.Stderr() != "" || costJSON.Stderr() != "" || costHuman.Stderr() != "" {
		t.Fatalf("compatibility diagnostics leaked: metrics-json=%q metrics-human=%q costs-json=%q costs-human=%q", metricsJSON.Stderr(), metricsHuman.Stderr(), costJSON.Stderr(), costHuman.Stderr())
	}
	assertBoundaryRequestLog(t, server.log,
		"GET /metrics?session_id="+fixture.sessionID,
		"GET /metrics?session_id="+fixture.sessionID,
		"GET /metrics?session_id="+fixture.sessionID,
		"GET /metrics/costs?session_id="+fixture.sessionID,
		"GET /metrics/costs?session_id="+fixture.sessionID,
		"GET /metrics/costs?session_id="+fixture.sessionID,
	)
}

func TestExistingCostsCommandFailureRemainsCodedAndAtomic(t *testing.T) {
	server := newBoundaryServer(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/metrics/costs" {
			writeBoundaryError(writer, http.StatusInternalServerError, factoryapi.ErrorResponseCode("COST_FIXTURE_FAILED"), factoryapi.ErrorFamilyInternalServerError, "the selected cost fixture is unavailable")
			return
		}
		writer.WriteHeader(http.StatusNotFound)
	})
	inputs := boundaryInputs(t, t.Context(), "you", "--json", "--server", server.URL(), "metrics", "costs", "--session", "compatibility-session")
	err := runtimeMetricsCLIProcess.Execute(inputs.Input)
	assertBoundaryCodedFailure(t, err, inputs, "COST_FIXTURE_FAILED")
	assertBoundaryRequestLog(t, server.log, "GET /metrics/costs?session_id=compatibility-session")
}

func TestMetricsSessionCostHumanOutputUsesCostsRenderer(t *testing.T) {
	t.Parallel()
	fixture := newBoundarySessionFixture(t, "human-cost", "human-cost-session", "0.0123")
	server := newBoundaryServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/metrics":
			writeBoundaryMetricsReport(writer, fixture)
		case "/factory-sessions/" + fixture.sessionID + "/events":
			writeBoundaryEvents(writer, fixture.events)
		case "/metrics/costs":
			writeBoundaryCostsReport(writer, fixture)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})
	inputs := boundaryInputs(t, t.Context(), "you", "--server", server.URL(), "metrics", "session", fixture.sessionID, "--lens", "cost")
	if err := runtimeMetricsCLIProcess.Execute(inputs.Input); err != nil {
		t.Fatalf("metrics session human cost error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	for _, marker := range []string{"COST", "Currency: USD", "Status: PRICED", "Cost (USD): $0.01", "Price source: BUILT_IN"} {
		if !strings.Contains(inputs.Stdout(), marker) {
			t.Fatalf("metrics session human cost output missing %q:\n%s", marker, inputs.Stdout())
		}
	}
	if inputs.Stderr() != "" {
		t.Fatalf("metrics session human cost stderr = %q, want empty", inputs.Stderr())
	}
	assertBoundaryRequestLog(t, server.log,
		"GET /metrics?session_id="+fixture.sessionID,
		"GET /factory-sessions/"+fixture.sessionID+"/events",
		"GET /metrics/costs?session_id="+fixture.sessionID,
	)
}

func assertCompatibleMetricsJSON(t *testing.T, output string, fixture boundarySessionFixture) {
	t.Helper()
	var document struct {
		GroupBy string `json:"group_by"`
		Cost    struct {
			Availability string `json:"availability"`
		} `json:"cost"`
		Totals struct {
			InputTokens         float64 `json:"input_tokens"`
			OutputTokens        float64 `json:"output_tokens"`
			CompletedDispatches float64 `json:"completed_dispatches"`
		} `json:"totals"`
	}
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		t.Fatalf("decode compatible metrics JSON: %v\n%s", err, output)
	}
	if document.GroupBy != "workstation" || document.Cost.Availability != "unavailable" || document.Totals.InputTokens != 10 || document.Totals.OutputTokens != 5 || document.Totals.CompletedDispatches != 1 {
		t.Fatalf("compatible metrics JSON = %#v, want selected session %q totals", document, fixture.sessionID)
	}
}

func getBoundaryMetricsReport(t *testing.T, serverURL, sessionID string) generatedclient.MetricsReport {
	t.Helper()
	response, err := http.Get(serverURL + "/metrics?session_id=" + sessionID)
	if err != nil {
		t.Fatalf("GET selected metrics report: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET selected metrics report status = %d, want 200", response.StatusCode)
	}
	var report generatedclient.MetricsReport
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatalf("decode selected metrics report: %v", err)
	}
	return report
}

func getBoundaryCostsReport(t *testing.T, serverURL, sessionID string) generatedclient.CostsReport {
	t.Helper()
	response, err := http.Get(serverURL + "/metrics/costs?session_id=" + sessionID)
	if err != nil {
		t.Fatalf("GET selected costs report: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET selected costs report status = %d, want 200", response.StatusCode)
	}
	var report generatedclient.CostsReport
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatalf("decode selected costs report: %v", err)
	}
	return report
}
