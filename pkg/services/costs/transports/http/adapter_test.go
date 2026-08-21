package http

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestAdapterMapsExactReportAndRuntimeInputs(t *testing.T) {
	t.Parallel()
	inputTokens := int64(12)
	outputTokens := int64(0)
	query := costs.CostsQuery(func(_ context.Context, request costs.QueryRequest) (costs.Report, error) {
		if request.MetricsRoot != "metrics-root" || request.OperatorSettingsPath != "settings.json" || request.FactorySessionID != "session-a" {
			t.Fatalf("Costs request = %#v", request)
		}
		amount := "123456789.000001"
		return costs.Report{
			Scope:          costs.Scope{Kind: costs.ScopeFactorySession, FactorySessionID: "session-a"},
			Currency:       "USD",
			Status:         costs.StatusPartial,
			PricedSubtotal: &amount,
			Coverage:       costs.Coverage{EncounteredRows: 2, PricedRows: 1, UnpricedRows: 1, EncounteredProviderModels: 2, PricedProviderModels: 1, UnpricedProviderModels: 1},
			LineItems: []costs.LineItem{
				{Model: "known", Status: costs.StatusPriced, PricedAmount: &amount, TokenCounts: costs.TokenCounts{InputTokens: &inputTokens, OutputTokens: &outputTokens}},
				{Model: "unknown", Status: costs.StatusUnpriced, Reason: "no configured price"},
			},
			ProviderModels: []costs.ProviderModelRollup{{Provider: "provider", Model: "known", Rollup: costs.Rollup{Key: "provider/known", Status: costs.StatusPriced, PricedSubtotal: &amount}}},
		}, nil
	})
	adapter := NewAdapter(query, " metrics-root ", " settings.json ")
	if adapter == nil {
		t.Fatal("NewAdapter() returned nil")
	}

	got, err := adapter.GetMetricsCosts(context.Background(), " session-a ")
	if err != nil {
		t.Fatalf("GetMetricsCosts() error = %v", err)
	}
	assertMappedReport(t, got, inputTokens, outputTokens)
}

func assertMappedReport(t *testing.T, got factoryapi.CostsReport, inputTokens, outputTokens int64) {
	t.Helper()
	assertMappedReportHeader(t, got)
	assertMappedReportScope(t, got)
	assertMappedReportLineItems(t, got, inputTokens, outputTokens)
	assertMappedReportProviderRollup(t, got)
	assertMappedReportOptionalJSON(t, got)
}

func assertMappedReportHeader(t *testing.T, got factoryapi.CostsReport) {
	t.Helper()
	if got.Currency != "USD" || got.Status != "PARTIAL" || got.PricedSubtotal == nil || *got.PricedSubtotal != "123456789.000001" {
		t.Fatalf("mapped report = %#v", got)
	}
}

func assertMappedReportScope(t *testing.T, got factoryapi.CostsReport) {
	t.Helper()
	if got.Scope.FactorySessionId == nil || *got.Scope.FactorySessionId != "session-a" {
		t.Fatalf("mapped scope = %#v", got.Scope)
	}
}

func assertMappedReportLineItems(t *testing.T, got factoryapi.CostsReport, inputTokens, outputTokens int64) {
	t.Helper()
	if len(got.LineItems) != 2 || got.LineItems[0].InputTokens == nil || *got.LineItems[0].InputTokens != inputTokens || got.LineItems[0].OutputTokens == nil || *got.LineItems[0].OutputTokens != outputTokens {
		t.Fatalf("mapped line items = %#v", got.LineItems)
	}
}

func assertMappedReportProviderRollup(t *testing.T, got factoryapi.CostsReport) {
	t.Helper()
	if got.LineItems[1].PricedAmount != nil || got.LineItems[1].Reason == nil || *got.LineItems[1].Reason != "no configured price" {
		t.Fatalf("unpriced line item = %#v", got.LineItems[1])
	}
	if len(got.ProviderModels) != 1 || got.ProviderModels[0].Key != "provider/known" || got.ProviderModels[0].PricedSubtotal == nil || *got.ProviderModels[0].PricedSubtotal != "123456789.000001" {
		t.Fatalf("provider rollups = %#v", got.ProviderModels)
	}
}

func assertMappedReportOptionalJSON(t *testing.T, got factoryapi.CostsReport) {
	t.Helper()
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal mapped report: %v", err)
	}
	jsonText := string(encoded)
	if strings.Contains(jsonText, `"priced_amount":""`) || strings.Contains(jsonText, `"reason":""`) {
		t.Fatalf("mapped JSON contains empty optional values: %s", jsonText)
	}
	if !strings.Contains(jsonText, `"key":"provider/known"`) || strings.Contains(jsonText, "\\u0000") {
		t.Fatalf("mapped JSON provider/model key is not public: %s", jsonText)
	}
}

func TestNewAdapterRejectsMissingQuery(t *testing.T) {
	t.Parallel()
	if got := NewAdapter(nil, "metrics", "settings"); got != nil {
		t.Fatalf("NewAdapter(nil) = %#v, want nil", got)
	}
}
