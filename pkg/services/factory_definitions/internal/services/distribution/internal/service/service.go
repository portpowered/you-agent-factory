package service

import (
	"context"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
)

// Service is the private nested distribution implementation behind the CTR-DEF
// root distribute slice.
type Service struct {
	packagedCatalog   factorydefinitions.PackagedFactoryCatalogOperations
	packagedInstaller factorydefinitions.PackagedFactoryInstallationOperations
	scaffold          distributionservice.ScaffoldOperation
}

var _ distributionservice.Service = (*Service)(nil)

// New constructs the distribution implementation from exact injected ports.
func New(
	packagedCatalog factorydefinitions.PackagedFactoryCatalogOperations,
	packagedInstaller factorydefinitions.PackagedFactoryInstallationOperations,
	scaffold distributionservice.ScaffoldOperation,
) *Service {
	return &Service{
		packagedCatalog:   packagedCatalog,
		packagedInstaller: packagedInstaller,
		scaffold:          scaffold,
	}
}

func (s *Service) ListBuiltInPackagedFactories(
	ctx context.Context,
	request factorydefinitions.ListBuiltInPackagedFactoriesRequest,
) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
	return s.packagedCatalog.ListBuiltInPackagedFactories(ctx, request)
}

func (s *Service) ResolveBuiltInPackagedFactory(
	ctx context.Context,
	request factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
	return s.packagedCatalog.ResolveBuiltInPackagedFactory(ctx, request)
}

func (s *Service) InstallPackagedFactory(
	ctx context.Context,
	request factorydefinitions.InstallPackagedFactoryRequest,
) (factorydefinitions.InstallPackagedFactoryResult, error) {
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

func (s *Service) CreateFactoryScaffold(ctx context.Context, request factorydefinitions.CreateFactoryScaffoldRequest) (factorydefinitions.CreateFactoryScaffoldResult, error) {
	return s.scaffold(ctx, request)
}

// Scaffold owns enabled scaffold creation from complete effects.
type Scaffold struct {
	initializer factorydefinitions.ScaffoldInitializer
	resolveName distributionservice.ScaffoldFactoryNameResolver
}

// NewScaffold captures effects without executing them.
func NewScaffold(initializer factorydefinitions.ScaffoldInitializer, resolveName distributionservice.ScaffoldFactoryNameResolver) *Scaffold {
	return &Scaffold{initializer: initializer, resolveName: resolveName}
}

func (s *Scaffold) CreateFactoryScaffold(
	ctx context.Context,
	request factorydefinitions.CreateFactoryScaffoldRequest,
) (factorydefinitions.CreateFactoryScaffoldResult, error) {
	if err := ctx.Err(); err != nil {
		return factorydefinitions.CreateFactoryScaffoldResult{}, err
	}
	if err := factorydefinitions.ValidateCreateFactoryScaffoldRequest(request); err != nil {
		return factorydefinitions.CreateFactoryScaffoldResult{}, err
	}
	targetDir := strings.TrimSpace(request.TargetDir)
	if err := s.initializer(factorydefinitions.ScaffoldConfig{Dir: targetDir}); err != nil {
		return factorydefinitions.CreateFactoryScaffoldResult{},
			fmt.Errorf("%w: %w", factorydefinitions.ErrFactoryDistributeFailed, err)
	}
	name, err := s.resolveName(targetDir)
	if err != nil {
		return factorydefinitions.CreateFactoryScaffoldResult{},
			fmt.Errorf("%w: %w", factorydefinitions.ErrFactoryDistributeFailed, err)
	}
	if strings.TrimSpace(name) == "" {
		return factorydefinitions.CreateFactoryScaffoldResult{},
			factorydefinitions.ErrFactoryDistributeFailed
	}
	scaffoldType := strings.TrimSpace(request.Type)
	if scaffoldType == "" {
		scaffoldType = factorydefinitions.DefaultScaffoldType
	}
	return factorydefinitions.CreateFactoryScaffoldResult{
		Definition: factorydefinitions.DistributedFactoryDefinitionFacts{
			Name:       name,
			FactoryDir: targetDir,
		},
		ScaffoldType: scaffoldType,
	}, nil
}

// DisabledScaffold preserves the unavailable capability's error precedence.
type DisabledScaffold struct{}

func (DisabledScaffold) CreateFactoryScaffold(context.Context, factorydefinitions.CreateFactoryScaffoldRequest) (factorydefinitions.CreateFactoryScaffoldResult, error) {
	return factorydefinitions.CreateFactoryScaffoldResult{}, fmt.Errorf("%w: scaffold collaborator is required", factorydefinitions.ErrFactoryDistributeFailed)
}
