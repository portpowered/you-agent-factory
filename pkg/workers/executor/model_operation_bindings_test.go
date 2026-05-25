package executor

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestResolveModelOperationBindings_UsesInputThenConfigThenDefaultAndRecordsSource(t *testing.T) {
	workstation := &interfaces.FactoryWorkstationConfig{
		Type:      interfaces.WorkstationTypeInvoke,
		Operation: "TTS",
		OperationBindings: []interfaces.ModelOperationBinding{
			{
				Slot: "text",
				Selector: &interfaces.ModelOperationBindingSelector{
					Label: "utterance",
					Type:  interfaces.ModelOperationContentTypeText,
				},
			},
			{
				Slot: "voice",
				Selector: &interfaces.ModelOperationBindingSelector{
					Role: "voice",
				},
				Config: []interfaces.WorkContentPart{{
					Type: interfaces.WorkContentPartTypeJSON,
					Role: "voice",
					JSON: []byte(`{"name":"alloy"}`),
				}},
			},
			{
				Slot: "style",
				DefaultContent: []interfaces.WorkContentPart{{
					Type: interfaces.WorkContentPartTypeText,
					Text: "neutral",
					Slot: "style",
				}},
			},
		},
	}
	worker := &interfaces.WorkerConfig{
		Name: "tts-worker",
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
			Inputs: []interfaces.ModelOperationSlot{
				{Name: "text", Required: true},
				{Name: "voice"},
				{Name: "style"},
				{Name: "optional"},
			},
		}},
	}
	inputs := []interfaces.Token{{
		ID: "tok-1",
		Color: interfaces.TokenColor{
			Content: []interfaces.WorkContentPart{
				{Type: interfaces.WorkContentPartTypeText, Slot: "ignored", Label: "utterance", Text: "first"},
				{Type: interfaces.WorkContentPartTypeText, Slot: "text", Label: "utterance", Text: "second"},
			},
		},
	}}

	got, err := resolveModelOperationBindings(workstation, worker, inputs)
	if err != nil {
		t.Fatalf("resolveModelOperationBindings: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("binding count = %d, want 4", len(got))
	}
	if got[0].Source != interfaces.ModelOperationBindingSourceInput || got[0].Content[0].Text != "first" {
		t.Fatalf("text binding = %#v, want first input match", got[0])
	}
	if got[1].Source != interfaces.ModelOperationBindingSourceConfig || string(got[1].Content[0].JSON) != `{"name":"alloy"}` {
		t.Fatalf("voice binding = %#v, want config fallback", got[1])
	}
	if got[2].Source != interfaces.ModelOperationBindingSourceDefault || got[2].Content[0].Text != "neutral" {
		t.Fatalf("style binding = %#v, want default fallback", got[2])
	}
	if got[3].Source != interfaces.ModelOperationBindingSourceOmitted || len(got[3].Content) != 0 {
		t.Fatalf("optional binding = %#v, want omitted", got[3])
	}
}

func TestResolveModelOperationBindings_ImplicitlyMatchesBySlotAndRejectsMissingRequiredInput(t *testing.T) {
	workstation := &interfaces.FactoryWorkstationConfig{
		Type:      interfaces.WorkstationTypeInvoke,
		Operation: "TTS",
	}
	worker := &interfaces.WorkerConfig{
		Name: "tts-worker",
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
			Inputs: []interfaces.ModelOperationSlot{
				{Name: "text", Required: true},
			},
		}},
	}

	got, err := resolveModelOperationBindings(workstation, worker, []interfaces.Token{{
		ID: "tok-1",
		Color: interfaces.TokenColor{
			Content: []interfaces.WorkContentPart{{
				Type: interfaces.WorkContentPartTypeText,
				Slot: "text",
				Text: "hello",
			}},
		},
	}})
	if err != nil {
		t.Fatalf("resolveModelOperationBindings implicit slot: %v", err)
	}
	if len(got) != 1 || got[0].Source != interfaces.ModelOperationBindingSourceInput || got[0].Content[0].Text != "hello" {
		t.Fatalf("implicit slot binding = %#v, want input text", got)
	}

	_, err = resolveModelOperationBindings(workstation, worker, nil)
	if err == nil {
		t.Fatal("expected missing required input slot to fail")
	}
}
