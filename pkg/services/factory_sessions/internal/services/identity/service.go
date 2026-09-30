// Package identity defines the Factory Sessions-owned logical identity and
// target-resolution capability. Consumers outside Factory Sessions use the
// outer Factory Sessions service instead of this private subservice contract.
package identity

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
)

// Service normalizes logical targets and resolves
// live sessions without selecting host filesystem effects implicitly.
type Service interface {
	Normalize(context.Context, NormalizeRequest) (ResolvedIdentity, error)
	NormalizeProvider(context.Context, NormalizeProviderRequest) (ResolvedIdentity, error)
	Resolve(sessionregistry.Service, string) *livesession.LiveSession
	ResolveLogical(sessionregistry.Service, string, string) *livesession.LiveSession
}

type NormalizeRequest struct {
	BackendScopeID string
	FolderPath     string
	Target         factorysessions.TargetRef
}

type NormalizeProviderRequest struct {
	BackendScopeID string
	FolderPath     string
	Boundary       factorysessions.LogicalTargetProviderBoundary
}

type ResolvedIdentity struct {
	Reference           factorysessions.CanonicalLogicalTargetReference
	LogicalSessionKeyID string
	RuntimeTarget       factorysessions.RuntimeLogicalTarget
}
