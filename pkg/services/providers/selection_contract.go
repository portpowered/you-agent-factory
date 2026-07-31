package providers

import (
	"context"
	"fmt"
	"strings"
)

// SelectionSource identifies which configured provider value won selection
// precedence. Providers owns the resolution authority; callers own which raw
// values they supply.
type SelectionSource string

const (
	SelectionSourceWorkstation    SelectionSource = "workstation"
	SelectionSourceFactory        SelectionSource = "factory"
	SelectionSourceLegacyProvider SelectionSource = "legacy_provider"
	SelectionSourceDefault        SelectionSource = "default"
)

// ResolveIdentityRequest asks Providers to canonicalize one provider ID or
// alias without starting execution.
type ResolveIdentityRequest struct {
	Identity string
}

// Validate checks request fields whose validity does not depend on catalog
// state.
func (request ResolveIdentityRequest) Validate() error {
	if strings.TrimSpace(request.Identity) == "" {
		return fmt.Errorf("%w: empty provider id", ErrInvalidID)
	}
	return nil
}

// ResolveIdentityResult is the detached canonical provider identity.
type ResolveIdentityResult struct {
	ID ID
}

// ResolveSelectionRequest asks Providers to apply workstation/factory/legacy
// provider precedence and return one canonical provider identity.
type ResolveSelectionRequest struct {
	Workstation   string
	Factory       string
	ModelProvider string
}

// ResolveSelectionResult is the detached selection outcome.
type ResolveSelectionResult struct {
	Provider ID
	Source   SelectionSource
}

// ValidatePrerequisitesRequest asks Providers whether one canonical provider is
// currently selectable. Callers must resolve aliases through ResolveIdentity
// first when they hold a non-canonical identity.
type ValidatePrerequisitesRequest struct {
	ID ID
}

// Validate checks request fields whose validity does not depend on catalog
// state.
func (request ValidatePrerequisitesRequest) Validate() error {
	if err := request.ID.Validate(); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// ResolveIdentity canonicalizes a Providers-owned ID or accepted alias through
// the singular Service catalog. Compatibility aliases such as openai/anthropic
// and the cursor-cli runner synonym remain Providers-owned.
func ResolveIdentity(
	ctx context.Context,
	service Service,
	request ResolveIdentityRequest,
) (ResolveIdentityResult, error) {
	if service == nil {
		return ResolveIdentityResult{}, fmt.Errorf("%w: Providers service is required", ErrInvalidID)
	}
	if err := ctx.Err(); err != nil {
		return ResolveIdentityResult{}, err
	}
	if err := request.Validate(); err != nil {
		return ResolveIdentityResult{}, err
	}
	aliases, err := catalogAliasIndex(ctx, service)
	if err != nil {
		return ResolveIdentityResult{}, err
	}
	normalized := strings.ToLower(strings.TrimSpace(request.Identity))
	canonical, ok := aliases[normalized]
	if !ok {
		return ResolveIdentityResult{}, fmt.Errorf("%w: %q", ErrUnknownProvider, request.Identity)
	}
	return ResolveIdentityResult{ID: canonical}, nil
}

// ResolveSelection applies workstation, factory, then legacy model-provider
// precedence and returns one canonical Providers identity. Empty candidates are
// skipped. When every candidate is empty, the default selectable provider is
// Codex when present in the catalog.
func ResolveSelection(
	ctx context.Context,
	service Service,
	request ResolveSelectionRequest,
) (ResolveSelectionResult, error) {
	if service == nil {
		return ResolveSelectionResult{}, fmt.Errorf("%w: Providers service is required", ErrInvalidID)
	}
	if err := ctx.Err(); err != nil {
		return ResolveSelectionResult{}, err
	}
	candidates := []struct {
		identity string
		source   SelectionSource
	}{
		{request.Workstation, SelectionSourceWorkstation},
		{request.Factory, SelectionSourceFactory},
		{request.ModelProvider, SelectionSourceLegacyProvider},
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.identity) == "" {
			continue
		}
		if candidate.source == SelectionSourceLegacyProvider &&
			isUnresolvedProviderTemplate(candidate.identity) {
			continue
		}
		resolved, err := ResolveIdentity(ctx, service, ResolveIdentityRequest{
			Identity: candidate.identity,
		})
		if err != nil {
			return ResolveSelectionResult{}, err
		}
		return ResolveSelectionResult{Provider: resolved.ID, Source: candidate.source}, nil
	}
	resolved, err := ResolveIdentity(ctx, service, ResolveIdentityRequest{Identity: IDCodex.String()})
	if err != nil {
		return ResolveSelectionResult{}, fmt.Errorf("resolve default provider: %w", err)
	}
	return ResolveSelectionResult{
		Provider: resolved.ID,
		Source:   SelectionSourceDefault,
	}, nil
}

// ValidatePrerequisites reports whether a canonical provider is currently
// selectable. Unknown identities fail with ErrUnknownProvider; blocked
// availability or prerequisites fail with ErrProviderUnavailable.
func ValidatePrerequisites(
	ctx context.Context,
	service Service,
	request ValidatePrerequisitesRequest,
) error {
	if service == nil {
		return fmt.Errorf("%w: Providers service is required", ErrInvalidID)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	_, err := service.GetProvider(ctx, GetProviderRequest{ID: request.ID})
	return err
}

func catalogAliasIndex(
	ctx context.Context,
	service Service,
) (map[string]ID, error) {
	listed, err := service.ListProviders(ctx, ListProvidersRequest{})
	if err != nil {
		return nil, err
	}
	aliases := make(map[string]ID, len(listed.Providers)*2)
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

func compatibilityAliases() map[string]ID {
	return map[string]ID{
		"openai":     IDCodex,
		"anthropic":  IDClaude,
		"cursor-cli": IDCursor,
		"agent":      IDCursor,
	}
}

func isUnresolvedProviderTemplate(identity string) bool {
	trimmed := strings.TrimSpace(identity)
	return strings.HasPrefix(trimmed, "${") && strings.HasSuffix(trimmed, "}")
}
