package service

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
)

// Service is the private nested distribution implementation behind the CTR-DEF
// root distribute slice.
type Service struct {
	packagedCatalog     factorydefinitions.PackagedFactoryCatalogOperations
	packagedInstaller   factorydefinitions.PackagedFactoryInstallationOperations
	scaffoldInitializer factorydefinitions.ScaffoldInitializer
}

var _ distributionservice.Service = (*Service)(nil)

// New constructs the distribution implementation from exact injected ports.
func New(
	packagedCatalog factorydefinitions.PackagedFactoryCatalogOperations,
	packagedInstaller factorydefinitions.PackagedFactoryInstallationOperations,
	scaffoldInitializer factorydefinitions.ScaffoldInitializer,
) *Service {
	if packagedCatalog.List == nil ||
		packagedCatalog.Resolve == nil ||
		packagedInstaller.Install == nil ||
		scaffoldInitializer == nil {
		return nil
	}
	return &Service{
		packagedCatalog:     packagedCatalog,
		packagedInstaller:   packagedInstaller,
		scaffoldInitializer: scaffoldInitializer,
	}
}

func (s *Service) ListBuiltInPackagedFactories(
	ctx context.Context,
	request factorydefinitions.ListBuiltInPackagedFactoriesRequest,
) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.ListBuiltInPackagedFactoriesResult{}, err
	}
	return s.packagedCatalog.ListBuiltInPackagedFactories(ctx, request)
}

func (s *Service) ResolveBuiltInPackagedFactory(
	ctx context.Context,
	request factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, err
	}
	return s.packagedCatalog.ResolveBuiltInPackagedFactory(ctx, request)
}

func (s *Service) InstallPackagedFactory(
	ctx context.Context,
	request factorydefinitions.InstallPackagedFactoryRequest,
) (factorydefinitions.InstallPackagedFactoryResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.InstallPackagedFactoryResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.InstallPackagedFactoryResult{}, err
	}
	if err := factorydefinitions.ValidateInstallPackagedFactoryRequest(request); err != nil {
		return factorydefinitions.InstallPackagedFactoryResult{}, err
	}
	resolved, err := s.ResolveBuiltInPackagedFactory(
		ctx,
		factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: request.Name},
	)
	if err != nil {
		return factorydefinitions.InstallPackagedFactoryResult{}, err
	}
	installed, err := s.packagedInstaller.InstallPackagedFactory(
		ctx,
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: request.RootDir,
			Definition:         resolved.Definition,
			Format:             request.Format,
			Replace:            request.Replace,
		},
	)
	if err != nil {
		return factorydefinitions.InstallPackagedFactoryResult{},
			fmt.Errorf("%w: %w", factorydefinitions.ErrFactoryDistributeFailed, err)
	}
	return factorydefinitions.InstallPackagedFactoryResult{
		Definition: factorydefinitions.DistributedFactoryDefinitionFacts{
			Name:       installed.Name,
			FactoryDir: installed.FactoryDir,
		},
		Outcome: installed.Outcome,
		Format:  installed.Format,
	}, nil
}

func (s *Service) CreateFactoryScaffold(
	_ context.Context,
	_ factorydefinitions.CreateFactoryScaffoldRequest,
) (factorydefinitions.CreateFactoryScaffoldResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.CreateFactoryScaffoldResult{}, err
	}
	return factorydefinitions.CreateFactoryScaffoldResult{}, factorydefinitions.ErrFactoryDistributeFailed
}

func (s *Service) requirePorts() error {
	if s == nil ||
		s.packagedCatalog.List == nil ||
		s.packagedCatalog.Resolve == nil ||
		s.packagedInstaller.Install == nil ||
		s.scaffoldInitializer == nil {
		return fmt.Errorf("Factory Definition distribution collaborator is required")
	}
	return nil
}
