package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

// authorizeProviderTarget resolves provider aliases and validates prerequisites
// through providers.Service before one agent attempt. Script and inference
// runners do not require Providers catalog authority.
func (s *Service) authorizeProviderTarget(
	ctx context.Context,
	request *workers.ExecuteRequest,
	identity string,
) error {
	if identity != runners.AgentIdentity {
		return nil
	}
	if s == nil || s.providers == nil {
		return fmt.Errorf(
			"%w: Providers service is required for agent execution",
			workers.ErrExecuteUnavailable,
		)
	}
	raw := firstNonEmpty(
		request.Target.Provider.ID,
		request.Target.Provider.Alias,
		request.Target.RunnerID,
	)
	if resume := request.Input.Resume; resume != nil {
		if provider := strings.TrimSpace(resume.Provider); provider != "" {
			raw = firstNonEmpty(raw, provider)
		}
	}
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf(
			"%w: provider identity is required for agent execution",
			workers.ErrInvalidExecuteRequest,
		)
	}
	resolved, err := providers.ResolveIdentity(
		ctx,
		s.providers,
		providers.ResolveIdentityRequest{Identity: raw},
	)
	if err != nil {
		return fmt.Errorf("%w: %w", workers.ErrInvalidExecuteRequest, err)
	}
	if err := providers.ValidatePrerequisites(
		ctx,
		s.providers,
		providers.ValidatePrerequisitesRequest{ID: resolved.ID},
	); err != nil {
		return fmt.Errorf("%w: %w", workers.ErrInvalidExecuteRequest, err)
	}
	request.Target.Provider.ID = resolved.ID.String()
	request.Target.Provider.Alias = ""
	if resume := request.Input.Resume; resume != nil {
		resume.Provider = resolved.ID.String()
	}
	if runnerID := strings.TrimSpace(request.Target.RunnerID); runnerID == "" ||
		strings.EqualFold(runnerID, runners.AgentIdentity) {
		request.Target.RunnerID = runnerIDForProvider(resolved.ID)
	}
	return nil
}

func runnerIDForProvider(id providers.ID) string {
	switch id {
	case providers.IDAntigravity:
		return workers.RunnerIDAntigravity
	default:
		return id.String()
	}
}
