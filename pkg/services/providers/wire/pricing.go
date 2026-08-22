package wire

import providers "github.com/portpowered/infinite-you/pkg/services/providers"

// NewPriceTableReader constructs the immutable Providers-owned pricing facts
// used by valuation. The returned reader exposes detached values and validates
// every authored row before it enters the process graph.
func NewPriceTableReader() (providers.PriceTableReader, error) {
	table, err := defaultPriceTable().Normalize()
	if err != nil {
		return nil, err
	}
	return staticPriceTableReader{table: table}, nil
}

type staticPriceTableReader struct {
	table providers.PriceTable
}

func (reader staticPriceTableReader) ReadPriceTable() (providers.PriceTable, error) {
	return reader.table.Clone(), nil
}

// defaultPriceTable is the only shipped pricing data source. The observed
// provider/model inventory came from the live 2026-08-22 metrics day: the
// factory used codex/gpt-5-codex for 192 rows and codex/OMNIVOICE_Q4_K_M for 12
// rows. The local voice model remains intentionally absent because no
// authoritative token price was found; fixture-only pairs are absent too.
//
// Source: https://developers.openai.com/api/docs/models/gpt-5-codex
// Source as-of: 2026-08-21. The source publishes $1.25 input, $0.125 cached
// input, and $10 output per million tokens. It does not publish a separate
// reasoning-output rate, so equality with output is explicit in the data.
func defaultPriceTable() providers.PriceTable {
	reasoningRate := "10"
	cachedInputRate := "0.125"
	return providers.PriceTable{
		Currency: providers.PriceTableCurrencyUSD,
		Models: []providers.PriceTableModel{{
			Provider:                        providers.IDCodex,
			Model:                           "gpt-5-codex",
			InputPerMillionTokens:           "1.25",
			OutputPerMillionTokens:          "10",
			CachedInputPerMillionTokens:     &cachedInputRate,
			ReasoningOutputPerMillionTokens: &reasoningRate,
			SourceURL:                       "https://developers.openai.com/api/docs/models/gpt-5-codex",
			AsOfDate:                        "2026-08-21",
			EqualRateClasses:                []providers.PriceClass{providers.PriceClassReasoningOutput},
		}},
	}
}
