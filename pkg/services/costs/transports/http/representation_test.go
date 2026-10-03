package http

import (
	"context"
	"encoding/json"
	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCostsReportRepresentationPricingCases(t *testing.T) {
	t.Parallel()
	for _, tc := range representationCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			totals := costs.TokenTotals{InputTokens: &tc.tokens.input, OutputTokens: &tc.tokens.output, TotalTokens: &tc.tokens.total, CachedInputTokens: &tc.tokens.cached, ReasoningOutputTokens: &tc.tokens.reasoning}
			var amount *string
			if tc.knownCost != "" {
				amount = &tc.knownCost
			}
			pairs := []costs.UnpricedPair{}
			if tc.unpricedPair != "" {
				provider, model := "CODEX", tc.unpricedPair[len("CODEX/"):]
				pairs = append(pairs, costs.UnpricedPair{Provider: &provider, Model: &model, DispatchCount: 1})
			}
			rollup := costs.Rollup{TokenTotals: totals}
			query := costs.CostsQuery(func(context.Context, costs.QueryRequest) (costs.Report, error) {
				return costs.Report{Currency: "USD", Status: tc.status, KnownCost: amount, TokenTotals: totals, UnpricedDispatchCount: tc.unpricedDispatchCount, UnpricedPairs: pairs, LineItems: tc.rows, WorkItems: []costs.Rollup{rollup}, WorkerSessions: []costs.Rollup{rollup}, FactorySessions: []costs.Rollup{rollup}, ProviderModels: []costs.ProviderModelRollup{{Rollup: rollup}}}, nil
			})
			recorder := httptest.NewRecorder()
			NewHandler(NewAdapter(query, "metrics", "settings", identityScopeResolver()), zap.NewNop()).GetMetricsCosts(recorder, httptest.NewRequest(http.MethodGet, "/metrics/costs", nil), factoryapi.GetMetricsCostsParams{})
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var report factoryapi.CostsReport
			if err := json.Unmarshal(recorder.Body.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			assertPublicAPIReport(t, report, tc)
			assertPublicJSONShape(t, recorder.Body.String(), tc.knownCost != "")
		})
	}
}

type publicBoundaryTokenWant struct{ input, output, total, cached, reasoning int64 }
type publicBoundaryCase struct {
	name, knownCost, unpricedPair string
	status                        costs.Status
	tokens                        publicBoundaryTokenWant
	unpricedDispatchCount         int
	rows                          []costs.LineItem
	human, noHuman                []string
}

func representationCases() []publicBoundaryCase {
	return []publicBoundaryCase{
		{name: "all-priced", status: costs.StatusPriced, knownCost: "11.75", tokens: publicBoundaryTokenWant{1000000, 2000000, 3000000, 250000, 500000}, rows: make([]costs.LineItem, 1), human: []string{"Status: PRICED", "Cost (USD): $11.75", "Total tokens: 3000000"}, noHuman: []string{"?? unknown"}},
		{name: "none-priced", status: costs.StatusUnpriced, tokens: publicBoundaryTokenWant{100, 50, 150, 25, 10}, unpricedDispatchCount: 1, unpricedPair: "CODEX/unpriced", rows: make([]costs.LineItem, 1), human: []string{"Status: UNPRICED", "Cost (USD): ?? unknown", "Unpriced dispatches: 1", "CODEX/unpriced: 1 dispatches", "Total tokens: 150"}, noHuman: []string{"$0.00"}},
		{name: "mixed", status: costs.StatusPartial, knownCost: "11.75", tokens: publicBoundaryTokenWant{1000100, 2000050, 3000150, 250025, 500010}, unpricedDispatchCount: 1, unpricedPair: "CODEX/unpriced", rows: make([]costs.LineItem, 2), human: []string{"Status: PARTIAL", "Cost (USD): $11.75 + ?? unknown", "Unpriced dispatches: 1", "CODEX/unpriced: 1 dispatches", "Total tokens: 3000150"}},
		{name: "unknown-model", status: costs.StatusUnpriced, tokens: publicBoundaryTokenWant{20, 10, 30, 5, 3}, unpricedDispatchCount: 1, unpricedPair: "CODEX/unknown-model", rows: make([]costs.LineItem, 1), human: []string{"Status: UNPRICED", "Cost (USD): ?? unknown", "CODEX/unknown-model: 1 dispatches", "Total tokens: 30"}, noHuman: []string{"$0.00"}},
	}
}
func assertPublicAPIReport(t *testing.T, report factoryapi.CostsReport, testCase publicBoundaryCase) {
	t.Helper()
	if report.Status != factoryapi.CostsReportStatus(testCase.status) || report.Currency != "USD" {
		t.Fatalf("API status/currency = %q/%q, want %q/USD", report.Status, report.Currency, testCase.status)
	}
	if testCase.knownCost == "" {
		if report.KnownCost != nil {
			t.Fatalf("API known cost = %q, want absent", *report.KnownCost)
		}
	} else if report.KnownCost == nil || *report.KnownCost != testCase.knownCost {
		t.Fatalf("API known cost = %v, want %q", report.KnownCost, testCase.knownCost)
	}
	assertPublicTokenTotals(t, report.TokenTotals, testCase.tokens)
	if report.UnpricedDispatchCount != testCase.unpricedDispatchCount {
		t.Fatalf("API unpriced dispatch count = %d, want %d", report.UnpricedDispatchCount, testCase.unpricedDispatchCount)
	}
	assertPublicUnpricedPair(t, report.UnpricedPairs, testCase.unpricedPair)
	if len(report.LineItems) != len(testCase.rows) {
		t.Fatalf("API line items = %d, want %d", len(report.LineItems), len(testCase.rows))
	}
	for _, rollup := range report.WorkItems {
		assertPublicTokenTotalsPresent(t, rollup.TokenTotals)
	}
	for _, rollup := range report.WorkerSessions {
		assertPublicTokenTotalsPresent(t, rollup.TokenTotals)
	}
	for _, rollup := range report.FactorySessions {
		assertPublicTokenTotalsPresent(t, rollup.TokenTotals)
	}
	for _, rollup := range report.ProviderModels {
		assertPublicTokenTotalsPresent(t, rollup.TokenTotals)
	}
}

func assertPublicTokenTotals(t *testing.T, totals factoryapi.CostsTokenTotals, want publicBoundaryTokenWant) {
	t.Helper()
	values := []struct {
		name string
		got  *int64
		want int64
	}{
		{name: "total", got: totals.TotalTokens, want: want.total},
		{name: "input", got: totals.InputTokens, want: want.input},
		{name: "output", got: totals.OutputTokens, want: want.output},
		{name: "cached-input", got: totals.CachedInputTokens, want: want.cached},
		{name: "reasoning-output", got: totals.ReasoningOutputTokens, want: want.reasoning},
	}
	for _, value := range values {
		if value.got == nil || *value.got != value.want {
			t.Fatalf("API %s token total = %v, want %d", value.name, value.got, value.want)
		}
	}
}

func assertPublicTokenTotalsPresent(t *testing.T, totals factoryapi.CostsTokenTotals) {
	t.Helper()
	if totals.TotalTokens == nil || totals.InputTokens == nil || totals.OutputTokens == nil || totals.CachedInputTokens == nil || totals.ReasoningOutputTokens == nil {
		t.Fatalf("API rollup token totals = %#v, want every token class present", totals)
	}
	if *totals.TotalTokens != *totals.InputTokens+*totals.OutputTokens {
		t.Fatalf("API rollup total tokens = %d, want input plus output %d", *totals.TotalTokens, *totals.InputTokens+*totals.OutputTokens)
	}
}

func assertPublicJSONShape(t *testing.T, body string, wantKnownCost bool) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &fields); err != nil {
		t.Fatalf("decode public Costs JSON: %v\n%s", err, body)
	}
	for _, field := range []string{"status", "currency", "known_cost", "token_totals", "unpriced_dispatch_count", "unpriced_pairs"} {
		if _, ok := fields[field]; !ok {
			t.Fatalf("public Costs JSON missing %q: %s", field, body)
		}
	}
	if wantKnownCost && string(fields["known_cost"]) != `"11.75"` {
		t.Fatalf("public Costs JSON known_cost = %s, want exact 11.75", fields["known_cost"])
	}
	if !wantKnownCost && string(fields["known_cost"]) != "null" {
		t.Fatalf("public Costs JSON known_cost = %s, want null", fields["known_cost"])
	}
	var tokenFields map[string]json.RawMessage
	if err := json.Unmarshal(fields["token_totals"], &tokenFields); err != nil {
		t.Fatalf("decode public token_totals: %v", err)
	}
	for _, field := range []string{"total_tokens", "input_tokens", "output_tokens", "cached_input_tokens", "reasoning_output_tokens"} {
		if _, ok := tokenFields[field]; !ok {
			t.Fatalf("public token_totals missing %q: %s", field, fields["token_totals"])
		}
	}
}

func assertPublicUnpricedPair(t *testing.T, pairs []factoryapi.CostsUnpricedPair, want string) {
	t.Helper()
	if want == "" {
		if len(pairs) != 0 {
			t.Fatalf("API unpriced pairs = %#v, want none", pairs)
		}
		return
	}
	if len(pairs) != 1 || pairs[0].Provider == nil || pairs[0].Model == nil || *pairs[0].Provider+"/"+*pairs[0].Model != want || pairs[0].DispatchCount != 1 {
		t.Fatalf("API unpriced pairs = %#v, want one %s pair", pairs, want)
	}
}
