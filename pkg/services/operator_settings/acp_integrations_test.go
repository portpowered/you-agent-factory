package operatorsettings

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestACPIntegrationSemanticUpdatesPreserveUnrelatedSettings(t *testing.T) {
	t.Parallel()

	service := ConfigDocumentService{}
	document := ConfigDocument{config: Config{
		BackendScopeID: "local-scope",
		Defaults:       Defaults{WorkerModelProvider: "CODEX", WorkerModel: "model"},
		WorkerPresets:  []WorkerPreset{{ID: "build", ModelProvider: "CODEX"}},
	}}
	added, err := service.AddACPIntegration(document, ACPIntegration{
		ID: "entry-1", Name: "cursor-acp", Transport: "STDIO", Command: " cursor-agent acp ",
	})
	if err != nil {
		t.Fatalf("AddACPIntegration() error = %v", err)
	}
	got := added.FileConfig()
	if got.BackendScopeID != "local-scope" || got.Defaults != document.config.Defaults || !reflect.DeepEqual(got.WorkerPresets, document.config.WorkerPresets) {
		t.Fatalf("unrelated settings changed: %#v", got)
	}
	wantIntegration := ACPIntegration{ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp"}
	if !reflect.DeepEqual(got.Workers.ACP.Integrations, []ACPIntegration{wantIntegration}) {
		t.Fatalf("integrations = %#v, want %#v", got.Workers.ACP.Integrations, wantIntegration)
	}

	deleted, err := service.DeleteACPIntegration(added, " cursor-acp ")
	if err != nil {
		t.Fatalf("DeleteACPIntegration() error = %v", err)
	}
	if len(deleted.FileConfig().Workers.ACP.Integrations) != 0 {
		t.Fatalf("integrations after delete = %#v, want empty", deleted.FileConfig().Workers.ACP.Integrations)
	}
	if _, err := service.DeleteACPIntegration(deleted, "cursor-acp"); !errors.Is(err, ErrACPIntegrationNotFound) {
		t.Fatalf("DeleteACPIntegration(missing) error = %v, want ErrACPIntegrationNotFound", err)
	}
}

func TestACPIntegrationRejectsDuplicateAndMalformedProviderIdentities(t *testing.T) {
	t.Parallel()

	service := ConfigDocumentService{}
	base := ConfigDocument{config: Config{Workers: WorkerSettings{ACP: ACPSettings{Integrations: []ACPIntegration{{
		ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
	}}}}}}
	if _, err := service.AddACPIntegration(base, ACPIntegration{
		ID: "entry-2", Name: "cursor-acp", Transport: "stdio", Command: "replacement",
	}); err == nil {
		t.Fatal("AddACPIntegration(duplicate name) error = nil")
	}
	if _, err := service.AddACPIntegration(ConfigDocument{}, ACPIntegration{
		ID: "entry-1", Name: "Cursor ACP", Transport: "stdio", Command: "cursor-agent acp",
	}); err == nil {
		t.Fatal("AddACPIntegration(malformed name) error = nil")
	}
	for _, integration := range []ACPIntegration{
		{ID: "entry-1", Name: "missing-command", Transport: "stdio"},
		{ID: "entry-1", Name: "custom-acp", Transport: "http", Command: "agent acp"},
	} {
		if _, err := service.AddACPIntegration(ConfigDocument{}, integration); err == nil {
			t.Fatalf("AddACPIntegration(%#v) error = nil", integration)
		}
	}
	duplicateID := ConfigDocument{config: Config{Workers: WorkerSettings{ACP: ACPSettings{Integrations: []ACPIntegration{
		{ID: "same", Name: "first-acp", Transport: "stdio", Command: "first acp"},
		{ID: "same", Name: "second-acp", Transport: "stdio", Command: "second acp"},
	}}}}}
	if _, err := duplicateID.config.Normalize(); err == nil {
		t.Fatal("Normalize(duplicate ACP ID) error = nil")
	}
}

func TestConfigureACPIntegrationHonorsCanceledContextBeforeIO(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := ConfigDocumentService{}
	if _, err := service.ConfigureACPIntegrationAdd(ctx, "config.json", ACPIntegration{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ConfigureACPIntegrationAdd(canceled) = %v", err)
	}
	if _, err := service.ConfigureACPIntegrationDelete(ctx, "config.json", "cursor-acp"); !errors.Is(err, context.Canceled) {
		t.Fatalf("ConfigureACPIntegrationDelete(canceled) = %v", err)
	}
}

func TestConfigureACPIntegrationRejectsNilContext(t *testing.T) {
	t.Parallel()

	service := ConfigDocumentService{}
	const want = "operator config context is required"
	if _, err := service.ConfigureACPIntegrationAdd(nil, "config.json", ACPIntegration{}); err == nil || err.Error() != want {
		t.Fatalf("ConfigureACPIntegrationAdd(nil) = %v, want %q", err, want)
	}
	if _, err := service.ConfigureACPIntegrationDelete(nil, "config.json", "cursor-acp"); err == nil || err.Error() != want {
		t.Fatalf("ConfigureACPIntegrationDelete(nil) = %v, want %q", err, want)
	}
	if _, err := service.EnsurePackagedACPIntegrations(nil, "config.json", nil); err == nil || err.Error() != want {
		t.Fatalf("EnsurePackagedACPIntegrations(nil) = %v, want %q", err, want)
	}
}

func TestPriceTableNormalizeRoundTripsCanonicalIdentitiesAndExplicitZero(t *testing.T) {
	t.Parallel()

	cached := "0"
	reasoning := "1.2500"
	table := PriceTable{
		Currency: "USD",
		Models: []PriceTableModel{{
			Provider:                        " openai ",
			Model:                           " gpt-5 ",
			InputPerMillionTokens:           "1.25",
			OutputPerMillionTokens:          "10",
			CachedInputPerMillionTokens:     &cached,
			ReasoningOutputPerMillionTokens: &reasoning,
		}},
	}

	normalized, err := table.Normalize()
	if err != nil {
		t.Fatalf("PriceTable.Normalize() = %v", err)
	}
	if normalized.Currency != PriceTableCurrencyUSD || len(normalized.Models) != 1 {
		t.Fatalf("normalized table = %#v, want one USD model", normalized)
	}
	model := normalized.Models[0]
	if model.Provider != "CODEX" || model.Model != "gpt-5" || model.InputPerMillionTokens != "1.25" || model.OutputPerMillionTokens != "10" {
		t.Fatalf("normalized model = %#v, want canonical identities and exact rates", model)
	}
	if model.CachedInputPerMillionTokens == nil || *model.CachedInputPerMillionTokens != "0" {
		t.Fatalf("cached rate = %#v, want explicit zero", model.CachedInputPerMillionTokens)
	}
	if model.ReasoningOutputPerMillionTokens == nil || *model.ReasoningOutputPerMillionTokens != "1.2500" {
		t.Fatalf("reasoning rate = %#v, want exact decimal spelling", model.ReasoningOutputPerMillionTokens)
	}

	cloned := normalized.Clone()
	*cloned.Models[0].CachedInputPerMillionTokens = "2"
	if *normalized.Models[0].CachedInputPerMillionTokens != "0" {
		t.Fatal("PriceTable.Clone() aliased optional rates")
	}
}

func TestDefaultPriceTableContainsSourcedObservedModel(t *testing.T) {
	t.Parallel()

	table := defaultPriceTable()
	if table.Currency != PriceTableCurrencyUSD || len(table.Models) != 1 {
		t.Fatalf("defaultPriceTable() = %#v, want one USD model", table)
	}

	normalized, err := table.Normalize()
	if err != nil {
		t.Fatalf("defaultPriceTable().Normalize() = %v", err)
	}
	model := normalized.Models[0]
	if model.Provider != "CODEX" || model.Model != "gpt-5-codex" {
		t.Fatalf("default model identity = %#v, want CODEX/gpt-5-codex", model)
	}
	if model.InputPerMillionTokens != "1.25" || model.OutputPerMillionTokens != "10" {
		t.Fatalf("default base rates = %#v, want 1.25/10", model)
	}
	if model.CachedInputPerMillionTokens == nil || *model.CachedInputPerMillionTokens != "0.125" {
		t.Fatalf("default cached-input rate = %#v, want explicit 0.125", model.CachedInputPerMillionTokens)
	}
	if model.ReasoningOutputPerMillionTokens == nil || *model.ReasoningOutputPerMillionTokens != "10" {
		t.Fatalf("default reasoning-output rate = %#v, want explicit output-equivalent 10", model.ReasoningOutputPerMillionTokens)
	}
}

func TestPriceTableNormalizeRejectsInvalidEntries(t *testing.T) {
	t.Parallel()

	zero := "0"
	valid := func() PriceTable {
		return PriceTable{Currency: "USD", Models: []PriceTableModel{{
			Provider: "CODEX", Model: "gpt-5", InputPerMillionTokens: "1", OutputPerMillionTokens: "2",
		}}}
	}
	cases := []struct {
		name   string
		mutate func(*PriceTable)
	}{
		{name: "unsupported currency", mutate: func(table *PriceTable) { table.Currency = "EUR" }},
		{name: "blank provider", mutate: func(table *PriceTable) { table.Models[0].Provider = " " }},
		{name: "blank model", mutate: func(table *PriceTable) { table.Models[0].Model = " " }},
		{name: "missing input rate", mutate: func(table *PriceTable) { table.Models[0].InputPerMillionTokens = "" }},
		{name: "missing output rate", mutate: func(table *PriceTable) { table.Models[0].OutputPerMillionTokens = "" }},
		{name: "negative rate", mutate: func(table *PriceTable) { table.Models[0].InputPerMillionTokens = "-1" }},
		{name: "malformed rate", mutate: func(table *PriceTable) { table.Models[0].OutputPerMillionTokens = "1e-3" }},
		{name: "whitespace rate", mutate: func(table *PriceTable) { table.Models[0].InputPerMillionTokens = " 1 " }},
		{name: "malformed optional rate", mutate: func(table *PriceTable) {
			table.Models[0].CachedInputPerMillionTokens = &zero
			*table.Models[0].CachedInputPerMillionTokens = ".5"
		}},
		{name: "duplicate normalized key", mutate: func(table *PriceTable) {
			table.Models = append(table.Models, PriceTableModel{Provider: " openai ", Model: "gpt-5", InputPerMillionTokens: "3", OutputPerMillionTokens: "4"})
		}},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			table := valid()
			testCase.mutate(&table)
			if _, err := table.Normalize(); !errors.Is(err, ErrPriceTableInvalid) {
				t.Fatalf("PriceTable.Normalize() = %v, want ErrPriceTableInvalid", err)
			}
		})
	}
}

func TestReplacePriceTablePreservesUnrelatedSettings(t *testing.T) {
	t.Parallel()

	service := ConfigDocumentService{}
	document := ConfigDocument{config: Config{
		BackendScopeID: "local-scope",
		Defaults:       Defaults{WorkerModelProvider: "CODEX", WorkerModel: "gpt-5"},
		Runtime:        defaultRuntimeSettings(),
		WorkerPresets:  []WorkerPreset{{ID: "build", ModelProvider: "CODEX", Model: "gpt-5"}},
		Workers: WorkerSettings{ACP: ACPSettings{Integrations: []ACPIntegration{{
			ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
		}}}},
	}}
	replacement := PriceTable{Currency: "USD", Models: []PriceTableModel{{
		Provider: "claude", Model: "claude-sonnet", InputPerMillionTokens: "3", OutputPerMillionTokens: "15",
	}}}
	updated, err := service.ReplacePriceTable(document, replacement)
	if err != nil {
		t.Fatalf("ReplacePriceTable() = %v", err)
	}
	got := updated.FileConfig()
	if got.BackendScopeID != document.config.BackendScopeID || got.Defaults != document.config.Defaults ||
		!reflect.DeepEqual(got.Runtime, document.config.Runtime) ||
		!reflect.DeepEqual(got.WorkerPresets, document.config.WorkerPresets) ||
		!reflect.DeepEqual(got.Workers, document.config.Workers) {
		t.Fatalf("unrelated settings changed: %#v", got)
	}
	if got.PriceTable.Currency != PriceTableCurrencyUSD || got.PriceTable.Models[0].Provider != "CLAUDE" {
		t.Fatalf("price table = %#v, want normalized replacement", got.PriceTable)
	}
}
