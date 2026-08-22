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
	if len(first.Models) != 1 {
		t.Fatalf("price models = %#v, want only the observed authoritative pair", first.Models)
	}
	model := first.Models[0]
	if model.Provider != providers.IDCodex || model.Model != "gpt-5-codex" {
		t.Fatalf("price identity = %#v, want codex/gpt-5-codex", model)
	}
	if model.SourceURL == "" || model.AsOfDate == "" {
		t.Fatalf("price provenance = %#v, want source URL and as-of date", model)
	}
	if len(model.EqualRateClasses) != 1 || model.EqualRateClasses[0] != providers.PriceClassReasoningOutput {
		t.Fatalf("equal-rate declarations = %#v, want explicit reasoning/output equality", model.EqualRateClasses)
	}

	first.Models[0].Model = "mutated"
	second, err := reader.ReadPriceTable()
	if err != nil {
		t.Fatalf("second ReadPriceTable() error = %v", err)
	}
	if second.Models[0].Model != "gpt-5-codex" {
		t.Fatal("ReadPriceTable() returned aliased mutable data")
	}
}
