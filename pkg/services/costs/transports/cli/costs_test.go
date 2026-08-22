package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	costscli "github.com/portpowered/infinite-you/pkg/services/costs/transports/cli"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
)

func TestCostsCommandHumanOutputUsesGeneratedAPIReport(t *testing.T) {
	t.Parallel()

	report := costsReportForCLI()
	var gotServer string
	var gotSession string
	client := &costsClientStub{response: &generatedclient.GetMetricsCostsClientResponse{JSON200: &report}}
	command := costscli.NewCostsCommand(costscli.CostsCommandConfig{
		Operation: costscli.NewOperation(func(server string) (costscli.Client, error) {
			gotServer = server
			return client, nil
		}),
		Server: func() string { return " https://factory.example " },
		JSON:   func() bool { return false },
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(io.Discard)
	command.SetArgs([]string{"--session", " session-a "})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute costs command: %v", err)
	}
	if gotServer != "https://factory.example" {
		t.Fatalf("server = %q, want trimmed server", gotServer)
	}
	if client.params == nil || client.params.SessionId == nil {
		t.Fatal("generated client did not receive a session filter")
	}
	gotSession = *client.params.SessionId
	if gotSession != "session-a" {
		t.Fatalf("session = %q, want trimmed session", gotSession)
	}
	for _, want := range []string{
		"Scope: Factory Session session-a",
		"Currency: USD",
		"Status: PARTIAL",
		"Priced subtotal (USD): 12.345678",
		"Coverage: rows priced 1/2; provider/models priced 1/2",
		"Work items: 1",
		"Worker Sessions: 1",
		"Provider/models: 1",
		"Factory Sessions: 1",
		"Unpriced usage: 1 rows",
		"UNPRICED provider=openai model=mystery",
		"Reason: no configured price",
		"Input tokens: 7",
		"Cached-input tokens: 2",
		"Output tokens: 3",
		"Reasoning-output tokens: 4",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("human output missing %q:\n%s", want, output.String())
		}
	}
}

func TestCostsCommandJSONIsTheAPIReportAndIsDeterministic(t *testing.T) {
	t.Parallel()

	report := costsReportForCLI()
	first := runJSONCostsCommand(t, report)
	second := runJSONCostsCommand(t, report)
	if first != second {
		t.Fatalf("JSON output is not deterministic:\nfirst: %s\nsecond: %s", first, second)
	}
	assertJSONCostsReport(t, first)
}

func runJSONCostsCommand(t *testing.T, report generatedclient.CostsReport) string {
	t.Helper()
	command := costscli.NewCostsCommand(costscli.CostsCommandConfig{
		Operation: costscli.NewOperation(func(string) (costscli.Client, error) {
			return &costsClientStub{response: &generatedclient.GetMetricsCostsClientResponse{JSON200: &report}}, nil
		}),
		Server: func() string { return "https://factory.example" },
		JSON:   func() bool { return true },
	})
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(io.Discard)
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute JSON costs command: %v", err)
	}
	return output.String()
}

func assertJSONCostsReport(t *testing.T, output string) {
	t.Helper()
	var decoded generatedclient.CostsReport
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("decode API-shaped JSON: %v\n%s", err, output)
	}
	if decoded.PricedSubtotal == nil || *decoded.PricedSubtotal != "12.345678" {
		t.Fatalf("decoded subtotal = %#v, want exact amount", decoded.PricedSubtotal)
	}
	if decoded.KnownCost == nil || *decoded.KnownCost != "12.345678" || decoded.TokenTotals.TotalTokens == nil || *decoded.TokenTotals.TotalTokens != 10 {
		t.Fatalf("decoded partial facts = %#v, want known cost and total tokens", decoded)
	}
	if decoded.UnpricedDispatchCount != 1 || len(decoded.UnpricedPairs) != 1 || decoded.UnpricedPairs[0].DispatchCount != 1 {
		t.Fatalf("decoded unpriced facts = %#v, want one unpriced dispatch/pair", decoded)
	}
	if decoded.Coverage.PricedRows != 1 || len(decoded.LineItems) != 2 || len(decoded.WorkItems) != 1 || len(decoded.WorkerSessions) != 1 {
		t.Fatalf("decoded report dimensions = %#v, want complete API report", decoded)
	}
	if len(decoded.ProviderModels) != 1 || decoded.ProviderModels[0].Key != "openai/mystery" || len(decoded.FactorySessions) != 1 {
		t.Fatalf("decoded provider/session rollups = %#v, want complete API report", decoded)
	}
	if strings.Contains(output, "group_by") || strings.Contains(output, "\"totals\"") || strings.Contains(output, "\\u0000") {
		t.Fatalf("JSON output used the legacy metrics envelope:\n%s", output)
	}
}

func TestCostsCommandRouteFailureWritesNoPartialOutput(t *testing.T) {
	t.Parallel()

	command := costscli.NewCostsCommand(costscli.CostsCommandConfig{
		Operation: costscli.NewOperation(func(string) (costscli.Client, error) {
			return &costsClientStub{
				response: &generatedclient.GetMetricsCostsClientResponse{
					JSON500: &generatedclient.InternalError{Message: "metrics unavailable"},
				},
			}, nil
		}),
		Server: func() string { return "https://factory.example" },
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(io.Discard)
	err := command.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "metrics unavailable") {
		t.Fatalf("error = %v, want route failure", err)
	}
	if output.Len() != 0 {
		t.Fatalf("route failure wrote partial output %q", output.String())
	}
}

type costsClientStub struct {
	response *generatedclient.GetMetricsCostsClientResponse
	params   *generatedclient.GetMetricsCostsParams
}

func (stub *costsClientStub) GetMetricsCostsWithResponse(
	_ context.Context,
	params *generatedclient.GetMetricsCostsParams,
	_ ...generatedclient.RequestEditorFn,
) (*generatedclient.GetMetricsCostsClientResponse, error) {
	stub.params = params
	return stub.response, nil
}

func costsReportForCLI() generatedclient.CostsReport {
	amount := "12.345678"
	session := "session-a"
	provider := "openai"
	model := "mystery"
	reason := "no configured price"
	input, cached, output, reasoning := int64(7), int64(2), int64(3), int64(4)
	coverage := generatedclient.CostsCoverage{
		EncounteredRows: 2, PricedRows: 1, UnpricedRows: 1,
		EncounteredProviderModels: 2, PricedProviderModels: 1, UnpricedProviderModels: 1,
	}
	total := int64(10)
	tokenTotals := generatedclient.CostsTokenTotals{
		TotalTokens: &total, InputTokens: &input, OutputTokens: &output,
		CachedInputTokens: &cached, ReasoningOutputTokens: &reasoning,
	}
	unpricedPair := generatedclient.CostsUnpricedPair{Provider: &provider, Model: &model, DispatchCount: 1}
	rollup := generatedclient.CostsRollup{
		Key: "work-a", Currency: generatedclient.CostsRollupCurrency("USD"), Status: generatedclient.CostsRollupStatus("PRICED"),
		KnownCost: &amount, PricedSubtotal: &amount, TokenTotals: tokenTotals,
		UnpricedDispatchCount: 1, UnpricedPairs: []generatedclient.CostsUnpricedPair{unpricedPair}, Coverage: coverage,
		InputTokens: &input, CachedInputTokens: &cached,
		OutputTokens: &output, ReasoningOutputTokens: &reasoning,
	}
	return generatedclient.CostsReport{
		Scope: generatedclient.CostsScope{
			Kind: generatedclient.CostsScopeKind("FACTORY_SESSION"), FactorySessionId: &session,
		},
		Currency:  generatedclient.CostsReportCurrency("USD"),
		Status:    generatedclient.CostsReportStatus("PARTIAL"),
		KnownCost: &amount, PricedSubtotal: &amount, TokenTotals: tokenTotals,
		UnpricedDispatchCount: 1, UnpricedPairs: []generatedclient.CostsUnpricedPair{unpricedPair},
		Coverage: coverage,
		LineItems: []generatedclient.CostsLineItem{
			{Provider: &provider, Model: &model, Status: generatedclient.CostsLineItemStatus("UNPRICED"), Reason: &reason,
				InputTokens: &input, CachedInputTokens: &cached, OutputTokens: &output, ReasoningOutputTokens: &reasoning},
			{Status: generatedclient.CostsLineItemStatus("PRICED"), PricedAmount: &amount},
		},
		WorkItems:       []generatedclient.CostsRollup{rollup},
		WorkerSessions:  []generatedclient.CostsRollup{{Key: "worker-a", Currency: generatedclient.CostsRollupCurrency("USD"), Status: generatedclient.CostsRollupStatus("PRICED"), TokenTotals: tokenTotals, UnpricedPairs: []generatedclient.CostsUnpricedPair{}, Coverage: coverage}},
		ProviderModels:  []generatedclient.CostsProviderModelRollup{{Provider: "openai", Model: "mystery", Key: "openai/mystery", Currency: generatedclient.CostsProviderModelRollupCurrency("USD"), Status: generatedclient.CostsProviderModelRollupStatus("UNPRICED"), TokenTotals: tokenTotals, UnpricedDispatchCount: 1, UnpricedPairs: []generatedclient.CostsUnpricedPair{unpricedPair}, Coverage: coverage}},
		FactorySessions: []generatedclient.CostsRollup{{Key: "session-a", Currency: generatedclient.CostsRollupCurrency("USD"), Status: generatedclient.CostsRollupStatus("PARTIAL"), TokenTotals: tokenTotals, UnpricedDispatchCount: 1, UnpricedPairs: []generatedclient.CostsUnpricedPair{unpricedPair}, Coverage: coverage}},
	}
}
