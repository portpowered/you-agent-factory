package service

import (
	"context"
	"fmt"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// ResolveIdentity canonicalizes a Providers-owned ID or accepted alias through
// the root's catalog, including the compatibility aliases Providers owns.
func (s *Service) ResolveIdentity(
	ctx context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	if s == nil {
		return providers.ResolveIdentityResult{}, fmt.Errorf("%w: Providers service is required", providers.ErrInvalidID)
	}
	if err := ctx.Err(); err != nil {
		return providers.ResolveIdentityResult{}, err
	}
	if err := request.Validate(); err != nil {
		return providers.ResolveIdentityResult{}, err
	}
	aliases, err := s.catalogAliasIndex(ctx)
	if err != nil {
		return providers.ResolveIdentityResult{}, err
	}
	normalized := strings.ToLower(strings.TrimSpace(request.Identity))
	canonical, ok := aliases[normalized]
	if !ok {
		return providers.ResolveIdentityResult{}, fmt.Errorf("%w: %q", providers.ErrUnknownProvider, request.Identity)
	}
	return providers.ResolveIdentityResult{ID: canonical}, nil
}

// ResolveSelection applies workstation, factory, then legacy model-provider
// precedence and returns one canonical Providers identity. Empty candidates
// are skipped. When every candidate is empty, Codex is selected when present
// in the catalog.
func (s *Service) ResolveSelection(
	ctx context.Context,
	request providers.ResolveSelectionRequest,
) (providers.ResolveSelectionResult, error) {
	if s == nil {
		return providers.ResolveSelectionResult{}, fmt.Errorf("%w: Providers service is required", providers.ErrInvalidID)
	}
	if err := ctx.Err(); err != nil {
		return providers.ResolveSelectionResult{}, err
	}
	candidates := []struct {
		identity string
		source   providers.SelectionSource
	}{
		{request.Workstation, providers.SelectionSourceWorkstation},
		{request.Factory, providers.SelectionSourceFactory},
		{request.ModelProvider, providers.SelectionSourceLegacyProvider},
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.identity) == "" {
			continue
		}
		if candidate.source == providers.SelectionSourceLegacyProvider &&
			isUnresolvedProviderTemplate(candidate.identity) {
			continue
		}
		resolved, err := s.ResolveIdentity(ctx, providers.ResolveIdentityRequest{
			Identity: candidate.identity,
		})
		if err != nil {
			return providers.ResolveSelectionResult{}, err
		}
		return providers.ResolveSelectionResult{
			Provider: resolved.ID,
			Source:   candidate.source,
		}, nil
	}
	resolved, err := s.ResolveIdentity(ctx, providers.ResolveIdentityRequest{Identity: providers.IDCodex.String()})
	if err != nil {
		return providers.ResolveSelectionResult{}, fmt.Errorf("resolve default provider: %w", err)
	}
	return providers.ResolveSelectionResult{
		Provider: resolved.ID,
		Source:   providers.SelectionSourceDefault,
	}, nil
}

// ValidatePrerequisites reports whether a canonical provider is currently
// selectable. Unknown identities fail with ErrUnknownProvider; blocked
// availability or prerequisites fail with ErrProviderUnavailable.
func (s *Service) ValidatePrerequisites(
	ctx context.Context,
	request providers.ValidatePrerequisitesRequest,
) error {
	if s == nil {
		return fmt.Errorf("%w: Providers service is required", providers.ErrInvalidID)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	_, err := s.GetProvider(ctx, providers.GetProviderRequest{ID: request.ID})
	return err
}

func (s *Service) catalogAliasIndex(ctx context.Context) (map[string]providers.ID, error) {
	listed, err := s.ListProviders(ctx, providers.ListProvidersRequest{})
	if err != nil {
		return nil, err
	}
	aliases := make(map[string]providers.ID, len(listed.Providers)*2)
	for _, descriptor := range listed.Providers {
		canonical := descriptor.ID
		aliases[strings.ToLower(canonical.String())] = canonical
		for _, alias := range descriptor.Aliases {
			aliases[strings.ToLower(strings.TrimSpace(alias))] = canonical
		}
	}
	for alias, canonical := range compatibilityAliases() {
		if _, exists := aliases[strings.ToLower(canonical.String())]; !exists {
			continue
		}
		aliases[alias] = canonical
	}
	return aliases, nil
}

func compatibilityAliases() map[string]providers.ID {
	return map[string]providers.ID{
		"openai":    providers.IDCodex,
		"anthropic": providers.IDClaude,
	}
}

func isUnresolvedProviderTemplate(identity string) bool {
	trimmed := strings.TrimSpace(identity)
	return strings.HasPrefix(trimmed, "${") && strings.HasSuffix(trimmed, "}")
}
