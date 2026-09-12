package cli_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
)

type zeroInputEffectsModelsRoot struct {
	stubModelsRoot
	preflightCalls int
}

func (root *zeroInputEffectsModelsRoot) PreflightModelAssets(
	context.Context,
	modelinference.PrepareModelAssetsRequest,
) (modelinference.PreflightModelAssetsResult, error) {
	root.preflightCalls++
	return modelinference.PreflightModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

type zeroInputRequiredSlotCase struct {
	name           string
	model          string
	operation      string
	inputs         []modelinference.OperationSlot
	outputs        []modelinference.OperationSlot
	wantMissing    []string
	wantValidNames []string
	wantMessage    string
}

func zeroInputRequiredSlotCases() []zeroInputRequiredSlotCase {
	return []zeroInputRequiredSlotCase{
		{
			name: "asr", model: modelinference.BuiltInModelNameASR, operation: modelinference.OperationASR,
			inputs:      []modelinference.OperationSlot{{Name: "audio", Modality: modelinference.ModalityAudio, Required: boolPointer(true)}},
			outputs:     []modelinference.OperationSlot{{Name: "transcript", Modality: modelinference.ModalityText}},
			wantMissing: []string{"audio"}, wantValidNames: []string{"audio"}, wantMessage: "required input slot is missing: audio",
		},
		{
			name: "tts", model: modelinference.BuiltInModelNameTTS, operation: modelinference.OperationTTS,
			inputs:      []modelinference.OperationSlot{{Name: "text", Modality: modelinference.ModalityText, Required: boolPointer(true)}},
			outputs:     []modelinference.OperationSlot{{Name: "audio", Modality: modelinference.ModalityAudio}},
			wantMissing: []string{"text"}, wantValidNames: []string{"text"}, wantMessage: "required input slot is missing: text",
		},
		{
			name: "embed", model: modelinference.BuiltInModelNameEmbed, operation: modelinference.OperationEMBED,
			inputs:      []modelinference.OperationSlot{{Name: "text", Modality: modelinference.ModalityText, Required: boolPointer(true)}},
			outputs:     []modelinference.OperationSlot{{Name: "embedding", Modality: modelinference.ModalityJSON}},
			wantMissing: []string{"text"}, wantValidNames: []string{"text"}, wantMessage: "required input slot is missing: text",
		},
		{
			name: "omni", model: modelinference.BuiltInModelNameLLM, operation: modelinference.OperationOMNI,
			inputs:      []modelinference.OperationSlot{{Name: "prompt", Modality: modelinference.ModalityText, Required: boolPointer(true)}},
			outputs:     []modelinference.OperationSlot{{Name: "text", Modality: modelinference.ModalityText}},
			wantMissing: []string{"prompt"}, wantValidNames: []string{"prompt"}, wantMessage: "required input slot is missing: prompt",
		},
		{
			name: "operator-defined multi-slot operation", model: "operator-model", operation: "CUSTOM",
			inputs: []modelinference.OperationSlot{
				{Name: "zeta", Modality: modelinference.ModalityText, Required: boolPointer(true)},
				{Name: "optional", Modality: modelinference.ModalityJSON, Required: boolPointer(false)},
				{Name: "alpha", Modality: modelinference.ModalityText, Required: boolPointer(true)},
			},
			outputs:     []modelinference.OperationSlot{{Name: "result", Modality: modelinference.ModalityText}},
			wantMissing: []string{"alpha", "zeta"}, wantValidNames: []string{"alpha", "optional", "zeta"},
			wantMessage: "required input slot is missing: alpha, zeta",
		},
	}
}

func TestRootAdapter_InvokeGenericZeroInputsUseCatalogRequiredSlots(t *testing.T) {
	t.Parallel()
	for _, test := range zeroInputRequiredSlotCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertRootAdapterZeroInputRequiredSlots(t, test)
		})
	}
}

func assertRootAdapterZeroInputRequiredSlots(t *testing.T, test zeroInputRequiredSlotCase) {
	t.Helper()
	catalogCalls, inputReads, leaseCalls, invokeCalls := 0, 0, 0, 0
	root := &zeroInputEffectsModelsRoot{}
	root.stubModelsRoot.getCatalogModel = func(_ context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
		catalogCalls++
		if request.Name != test.model || request.Operation != test.operation {
			t.Fatalf("catalog request = %#v, want model=%q operation=%q", request, test.model, test.operation)
		}
		return genericCLIOperationModel(test.model, test.operation, test.inputs, test.outputs), nil
	}
	root.stubModelsRoot.acquireModelLease = func(context.Context, modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error) {
		leaseCalls++
		return modelinference.AcquireModelLeaseResult{}, nil
	}
	root.stubModelsRoot.invokeModel = func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
		invokeCalls++
		return modelinference.InvokeModelResult{}, nil
	}
	service := modelscli.NewService(modelscli.Config{
		Models: root,
		InputFileReader: func(context.Context, string, int64) ([]byte, error) {
			inputReads++
			return []byte("unexpected input"), nil
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: testRuntimeScope(t)}, nil
		},
	})

	outputPath := filepath.Join(t.TempDir(), "result.out")
	err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(), ModelName: test.model, Operation: test.operation,
		OutputMappings: []string{test.outputs[0].Name + "=" + outputPath}, Output: io.Discard,
	})
	var failure *modelinference.InvocationFailure
	if !errors.As(err, &failure) || failure == nil {
		t.Fatalf("Invoke() error = %v, want typed invocation failure", err)
	}
	if failure.Class != modelinference.InvocationFailureClassInvalidSlot {
		t.Fatalf("InvocationFailure class = %q, want INVALID_SLOT", failure.Class)
	}
	if failure.Message != test.wantMessage {
		t.Fatalf("InvocationFailure message = %q, want %q", failure.Message, test.wantMessage)
	}
	if len(test.wantMissing) == 0 || failure.Slot != test.wantMissing[0] {
		t.Fatalf("InvocationFailure slot = %q, want first missing slot %q", failure.Slot, test.wantMissing[0])
	}
	if !reflect.DeepEqual(failure.ValidNames, test.wantValidNames) {
		t.Fatalf("InvocationFailure valid names = %#v, want %#v", failure.ValidNames, test.wantValidNames)
	}
	if catalogCalls != 1 || root.preflightCalls != 0 || inputReads != 0 || leaseCalls != 0 || invokeCalls != 0 {
		t.Fatalf("zero-input effects = catalog:%d preflight:%d reads:%d leases:%d invokes:%d, want 1/0/0/0/0", catalogCalls, root.preflightCalls, inputReads, leaseCalls, invokeCalls)
	}
}
