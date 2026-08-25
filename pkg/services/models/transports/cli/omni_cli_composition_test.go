package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
)

func TestRootAdapter_InvokeNamedLLMInputInfersOmniAndWritesRequiredText(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	required := boolPointer(true)
	optional := boolPointer(false)
	var gotRequest modelinference.InvokeModelRequest
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(_ context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				if request.Operation != modelinference.OperationOMNI {
					return modelinference.GetModelResult{}, fmt.Errorf("operation = %q, want OMNI", request.Operation)
				}
				return genericCLIOperationModel(
					request.Name,
					modelinference.OperationOMNI,
					[]modelinference.OperationSlot{{Name: "prompt", Modality: modelinference.ModalityText, Required: required, MediaTypes: []string{"text/plain"}}},
					[]modelinference.OperationSlot{
						{Name: "text", Modality: modelinference.ModalityText, Required: required},
						{Name: "usage", Modality: modelinference.ModalityJSON, Required: optional},
					},
				), nil
			},
			invokeModel: func(_ context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				gotRequest = request
				return modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{
					{Name: "text", Modality: modelinference.ModalityText, Content: "fixture answer"},
					{Name: "usage", Modality: modelinference.ModalityJSON, Content: `{"tokens":3}`},
				}}, nil
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	})

	var out bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: modelinference.BuiltInModelNameLLM,
		InputMappings: []string{"prompt=Write a haiku"}, Output: &out,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if out.String() != "fixture answer" {
		t.Fatalf("stdout = %q, want required text only", out.String())
	}
	if gotRequest.Operation != modelinference.OperationOMNI || len(gotRequest.Inputs) != 1 || gotRequest.Inputs[0].Name != "prompt" || gotRequest.Inputs[0].Content != "Write a haiku" {
		t.Fatalf("joined request = %#v, want inferred OMNI prompt input", gotRequest)
	}
}

func TestRootAdapter_InvokeGenericOutputPathPublishesSingleOutput(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	outputPath := filepath.Join(t.TempDir(), "answer.txt")
	if err := os.WriteFile(outputPath, []byte("old answer"), 0o600); err != nil {
		t.Fatalf("seed output path: %v", err)
	}
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIModel("omni", modelinference.OperationOMNI,
					modelinference.OperationSlot{Name: "text", Modality: modelinference.ModalityText, Required: boolPointer(true)}), nil
			},
			invokeModel: func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				return modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{
					Name: "text", Modality: modelinference.ModalityText, Content: "path answer",
				}}}, nil
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
		OutputFileSystem: localOutputFileSystem{},
	})

	var out bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "omni", Operation: modelinference.OperationOMNI,
		Text: "hello", OutputPath: outputPath, Output: &out,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	assertMappedCLIFile(t, outputPath, "path answer")
	if out.String() != "Wrote audio: "+outputPath+"\n" {
		t.Fatalf("output notice = %q, want publication notice", out.String())
	}
}
