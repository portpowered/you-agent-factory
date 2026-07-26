package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
)

type effectiveCatalog struct {
	discovery factorydefinitions.EffectiveFactoryCatalogDiscovery
	normalize factorydefinitions.EffectiveFactoryDefinitionNormalizer
}

type effectiveCatalogService struct {
	factorydefinitions.Service
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

func (s effectiveCatalogService) ListEffectiveFactories(
	ctx context.Context,
	request factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	return s.listEffective(ctx, request)
}

func (c effectiveCatalog) listEffectiveFactories(
	ctx context.Context,
	request factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	if err := validateRequest(request); err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, err
	}

	project, err := c.discovery.ListRoot(ctx, request.ProjectRoot)
	if err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, fmt.Errorf(
			"discover project-local Factories: %w",
			err,
		)
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, err
	}
	global, err := c.discovery.ListRoot(ctx, request.GlobalRoot)
	if err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, fmt.Errorf(
			"discover global Factories: %w",
			err,
		)
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, err
	}
	packaged, err := c.discovery.ListPackaged(ctx)
	if err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, fmt.Errorf(
			"discover packaged Factories: %w",
			err,
		)
	}

	entries, err := c.merge(ctx, project, global, packaged)
	if err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, err
	}
	return factorydefinitions.ListEffectiveFactoriesResult{Entries: entries}, nil
}

func (c effectiveCatalog) merge(
	ctx context.Context,
	sources ...[]factorydefinitions.EffectiveFactoryCatalogCandidate,
) ([]factorydefinitions.EffectiveFactoryCatalogEntry, error) {
	claimed := make(map[string]struct{})
	entries := make([]factorydefinitions.EffectiveFactoryCatalogEntry, 0)
	for _, source := range sources {
		candidates, err := canonicalCandidates(source)
		if err != nil {
			return nil, err
		}
		for _, candidate := range candidates {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if _, shadowed := claimed[candidate.Name]; shadowed {
				continue
			}
			claimed[candidate.Name] = struct{}{}
			definition, err := c.normalize(ctx, cloneCandidate(candidate))
			if err != nil {
				return nil, fmt.Errorf("normalize Factory %q: %w", candidate.Name, err)
			}
			if definition == nil {
				return nil, fmt.Errorf("normalize Factory %q: definition is required", candidate.Name)
			}
			entry, err := detachedEntry(candidate, definition)
			if err != nil {
				return nil, fmt.Errorf("detach Factory %q: %w", candidate.Name, err)
			}
			entries = append(entries, entry)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
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

func canonicalCandidates(
	source []factorydefinitions.EffectiveFactoryCatalogCandidate,
) ([]factorydefinitions.EffectiveFactoryCatalogCandidate, error) {
	candidates := make([]factorydefinitions.EffectiveFactoryCatalogCandidate, len(source))
	for index, candidate := range source {
		segments, err := factorynamedpaths.PathSegments(candidate.Name)
		if err != nil {
			return nil, err
		}
		name, err := factorynamedpaths.NameFromPathSegments(segments)
		if err != nil {
			return nil, err
		}
		candidate.Name = name
		candidates[index] = cloneCandidate(candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name
	})
	return candidates, nil
}

func detachedEntry(
	candidate factorydefinitions.EffectiveFactoryCatalogCandidate,
	definition *factorydefinitions.FactoryConfig,
) (factorydefinitions.EffectiveFactoryCatalogEntry, error) {
	cloned, err := factorycontracts.CloneFactoryConfig(definition)
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
