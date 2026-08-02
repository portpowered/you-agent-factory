package providers

import (
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
