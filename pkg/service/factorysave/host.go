package factorysave

import (
	"context"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Host exposes only the definition-owned save operation needed by this
// compatibility delegate.
type Host interface {
	SaveFactoryForSession(
		ctx context.Context,
		sessionID string,
		mode factoryapi.FactorySaveMode,
		request factoryapi.Factory,
	) (factoryapi.Factory, error)
}
