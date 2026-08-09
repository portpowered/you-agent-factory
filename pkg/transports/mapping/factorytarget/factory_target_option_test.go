package factorytarget_test

import (
	"encoding/json"
	"testing"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorytarget"
)

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestFromCatalogResultSerializesSelectOptionContract(t *testing.T) {
	t.Parallel()

	result := chatsessions.ResolveFactoryTargetCatalogResult{
		CurrentTarget: "factory:@you/factory-builder",
		Choices: []chatsessions.FactoryTargetCatalogChoice{
			{Value: "factory:@you/factory-builder", Name: "Factory Builder"},
			{Value: "factory:@you/review", Name: "Review"},
		},
	}

	option := factorytarget.FromCatalogResult(result)
	wire, err := option.ToSessionConfigOption()
	if err != nil {
		t.Fatalf("ToSessionConfigOption() unexpected error: %v", err)
	}
	if wire.Select == nil {
		t.Fatal("ToSessionConfigOption() Select is nil, want the select-option variant")
	}
	if wire.Select.Id != acp.FactoryTargetOptionID {
		t.Fatalf("Id = %q, want %q", wire.Select.Id, acp.FactoryTargetOptionID)
	}
	if wire.Select.Name != acp.FactoryTargetOptionName {
		t.Fatalf("Name = %q, want %q", wire.Select.Name, acp.FactoryTargetOptionName)
	}
	if wire.Select.Type != "select" {
		t.Fatalf("Type = %q, want %q", wire.Select.Type, "select")
	}
	if wire.Select.Category == nil || *wire.Select.Category != acp.FactoryTargetOptionCategory {
		t.Fatalf("Category = %+v, want %q", wire.Select.Category, acp.FactoryTargetOptionCategory)
	}
	if string(wire.Select.CurrentValue) != result.CurrentTarget {
		t.Fatalf("CurrentValue = %q, want %q", wire.Select.CurrentValue, result.CurrentTarget)
	}
	if wire.Select.Options.Ungrouped == nil {
		t.Fatal("Options.Ungrouped is nil")
	}
	ungrouped := *wire.Select.Options.Ungrouped
	if len(ungrouped) != len(result.Choices) {
		t.Fatalf("choice count = %d, want %d", len(ungrouped), len(result.Choices))
	}
	for index, choice := range result.Choices {
		if string(ungrouped[index].Value) != choice.Value {
			t.Fatalf("choice[%d].Value = %q, want %q", index, ungrouped[index].Value, choice.Value)
		}
		if ungrouped[index].Name != choice.Name {
			t.Fatalf("choice[%d].Name = %q, want %q", index, ungrouped[index].Name, choice.Name)
		}
	}

	if err := wire.Validate(); err != nil {
		t.Fatalf("wire.Validate() unexpected error: %v", err)
	}
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("json.Marshal(wire) unexpected error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(wire) unexpected error: %v", err)
	}
	if decoded["category"] != "model" {
		t.Fatalf("serialized category = %v, want %q", decoded["category"], "model")
	}
	for _, forbidden := range []string{"modelId", "models"} {
		if _, present := decoded[forbidden]; present {
			t.Fatalf("serialized Factory target option unexpectedly contains model field %q", forbidden)
		}
	}
}

func TestFromCatalogResultPreservesCallerSuppliedOrder(t *testing.T) {
	t.Parallel()

	builder := chatsessions.FactoryTargetCatalogChoice{Value: "factory:@you/factory-builder", Name: "Factory Builder"}
	analysis := chatsessions.FactoryTargetCatalogChoice{Value: "factory:@you/analysis", Name: "Analysis"}
	review := chatsessions.FactoryTargetCatalogChoice{Value: "factory:@you/review", Name: "Review"}

	cases := []struct {
		name    string
		choices []chatsessions.FactoryTargetCatalogChoice
	}{
		{name: "canonical order", choices: []chatsessions.FactoryTargetCatalogChoice{builder, analysis, review}},
		{name: "reordered", choices: []chatsessions.FactoryTargetCatalogChoice{review, builder, analysis}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			option := factorytarget.FromCatalogResult(chatsessions.ResolveFactoryTargetCatalogResult{
				CurrentTarget: "factory:@you/factory-builder",
				Choices:       testCase.choices,
			})
			if len(option.Choices) != len(testCase.choices) {
				t.Fatalf("choice count = %d, want %d", len(option.Choices), len(testCase.choices))
			}
			for index, want := range testCase.choices {
				if string(option.Choices[index].Value) != want.Value || option.Choices[index].Name != want.Name {
					t.Fatalf("choice[%d] = %+v, want {Value:%s Name:%s} (projection must not reorder)", index, option.Choices[index], want.Value, want.Name)
				}
			}
		})
	}
}

func TestFromCatalogResultRejectsEmptyChoicesThroughSerialization(t *testing.T) {
	t.Parallel()

	option := factorytarget.FromCatalogResult(chatsessions.ResolveFactoryTargetCatalogResult{
		CurrentTarget: "factory:@you/factory-builder",
	})
	if _, err := option.ToSessionConfigOption(); err == nil {
		t.Fatal("ToSessionConfigOption() unexpected nil error for an empty catalog projection")
	}
}
