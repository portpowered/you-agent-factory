package factorydefinition

import (
	"context"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
)

// AttachDistribution wires the private distribution subservice behind the
// CTR-DEF root distribute slice. When unset, UnimplementedService stubs remain.
func (s *Service) AttachDistribution(distributionService distribution.Service) *Service {
	if s == nil {
		return nil
	}
	s.distribution = distributionService
	return s
}

// ListBuiltInPackagedFactories delegates to the private distribution ownership
// when attached.
func (s *Service) ListBuiltInPackagedFactories(
	ctx context.Context,
	request factoryroot.ListBuiltInPackagedFactoriesRequest,
) (factoryroot.ListBuiltInPackagedFactoriesResult, error) {
	if s == nil || s.distribution == nil {
		return factoryroot.UnimplementedService{}.ListBuiltInPackagedFactories(ctx, request)
	}
	return s.distribution.ListBuiltInPackagedFactories(ctx, request)
}

// InstallPackagedFactory delegates to the private distribution ownership when
// attached.
func (s *Service) InstallPackagedFactory(
	ctx context.Context,
	request factoryroot.InstallPackagedFactoryRequest,
) (factoryroot.InstallPackagedFactoryResult, error) {
	if s == nil || s.distribution == nil {
		return factoryroot.UnimplementedService{}.InstallPackagedFactory(ctx, request)
	}
	return s.distribution.InstallPackagedFactory(ctx, request)
}

// CreateFactoryScaffold delegates to the private distribution ownership when
// attached.
func (s *Service) CreateFactoryScaffold(
	ctx context.Context,
	request factoryroot.CreateFactoryScaffoldRequest,
) (factoryroot.CreateFactoryScaffoldResult, error) {
	if s == nil || s.distribution == nil {
		return factoryroot.UnimplementedService{}.CreateFactoryScaffold(ctx, request)
	}
	return s.distribution.CreateFactoryScaffold(ctx, request)
}
