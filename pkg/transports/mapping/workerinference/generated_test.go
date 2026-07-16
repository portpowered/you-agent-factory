package workerinference

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestOperationBindingsFromGeneratedMapsSelectorFields(t *testing.T) {
	t.Parallel()

	textType := factoryapi.ModelOperationContentTypeText
	label := "utterance"
	bindings := OperationBindingsFromGenerated(&[]factoryapi.WorkstationOperationBinding{{
		Slot: " text ",
		Selector: &factoryapi.WorkstationOperationBindingSelector{
			Label: &label,
			Type:  &textType,
		},
	}})
	if len(bindings) != 1 || bindings[0].Slot != "text" || bindings[0].Selector == nil || bindings[0].Selector.Label != label {
		t.Fatalf("bindings = %#v, want trimmed slot and mapped selector", bindings)
	}
}

func TestOperationBindingsFromGeneratedPreservesContent(t *testing.T) {
	t.Parallel()

	var contentPart factoryapi.WorkContentPart
	if err := contentPart.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: "hello",
	}); err != nil {
		t.Fatalf("build generated content: %v", err)
	}
	content := factoryapi.WorkContent{contentPart}
	bindings := OperationBindingsFromGenerated(&[]factoryapi.WorkstationOperationBinding{{
		Slot:           "text",
		DefaultContent: &content,
	}})
	if len(bindings) != 1 || len(bindings[0].DefaultContent) != 1 || bindings[0].DefaultContent[0].Text != "hello" {
		t.Fatalf("bindings = %#v, want generated default content mapped", bindings)
	}
}
