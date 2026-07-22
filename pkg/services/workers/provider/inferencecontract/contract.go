// Package inferencecontract defines the external provider-inference port.
// Concrete provider adapters implement this contract, while composition and
// runtime packages depend on it without importing provider implementations.
package inferencecontract

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Provider performs one inference request against an external model provider.
type Provider interface {
	Infer(context.Context, workers.ProviderInferenceRequest) (workers.InferenceResponse, error)
}
