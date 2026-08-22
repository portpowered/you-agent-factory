package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
)

type metricsClientStub struct {
	response *generatedclient.GetMetricsClientResponse
	err      error
	params   *generatedclient.GetMetricsParams
}

func (stub *metricsClientStub) GetMetricsWithResponse(
	_ context.Context,
	params *generatedclient.GetMetricsParams,
	_ ...generatedclient.RequestEditorFn,
) (*generatedclient.GetMetricsClientResponse, error) {
	stub.params = params
	return stub.response, stub.err
}

func TestMetricsOperationUsesGeneratedClientAndRendersCompleteReport(t *testing.T) {
	client := &metricsClientStub{response: &generatedclient.GetMetricsClientResponse{
		JSON200: &generatedclient.MetricsReport{
			Cost: generatedclient.MetricsCost{Availability: "UNAVAILABLE"},
			Scope: generatedclient.MetricsScope{
				Kind: "FACTORY_SESSION",
			},
			Totals: generatedclient.MetricsAggregate{
				InputTokens:         12,
				OutputTokens:        8,
				CompletedDispatches: 3,
				FailuresByReason:    map[string]float64{"timeout": 1},
				DispatchLatency:     generatedclient.MetricsDuration{Unit: "milliseconds", Samples: 1},
				ProviderLatency:     generatedclient.MetricsDuration{Unit: "milliseconds", Samples: 1},
			},
			Providers: []generatedclient.MetricsBreakdown{{
				Key: "provider-a",
				Aggregate: generatedclient.MetricsAggregate{
					InputTokens: 12,
				},
			}},
		}},
	}
	operation := NewOperation(func(server string) (Client, error) {
		if server != "http://metrics.test" {
			t.Fatalf("client server = %q, want http://metrics.test", server)
		}
		return client, nil
	})
	var output bytes.Buffer
	err := RunMetricsOperation(context.Background(), operation, MetricsConfig{
		Server:    " http://metrics.test ",
		GroupBy:   "provider",
		SessionID: "public-live-id",
		JSON:      true,
		Output:    &output,
	})
	if err != nil {
		t.Fatalf("RunMetricsOperation() error = %v", err)
	}
	if client.params == nil || client.params.SessionId == nil || *client.params.SessionId != "public-live-id" {
		t.Fatalf("generated client params = %#v, want session_id public-live-id", client.params)
	}
	for _, want := range []string{`"kind":"factory_session"`, `"group_by":"provider"`, `"provider-a"`, `"input_tokens":12`} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("metrics output missing %q:\n%s", want, output.String())
		}
	}
}

func TestMetricsOperationNormalizesAcceptedGroupBeforeRendering(t *testing.T) {
	client := &metricsClientStub{response: &generatedclient.GetMetricsClientResponse{
		JSON200: &generatedclient.MetricsReport{
			Cost:      generatedclient.MetricsCost{Availability: "UNAVAILABLE"},
			Providers: []generatedclient.MetricsBreakdown{{Key: "provider-a"}},
		},
	}}
	operation := NewOperation(func(string) (Client, error) { return client, nil })

	var output bytes.Buffer
	if err := RunMetricsOperation(context.Background(), operation, MetricsConfig{
		Server:  "http://metrics.test",
		GroupBy: " PROVIDER ",
		JSON:    false,
		Output:  &output,
	}); err != nil {
		t.Fatalf("RunMetricsOperation() error = %v", err)
	}
	if !strings.Contains(output.String(), "Breakdown by provider: 1 rows") ||
		!strings.Contains(output.String(), "provider-a:") {
		t.Fatalf("normalized provider output = %q", output.String())
	}
}

func TestMetricsOperationMapsTypedHTTPFailuresWithoutPartialOutput(t *testing.T) {
	tests := []struct {
		name     string
		response *generatedclient.GetMetricsClientResponse
		wantCode string
		wantFam  string
	}{
		{
			name: "session not found",
			response: &generatedclient.GetMetricsClientResponse{
				JSON404: &generatedclient.NotFound{Message: "Factory Session missing-live-id was not found; use `you session list --scope live`"},
			},
			wantCode: MetricsSessionNotFoundCode,
			wantFam:  "NOT_FOUND",
		},
		{
			name: "scope unavailable",
			response: &generatedclient.GetMetricsClientResponse{
				JSON503: &generatedclient.MetricsSessionScopeUnavailable{Message: "Factory Session known-live-id has no retained metrics scope; use `you session list --scope live`"},
			},
			wantCode: MetricsScopeUnavailableCode,
			wantFam:  "INTERNAL_SERVER_ERROR",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operation := NewOperation(func(string) (Client, error) {
				return &metricsClientStub{response: test.response}, nil
			})
			var output bytes.Buffer
			err := RunMetricsOperation(context.Background(), operation, MetricsConfig{
				Server: "http://metrics.test", GroupBy: "workstation", Output: &output,
			})
			if err == nil {
				t.Fatal("RunMetricsOperation() error = nil, want typed HTTP failure")
			}
			var metricsErr *MetricsError
			if !errors.As(err, &metricsErr) || metricsErr.CLIErrorCode() != test.wantCode || string(metricsErr.CLIErrorFamily()) != test.wantFam {
				t.Fatalf("error = %#v, want code %q family %q", err, test.wantCode, test.wantFam)
			}
			if !strings.Contains(metricsErr.CLIErrorMessage(), "you session list --scope live") {
				t.Fatalf("error message = %q, want live-session guidance", metricsErr.CLIErrorMessage())
			}
			if output.Len() != 0 {
				t.Fatalf("failed metrics request wrote partial output %q", output.String())
			}
		})
	}
}

func TestMetricsReportFromAPIPreservesMissingLatencyAsEmptySamples(t *testing.T) {
	result := metricsReportFromAPI(generatedclient.MetricsReport{
		Totals: generatedclient.MetricsAggregate{
			DispatchLatency: generatedclient.MetricsDuration{Unit: "milliseconds"},
			ProviderLatency: generatedclient.MetricsDuration{Unit: "milliseconds"},
		},
	})
	if result.Totals.DispatchDuration == nil || result.Totals.DispatchDuration.Samples != 0 || result.Totals.DispatchDuration.P50 != nil {
		t.Fatalf("dispatch duration = %#v, want explicit empty duration", result.Totals.DispatchDuration)
	}
	if result.Totals.ProviderDuration == nil || result.Totals.ProviderDuration.Samples != 0 || result.Totals.ProviderDuration.P95 != nil {
		t.Fatalf("provider duration = %#v, want explicit empty duration", result.Totals.ProviderDuration)
	}
	if result.Cost.Availability != factoryvisualization.RuntimeMetricsCostAvailability("") {
		t.Fatalf("cost availability = %q, want preserved empty value", result.Cost.Availability)
	}
}
