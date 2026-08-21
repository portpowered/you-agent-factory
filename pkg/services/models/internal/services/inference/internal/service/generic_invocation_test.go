package service_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

func TestPrepareGenericInvocationInfersOneOperationAndRetainsInputOrder(t *testing.T) {
	t.Parallel()

	scope := mustScopeRef(t, "scope-generic-preflight")
	request := models.GenericInvocationRequest{
		Scope:  scope,
		Holder: "caller",
		Model:  models.ModelReference{NameOrURI: "custom"},
		Inputs: []models.InferenceInput{
			{Name: "prompt", Modality: models.ModalityText, Content: "hello"},
			{Name: "image", Modality: models.ModalityImage, MediaType: "image/png", Content: "one"},
			{Name: "image", Modality: models.ModalityImage, MediaType: "image/jpeg", Content: "two"},
		},
		Parameters: []models.OperationParameter{{Name: "temperature", Value: map[string]any{"value": 0.2}}},
	}
	operation := genericOperation(
		"generate",
		[]models.OperationSlot{
			{Name: "prompt", Modality: models.ModalityText, Required: boolPointer(true), MediaTypes: []string{"text/plain"}},
			{Name: "image", Modality: models.ModalityImage, Repeatable: true, MediaTypes: []string{"image/*"}},
			{Name: "parameters", Modality: models.ModalityJSON, MediaTypes: []string{"application/json"}},
		},
		[]models.OperationSlot{{Name: "text", Modality: models.ModalityText, Required: boolPointer(true), MediaTypes: []string{"text/plain"}}},
	)

	prepared, selected, err := models.PrepareGenericInvocation(request, models.ModelDefinition{
		Name: "custom", Operations: []models.Operation{operation},
	})
	if err != nil {
		t.Fatalf("PrepareGenericInvocation: %v", err)
	}
	if selected.Name != "GENERATE" || prepared.Operation != "GENERATE" {
		t.Fatalf("selected operation = %#v, prepared operation = %q", selected, prepared.Operation)
	}
	if len(prepared.Inputs) != 3 || prepared.Inputs[1].Content != "one" || prepared.Inputs[2].Content != "two" {
		t.Fatalf("prepared inputs = %#v, want ordered repeatable values", prepared.Inputs)
	}
	prepared.Inputs[1].Content = "mutated"
	prepared.Parameters[0].Value.(map[string]any)["value"] = 1.0
	if request.Inputs[1].Content != "one" || request.Parameters[0].Value.(map[string]any)["value"] != 0.2 {
		t.Fatalf("prepared request retained mutable caller data: %#v", request)
	}
}

func TestPrepareGenericInvocationRejectsContractFailuresBeforeEffects(t *testing.T) {
	t.Parallel()

	scope := mustScopeRef(t, "scope-generic-failures")
	base := models.GenericInvocationRequest{
		Scope:     scope,
		Holder:    "caller",
		Model:     models.ModelReference{NameOrURI: "llm"},
		Operation: "OMNI",
		Inputs:    []models.InferenceInput{{Name: "prompt", Modality: models.ModalityText, Content: "hello"}},
	}
	operation := genericOperation(
		models.OperationOMNI,
		[]models.OperationSlot{{Name: "prompt", Modality: models.ModalityText, Required: boolPointer(true), MediaTypes: []string{"text/plain"}}},
		[]models.OperationSlot{{Name: "text", Modality: models.ModalityText, Required: boolPointer(true), MediaTypes: []string{"text/plain"}}},
	)
	tests := []struct {
		name  string
		edit  func(*models.GenericInvocationRequest)
		class models.InvocationFailureClass
	}{
		{
			name: "unknown slot",
			edit: func(request *models.GenericInvocationRequest) {
				request.Inputs = []models.InferenceInput{{Name: "image", Modality: models.ModalityImage}}
			},
			class: models.InvocationFailureClassInvalidSlot,
		},
		{
			name: "non repeatable arity",
			edit: func(request *models.GenericInvocationRequest) {
				request.Inputs = append(request.Inputs, request.Inputs[0])
			},
			class: models.InvocationFailureClassSlotArity,
		},
		{
			name: "unsupported modality",
			edit: func(request *models.GenericInvocationRequest) {
				request.Inputs[0].Modality = models.ModalityAudio
			},
			class: models.InvocationFailureClassMediaCapability,
		},
		{
			name: "invalid parameter value",
			edit: func(request *models.GenericInvocationRequest) {
				request.Parameters = []models.OperationParameter{{Name: "temperature", Value: func() {}}}
			},
			class: models.InvocationFailureClassInvalidParameter,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := base
			request.Inputs = append([]models.InferenceInput(nil), base.Inputs...)
			test.edit(&request)
			_, _, err := models.PrepareGenericInvocation(request, models.ModelDefinition{
				Name: "llm", Operations: []models.Operation{operation},
			})
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != test.class {
				t.Fatalf("error = %v, failure = %#v, want class %q", err, failure, test.class)
			}
			if strings.Contains(err.Error(), "cache") || strings.Contains(err.Error(), "endpoint") {
				t.Fatalf("contract error leaked runtime detail: %v", err)
			}
		})
	}
}

func TestPrepareGenericInvocationRejectsAmbiguousAndUnknownOperations(t *testing.T) {
	t.Parallel()

	scope := mustScopeRef(t, "scope-generic-operations")
	request := models.GenericInvocationRequest{
		Scope:  scope,
		Holder: "caller",
		Model:  models.ModelReference{NameOrURI: "llm"},
	}
	definitions := []models.Operation{
		{Name: "OMNI"},
		{Name: "EMBED"},
	}
	_, _, err := models.PrepareGenericInvocation(request, models.ModelDefinition{
		Name: "llm", Operations: definitions,
	})
	assertInvocationFailure(t, err, models.InvocationFailureClassInvalidOperation, "EMBED", "OMNI")

	request.Operation = "missing"
	_, _, err = models.PrepareGenericInvocation(request, models.ModelDefinition{
		Name: "llm", Operations: definitions,
	})
	assertInvocationFailure(t, err, models.InvocationFailureClassInvalidOperation, "EMBED", "OMNI")
}

func TestNormalizeGenericInvocationOutputsPreservesNamesAndMetadata(t *testing.T) {
	t.Parallel()

	artifactRef, err := (models.InferenceArtifactRef{}).Parse("artifact:segments")
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	operation := genericOperation(
		models.OperationASR,
		nil,
		[]models.OperationSlot{
			{Name: "transcript", Modality: models.ModalityText, Required: boolPointer(true), MediaTypes: []string{"text/plain"}},
			{Name: "segments", Modality: models.ModalityJSON, Required: boolPointer(true), MediaTypes: []string{"application/json"}},
		},
	)
	outputs, err := models.NormalizeGenericInvocationOutputs(
		operation,
		[]models.InferenceContent{{Name: "transcript", Modality: models.ModalityText, MediaType: "text/plain", Content: "hello"}},
		[]models.InferenceArtifact{{Name: "segments", MediaType: "application/json", SizeBytes: 12, Artifact: artifactRef, Properties: map[string]string{"digest": "sha256:segments"}}},
	)
	if err != nil {
		t.Fatalf("NormalizeGenericInvocationOutputs: %v", err)
	}
	if len(outputs) != 2 || outputs[0].Name != "transcript" || outputs[1].Name != "segments" {
		t.Fatalf("outputs = %#v, want declared named order", outputs)
	}
	if outputs[0].Content != "hello" || outputs[0].MediaType != "text/plain" {
		t.Fatalf("inline output = %#v", outputs[0])
	}
	if outputs[1].Artifact == nil || outputs[1].Artifact.SizeBytes != 12 || outputs[1].Artifact.Properties["digest"] != "sha256:segments" {
		t.Fatalf("artifact output = %#v", outputs[1])
	}

	singleOutput, err := models.NormalizeGenericInvocationOutputs(
		models.Operation{Name: models.OperationOMNI, Outputs: []models.OperationSlot{{Name: "text", Modality: models.ModalityText, Required: boolPointer(true), MediaTypes: []string{"text/plain"}}}},
		[]models.InferenceContent{{Modality: models.ModalityText, MediaType: "text/plain", Content: "single"}},
		nil,
	)
	if err != nil || len(singleOutput) != 1 || singleOutput[0].Name != "text" {
		t.Fatalf("unnamed single output = %#v, error = %v, want inferred text slot", singleOutput, err)
	}
}

func TestNormalizeGenericInvocationOutputsRejectsMalformedAndOversizedResponsesAtomically(t *testing.T) {
	t.Parallel()

	operation := genericOperation(
		models.OperationOMNI,
		nil,
		[]models.OperationSlot{{Name: "text", Modality: models.ModalityText, Required: boolPointer(true), MediaTypes: []string{"text/plain"}}},
	)
	for _, test := range []struct {
		name        string
		outputName  string
		outputValue string
	}{
		{name: "unknown output", outputName: "unknown", outputValue: "bad"},
		{name: "oversized output", outputName: "text", outputValue: strings.Repeat("x", 16<<20+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			outputs, err := models.NormalizeGenericInvocationOutputs(
				operation,
				[]models.InferenceContent{{Name: "text", Modality: models.ModalityText, Content: "ok"}, {Name: test.outputName, Modality: models.ModalityText, Content: test.outputValue}},
				nil,
			)
			var failure *models.InvocationFailure
			if outputs != nil || !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMalformedResponse {
				t.Fatalf("outputs = %#v, error = %v, failure = %#v, want atomic malformed response", outputs, err, failure)
			}
		})
	}
}

func genericOperation(name string, inputs, outputs []models.OperationSlot) models.Operation {
	return models.Operation{Name: name, Inputs: inputs, Outputs: outputs}
}

func boolPointer(value bool) *bool {
	return &value
}

func assertInvocationFailure(t *testing.T, err error, class models.InvocationFailureClass, validNames ...string) {
	t.Helper()
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != class {
		t.Fatalf("error = %v, failure = %#v, want class %q", err, failure, class)
	}
	for _, name := range validNames {
		found := false
		for _, valid := range failure.ValidNames {
			if valid == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("failure valid names = %#v, want %q", failure.ValidNames, name)
		}
	}
}
