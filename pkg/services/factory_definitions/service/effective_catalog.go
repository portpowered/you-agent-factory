package service

import (
	"context"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type effectiveCatalog struct {
	discovery factorydefinitions.EffectiveFactoryCatalogDiscovery
	normalize factorydefinitions.EffectiveFactoryDefinitionNormalizer
}

type effectiveCatalogService struct {
	factorydefinitions.Service
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation
}

// EffectiveCatalogService is the read-only Factory Definitions owner used by
// transports that do not require a Factory Session.
type EffectiveCatalogService struct {
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation
}

// NewEffectiveCatalog constructs the stateless effective Factory catalog.
func NewEffectiveCatalog(
	discovery factorydefinitions.EffectiveFactoryCatalogDiscovery,
	normalize factorydefinitions.EffectiveFactoryDefinitionNormalizer,
) (factorydefinitions.EffectiveFactoryCatalogOperation, error) {
	if discovery.ListRoot == nil || discovery.ListPackaged == nil {
		return nil, fmt.Errorf("effective Factory catalog source is required")
	}
	if normalize == nil {
		return nil, fmt.Errorf("effective Factory definition normalizer is required")
	}
	catalog := effectiveCatalog{discovery: discovery, normalize: normalize}
	return catalog.listEffectiveFactories, nil
}

// AttachEffectiveCatalog returns the Factory Definitions service with
// effective discovery delegated to listEffective while preserving every other
// root operation.
func AttachEffectiveCatalog(
	service factorydefinitions.Service,
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
) (factorydefinitions.Service, error) {
	if service == nil {
		return nil, fmt.Errorf("Factory Definitions service is required")
	}
	if listEffective == nil {
		return nil, fmt.Errorf("effective Factory catalog is required")
	}
	return effectiveCatalogService{Service: service, listEffective: listEffective}, nil
}

// NewEffectiveCatalogService constructs the read-only Factory Definitions
// service slice used by transports that do not require a Factory Session.
func NewEffectiveCatalogService(
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
) (*EffectiveCatalogService, error) {
	if listEffective == nil {
		return nil, fmt.Errorf("effective Factory catalog is required")
	}
	return &EffectiveCatalogService{listEffective: listEffective}, nil
}

func (s effectiveCatalogService) ListEffectiveFactories(
	ctx context.Context,
	request factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	return s.listEffective(ctx, request)
}

func (s *EffectiveCatalogService) ListEffectiveFactories(
	ctx context.Context,
	request factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	return s.listEffective(ctx, request)
}

func (c effectiveCatalog) listEffectiveFactories(
	ctx context.Context,
	request factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	if err := ctx.Err(); err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, err
	}
	if err := validateRequest(request); err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, err
	}

	project, err := c.discovery.ListRoot(ctx, request.ProjectRoot)
	if contextErr := ctx.Err(); contextErr != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, contextErr
	}
	if err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, fmt.Errorf(
			"discover project-local Factories: %w",
			err,
		)
	}
	global, err := c.discovery.ListRoot(ctx, request.GlobalRoot)
	if contextErr := ctx.Err(); contextErr != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, contextErr
	}
	if err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, fmt.Errorf(
			"discover global Factories: %w",
			err,
		)
	}
	packaged, err := c.discovery.ListPackaged(ctx)
	if contextErr := ctx.Err(); contextErr != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, contextErr
	}
	if err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, fmt.Errorf(
			"discover packaged Factories: %w",
			err,
		)
	}

	entries, diagnostics, err := c.merge(ctx,
		catalogSource{
			kind:       factorydefinitions.EffectiveFactoryCatalogSourceProjectLocal,
			candidates: project,
		},
		catalogSource{
			kind:       factorydefinitions.EffectiveFactoryCatalogSourceGlobal,
			candidates: global,
		},
		catalogSource{
			kind:       factorydefinitions.EffectiveFactoryCatalogSourcePackaged,
			candidates: packaged,
		},
	)
	if err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, err
	}
	return factorydefinitions.ListEffectiveFactoriesResult{
		Entries:     entries,
		Diagnostics: diagnostics,
	}, nil
}

func validateRequest(request factorydefinitions.ListEffectiveFactoriesRequest) error {
	if strings.TrimSpace(request.ProjectRoot) == "" {
		return fmt.Errorf("project Factory root is required")
	}
	if strings.TrimSpace(request.GlobalRoot) == "" {
		return fmt.Errorf("global Factory root is required")
	}
	return nil
}

func detachedEntry(
	candidate factorydefinitions.EffectiveFactoryCatalogCandidate,
	definition *factorydefinitions.FactoryConfig,
) (factorydefinitions.EffectiveFactoryCatalogEntry, error) {
	cloned, err := factorydefinitions.CloneFactoryConfig(definition)
	if err != nil {
		return factorydefinitions.EffectiveFactoryCatalogEntry{}, err
	}
	return factorydefinitions.EffectiveFactoryCatalogEntry{
		Name:                candidate.Name,
		Location:            cloneString(candidate.Location),
		Definition:          cloned,
		InvocationSignature: cloned.InvocationSignature,
	}, nil
}

func cloneCandidate(
	candidate factorydefinitions.EffectiveFactoryCatalogCandidate,
) factorydefinitions.EffectiveFactoryCatalogCandidate {
	candidate.Location = cloneString(candidate.Location)
	candidate.Canonical = append([]byte(nil), candidate.Canonical...)
	return candidate
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
