package service

import (
	"context"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
)

type Service struct {
	resolveSymlinks factorysessions.LogicalTargetResolveSymlinks
	resolveHome     factorysessions.HomeDirectoryResolver
}

var _ identity.Service = (*Service)(nil)

func New(
	resolveSymlinks factorysessions.LogicalTargetResolveSymlinks,
	resolveHome factorysessions.HomeDirectoryResolver,
) *Service {
	if resolveSymlinks == nil || resolveHome == nil {
		return nil
	}
	return &Service{resolveSymlinks: resolveSymlinks, resolveHome: resolveHome}
}

func (s *Service) Normalize(_ context.Context, request identity.NormalizeRequest) (identity.ResolvedIdentity, error) {
	ref, err := logicaltarget.NormalizeTargetRefWithEffects(
		s.resolveSymlinks, s.resolveHome, request.BackendScopeID, request.FolderPath, request.Target,
	)
	if err != nil {
		return identity.ResolvedIdentity{}, err
	}
	return resolvedIdentity(ref), nil
}

func (s *Service) NormalizeProvider(_ context.Context, request identity.NormalizeProviderRequest) (identity.ResolvedIdentity, error) {
	ref, err := logicaltarget.NormalizeProviderTargetWithEffects(
		s.resolveSymlinks, s.resolveHome, request.BackendScopeID, request.FolderPath, request.Boundary,
	)
	if err != nil {
		return identity.ResolvedIdentity{}, err
	}
	return resolvedIdentity(ref), nil
}

func resolvedIdentity(ref factorysessions.CanonicalLogicalTargetReference) identity.ResolvedIdentity {
	return identity.ResolvedIdentity{
		Reference:           ref,
		LogicalSessionKeyID: logicaltarget.DeriveLogicalSessionKeyID(ref),
		RuntimeTarget:       logicaltarget.RuntimeLogicalTarget(ref),
	}
}

func (s *Service) Resolve(registry sessionregistry.Service, selector string) *livesession.LiveSession {
	if registry == nil {
		return nil
	}
	trimmed := strings.TrimSpace(selector)
	if session := registry.Get(trimmed); session != nil {
		return session
	}
	for _, id := range registry.IDs() {
		session := registry.Get(id)
		if session != nil && livesession.CanonicalID(session) == trimmed {
			return session
		}
	}
	return nil
}

func (s *Service) ResolveLogical(registry sessionregistry.Service, backendScopeID, logicalSessionKeyID string) *livesession.LiveSession {
	if registry == nil || strings.TrimSpace(logicalSessionKeyID) == "" {
		return nil
	}
	for _, id := range registry.IDs() {
		session := registry.Get(id)
		if session == nil {
			continue
		}
		resolved, err := s.Normalize(context.Background(), identity.NormalizeRequest{
			BackendScopeID: backendScopeID, FolderPath: session.FolderPath, Target: session.Target,
		})
		if err == nil && resolved.LogicalSessionKeyID == strings.TrimSpace(logicalSessionKeyID) {
			return session
		}
	}
	return nil
}
