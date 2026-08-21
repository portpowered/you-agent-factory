package http

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestGenericInvocationMappingPreservesRepeatedOMNIInputsAndJSONOrder(t *testing.T) {
	t.Parallel()

	outputMode := factoryapi.ModelInvocationOutputModeJSON
	offline := true
	inputs := []factoryapi.ModelInvocationInput{
		{Name: "prompt", Modality: factoryapi.ModelOperationContentTypeText, Content: stringPointer("compare")},
		{Name: "image", Modality: factoryapi.ModelOperationContentTypeImage, MediaType: stringPointer("image/png"), Content: stringPointer("first")},
		{Name: "image", Modality: factoryapi.ModelOperationContentTypeImage, MediaType: stringPointer("image/jpeg"), Content: stringPointer("second")},
	}
	parameters := []factoryapi.ModelInvocationParameter{{Name: "temperature", Value: map[string]any{"value": 0.2}}}
	generated := factoryapi.GenericModelInvocationRequest{
		Scope:      "scope-http-001",
		Holder:     "http",
		Model:      factoryapi.ModelReference{NameOrUri: "llm"},
		Operation:  models.OperationOMNI,
		Inputs:     &inputs,
		Parameters: &parameters,
		OutputMode: &outputMode,
		Offline:    &offline,
	}

	mapped, err := GenericInvocationRequestFromGenerated(generated)
	if err != nil {
		t.Fatalf("GenericInvocationRequestFromGenerated() error = %v", err)
	}
	if err := mapped.Validate(); err != nil {
		t.Fatalf("mapped request Validate() error = %v", err)
	}
	if mapped.Model.NameOrURI != "llm" || len(mapped.Inputs) != 3 || mapped.Inputs[1].Name != "image" || mapped.Inputs[2].Content != "second" {
		t.Fatalf("mapped request = %#v", mapped)
	}
	if mapped.OutputMode != models.OutputModeJSON || !mapped.Offline || mapped.Parameters[0].Name != "temperature" {
		t.Fatalf("mapped controls = %#v", mapped)
	}

	encoded, err := json.Marshal(generated)
	if err != nil {
		t.Fatalf("marshal generated request: %v", err)
	}
	var decoded factoryapi.GenericModelInvocationRequest
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal generated request: %v", err)
	}
	if decoded.Inputs == nil || len(*decoded.Inputs) != 3 || (*decoded.Inputs)[1].Name != "image" || (*decoded.Inputs)[2].MediaType == nil || *(*decoded.Inputs)[2].MediaType != "image/jpeg" {
		t.Fatalf("serialized inputs = %#v", decoded.Inputs)
	}
}

func TestGenericInvocationResponseMappingPreservesASROutputsAndFailureIdentity(t *testing.T) {
	t.Parallel()

	artifact, err := (models.InferenceArtifactRef{}).Parse("artifact:segments")
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	result := models.GenericInvocationResult{Outputs: []models.InferenceOutput{
		{Name: "transcript", Modality: models.ModalityText, ContentType: "text/plain", MediaType: "text/plain", Content: "hello"},
		{Name: "segments", Modality: models.ModalityJSON, ContentType: "application/json", MediaType: "application/json", Artifact: &models.InferenceArtifact{
			Artifact: artifact, Name: "segments.json", MediaType: "application/json", SizeBytes: 12,
		}},
	}}
	projected := GenericInvocationResponseToGenerated(result)
	if len(projected.Outputs) != 2 || projected.Outputs[0].Name != "transcript" || projected.Outputs[1].Name != "segments" {
		t.Fatalf("projected outputs = %#v", projected.Outputs)
	}
	if projected.Outputs[1].Artifact == nil || projected.Outputs[1].Artifact.ArtifactRef != "artifact:segments" {
		t.Fatalf("projected artifact = %#v", projected.Outputs[1].Artifact)
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatalf("marshal generated response: %v", err)
	}
	var decoded factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal generated response: %v", err)
	}
	if len(decoded.Outputs) != 2 || decoded.Outputs[0].Name != "transcript" || decoded.Outputs[1].Artifact == nil || decoded.Outputs[1].Artifact.ArtifactRef != "artifact:segments" {
		t.Fatalf("serialized outputs = %#v", decoded.Outputs)
	}

	failure := &models.InvocationFailure{
		Class:     models.InvocationFailureClassBackendProtocol,
		Message:   "backend protocol is incompatible",
		Model:     models.ModelReference{NameOrURI: "asr"},
		Operation: models.OperationASR,
		Slot:      "segments",
	}
	projectedFailure := GenericInvocationFailureToGenerated(failure)
	if projectedFailure == nil || projectedFailure.Class != factoryapi.ModelInvocationFailureClassBackendProtocol || projectedFailure.Model == nil || projectedFailure.Model.NameOrUri != "asr" || projectedFailure.Slot == nil || *projectedFailure.Slot != "segments" {
		t.Fatalf("projected failure = %#v", projectedFailure)
	}
	if projectedFailure.Message != failure.Message {
		t.Fatalf("projected failure message = %q, want %q", projectedFailure.Message, failure.Message)
	}
}

func TestGenericInvocationRequestMappingRejectsInvalidArtifactAsTypedFailure(t *testing.T) {
	t.Parallel()

	inputs := []factoryapi.ModelInvocationInput{{
		Name:        "image",
		Modality:    factoryapi.ModelOperationContentTypeImage,
		ArtifactRef: stringPointer(" "),
	}}
	_, err := GenericInvocationRequestFromGenerated(factoryapi.GenericModelInvocationRequest{
		Scope:     "scope-http-002",
		Holder:    "http",
		Model:     factoryapi.ModelReference{NameOrUri: "llm"},
		Operation: models.OperationOMNI,
		Inputs:    &inputs,
	})
	var failure *models.InvocationFailure
	if err == nil || !asInvocationFailure(err, &failure) || failure.Class != models.InvocationFailureClassArtifact {
		t.Fatalf("error = %v, failure = %#v, want typed artifact failure", err, failure)
	}
}

func asInvocationFailure(err error, target **models.InvocationFailure) bool {
	if typed, ok := err.(*models.InvocationFailure); ok {
		*target = typed
		return true
	}
	return false
}

func stringPointer(value string) *string {
	return &value
}
