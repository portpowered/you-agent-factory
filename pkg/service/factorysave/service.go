package factorysave

import (
	"context"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Service preserves the legacy session-scoped save collaborator shape while
// delegating policy and side effects to the factory-definition owner.
type Service struct {
	host Host
}

// New constructs a factory-save compatibility collaborator.
func New(host Host) *Service {
	return &Service{host: host}
}

// Save delegates the session-scoped factory submission pipeline for the given mode.
func (s *Service) Save(
	ctx context.Context,
	sessionID string,
	mode factoryapi.FactorySaveMode,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	if s == nil || s.host == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory save service is required")
	}
	return s.host.SaveFactoryForSession(ctx, sessionID, mode, request)
}
