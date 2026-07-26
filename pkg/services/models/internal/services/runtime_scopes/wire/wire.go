// Package wire constructs the private Models Runtime Scopes subservice.
package wire

import (
	"fmt"
	"strings"

	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/internal/service"
)

// NewService constructs an inert Runtime Scopes registry.
func NewService(generateIssuerID runtimescopes.IssuerIDGenerator) (runtimescopes.Service, error) {
	if generateIssuerID == nil {
		return nil, fmt.Errorf("Models Runtime Scopes issuer ID generator is required")
	}
	issuerID := strings.TrimSpace(generateIssuerID())
	if issuerID == "" {
		return nil, fmt.Errorf("Models Runtime Scopes issuer ID generator returned an empty identity")
	}
	return internalservice.New(issuerID), nil
}
