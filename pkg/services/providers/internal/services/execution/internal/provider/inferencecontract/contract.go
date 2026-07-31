// Package inferencecontract defines the external provider-inference port.
// Concrete provider adapters implement this contract, while composition and
// runtime packages depend on it without importing provider implementations.
package inferencecontract

import (
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Provider performs one inference request against an external model provider.
//
// Deprecated: Provider is the runtime port implemented by the repository's
// built-in adapters. New provider integrations should implement Integration.
type Provider = workers.Provider
