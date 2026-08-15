// Package inferencecontract defines the external provider-inference port.
// Concrete provider adapters implement this contract, while composition and
// runtime packages depend on it without importing provider implementations.
package inferencecontract

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// LegacyProvider performs one inference request against an external model
// provider for the retained compatibility subtree.
//
// This compatibility port is declared by Providers rather than aliasing the
// Workers request-scoped Provider interface. The legacy request/result values
// remain in the method signature until the retained compatibility subtree is
// retired by its owning WSE-09 packet; the interface ownership direction is
// already Providers -> compatibility values, never Providers -> Workers port.
// New provider integrations should implement Integration.
type LegacyProvider interface {
	Infer(context.Context, workers.ProviderInferenceRequest) (workers.InferenceResponse, error)
}
