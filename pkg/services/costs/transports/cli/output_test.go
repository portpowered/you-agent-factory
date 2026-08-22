package cli_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	costscli "github.com/portpowered/infinite-you/pkg/services/costs/transports/cli"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
)

func TestCostsHumanOutputAllPricedShowsRoundedCostAndAllTokenClasses(t *testing.T) {
	t.Parallel()

	amount := "1.235"
	report := humanReport("PRICED", &amount, 0, nil)
	output, err := runHumanCostsOutput(t, report)
	if err != nil {
		t.Fatalf("execute all-priced costs command: %v", err)
	}
	for _, want := range []string{
		"Cost (USD): $1.24",
		"Total tokens: 10",
		"Input tokens: 7",
		"Cached-input tokens: 2",
		"Output tokens: 3",
		"Reasoning-output tokens: 4",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("all-priced output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "?? unknown") {
		t.Fatalf("all-priced output contains an unknown marker:\n%s", output)
	}
}

func TestCostsHumanOutputNonePricedShowsUnknownCoverage(t *testing.T) {
	t.Parallel()

	provider, model := "codex", "mystery"
	report := humanReport("UNPRICED", nil, 2, []generatedclient.CostsUnpricedPair{
		{Provider: &provider, Model: &model, DispatchCount: 2},
	})
	output, err := runHumanCostsOutput(t, report)
	if err != nil {
		t.Fatalf("execute none-priced costs command: %v", err)
	}
	for _, want := range []string{
		"Cost (USD): ?? unknown",
		"Unpriced dispatches: 2",
		"Unpriced provider/models: 1",
		"codex/mystery: 2 dispatches",
		"Total tokens: 10",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("none-priced output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "$0.00") {
		t.Fatalf("none-priced output presents unknown usage as zero:\n%s", output)
	}
}

func TestCostsHumanOutputMixedShowsKnownCostAndUnknownRemainder(t *testing.T) {
	t.Parallel()

	amount := "12.345678"
	openAIProvider, mysteryModel := "openai", "mystery"
	codexProvider, alphaModel := "codex", "alpha"
	report := humanReport("PARTIAL", &amount, 2, []generatedclient.CostsUnpricedPair{
		{Provider: &openAIProvider, Model: &mysteryModel, DispatchCount: 1},
		{Provider: &codexProvider, Model: &alphaModel, DispatchCount: 1},
	})
	output, err := runHumanCostsOutput(t, report)
	if err != nil {
		t.Fatalf("execute mixed costs command: %v", err)
	}
	for _, want := range []string{
		"Cost (USD): $12.35 + ?? unknown",
		"Unpriced dispatches: 2",
		"openai/mystery: 1 dispatches",
		"codex/alpha: 1 dispatches",
		"Total tokens: 10",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("mixed output missing %q:\n%s", want, output)
		}
	}
	if strings.Index(output, "codex/alpha: 1 dispatches") > strings.Index(output, "openai/mystery: 1 dispatches") {
		t.Fatalf("mixed unpriced pairs are not deterministically ordered:\n%s", output)
	}
}

func TestCostsHumanOutputUnknownModelRemainsUnpriced(t *testing.T) {
	t.Parallel()

	provider, model := "codex", "unknown-model"
	report := humanReport("UNPRICED", nil, 1, []generatedclient.CostsUnpricedPair{
		{Provider: &provider, Model: &model, DispatchCount: 1},
	})
	output, err := runHumanCostsOutput(t, report)
	if err != nil {
		t.Fatalf("execute unknown-model costs command: %v", err)
	}
	if !strings.Contains(output, "codex/unknown-model: 1 dispatches") {
		t.Fatalf("unknown model was not shown as an unpriced pair:\n%s", output)
	}
	if strings.Contains(output, "$0.00") {
		t.Fatalf("unknown model was silently valued at zero:\n%s", output)
	}
}

func runHumanCostsOutput(t *testing.T, report generatedclient.CostsReport) (string, error) {
	t.Helper()
	command := costscli.NewCostsCommand(costscli.CostsCommandConfig{
		Operation: costscli.NewOperation(func(string) (costscli.Client, error) {
			return &humanOutputClientStub{
				response: &generatedclient.GetMetricsCostsClientResponse{JSON200: &report},
			}, nil
		}),
		Server: func() string { return "https://factory.example" },
	})
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(io.Discard)
	err := command.ExecuteContext(context.Background())
	return output.String(), err
}

func humanReport(status string, knownCost *string, unpricedDispatchCount int, pairs []generatedclient.CostsUnpricedPair) generatedclient.CostsReport {
	input, cached, output, reasoning := int64(7), int64(2), int64(3), int64(4)
	total := input + output
	return generatedclient.CostsReport{
		Currency:  generatedclient.CostsReportCurrency("USD"),
		Status:    generatedclient.CostsReportStatus(status),
		KnownCost: knownCost, PricedSubtotal: knownCost,
		TokenTotals: generatedclient.CostsTokenTotals{
			TotalTokens: &total, InputTokens: &input, OutputTokens: &output,
			CachedInputTokens: &cached, ReasoningOutputTokens: &reasoning,
		},
		UnpricedDispatchCount: unpricedDispatchCount,
		UnpricedPairs:         pairs,
	}
}

type humanOutputClientStub struct {
	response *generatedclient.GetMetricsCostsClientResponse
}

func (stub *humanOutputClientStub) GetMetricsCostsWithResponse(
	_ context.Context,
	_ *generatedclient.GetMetricsCostsParams,
	_ ...generatedclient.RequestEditorFn,
) (*generatedclient.GetMetricsCostsClientResponse, error) {
	return stub.response, nil
}
