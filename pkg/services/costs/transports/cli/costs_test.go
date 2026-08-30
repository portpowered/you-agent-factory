package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	assertJSONCostAmounts(t, decoded)
	assertJSONUnpricedFacts(t, decoded)
	assertJSONReportDimensions(t, decoded)
	assertNoLegacyJSONEnvelope(t, output)
}

func assertJSONCostAmounts(t *testing.T, decoded generatedclient.CostsReport) {
	t.Helper()
	if decoded.PricedSubtotal == nil || *decoded.PricedSubtotal != "12.345678" {
		t.Fatalf("decoded subtotal = %#v, want exact amount", decoded.PricedSubtotal)
	}
	if decoded.KnownCost == nil || *decoded.KnownCost != "12.345678" || decoded.TokenTotals.TotalTokens == nil || *decoded.TokenTotals.TotalTokens != 10 {
		t.Fatalf("decoded partial facts = %#v, want known cost and total tokens", decoded)
	}
}

func assertJSONUnpricedFacts(t *testing.T, decoded generatedclient.CostsReport) {
	t.Helper()
	if decoded.UnpricedDispatchCount != 1 || len(decoded.UnpricedPairs) != 1 || decoded.UnpricedPairs[0].DispatchCount != 1 {
		t.Fatalf("decoded unpriced facts = %#v, want one unpriced dispatch/pair", decoded)
	}
}

func assertJSONReportDimensions(t *testing.T, decoded generatedclient.CostsReport) {
	t.Helper()
	if decoded.Coverage.PricedRows != 1 || len(decoded.LineItems) != 2 || len(decoded.WorkItems) != 1 || len(decoded.WorkerSessions) != 1 {
		t.Fatalf("decoded report dimensions = %#v, want complete API report", decoded)
	}
	if decoded.LineItems[1].PriceSource == nil || *decoded.LineItems[1].PriceSource != generatedclient.CostsLineItemPriceSource("BUILT_IN") {
		t.Fatalf("decoded priced line source = %#v, want BUILT_IN", decoded.LineItems[1].PriceSource)
	}
	if decoded.LineItems[0].PriceSource != nil {
		t.Fatalf("decoded unpriced line source = %#v, want omitted", decoded.LineItems[0].PriceSource)
	}
	if len(decoded.ProviderModels) != 1 || decoded.ProviderModels[0].Key != "openai/mystery" || len(decoded.FactorySessions) != 1 {
		t.Fatalf("decoded provider/session rollups = %#v, want complete API report", decoded)
	}
}

func assertNoLegacyJSONEnvelope(t *testing.T, output string) {
	t.Helper()
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

func TestCostsCommandTimeoutNamesEndpointAndConfiguredDuration(t *testing.T) {
	t.Parallel()

	const requestTimeout = 25 * time.Millisecond
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()

	command := costscli.NewCostsCommand(costscli.CostsCommandConfig{
		Operation: costscli.NewOperation(func(serverURL string) (costscli.Client, error) {
			return generatedclient.NewClientWithResponses(
				serverURL,
				generatedclient.WithHTTPClient(&http.Client{}),
			)
		}),
		Server:         func() string { return server.URL },
		RequestTimeout: requestTimeout,
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(io.Discard)
	err := command.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("execute costs command returned nil error")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("delayed costs server did not receive the request")
	}
	var costsErr *costscli.CostsError
	if !errors.As(err, &costsErr) {
		t.Fatalf("error = %T (%v), want typed CostsError", err, err)
	}
	if costsErr.CLIErrorCode() != costscli.CostsRequestTimeoutCode {
		t.Fatalf("costs error code = %q, want %q", costsErr.CLIErrorCode(), costscli.CostsRequestTimeoutCode)
	}
	for _, want := range []string{
		"GET /metrics/costs",
		server.URL,
		requestTimeout.String(),
		"retry",
		"--session",
	} {
		if !strings.Contains(costsErr.CLIErrorMessage(), want) {
			t.Fatalf("timeout diagnostic = %q, want %q", costsErr.CLIErrorMessage(), want)
		}
	}
	if output.Len() != 0 {
		t.Fatalf("timeout wrote partial output %q", output.String())
	}
}

func TestCostsCommandTypedHTTPFailurePreservesCodeAndSafeMessage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusGatewayTimeout)
		_, _ = io.WriteString(writer, `{"code":"COSTS_QUERY_TIMEOUT","family":"INTERNAL_SERVER_ERROR","message":"metrics costs query exceeded the server timeout; narrow the session scope or retry"}`)
	}))
	defer server.Close()

	command := costscli.NewCostsCommand(costscli.CostsCommandConfig{
		Operation: costscli.NewOperation(func(serverURL string) (costscli.Client, error) {
			return generatedclient.NewClientWithResponses(
				serverURL,
				generatedclient.WithHTTPClient(&http.Client{}),
			)
		}),
		Server: func() string { return server.URL },
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(io.Discard)
	err := command.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("execute costs command returned nil error")
	}
	var costsErr *costscli.CostsError
	if !errors.As(err, &costsErr) {
		t.Fatalf("error = %T (%v), want typed CostsError", err, err)
	}
	if costsErr.CLIErrorCode() != "COSTS_QUERY_TIMEOUT" {
		t.Fatalf("costs error code = %q, want COSTS_QUERY_TIMEOUT", costsErr.CLIErrorCode())
	}
	for _, want := range []string{
		"GET /metrics/costs",
		server.URL,
		"metrics costs query exceeded the server timeout",
	} {
		if !strings.Contains(costsErr.CLIErrorMessage(), want) {
			t.Fatalf("typed HTTP diagnostic = %q, want %q", costsErr.CLIErrorMessage(), want)
		}
	}
	if output.Len() != 0 {
		t.Fatalf("typed HTTP failure wrote partial output %q", output.String())
	}
}

func TestCostsCommandTypedNotFoundFailurePreservesScopeDiagnostic(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(writer, `{"code":"METRICS_SESSION_NOT_FOUND","family":"NOT_FOUND","message":"Factory Session \"missing-live-id\" was not found; use \u0060you session list --scope live\u0060 to choose a live ID"}`)
	}))
	defer server.Close()

	command := costscli.NewCostsCommand(costscli.CostsCommandConfig{
		Operation: costscli.NewOperation(func(serverURL string) (costscli.Client, error) {
			return generatedclient.NewClientWithResponses(
				serverURL,
				generatedclient.WithHTTPClient(&http.Client{}),
			)
		}),
		Server: func() string { return server.URL },
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(io.Discard)
	err := command.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("execute costs command returned nil error")
	}
	var costsErr *costscli.CostsError
	if !errors.As(err, &costsErr) {
		t.Fatalf("error = %T (%v), want typed CostsError", err, err)
	}
	if costsErr.CLIErrorCode() != "METRICS_SESSION_NOT_FOUND" || string(costsErr.CLIErrorFamily()) != "NOT_FOUND" {
		t.Fatalf("costs error = %#v, want typed not-found identity", costsErr)
	}
	for _, want := range []string{"GET /metrics/costs", "HTTP 404", "missing-live-id", "you session list --scope live"} {
		if !strings.Contains(costsErr.CLIErrorMessage(), want) {
			t.Fatalf("not-found diagnostic = %q, want %q", costsErr.CLIErrorMessage(), want)
		}
	}
	if output.Len() != 0 {
		t.Fatalf("not-found failure wrote partial output %q", output.String())
	}
}

func TestCostsCommandDistinguishesCanceledAndNetworkFailures(t *testing.T) {
	t.Parallel()

	privateServer := "https://operator:secret@example.test/api?token=secret"
	cases := []struct {
		name       string
		cause      error
		wantCode   string
		wantPhrase string
	}{
		{
			name:       "canceled",
			cause:      context.Canceled,
			wantCode:   costscli.CostsRequestCanceledCode,
			wantPhrase: "was canceled",
		},
		{
			name:       "network failure",
			cause:      errors.New("dial tcp 203.0.113.10:7437: connect: permission denied"),
			wantCode:   costscli.CostsNetworkFailureCode,
			wantPhrase: "failed before a response",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			command := costscli.NewCostsCommand(costscli.CostsCommandConfig{
				Operation: costscli.NewOperation(func(string) (costscli.Client, error) {
					return &costsClientStub{err: test.cause}, nil
				}),
				Server: func() string { return privateServer },
			})
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetErr(io.Discard)
			err := command.ExecuteContext(context.Background())
			if err == nil {
				t.Fatal("execute costs command returned nil error")
			}
			var costsErr *costscli.CostsError
			if !errors.As(err, &costsErr) {
				t.Fatalf("error = %T (%v), want typed CostsError", err, err)
			}
			if costsErr.CLIErrorCode() != test.wantCode {
				t.Fatalf("costs error code = %q, want %q", costsErr.CLIErrorCode(), test.wantCode)
			}
			if !strings.Contains(costsErr.CLIErrorMessage(), test.wantPhrase) ||
				!strings.Contains(costsErr.CLIErrorMessage(), "https://example.test/api") {
				t.Fatalf("costs error message = %q, want %q and sanitized endpoint", costsErr.CLIErrorMessage(), test.wantPhrase)
			}
			if strings.Contains(costsErr.Error(), "operator") || strings.Contains(costsErr.Error(), "secret") || strings.Contains(costsErr.Error(), "token") {
				t.Fatalf("costs diagnostic leaked endpoint credentials/query: %q", costsErr.Error())
			}
			if !errors.Is(err, test.cause) {
				t.Fatalf("error = %v, want to preserve cause %v", err, test.cause)
			}
			if output.Len() != 0 {
				t.Fatalf("failure wrote partial output %q", output.String())
			}
		})
	}
}

type costsClientStub struct {
	response *generatedclient.GetMetricsCostsClientResponse
	params   *generatedclient.GetMetricsCostsParams
	err      error
}

func (stub *costsClientStub) GetMetricsCostsWithResponse(
	_ context.Context,
	params *generatedclient.GetMetricsCostsParams,
	_ ...generatedclient.RequestEditorFn,
) (*generatedclient.GetMetricsCostsClientResponse, error) {
	stub.params = params
	return stub.response, stub.err
}

func costsReportForCLI() generatedclient.CostsReport {
	amount := "12.345678"
	session := "session-a"
	provider := "openai"
	model := "mystery"
	reason := "no configured price"
	pricedProvider := "CODEX"
	pricedModel := "gpt-5-codex"
	builtInSource := generatedclient.CostsLineItemPriceSource("BUILT_IN")
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
			{Provider: &pricedProvider, Model: &pricedModel, Status: generatedclient.CostsLineItemStatus("PRICED"), PriceSource: &builtInSource,
				PricedAmount: &amount, InputTokens: &input, CachedInputTokens: &cached, OutputTokens: &output, ReasoningOutputTokens: &reasoning},
		},
		WorkItems:       []generatedclient.CostsRollup{rollup},
		WorkerSessions:  []generatedclient.CostsRollup{{Key: "worker-a", Currency: generatedclient.CostsRollupCurrency("USD"), Status: generatedclient.CostsRollupStatus("PRICED"), TokenTotals: tokenTotals, UnpricedPairs: []generatedclient.CostsUnpricedPair{}, Coverage: coverage}},
		ProviderModels:  []generatedclient.CostsProviderModelRollup{{Provider: "openai", Model: "mystery", Key: "openai/mystery", Currency: generatedclient.CostsProviderModelRollupCurrency("USD"), Status: generatedclient.CostsProviderModelRollupStatus("UNPRICED"), TokenTotals: tokenTotals, UnpricedDispatchCount: 1, UnpricedPairs: []generatedclient.CostsUnpricedPair{unpricedPair}, Coverage: coverage}},
		FactorySessions: []generatedclient.CostsRollup{{Key: "session-a", Currency: generatedclient.CostsRollupCurrency("USD"), Status: generatedclient.CostsRollupStatus("PARTIAL"), TokenTotals: tokenTotals, UnpricedDispatchCount: 1, UnpricedPairs: []generatedclient.CostsUnpricedPair{unpricedPair}, Coverage: coverage}},
	}
}
