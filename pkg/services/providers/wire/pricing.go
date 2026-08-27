package wire

import providers "github.com/portpowered/infinite-you/pkg/services/providers"

// NewPriceTableReader constructs the immutable Providers-owned pricing facts
// used by valuation. The returned reader exposes detached values and validates
// every authored row before it enters the process graph.
func NewPriceTableReader() (providers.PriceTableReaderFunc, error) {
	table, err := defaultPriceTable().Normalize()
	if err != nil {
		return nil, err
	}
	return func() (providers.PriceTable, error) {
		return table.Clone(), nil
	}, nil
}

// defaultPriceTable is the only shipped pricing data source. The observed
// provider/model inventory came from the live 2026-08-22 metrics day, while the
// current Codex catalog also exposes the GPT-5.6 identities below. The local
// voice model remains intentionally absent because no authoritative token price
// was found; fixture-only pairs are absent too.
//
// Source: https://developers.openai.com/api/docs/models/gpt-5-codex
// Source as-of: 2026-08-21. The source publishes $1.25 input, $0.125 cached
// input, and $10 output per million tokens. It does not publish a separate
// reasoning-output rate, so equality with output is explicit in the data.
func defaultPriceTable() providers.PriceTable {
	return providers.PriceTable{
		Currency: providers.PriceTableCurrencyUSD,
		Models: []providers.PriceTableModel{
			defaultCodexPriceModel("gpt-5-codex", "1.25", "0.125", "10", "https://developers.openai.com/api/docs/models/gpt-5-codex", "2026-08-21"),
			// Source: https://developers.openai.com/api/docs/models/gpt-5.6-sol
			// Source as-of: 2026-08-26. The source publishes $4 input, $0.40
			// cached input, and $20 output per million tokens. It documents that
			// the gpt-5.6 alias routes to gpt-5.6-sol and publishes no distinct
			// reasoning-output rate.
			defaultCodexPriceModel("gpt-5.6", "4", "0.40", "20", "https://developers.openai.com/api/docs/models/gpt-5.6-sol", "2026-08-26"),
			// Source: https://developers.openai.com/api/docs/models/gpt-5.6-sol
			// Source as-of: 2026-08-26. The source publishes $4 input, $0.40
			// cached input, and $20 output per million tokens, with no distinct
			// reasoning-output rate.
			defaultCodexPriceModel("gpt-5.6-sol", "4", "0.40", "20", "https://developers.openai.com/api/docs/models/gpt-5.6-sol", "2026-08-26"),
			// Source: https://developers.openai.com/api/docs/models/gpt-5.6-terra
			// Source as-of: 2026-08-26. The source publishes $2 input, $0.20
			// cached input, and $12 output per million tokens, with no distinct
			// reasoning-output rate.
			defaultCodexPriceModel("gpt-5.6-terra", "2", "0.20", "12", "https://developers.openai.com/api/docs/models/gpt-5.6-terra", "2026-08-26"),
			// Source: https://developers.openai.com/api/docs/models/gpt-5.6-luna
			// Source as-of: 2026-08-26. The source publishes $0.20 input, $0.02
			// cached input, and $1.20 output per million tokens, with no distinct
			// reasoning-output rate.
			defaultCodexPriceModel("gpt-5.6-luna", "0.20", "0.02", "1.20", "https://developers.openai.com/api/docs/models/gpt-5.6-luna", "2026-08-26"),
		},
	}
}

func defaultCodexPriceModel(model, input, cachedInput, output, sourceURL, asOfDate string) providers.PriceTableModel {
	reasoningOutput := output
	return providers.PriceTableModel{
		Provider:                        providers.IDCodex,
		Model:                           model,
		InputPerMillionTokens:           input,
		OutputPerMillionTokens:          output,
		CachedInputPerMillionTokens:     &cachedInput,
		ReasoningOutputPerMillionTokens: &reasoningOutput,
		SourceURL:                       sourceURL,
		AsOfDate:                        asOfDate,
		EqualRateClasses:                []providers.PriceClass{providers.PriceClassReasoningOutput},
	}
}
