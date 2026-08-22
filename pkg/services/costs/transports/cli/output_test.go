package cli_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	costscli "github.com/portpowered/infinite-you/pkg/services/costs/transports/cli"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
)

func TestCostsHumanOutputAllPricedGolden(t *testing.T) {
	amount := "1.235"
	assertHumanOutputGolden(t, "all-priced", humanReport("PRICED", &amount, 0, nil), `Scope: all Factory Sessions
Currency: USD
Status: PRICED
Cost (USD): $1.24
Priced subtotal (USD): 1.235
Total tokens: 10
Input tokens: 7
Cached-input tokens: 2
Output tokens: 3
Reasoning-output tokens: 4
Coverage: rows priced 0/0; provider/models priced 0/0
Unpriced dispatches: 0
Unpriced provider/models: 0

Work items: 0
Worker Sessions: 0
Provider/models: 0
Factory Sessions: 0
Unpriced usage: 0 rows
`,
	)
}

func TestCostsHumanOutputNonePricedGolden(t *testing.T) {
	codexProvider, mysteryModel := "codex", "mystery"
	assertHumanOutputGolden(t, "none-priced", humanReport("UNPRICED", nil, 2, []generatedclient.CostsUnpricedPair{
		{Provider: &codexProvider, Model: &mysteryModel, DispatchCount: 2},
	}), `Scope: all Factory Sessions
Currency: USD
Status: UNPRICED
Cost (USD): ?? unknown
Priced subtotal (USD): unavailable
Total tokens: 10
Input tokens: 7
Cached-input tokens: 2
Output tokens: 3
Reasoning-output tokens: 4
Coverage: rows priced 0/0; provider/models priced 0/0
Unpriced dispatches: 2
Unpriced provider/models: 1
  codex/mystery: 2 dispatches

Work items: 0
Worker Sessions: 0
Provider/models: 0
Factory Sessions: 0
Unpriced usage: 0 rows
`,
	)
}

func TestCostsHumanOutputMixedGolden(t *testing.T) {
	mixedAmount := "12.345678"
	openAIProvider, mysteryModel := "openai", "mystery"
	codexProvider, alphaModel := "codex", "alpha"
	assertHumanOutputGolden(t, "mixed", humanReport("PARTIAL", &mixedAmount, 2, []generatedclient.CostsUnpricedPair{
		{Provider: &openAIProvider, Model: &mysteryModel, DispatchCount: 1},
		{Provider: &codexProvider, Model: &alphaModel, DispatchCount: 1},
	}), `Scope: all Factory Sessions
Currency: USD
Status: PARTIAL
Cost (USD): $12.35 + ?? unknown
Priced subtotal (USD): 12.345678
Total tokens: 10
Input tokens: 7
Cached-input tokens: 2
Output tokens: 3
Reasoning-output tokens: 4
Coverage: rows priced 0/0; provider/models priced 0/0
Unpriced dispatches: 2
Unpriced provider/models: 2
  codex/alpha: 1 dispatches
  openai/mystery: 1 dispatches

Work items: 0
Worker Sessions: 0
Provider/models: 0
Factory Sessions: 0
Unpriced usage: 0 rows
`,
	)
}

func TestCostsHumanOutputUnknownModelGolden(t *testing.T) {
	unknownProvider, unknownModel := "codex", "unknown-model"
	assertHumanOutputGolden(t, "unknown-model", humanReport("UNPRICED", nil, 1, []generatedclient.CostsUnpricedPair{
		{Provider: &unknownProvider, Model: &unknownModel, DispatchCount: 1},
	}), `Scope: all Factory Sessions
Currency: USD
Status: UNPRICED
Cost (USD): ?? unknown
Priced subtotal (USD): unavailable
Total tokens: 10
Input tokens: 7
Cached-input tokens: 2
Output tokens: 3
Reasoning-output tokens: 4
Coverage: rows priced 0/0; provider/models priced 0/0
Unpriced dispatches: 1
Unpriced provider/models: 1
  codex/unknown-model: 1 dispatches

Work items: 0
Worker Sessions: 0
Provider/models: 0
Factory Sessions: 0
Unpriced usage: 0 rows
`,
	)
}

func assertHumanOutputGolden(t *testing.T, name string, report generatedclient.CostsReport, want string) {
	t.Helper()
	output, err := runHumanCostsOutput(t, report)
	if err != nil {
		t.Fatalf("execute %s costs command: %v", name, err)
	}
	if output != want {
		t.Errorf("%s output mismatch:\n--- want ---\n%s--- got ---\n%s", name, want, output)
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
