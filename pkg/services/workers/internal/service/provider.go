package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/skippermissions"
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
	resolved, err := s.providers.ResolveIdentity(
		ctx,
		providers.ResolveIdentityRequest{Identity: raw},
	)
	if err != nil {
		return fmt.Errorf("%w: %w", workers.ErrInvalidExecuteRequest, err)
	}
	if !request.Input.SkipBuiltInPrerequisiteValidation {
		if err := s.providers.ValidatePrerequisites(
			ctx,
			providers.ValidatePrerequisitesRequest{ID: resolved.ID},
		); err != nil {
			return fmt.Errorf("%w: %w", workers.ErrInvalidExecuteRequest, err)
		}
	}
	request.Target.Permissions.SkipPermissions = skippermissions.EffectiveSkipPermissions(
		request.Target.Permissions.SkipPermissions,
		request.Target.WorkerType,
		request.Input.InvocationSkipPermissionsOverride,
	)
	// Antigravity's catalog intentionally publishes only its stable baseline
	// capabilities. Its command adapter still accepts authored media and
	// structured-output flags, so those legacy request capabilities must not be
	// rejected before the adapter gets a chance to handle them.
	if resolved.ID != providers.IDAntigravity {
		if err := validateProviderCapabilities(ctx, s.providers, resolved.ID, request.Target.Tools.RequiredOptionalCapabilities); err != nil {
			return err
		}
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

func validateProviderCapabilities(
	ctx context.Context,
	service providers.Service,
	id providers.ID,
	required []workers.RunnerOptionalCapability,
) error {
	providerCapabilities := make([]providers.Capability, 0, len(required))
	for _, capability := range required {
		providerCapability, ok := providerCapabilityForRunnerCapability(capability)
		if !ok {
			continue
		}
		providerCapabilities = append(providerCapabilities, providerCapability)
	}
	if len(providerCapabilities) == 0 {
		return nil
	}
	descriptor, err := service.GetProvider(ctx, providers.GetProviderRequest{ID: id})
	if err != nil {
		return fmt.Errorf("%w: get provider capabilities: %v", workers.ErrInvalidExecuteRequest, err)
	}
	for _, requiredCapability := range providerCapabilities {
		if providerDescriptorHasCapability(descriptor.Provider, requiredCapability) {
			continue
		}
		return workers.NewProviderError(
			workers.WorkFailureTypePermanentBadRequest,
			fmt.Sprintf("provider %q does not support capability %q", id, requiredCapability),
			providers.ErrCapabilityMismatch,
		)
	}
	return nil
}

func providerCapabilityForRunnerCapability(
	capability workers.RunnerOptionalCapability,
) (providers.Capability, bool) {
	switch capability {
	case workers.RunnerOptionalCapabilityImageInput:
		return providers.CapabilityImageInput, true
	case workers.RunnerOptionalCapabilitySessionResume:
		return providers.CapabilitySessionResume, true
	case workers.RunnerOptionalCapabilityStructuredOutput:
		return providers.CapabilityStructuredOutput, true
	default:
		return "", false
	}
}

func providerDescriptorHasCapability(
	descriptor providers.Descriptor,
	required providers.Capability,
) bool {
	for _, capability := range descriptor.Capabilities {
		if capability == required {
			return true
		}
	}
	return false
}

func runnerIDForProvider(id providers.ID) string {
	switch id {
	case providers.IDAntigravity:
		return workers.RunnerIDAntigravity
	default:
		return id.String()
	}
}
