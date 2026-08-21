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
	newCommand := func() *bytes.Buffer {
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
		return output
	}
	first := newCommand().String()
	second := newCommand().String()
	if first != second {
		t.Fatalf("JSON output is not deterministic:\nfirst: %s\nsecond: %s", first, second)
	}
	var decoded generatedclient.CostsReport
	if err := json.Unmarshal([]byte(first), &decoded); err != nil {
		t.Fatalf("decode API-shaped JSON: %v\n%s", err, first)
	}
	if decoded.PricedSubtotal == nil || *decoded.PricedSubtotal != "12.345678" ||
		decoded.Coverage.PricedRows != 1 || len(decoded.LineItems) != 2 ||
		len(decoded.WorkItems) != 1 || len(decoded.WorkerSessions) != 1 ||
		len(decoded.ProviderModels) != 1 || len(decoded.FactorySessions) != 1 {
		t.Fatalf("decoded report = %#v, want complete API report", decoded)
	}
	if strings.Contains(first, "group_by") || strings.Contains(first, "totals") {
		t.Fatalf("JSON output used the legacy metrics envelope:\n%s", first)
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
	rollup := generatedclient.CostsRollup{
		Key: "work-a", Status: generatedclient.CostsRollupStatus("PRICED"),
		PricedSubtotal: &amount, Coverage: coverage,
		InputTokens: &input, CachedInputTokens: &cached,
		OutputTokens: &output, ReasoningOutputTokens: &reasoning,
	}
	return generatedclient.CostsReport{
		Scope: generatedclient.CostsScope{
			Kind: generatedclient.CostsScopeKind("FACTORY_SESSION"), FactorySessionId: &session,
		},
		Currency:       generatedclient.CostsReportCurrency("USD"),
		Status:         generatedclient.CostsReportStatus("PARTIAL"),
		PricedSubtotal: &amount,
		Coverage:       coverage,
		LineItems: []generatedclient.CostsLineItem{
			{Provider: &provider, Model: &model, Status: generatedclient.CostsLineItemStatus("UNPRICED"), Reason: &reason,
				InputTokens: &input, CachedInputTokens: &cached, OutputTokens: &output, ReasoningOutputTokens: &reasoning},
			{Status: generatedclient.CostsLineItemStatus("PRICED"), PricedAmount: &amount},
		},
		WorkItems:       []generatedclient.CostsRollup{rollup},
		WorkerSessions:  []generatedclient.CostsRollup{{Key: "worker-a", Status: generatedclient.CostsRollupStatus("PRICED"), Coverage: coverage}},
		ProviderModels:  []generatedclient.CostsProviderModelRollup{{Provider: "openai", Model: "mystery", Key: "openai/mystery", Status: generatedclient.CostsProviderModelRollupStatus("UNPRICED"), Coverage: coverage}},
		FactorySessions: []generatedclient.CostsRollup{{Key: "session-a", Status: generatedclient.CostsRollupStatus("PARTIAL"), Coverage: coverage}},
	}
}
