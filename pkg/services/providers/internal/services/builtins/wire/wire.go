// Package wire exposes the parent-private packaged Providers catalog decoder.
package wire

import (
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	builtinsservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/builtins/internal/service"
)

// DecodeACPIntegrations projects validated runtime data without constructing
// a service or selecting execution collaborators.
func DecodeACPIntegrations(document []byte) ([]providers.ACPIntegration, error) {
	return builtinsservice.DecodeACPIntegrations(document)
}
