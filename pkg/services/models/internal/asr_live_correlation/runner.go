// Package asrlivecorrelation contains the private compiled Models harness for
// correlating one decoded LocalAI ASR response with its managed-child Wait.
package asrlivecorrelation

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

// InvokeASRWithLease enters through the Models root and carries only the
// invocation-local correlation owner into the private LocalAI adapter.
func InvokeASRWithLease(
	ctx context.Context,
	service models.Service,
	scope models.RuntimeScopeRef,
	lease models.ModelLeaseRef,
	audio []byte,
	controller *modelseffects.ASRLiveCorrelationController,
) (models.InvokeModelResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return service.InvokeModelWithLease(
		modelseffects.WithASRLiveCorrelation(ctx, controller),
		models.InvokeModelRequest{
			Scope: scope, Lease: lease, Holder: "asr-worker",
			ModelName: models.BuiltInModelNameASR,
			Model:     models.ModelReference{NameOrURI: models.BuiltInModelNameASR},
			Operation: models.OperationASR, Offline: true,
			Inputs: []models.InferenceInput{{
				Name: "audio", Modality: models.ModalityAudio,
				ContentType: "audio/wav", MediaType: "audio/wav", Content: string(audio),
			}},
		},
	)
}
