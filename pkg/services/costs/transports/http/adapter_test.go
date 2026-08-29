package http

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestAdapterMapsExactReportAndRuntimeInputs(t *testing.T) {
	t.Parallel()
	inputTokens := int64(12)
	outputTokens := int64(0)
	totalTokens := int64(12)
	provider := "provider"
	model := "known"
	dispatchCount := 1
	knownPair := costs.UnpricedPair{Provider: &provider, Model: &model, DispatchCount: dispatchCount}
	query := costs.CostsQuery(func(_ context.Context, request costs.QueryRequest) (costs.Report, error) {
		if request.MetricsRoot != "metrics-root" || request.OperatorSettingsPath != "settings.json" || request.FactorySessionID != "session-a" {
			t.Fatalf("Costs request = %#v", request)
		}
		amount := "123456789.000001"
		return costs.Report{
			Scope:          costs.Scope{Kind: costs.ScopeFactorySession, FactorySessionID: "session-a"},
			Currency:       "USD",
			Status:         costs.StatusPartial,
			KnownCost:      &amount,
			PricedSubtotal: &amount,
			TokenTotals: costs.TokenTotals{
				TotalTokens: &totalTokens, InputTokens: &inputTokens, OutputTokens: &outputTokens,
			},
			UnpricedDispatchCount: 1,
			UnpricedPairs:         []costs.UnpricedPair{knownPair},
			Coverage:              costs.Coverage{EncounteredRows: 2, PricedRows: 1, UnpricedRows: 1, EncounteredProviderModels: 2, PricedProviderModels: 1, UnpricedProviderModels: 1},
			LineItems: []costs.LineItem{
				{Model: "known", Status: costs.StatusPriced, PricedAmount: &amount, TokenCounts: costs.TokenCounts{InputTokens: &inputTokens, OutputTokens: &outputTokens}},
				{Model: "unknown", Status: costs.StatusUnpriced, Reason: "no configured price"},
			},
			ProviderModels: []costs.ProviderModelRollup{{Provider: "provider", Model: "known", Rollup: costs.Rollup{Key: "provider/known", Currency: "USD", Status: costs.StatusPriced, KnownCost: &amount, PricedSubtotal: &amount, TokenTotals: costs.TokenTotals{TotalTokens: &totalTokens, InputTokens: &inputTokens, OutputTokens: &outputTokens}, UnpricedPairs: []costs.UnpricedPair{}}}},
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

func TestAdapterPassesExactResolvedScopeToCostsQuery(t *testing.T) {
	t.Parallel()

	var gotRequest costs.QueryRequest
	query := costs.CostsQuery(func(_ context.Context, request costs.QueryRequest) (costs.Report, error) {
		gotRequest = request
		return costs.Report{
			Scope:  costs.Scope{Kind: costs.ScopeFactorySession, FactorySessionID: "~default"},
			Status: costs.StatusNoUsage,
		}, nil
	})
	resolver := metricsScopeResolverFunc(func(_ context.Context, sessionID string) (factorysessions.RuntimeMetricsScope, error) {
		if sessionID != "~default" {
			t.Fatalf("resolver session ID = %q, want ~default", sessionID)
		}
		return factorysessions.RuntimeMetricsScope{
			RequestedFactorySessionID: "~default",
			RetainedFactorySessionIDs: []string{" canonical-live-id ", "canonical-live-id"},
		}, nil
	})

	got, err := NewAdapter(query, "metrics", "settings", resolver).GetMetricsCosts(context.Background(), " ~default ")
	if err != nil {
		t.Fatalf("GetMetricsCosts() error = %v", err)
	}
	if gotRequest.FactorySessionID != "~default" {
		t.Fatalf("requested Factory Session ID = %q, want ~default", gotRequest.FactorySessionID)
	}
	if len(gotRequest.RetainedFactorySessionIDs) != 1 || gotRequest.RetainedFactorySessionIDs[0] != "canonical-live-id" {
		t.Fatalf("retained Factory Session IDs = %#v, want one canonical ID", gotRequest.RetainedFactorySessionIDs)
	}
	if got.Scope.FactorySessionId == nil || *got.Scope.FactorySessionId != "~default" || got.Status != factoryapi.CostsReportStatusNOUSAGE {
		t.Fatalf("mapped scope/status = %#v/%q, want ~default/NO_USAGE", got.Scope, got.Status)
	}
}

func TestAdapterPreservesUnknownSelectorNoUsageCompatibility(t *testing.T) {
	t.Parallel()

	var gotRequest costs.QueryRequest
	query := costs.CostsQuery(func(_ context.Context, request costs.QueryRequest) (costs.Report, error) {
		gotRequest = request
		return costs.Report{
			Scope:  costs.Scope{Kind: costs.ScopeFactorySession, FactorySessionID: "missing-session"},
			Status: costs.StatusNoUsage,
		}, nil
	})
	resolver := metricsScopeResolverFunc(func(context.Context, string) (factorysessions.RuntimeMetricsScope, error) {
		return factorysessions.RuntimeMetricsScope{}, factorysessions.ErrSessionNotFound
	})

	got, err := NewAdapter(query, "metrics", "settings", resolver).GetMetricsCosts(context.Background(), "missing-session")
	if err != nil {
		t.Fatalf("GetMetricsCosts() error = %v, want historical NO_USAGE compatibility", err)
	}
	if gotRequest.FactorySessionID != "missing-session" || len(gotRequest.RetainedFactorySessionIDs) != 0 {
		t.Fatalf("unknown selector request = %#v, want requested ID without retained scope", gotRequest)
	}
	if got.Status != factoryapi.CostsReportStatusNOUSAGE {
		t.Fatalf("unknown selector status = %q, want NO_USAGE", got.Status)
	}
}

func TestAdapterFailsClosedWhenScopeResolutionIsUnavailable(t *testing.T) {
	t.Parallel()

	query := costs.CostsQuery(func(context.Context, costs.QueryRequest) (costs.Report, error) {
		t.Fatal("Costs query invoked after scope resolution failed")
		return costs.Report{}, nil
	})
	want := errors.New("scope unavailable")
	resolver := metricsScopeResolverFunc(func(context.Context, string) (factorysessions.RuntimeMetricsScope, error) {
		return factorysessions.RuntimeMetricsScope{}, want
	})

	_, err := NewAdapter(query, "metrics", "settings", resolver).GetMetricsCosts(context.Background(), "selected-session")
	var queryErr *costs.QueryError
	if !errors.As(err, &queryErr) || queryErr.Kind != costs.QueryErrorMetricsFailed || !errors.Is(err, want) {
		t.Fatalf("scope resolution error = %v, want typed wrapped metrics failure", err)
	}
}

type metricsScopeResolverFunc func(context.Context, string) (factorysessions.RuntimeMetricsScope, error)

func (resolver metricsScopeResolverFunc) ResolveRuntimeMetricsScope(ctx context.Context, sessionID string) (factorysessions.RuntimeMetricsScope, error) {
	return resolver(ctx, sessionID)
}

func TestAdapterMapsBothPriceSourcesAndOmitsSourceForUnpricedRows(t *testing.T) {
	t.Parallel()

	builtInAmount := "1.25"
	operatorAmount := "2.50"
	query := costs.CostsQuery(func(context.Context, costs.QueryRequest) (costs.Report, error) {
		return costs.Report{
			Currency: "USD",
			Status:   costs.StatusPartial,
			LineItems: []costs.LineItem{
				{Provider: "CODEX", Model: "built-in", Status: costs.StatusPriced, PriceSource: costs.PriceSourceBuiltIn, PricedAmount: &builtInAmount},
				{Provider: "CLAUDE", Model: "operator", Status: costs.StatusPriced, PriceSource: costs.PriceSourceOperatorSupplied, PricedAmount: &operatorAmount},
				{Provider: "CLAUDE", Model: "unknown", Status: costs.StatusUnpriced, Reason: "no configured price"},
			},
		}, nil
	})

	got, err := NewAdapter(query, "metrics", "settings").GetMetricsCosts(context.Background(), "")
	if err != nil {
		t.Fatalf("GetMetricsCosts() error = %v", err)
	}
	if len(got.LineItems) != 3 {
		t.Fatalf("mapped line items = %#v, want three rows", got.LineItems)
	}
	assertMappedPriceSource(t, got.LineItems[0].PriceSource, "BUILT_IN")
	assertMappedPriceSource(t, got.LineItems[1].PriceSource, "OPERATOR_SUPPLIED")
	if got.LineItems[2].PriceSource != nil {
		t.Fatalf("unpriced mapped source = %q, want omitted", *got.LineItems[2].PriceSource)
	}

	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal mapped report: %v", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode mapped report JSON: %v", err)
	}
	var lineItems []map[string]json.RawMessage
	if err := json.Unmarshal(payload["line_items"], &lineItems); err != nil {
		t.Fatalf("decode mapped line items JSON: %v", err)
	}
	if _, ok := lineItems[0]["price_source"]; !ok {
		t.Fatalf("built-in JSON line item = %s, want price_source", encoded)
	}
	if _, ok := lineItems[1]["price_source"]; !ok {
		t.Fatalf("operator JSON line item = %s, want price_source", encoded)
	}
	if _, ok := lineItems[2]["price_source"]; ok {
		t.Fatalf("unpriced JSON line item = %s, want source-less row", encoded)
	}
}

func assertMappedPriceSource(t *testing.T, source *factoryapi.CostsLineItemPriceSource, want string) {
	t.Helper()
	if source == nil || string(*source) != want {
		t.Fatalf("mapped price source = %v, want %q", source, want)
	}
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
	if got.Currency != "USD" || got.Status != "PARTIAL" || got.KnownCost == nil || *got.KnownCost != "123456789.000001" || got.PricedSubtotal == nil || *got.PricedSubtotal != "123456789.000001" {
		t.Fatalf("mapped report = %#v", got)
	}
	if got.TokenTotals.TotalTokens == nil || *got.TokenTotals.TotalTokens != 12 || got.UnpricedDispatchCount != 1 || len(got.UnpricedPairs) != 1 {
		t.Fatalf("mapped report partial facts = %#v", got)
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
	if len(got.ProviderModels) != 1 || got.ProviderModels[0].Key != "provider/known" || got.ProviderModels[0].PricedSubtotal == nil || *got.ProviderModels[0].PricedSubtotal != "123456789.000001" || got.ProviderModels[0].Currency != "USD" {
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
