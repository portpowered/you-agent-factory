package wire

import (
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
)

// NewOpeningPresentationOwner constructs the process-scoped opening owner.
func NewOpeningPresentationOwner() factorysessions.OpeningPresentationOwner {
	return invocation.NewOpeningPresentationOwner()
}
