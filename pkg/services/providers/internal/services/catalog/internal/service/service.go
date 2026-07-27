package service

import (
	"context"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
)

type service struct {
	providers []providers.Descriptor
	byID      map[providers.ID]providers.Descriptor
	aliases   map[string]providers.ID
	probe     catalog.ProbeQuery
}

var _ catalog.Service = (*service)(nil)

// New constructs an inert catalog over the accepted standardized provider
// catalog publication.
func New(options ...Option) (catalog.Service, error) {
	descriptors, err := projectPublishedCatalog()
	if err != nil {
		return nil, err
	}
	byID, aliases := indexDescriptors(descriptors)
	s := &service{
		providers: descriptors,
		byID:      byID,
		aliases:   aliases,
	}
	for _, option := range options {
		option(s)
	}
	return s, nil
}

func indexDescriptors(descriptors []providers.Descriptor) (
	map[providers.ID]providers.Descriptor,
	map[string]providers.ID,
) {
	byID := make(map[providers.ID]providers.Descriptor, len(descriptors))
	aliases := make(map[string]providers.ID, len(descriptors))
	for _, descriptor := range descriptors {
		byID[descriptor.ID] = descriptor
		aliases[strings.ToLower(descriptor.ID.String())] = descriptor.ID
		for _, alias := range descriptor.Aliases {
			aliases[alias] = descriptor.ID
		}
	}
	return byID, aliases
}

func (s *service) ListProviders(
	ctx context.Context,
	_ providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	if err := ctx.Err(); err != nil {
		return providers.ListProvidersResult{}, err
	}
	results := make([]providers.Descriptor, len(s.providers))
	for i, descriptor := range s.providers {
		merged, err := s.applyProbe(ctx, descriptor)
		if err != nil {
			return providers.ListProvidersResult{}, err
		}
		results[i] = merged
	}
	return providers.ListProvidersResult{Providers: results}, nil
}

func (s *service) GetProvider(
	ctx context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if err := ctx.Err(); err != nil {
		return providers.GetProviderResult{}, err
	}
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	canonical, ok := s.resolveID(request.ID)
	if !ok {
		return providers.GetProviderResult{}, providers.ErrUnknownProvider
	}
	descriptor, ok := s.byID[canonical]
	if !ok {
		return providers.GetProviderResult{}, providers.ErrUnknownProvider
	}
	merged, err := s.applyProbe(ctx, descriptor)
	if err != nil {
		return providers.GetProviderResult{}, err
	}
	if !isProviderSelectable(merged) {
		return providers.GetProviderResult{}, providers.ErrProviderUnavailable
	}
	return providers.GetProviderResult{Provider: merged}, nil
}

func (s *service) ResolveProviderID(id providers.ID) (providers.ID, error) {
	if err := id.Validate(); err != nil {
		return "", err
	}
	canonical, ok := s.resolveID(id)
	if !ok {
		return "", providers.ErrUnknownProvider
	}
	return canonical, nil
}

func (s *service) applyProbe(
	ctx context.Context,
	descriptor providers.Descriptor,
) (providers.Descriptor, error) {
	if s.probe == nil {
		return descriptor.Clone(), nil
	}
	if err := ctx.Err(); err != nil {
		return providers.Descriptor{}, err
	}
	facts, err := s.probe(ctx, descriptor.Clone())
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return providers.Descriptor{}, ctxErr
		}
		facts = probeFailureFacts(descriptor)
	}
	return mergeProbeFacts(descriptor, facts), nil
}

func mergeProbeFacts(
	base providers.Descriptor,
	facts catalog.ProbeFacts,
) providers.Descriptor {
	merged := base.Clone()
	merged.Readiness = facts.Readiness
	if facts.Prerequisites != nil {
		merged.Prerequisites = clonePrerequisites(facts.Prerequisites)
	}
	return merged
}

func probeFailureFacts(descriptor providers.Descriptor) catalog.ProbeFacts {
	name := descriptor.DisplayName
	if strings.TrimSpace(name) == "" {
		name = descriptor.ID.String()
	}
	return catalog.ProbeFacts{
		Readiness: providers.ReadinessUnavailable,
		Prerequisites: []providers.Prerequisite{{
			Kind:        providers.PrerequisiteDependency,
			Name:        "readiness-probe",
			Status:      providers.PrerequisiteMissing,
			Description: name + " readiness probe failed.",
		}},
	}
}

func clonePrerequisites(prerequisites []providers.Prerequisite) []providers.Prerequisite {
	if prerequisites == nil {
		return nil
	}
	cloned := make([]providers.Prerequisite, len(prerequisites))
	copy(cloned, prerequisites)
	return cloned
}

func isProviderSelectable(descriptor providers.Descriptor) bool {
	if descriptor.Availability != providers.AvailabilitySelectable ||
		descriptor.Readiness != providers.ReadinessReady ||
		hasMissingPrerequisite(descriptor.Prerequisites) {
		return false
	}
	return true
}

func hasMissingPrerequisite(prerequisites []providers.Prerequisite) bool {
	for _, prerequisite := range prerequisites {
		if prerequisite.Status == providers.PrerequisiteMissing {
			return true
		}
	}
	return false
}

func (s *service) resolveID(id providers.ID) (providers.ID, bool) {
	normalized := strings.ToLower(strings.TrimSpace(id.String()))
	canonical, ok := s.aliases[normalized]
	return canonical, ok
}
