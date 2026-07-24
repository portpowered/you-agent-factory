// Package identity defines the Factory Sessions-owned logical identity and
// target-resolution capability. Consumers outside Factory Sessions use the
// outer Factory Sessions service instead of this private subservice contract.
package identity

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
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
	Resolve(sessionregistry.Service, string) *livesession.LiveSession
	ResolveLogical(sessionregistry.Service, string, string) *livesession.LiveSession
}

// Dependencies are the exact host-effect ports required to construct identity.
// They contain no peer-service implementations, filesystem/SQL/OS concrete
// types, Runtime/Petri types, or Wire/root construction ownership.
type Dependencies struct {
	ResolveSymlinks factorysessions.LogicalTargetResolveSymlinks
	ResolveHome     factorysessions.HomeDirectoryResolver
	Directories     factorysessions.DirectoryInspection
}

// NormalizeRequest is the private alias for the CTR-SES root identity
// normalize request. Peers continue to import only the Factory Sessions root.
type NormalizeRequest = factorysessions.IdentityNormalizeRequest

// NormalizeProviderRequest is the private alias for the CTR-SES root provider
// identity normalize request.
type NormalizeProviderRequest = factorysessions.IdentityNormalizeProviderRequest

// ResolvedIdentity is the private alias for the CTR-SES root identity result.
type ResolvedIdentity = factorysessions.ResolvedIdentity

type DiscoverRequest struct {
	FolderPath        string
	WorkstationLoader factorydefinitions.WorkstationLoader
	LoadFactory       factorydefinitions.LoadedFactoryLoader
	Logger            *zap.Logger
}
