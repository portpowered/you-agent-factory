package inference_test

import (
	"errors"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

func TestInputEchoInvocationRuntimeUsesGenericOutputMediaTypes(t *testing.T) {
	t.Parallel()

	request := models.InvokeModelRequest{
		Model:     models.ModelReference{NameOrURI: "fixture-model"},
		Operation: models.OperationOMNI,
		Inputs: []models.InferenceInput{
			{Name: "prompt", Content: "first"},
			{Name: "prompt", Content: "second"},
		},
	}
	operation := models.Operation{
		Name: models.OperationOMNI,
		Outputs: []models.OperationSlot{
			{Name: "explicit", Modality: models.ModalityText, MediaTypes: []string{"application/x-fixture"}},
			{Name: "text", Modality: models.ModalityText, ContentTypes: []string{"TEXT"}},
			{Name: "json", Modality: models.ModalityJSON, ContentTypes: []string{"JSON"}},
			{Name: "audio", Modality: models.ModalityAudio, ContentTypes: []string{"AUDIO"}},
			{Name: "image", Modality: models.ModalityImage, ContentTypes: []string{"IMAGE"}},
			{Name: "video", Modality: models.ModalityVideo, ContentTypes: []string{"VIDEO"}},
			{Name: "binary", Modality: models.ModalityBinary, ContentTypes: []string{"BINARY"}},
			{Name: "mime", Modality: models.ModalityText, ContentTypes: []string{"application/custom"}},
			{Name: "fallback", Modality: models.ModalityText, ContentTypes: []string{"unknown"}},
		},
	}
	result, err := (inference.InputEchoInvocationRuntime{}).Invoke(
		t.Context(), inference.InvocationRuntimeRequest{Request: request, Operation: operation},
	)
	if err != nil {
		t.Fatalf("InputEchoInvocationRuntime.Invoke() error = %v", err)
	}
	wantTypes := []string{
		"application/x-fixture", "text/plain", "application/json", "audio/wav",
		"image/*", "video/*", "application/octet-stream", "application/custom", "text/plain",
	}
	if len(result.Content) != len(wantTypes) {
		t.Fatalf("content count = %d, want %d", len(result.Content), len(wantTypes))
	}
	for index, content := range result.Content {
		if content.Name != operation.Outputs[index].Name || content.ContentType != wantTypes[index] || content.MediaType != wantTypes[index] {
			t.Fatalf("content[%d] = %#v, want name %q and media %q", index, content, operation.Outputs[index].Name, wantTypes[index])
		}
		if content.Content != "first\nsecond" {
			t.Fatalf("content[%d].Content = %q, want ordered joined inputs", index, content.Content)
		}
	}
}

func TestInputEchoInvocationRuntimeSelectsLegacyAndGenericOperationDefaults(t *testing.T) {
	t.Parallel()

	legacy, err := (inference.InputEchoInvocationRuntime{}).Invoke(
		t.Context(), inference.InvocationRuntimeRequest{
			Request: models.InvokeModelRequest{Input: models.InferenceInput{Content: "legacy"}},
		},
	)
	if err != nil || len(legacy.Content) != 1 || legacy.Content[0].ContentType != "text/plain" || legacy.Content[0].Content != "legacy" {
		t.Fatalf("legacy result = %#v, error = %v, want default text content", legacy, err)
	}

	for _, test := range []struct {
		operation string
		content   string
	}{
		{operation: models.OperationTTS, content: "audio/wav"},
		{operation: models.OperationEMBED, content: "application/json"},
		{operation: models.OperationOMNI, content: "text/plain"},
	} {
		t.Run(test.operation, func(t *testing.T) {
			result, err := (inference.InputEchoInvocationRuntime{}).Invoke(
				t.Context(), inference.InvocationRuntimeRequest{Request: models.InvokeModelRequest{
					Model: models.ModelReference{NameOrURI: "fixture-model"}, Operation: test.operation,
					Inputs: []models.InferenceInput{{Content: "generic"}},
				}},
			)
			if err != nil || len(result.Content) != 1 || result.Content[0].ContentType != test.content {
				t.Fatalf("operation %s result = %#v, error = %v, want %s", test.operation, result, err, test.content)
			}
		})
	}
}

func TestInertArtifactFileSystemRejectsUninjectedEffects(t *testing.T) {
	t.Parallel()

	fileSystem := inference.InertArtifactFileSystem{}
	for _, operation := range []struct {
		name string
		call func() error
	}{
		{name: "open", call: func() error { _, err := fileSystem.Open("fixture"); return err }},
		{name: "create", call: func() error { _, err := fileSystem.Create("fixture"); return err }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			err := operation.call()
			if err == nil || !strings.Contains(err.Error(), "explicit export filesystem") {
				t.Fatalf("%s error = %v, want explicit export filesystem diagnostic", operation.name, err)
			}
			if errors.Is(err, models.ErrUnavailable) {
				t.Fatalf("%s error = %v, did not expect unrelated unavailable classification", operation.name, err)
			}
		})
	}
}
