// Package inferencecontract defines the external provider-inference port.
// Concrete provider adapters implement this contract, while composition and
// runtime packages depend on it without importing provider implementations.
package inferencecontract

import (
	"context"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// Provider performs one inference request against an external model provider.
//
// Deprecated: Provider is the Providers-owned compatibility port implemented
// by the retained built-in adapters. Workers consumers use workers.Runner and
// never import this package.
type Provider interface {
	Infer(context.Context, workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error)
}
