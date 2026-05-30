package factorysave

import (
	"context"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
)

// Service owns session-scoped factory save orchestration.
type Service struct {
	factoryRootDir string
	clock          factory.Clock
	loader         func() factoryconfig.WorkstationLoader
	host           Host
}

// New constructs a factory-save collaborator with explicit dependencies.
func New(
	factoryRootDir string,
	clock factory.Clock,
	loader func() factoryconfig.WorkstationLoader,
	host Host,
) *Service {
	return &Service{
		factoryRootDir: factoryRootDir,
		clock:          clock,
		loader:         loader,
		host:           host,
	}
}

// Save runs the session-scoped factory submission pipeline for the given mode.
func (s *Service) Save(
	ctx context.Context,
	sessionID string,
	mode factoryapi.FactorySaveMode,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	if s == nil || s.host == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory save service is required")
	}
	switch mode {
	case factoryapi.FactorySaveModeUpsertNamedAndActivate:
		return s.saveUpsertNamedAndActivateForSession(ctx, sessionID, request)
	default:
		return s.saveReplaceCurrentForSession(ctx, sessionID, request)
	}
}

func (s *Service) workstationLoader() factoryconfig.WorkstationLoader {
	if s == nil || s.loader == nil {
		return nil
	}
	return s.loader()
}

func (s *Service) now() factory.Clock {
	if s == nil || s.clock == nil {
		return factory.EnsureClock(nil)
	}
	return factory.EnsureClock(s.clock)
}
