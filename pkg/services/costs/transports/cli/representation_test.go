package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	costscli "github.com/portpowered/infinite-you/pkg/services/costs/transports/cli"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	"io"
	"strings"
	"testing"
)

func TestCostsRenderingPricingCases(t *testing.T) {
	t.Parallel()
	for _, tc := range representationCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var amount *string
			if tc.knownCost != "" {
				amount = &tc.knownCost
			}
			pairs := []generatedclient.CostsUnpricedPair{}
			if tc.unpricedPair != "" {
				provider, model := "CODEX", tc.unpricedPair[len("CODEX/"):]
				pairs = append(pairs, generatedclient.CostsUnpricedPair{Provider: &provider, Model: &model, DispatchCount: 1})
			}
			report := generatedclient.CostsReport{Currency: "USD", Status: generatedclient.CostsReportStatus(tc.status), KnownCost: amount, TokenTotals: generatedclient.CostsTokenTotals{TotalTokens: &tc.tokens.total}, UnpricedDispatchCount: tc.unpricedDispatchCount, UnpricedPairs: pairs}
			assertPublicCLIOutputs(t, report, tc)
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
func assertPublicCLIOutputs(t *testing.T, report generatedclient.CostsReport, testCase publicBoundaryCase) {
	t.Helper()
	humanOutput := runPublicBoundaryCLI(t, report, false)
	for _, want := range testCase.human {
		if !strings.Contains(humanOutput, want) {
			t.Fatalf("human output missing %q:\n%s", want, humanOutput)
		}
	}
	for _, unwanted := range testCase.noHuman {
		if strings.Contains(humanOutput, unwanted) {
			t.Fatalf("human output contains %q:\n%s", unwanted, humanOutput)
		}
	}

	jsonOutput := runPublicBoundaryCLI(t, report, true)
	var cliReport generatedclient.CostsReport
	if err := json.Unmarshal([]byte(jsonOutput), &cliReport); err != nil {
		t.Fatalf("decode --json output: %v\n%s", err, jsonOutput)
	}
	if cliReport.Status != report.Status || cliReport.KnownCost == nil && report.KnownCost != nil || cliReport.KnownCost != nil && report.KnownCost == nil {
		t.Fatalf("CLI JSON report = %#v, want API report status/cost %#v/%#v", cliReport, report.Status, report.KnownCost)
	}
	if testCase.knownCost != "" && cliReport.KnownCost != nil && *cliReport.KnownCost != testCase.knownCost {
		t.Fatalf("CLI JSON known cost = %q, want %q", *cliReport.KnownCost, testCase.knownCost)
	}
}

func runPublicBoundaryCLI(t *testing.T, report generatedclient.CostsReport, jsonOutput bool) string {
	t.Helper()
	clientReport := report
	command := costscli.NewCostsCommand(costscli.CostsCommandConfig{
		Operation: costscli.NewOperation(func(string) (costscli.Client, error) {
			return &publicBoundaryClient{report: clientReport}, nil
		}),
		Server: func() string { return "https://factory.example" },
		JSON:   func() bool { return jsonOutput },
	})
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(io.Discard)
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute public Costs CLI: %v", err)
	}
	return output.String()
}

type publicBoundaryClient struct {
	report generatedclient.CostsReport
}

func (client *publicBoundaryClient) GetMetricsCostsWithResponse(context.Context, *generatedclient.GetMetricsCostsParams, ...generatedclient.RequestEditorFn) (*generatedclient.GetMetricsCostsClientResponse, error) {
	return &generatedclient.GetMetricsCostsClientResponse{JSON200: &client.report}, nil
}
