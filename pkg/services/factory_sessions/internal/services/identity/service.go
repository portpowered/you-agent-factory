// Package identity defines the Factory Sessions-owned logical identity and
// target-resolution capability. Consumers outside Factory Sessions use the
// outer Factory Sessions service instead of this private subservice contract.
package identity

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	"go.uber.org/zap"
)

// Service normalizes logical targets, discovers runnable targets, and resolves
// live sessions without selecting host filesystem effects implicitly.
type Service interface {
	Normalize(context.Context, NormalizeRequest) (ResolvedIdentity, error)
	NormalizeProvider(context.Context, NormalizeProviderRequest) (ResolvedIdentity, error)
	Discover(context.Context, DiscoverRequest) ([]factorysessions.Target, error)
	ResolveFolder(string) (string, error)
	Select([]factorysessions.Target, *factorysessions.TargetRef) (*factorysessions.Target, error)
	Resolve(sessionregistry.Service, string) *factorysessions.LiveSession
	ResolveLogical(sessionregistry.Service, string, string) *factorysessions.LiveSession
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

type DiscoverRequest struct {
	FolderPath        string
	WorkstationLoader factorydefinitions.WorkstationLoader
	LoadFactory       factorydefinitions.LoadedFactoryLoader
	Logger            *zap.Logger
}
