package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
)

func TestRootAdapterPullProgressStaysOnStderr(t *testing.T) {
	scope := testRuntimeScope(t)
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			pullModel: func(ctx context.Context, name string) (modelinference.PullResult, error) {
				modelinference.ReportPullProgress(ctx, modelinference.PullProgressObservation{
					ModelName: name, Artifact: "model.bin", TransferredBytes: 512, TotalBytes: 1024,
				})
				return modelinference.PullResult{ModelName: name}, nil
			},
		},
		OpenCatalogScope: func(context.Context) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
		PullProgressInterval: time.Hour,
	})

	var stdout, stderr bytes.Buffer
	if err := service.Pull(modelscli.PullConfig{
		Context: context.Background(), ModelName: "voice", Output: &stdout, Progress: &stderr,
	}); err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !strings.Contains(stderr.String(), `phase=pull`) ||
		!strings.Contains(stderr.String(), `transferredBytes=512 totalBytes=1024 percent=50.0%`) {
		t.Fatalf("pull stderr = %q, want pull progress with byte totals", stderr.String())
	}
	if strings.Contains(stdout.String(), "models pull progress") {
		t.Fatalf("pull stdout = %q, must contain final result only", stdout.String())
	}
}

func TestRootAdapterInvokeProgressCoversImplicitPreparation(t *testing.T) {
	scope := testRuntimeScope(t)
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(_ context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return genericCLIOperationModel(
					request.Name,
					modelinference.OperationTTS,
					[]modelinference.OperationSlot{{
						Name: "text", Modality: modelinference.ModalityText, Required: boolPointer(true),
					}},
					[]modelinference.OperationSlot{{
						Name: "result", Modality: modelinference.ModalityText, Required: boolPointer(true),
					}},
				), nil
			},
			invokeModel: func(ctx context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				modelinference.ReportPullProgress(ctx, modelinference.PullProgressObservation{
					ModelName: request.Model.NameOrURI, Artifact: "voice.bin", TransferredBytes: 3, TotalBytes: 4,
				})
				return modelinference.InvokeModelResult{
					ModelName: request.Model.NameOrURI, Operation: request.Operation,
					Outputs: []modelinference.InferenceOutput{{
						Name: "result", Modality: modelinference.ModalityText, Content: "ready",
					}},
				}, nil
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
		PullProgressInterval: time.Hour,
	})

	var stdout, stderr bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: "voice", Operation: modelinference.OperationTTS,
		Text: "hello", JSON: true, Output: &stdout, Progress: &stderr,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !strings.Contains(stderr.String(), `phase=preparation`) ||
		!strings.Contains(stderr.String(), `transferredBytes=3 totalBytes=4 percent=75.0%`) {
		t.Fatalf("invoke stderr = %q, want preparation progress with byte totals", stderr.String())
	}
	if strings.Contains(stdout.String(), "models pull progress") || !strings.Contains(stdout.String(), "ready") {
		t.Fatalf("invoke stdout = %q, want final JSON result only", stdout.String())
	}
}

func TestRootAdapterPullProgressPreservesFailureAndStops(t *testing.T) {
	scope := testRuntimeScope(t)
	wantErr := errors.New("transfer failed")
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			pullModel: func(ctx context.Context, name string) (modelinference.PullResult, error) {
				modelinference.ReportPullProgress(ctx, modelinference.PullProgressObservation{
					ModelName: name, Artifact: "model.bin", TransferredBytes: 1, TotalBytes: 2,
				})
				return modelinference.PullResult{}, wantErr
			},
		},
		OpenCatalogScope: func(context.Context) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
		PullProgressInterval: time.Hour,
	})

	var stdout, stderr bytes.Buffer
	err := service.Pull(modelscli.PullConfig{
		Context: context.Background(), ModelName: "voice", Output: &stdout, Progress: &stderr,
	})
	if err == nil || (!errors.Is(err, wantErr) && !strings.Contains(err.Error(), wantErr.Error())) {
		t.Fatalf("Pull() error = %v, want transfer failure", err)
	}
	if !strings.Contains(stderr.String(), `phase=pull`) {
		t.Fatalf("failure stderr = %q, want progress before failure", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("failure stdout = %q, want no success result", stdout.String())
	}
}
