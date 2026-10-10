package service

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

// ValidateExecution leaves workspace, process, and observation effects to Execute.
func (s *Service) ValidateExecution(ctx context.Context, request workers.ExecuteRequest) error {
	if s == nil {
		return workers.ErrExecuteUnavailable
	}
	if ctx == nil {
		return fmt.Errorf("%w: context is required", workers.ErrInvalidExecuteRequest)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	request = request.Clone()
	if err := request.Validate(); err != nil {
		return err
	}
	if request.Target.Noop {
		return nil
	}
	identity := resolveRunnerIdentity(request.Target)
	request.Target.Tools.RequiredOptionalCapabilities = requiredOptionalCapabilities(request, identity)
	if err := s.authorizeProviderTarget(ctx, &request, identity); err != nil {
		return err
	}
	if identity == runners.AgentIdentity && !providerOverrideApplies(&request, configuredProviderOverride(s)) {
		if err := s.providers.ValidateExecution(ctx, providers.ExecuteRequest{
			Provider: providers.ID(request.Target.Provider.ID), AttemptID: request.Correlation.AttemptID,
			Model: request.Target.Model.Name, ReasoningEffort: request.Target.Model.ReasoningEffort,
			SkipPermissions: request.Target.Permissions.SkipPermissions,
		}); err != nil {
			return fmt.Errorf("%w: %w", workers.ErrInvalidExecuteRequest, err)
		}
	}
	if _, err := s.runners.Resolve(runners.ResolutionRequest{
		Identity:             identity,
		RequiredCapabilities: runnerResolutionCapabilities(request.Target.Tools.RequiredOptionalCapabilities, identity),
	}); err != nil {
		return fmt.Errorf("%w: resolve runner %q: %w", workers.ErrInvalidExecuteRequest, identity, err)
	}
	return ctx.Err()
}
