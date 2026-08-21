package openapitests

import (
	"testing"

	. "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestFactoryConfigFromOpenAPIJSON_MapsModelInvokeOperation(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"tts-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{
			"name":"tts-worker",
			"type":"MODEL_WORKER",
			"operations":[{
				"name":"TTS",
				"inputs":[{"name":"text","contentTypes":["TEXT"],"required":true}],
				"outputs":[{"name":"audio","contentTypes":["AUDIO"]}]
			}]
		}],
		"workstations": [{
			"name":"speak-story",
			"worker":"tts-worker",
			"type":"MODEL_INVOKE",
			"operation":"TTS",
			"operationBindings":[{
				"slot":"text",
				"selector":{"slot":"text","type":"TEXT"},
				"defaultContent":[{"type":"TEXT","slot":"text","text":"fallback"}]
			}],
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if len(cfg.Workstations) != 1 {
		t.Fatalf("expected one workstation, got %d", len(cfg.Workstations))
	}
	if got := cfg.Workstations[0].Type; got != interfaces.WorkstationTypeInvoke {
		t.Fatalf("workstation type = %q, want MODEL_INVOKE", got)
	}
	if got := cfg.Workstations[0].Operation; got != "TTS" {
		t.Fatalf("workstation operation = %q, want TTS", got)
	}
	if len(cfg.Workstations[0].OperationBindings) != 1 {
		t.Fatalf("operation bindings = %#v, want one binding", cfg.Workstations[0].OperationBindings)
	}
	binding := cfg.Workstations[0].OperationBindings[0]
	if binding.Slot != "text" {
		t.Fatalf("binding slot = %q, want text", binding.Slot)
	}
	if binding.Selector == nil || binding.Selector.Type != interfaces.ModelOperationContentTypeText {
		t.Fatalf("binding selector = %#v, want TEXT selector", binding.Selector)
	}
	if len(binding.DefaultContent) != 1 || binding.DefaultContent[0].Slot != "text" || binding.DefaultContent[0].Text != "fallback" {
		t.Fatalf("binding default content = %#v", binding.DefaultContent)
	}
}

func TestFactoryConfigModelOperationSlotsPreserveVideoAndGenericMetadata(t *testing.T) {
	t.Parallel()

	cfgJSON := []byte(`{
		"name":"generic-factory",
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{
			"name":"omni-worker",
			"type":"MODEL_WORKER",
			"operations":[{
				"name":"OMNI",
				"inputs":[
					{"name":"prompt","contentTypes":["TEXT"],"modality":"TEXT","required":true},
					{"name":"image","contentTypes":["IMAGE"],"modality":"IMAGE","repeatable":true,"mediaTypes":["image/*"]}
				],
				"outputs":[{"name":"video","contentTypes":["VIDEO"],"modality":"VIDEO","repeatable":false,"mediaTypes":["video/*"]}]
			}]
		}],
		"workstations": [{"name":"invoke-task","worker":"omni-worker","type":"MODEL_INVOKE","operation":"OMNI","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}]}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	slots := cfg.Workers[0].Operations[0]
	if len(slots.Inputs) != 2 || slots.Inputs[1].Modality != interfaces.ModelOperationContentTypeImage {
		t.Fatalf("mapped inputs = %#v, want IMAGE metadata", slots.Inputs)
	}
	image := slots.Inputs[1]
	if image.Repeatable == nil || !*image.Repeatable || len(image.MediaTypes) != 1 || image.MediaTypes[0] != "image/*" {
		t.Fatalf("mapped image slot = %#v, want repeatable image metadata", image)
	}
	video := slots.Outputs[0]
	if len(video.ContentTypes) != 1 || video.ContentTypes[0] != interfaces.ModelOperationContentTypeVideo || video.Modality != interfaces.ModelOperationContentTypeVideo {
		t.Fatalf("mapped video slot = %#v, want VIDEO content and modality", video)
	}
	if video.Repeatable == nil || *video.Repeatable || len(video.MediaTypes) != 1 || video.MediaTypes[0] != "video/*" {
		t.Fatalf("mapped video slot = %#v, want explicit non-repeatable metadata", video)
	}

	encoded, err := MarshalCanonicalFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	roundTripped, err := FactoryConfigFromOpenAPIJSON(encoded)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON(round trip): %v", err)
	}
	roundTrippedVideo := roundTripped.Workers[0].Operations[0].Outputs[0]
	if roundTrippedVideo.Modality != interfaces.ModelOperationContentTypeVideo || roundTrippedVideo.Repeatable == nil || *roundTrippedVideo.Repeatable || roundTrippedVideo.MediaTypes[0] != "video/*" {
		t.Fatalf("round-tripped video slot = %#v, want VIDEO metadata preserved", roundTrippedVideo)
	}
}

func TestFactoryConfigFromOpenAPIJSON_MapsTypedModelResources(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"tts-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"resources": [{
			"name":"omnivoice-cache",
			"type":"MODEL",
			"capacity":1,
			"model":"OMNIVOICE_Q4_K_M",
			"backend":"LLAMACPP",
			"loadPolicy":"ON_DEMAND"
		}],
		"workers": [{"name":"tts-worker","type":"MODEL_WORKER"}],
		"workstations": [{
			"name":"speak-story",
			"worker":"tts-worker",
			"type":"MODEL_WORKSTATION",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if len(cfg.Resources) != 1 {
		t.Fatalf("expected one resource, got %#v", cfg.Resources)
	}
	resource := cfg.Resources[0]
	if resource.Type != interfaces.ResourceTypeModel {
		t.Fatalf("resource type = %q, want MODEL", resource.Type)
	}
	if resource.Model != "OMNIVOICE_Q4_K_M" || resource.Backend != "LLAMACPP" || resource.LoadPolicy != "ON_DEMAND" {
		t.Fatalf("resource metadata = %#v, want model/backend/loadPolicy preserved", resource)
	}
}
