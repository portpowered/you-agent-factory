package factorysave

import (
	"context"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func (s *Service) saveUpsertNamedAndActivateForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	return s.host.SaveUpsertNamedAndActivateForSession(ctx, sessionID, request)
}
