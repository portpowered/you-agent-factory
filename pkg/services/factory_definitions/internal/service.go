// Package internal composes the Factory Definitions root from parent-private
// capability services. Concrete persistence, loading, and snapshot packages
// remain private to composition; peers receive only factory_definitions.Service.
package internal

import (
	"fmt"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	invocationpolicyservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy"
	validationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
	validationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/wire"
)

// Dependencies are the exact capabilities required to assemble the private
// Factory Definitions subservices. Session/runtime effects intentionally do
// not appear here; session-scoped operations belong to Factory Sessions.
type Dependencies struct {
	Validator                     factoryroot.Validator
	DefinitionValidation          factoryroot.DefinitionValidationOperation
	EffectiveDefinitionValidation factoryroot.EffectiveDefinitionValidationOperation
	LoadCanonical                 factoryroot.CanonicalFactoryJSONLoader
	NamedPaths                    factoryeffects.NamedPathResolver
	NamedFactoryCatalogFileSystem factoryeffects.NamedFactoryCatalogFileSystem
	PackagedCatalog               factoryroot.PackagedFactoryCatalogOperations
	PackagedInstaller             factoryroot.PackagedFactoryInstallationOperations
	RequiredToolChecker           factoryroot.RequiredToolChecker
	OrchestratorValidator         factoryroot.OrchestratorDefinitionValidator
	AuthoringLayout               authoringlayout.Service
	DistributionScaffold          factoryroot.ScaffoldInitializer
	ScaffoldFactoryNameResolver   distributionservice.ScaffoldFactoryNameResolver
	InvocationPolicy              invocationpolicyservice.Service
}

// NewService constructs the singular Factory Definitions root from already
// assembled private capabilities. It performs no lifecycle activation.
func NewService(deps Dependencies, options ...CompositionOption) (factoryroot.Service, error) {
	composition := applyCompositionOptions(options)
	if composition.scaffoldInitializer != nil {
		deps.DistributionScaffold = composition.scaffoldInitializer
	}
	if composition.scaffoldFactoryNameResolver != nil {
		deps.ScaffoldFactoryNameResolver = composition.scaffoldFactoryNameResolver
	}
	if deps.Validator == nil {
		return nil, fmt.Errorf("construct Factory Definitions: validator is required")
	}
	if deps.DefinitionValidation == nil {
		return nil, fmt.Errorf("construct Factory Definitions: definition validation operation is required")
	}
	if deps.EffectiveDefinitionValidation == nil {
		return nil, fmt.Errorf("construct Factory Definitions: effective definition validation operation is required")
	}
	if deps.LoadCanonical == nil {
		return nil, fmt.Errorf("construct Factory Definitions: canonical Factory loader is required")
	}
	if deps.NamedPaths == nil {
		return nil, fmt.Errorf("construct Factory Definitions: named path resolver is required")
	}
	if deps.NamedFactoryCatalogFileSystem == nil {
		return nil, fmt.Errorf("construct Factory Definitions: named Factory catalog filesystem is required")
	}
	if deps.PackagedCatalog.List == nil || deps.PackagedCatalog.Resolve == nil {
		return nil, fmt.Errorf("construct Factory Definitions: packaged Factory catalog is required")
	}
	if deps.PackagedInstaller.Install == nil {
		return nil, fmt.Errorf("construct Factory Definitions: packaged Factory installer is required")
	}
	if deps.RequiredToolChecker == nil {
		return nil, fmt.Errorf("construct Factory Definitions: required tool checker is required")
	}
	if deps.OrchestratorValidator == nil {
		return nil, fmt.Errorf("construct Factory Definitions: orchestrator definition validator is required")
	}
	if deps.AuthoringLayout == nil {
		return nil, fmt.Errorf("construct Factory Definitions: authoring_layout service is required")
	}
	if deps.InvocationPolicy == nil {
		return nil, fmt.Errorf("construct Factory Definitions: invocation policy is required")
	}

	catalogService, err := catalogwire.NewService(catalog.Dependencies{
		Paths:      deps.NamedPaths,
		FileSystem: deps.NamedFactoryCatalogFileSystem,
	})
	if err != nil {
		return nil, err
	}
	validationService, err := validationwire.NewService(validationservice.Dependencies{
		Operations:            deps.DefinitionValidation,
		Effective:             deps.EffectiveDefinitionValidation,
		LoadCanonical:         deps.LoadCanonical,
		RequiredToolChecker:   deps.RequiredToolChecker,
		OrchestratorValidator: deps.OrchestratorValidator,
	})
	if err != nil {
		return nil, err
	}
	distributionService := lifecycle.ComposeDistributionService(
		deps.PackagedCatalog,
		deps.PackagedInstaller,
		deps.DistributionScaffold,
		deps.ScaffoldFactoryNameResolver,
	)
	if distributionService == nil {
		return nil, fmt.Errorf("construct Factory Definitions: distribution service rejected its dependencies")
	}
	root := lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(
		catalogService,
		validationService,
		deps.AuthoringLayout,
		distributionService,
		deps.InvocationPolicy,
	)
	return root, nil
}
