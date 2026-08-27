package wire

import (
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestNewPriceTableReaderPublishesObservedSourcedFacts(t *testing.T) {
	t.Parallel()

	reader, err := NewPriceTableReader()
	if err != nil {
		t.Fatalf("NewPriceTableReader() error = %v", err)
	}
	first, err := reader.ReadPriceTable()
	if err != nil {
		t.Fatalf("ReadPriceTable() error = %v", err)
	}
	want := map[string]priceModelExpectation{
		"gpt-5-codex": {
			input: "1.25", cachedInput: "0.125", output: "10",
			sourceURL: "https://developers.openai.com/api/docs/models/gpt-5-codex", asOfDate: "2026-08-21",
		},
		"gpt-5.6": {
			input: "4", cachedInput: "0.40", output: "20",
			sourceURL: "https://developers.openai.com/api/docs/models/gpt-5.6-sol", asOfDate: "2026-08-26",
		},
		"gpt-5.6-sol": {
			input: "4", cachedInput: "0.40", output: "20",
			sourceURL: "https://developers.openai.com/api/docs/models/gpt-5.6-sol", asOfDate: "2026-08-26",
		},
		"gpt-5.6-terra": {
			input: "2", cachedInput: "0.20", output: "12",
			sourceURL: "https://developers.openai.com/api/docs/models/gpt-5.6-terra", asOfDate: "2026-08-26",
		},
		"gpt-5.6-luna": {
			input: "0.20", cachedInput: "0.02", output: "1.20",
			sourceURL: "https://developers.openai.com/api/docs/models/gpt-5.6-luna", asOfDate: "2026-08-26",
		},
	}
	if len(first.Models) != len(want) {
		t.Fatalf("price models = %#v, want exactly %d shipped identities", first.Models, len(want))
	}
	seen := make(map[string]struct{}, len(first.Models))
	for _, model := range first.Models {
		expectation, ok := want[model.Model]
		if !ok {
			t.Errorf("unexpected price identity = %s/%s", model.Provider, model.Model)
			continue
		}
		if _, ok := seen[model.Model]; ok {
			t.Errorf("duplicate price identity = %s/%s", model.Provider, model.Model)
			continue
		}
		seen[model.Model] = struct{}{}
		assertPriceModel(t, model, expectation)
	}
	for model := range want {
		if _, ok := seen[model]; !ok {
			t.Errorf("missing expected price identity = codex/%s", model)
		}
	}

	if _, err := first.Normalize(); err != nil {
		t.Fatalf("reader result Normalize() error = %v", err)
	}
	model := findPriceModel(t, &first, "gpt-5-codex")
	model.Model = "mutated"
	*model.CachedInputPerMillionTokens = "mutated"
	*model.ReasoningOutputPerMillionTokens = "mutated"
	model.EqualRateClasses[0] = "mutated"
	second, err := reader.ReadPriceTable()
	if err != nil {
		t.Fatalf("second ReadPriceTable() error = %v", err)
	}
	if len(second.Models) != len(want) {
		t.Fatalf("second price read models = %#v, want exactly %d shipped identities", second.Models, len(want))
	}
	for _, model := range second.Models {
		expectation, ok := want[model.Model]
		if !ok {
			t.Errorf("second read returned mutated or unexpected identity %q", model.Model)
			continue
		}
		assertPriceModel(t, model, expectation)
	}
}

type priceModelExpectation struct {
	input, cachedInput, output string
	sourceURL, asOfDate        string
}

func assertPriceModel(t *testing.T, model providers.PriceTableModel, expectation priceModelExpectation) {
	t.Helper()
	if model.Provider != providers.IDCodex {
		t.Errorf("%s provider = %q, want %q", model.Model, model.Provider, providers.IDCodex)
	}
	if model.InputPerMillionTokens != expectation.input {
		t.Errorf("%s input rate = %q, want %q", model.Model, model.InputPerMillionTokens, expectation.input)
	}
	if model.OutputPerMillionTokens != expectation.output {
		t.Errorf("%s output rate = %q, want %q", model.Model, model.OutputPerMillionTokens, expectation.output)
	}
	if model.CachedInputPerMillionTokens == nil || *model.CachedInputPerMillionTokens != expectation.cachedInput {
		t.Errorf("%s cached input rate = %v, want %q", model.Model, model.CachedInputPerMillionTokens, expectation.cachedInput)
	}
	if model.ReasoningOutputPerMillionTokens == nil || *model.ReasoningOutputPerMillionTokens != expectation.output {
		t.Errorf("%s reasoning output rate = %v, want equal output rate %q", model.Model, model.ReasoningOutputPerMillionTokens, expectation.output)
	}
	if model.SourceURL != expectation.sourceURL {
		t.Errorf("%s source URL = %q, want %q", model.Model, model.SourceURL, expectation.sourceURL)
	}
	if model.AsOfDate != expectation.asOfDate {
		t.Errorf("%s as-of date = %q, want %q", model.Model, model.AsOfDate, expectation.asOfDate)
	}
	if len(model.EqualRateClasses) != 1 || model.EqualRateClasses[0] != providers.PriceClassReasoningOutput {
		t.Errorf("%s equal-rate declarations = %#v, want explicit reasoning/output equality", model.Model, model.EqualRateClasses)
	}
}

func findPriceModel(t *testing.T, table *providers.PriceTable, name string) *providers.PriceTableModel {
	t.Helper()
	for index := range table.Models {
		if table.Models[index].Model == name {
			return &table.Models[index]
		}
	}
	t.Fatalf("price model %q not found", name)
	return nil
}
