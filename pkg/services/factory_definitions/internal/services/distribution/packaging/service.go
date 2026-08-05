// Package packaging owns the focused Factory Definitions packaged-catalog
// capability. It resolves package identity and validates installation intent
// without exposing catalog callbacks, installer callbacks, or filesystem
// effects to callers.
package packaging

import (
	"context"
	"errors"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service implements the public focused Packaging capability from direct,
// already-constructed Factory Definitions ports.
type Service struct {
	catalog      factorydefinitions.PackagedFactoryCatalog
	installation factorydefinitions.PackagedFactoryInstallation
}

var _ factorydefinitions.Packaging = (*Service)(nil)

// New constructs an inert Packaging capability. It performs no catalog lookup,
// validation, or filesystem work until a caller invokes an operation.
func New(
	catalog factorydefinitions.PackagedFactoryCatalog,
	installation factorydefinitions.PackagedFactoryInstallation,
) (*Service, error) {
	if catalog == nil {
		return nil, fmt.Errorf("construct Factory Definitions packaging: package catalog is required")
	}
	if installation == nil {
		return nil, fmt.Errorf("construct Factory Definitions packaging: package installation is required")
	}
	return &Service{catalog: catalog, installation: installation}, nil
}

func (s *Service) ListBuiltInPackagedFactories(
	ctx context.Context,
	request factorydefinitions.ListBuiltInPackagedFactoriesRequest,
) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.ListBuiltInPackagedFactoriesResult{}, err
	}
	return s.catalog.ListBuiltInPackagedFactories(ctx, request)
}

func (s *Service) ResolveBuiltInPackagedFactory(
	ctx context.Context,
	request factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, err
	}
	resolved, err := s.catalog.ResolveBuiltInPackagedFactory(ctx, request)
	if err != nil {
		return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, classifyPackageError(
			request.Name,
			"",
			err,
		)
	}
	return resolved, nil
}

func (s *Service) InstallPackagedFactory(
	ctx context.Context,
	request factorydefinitions.InstallPackagedFactoryRequest,
) (factorydefinitions.InstallPackagedFactoryResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.InstallPackagedFactoryResult{}, err
	}
	if ctx == nil {
		return factorydefinitions.InstallPackagedFactoryResult{}, fmt.Errorf("install packaged factory context is required")
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
	installed, err := s.installation.InstallPackagedFactory(
		ctx,
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: request.RootDir,
			Definition:         resolved.Definition,
			Format:             request.Format,
			Replace:            request.Replace,
		},
	)
	if err != nil {
		return factorydefinitions.InstallPackagedFactoryResult{}, classifyPackageError(
			request.Name,
			request.Format,
			err,
		)
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

func (s *Service) requirePorts() error {
	if s == nil || s.catalog == nil || s.installation == nil {
		return fmt.Errorf("Factory Definitions packaging capability is required")
	}
	return nil
}

func classifyPackageError(
	name string,
	format factorydefinitions.PackagedFactoryFormat,
	err error,
) error {
	if err == nil {
		return nil
	}
	var typed *factorydefinitions.PackagedFactoryInputError
	if errors.As(err, &typed) {
		return err
	}
	if errors.Is(err, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		return factorydefinitions.NewPackagedFactoryInputError(
			factorydefinitions.PackagedFactoryErrorMissing,
			name,
			format,
			"",
			err,
		)
	}
	if errors.Is(err, factorydefinitions.ErrUnsupportedPackagedFactoryFormat) {
		return err
	}
	if errors.Is(err, factorydefinitions.ErrPackagedFactoryIntegrity) {
		return err
	}
	if errors.Is(err, factorydefinitions.ErrMalformedPackagedFactory) {
		return err
	}
	return factorydefinitions.NewPackagedFactoryInputError(
		factorydefinitions.PackagedFactoryErrorMalformed,
		name,
		format,
		"",
		err,
	)
}
