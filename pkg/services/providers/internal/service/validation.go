package service

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

// ValidateExecution observes current readiness without binding a live attempt.
func (s *Service) ValidateExecution(ctx context.Context, request providers.ExecuteRequest) error {
	if ctx == nil {
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindInvalidRequest, Message: "context is required"}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	normalized, err := s.normalizeExecutionPolicy(request)
	if err != nil {
		return err
	}
	return s.ValidatePrerequisites(ctx, providers.ValidatePrerequisitesRequest{ID: normalized.Provider})
}

// normalizeExecutionPolicy is shared by preflight and execution so policy
// cannot drift between admission and the actual provider attempt.
func (s *Service) normalizeExecutionPolicy(request providers.ExecuteRequest) (providers.ExecuteRequest, error) {
	if err := request.Validate(); err != nil {
		return request, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindInvalidRequest, Message: err.Error()}
	}
	request.ReasoningEffort, _ = providers.ReasoningEffort(request.ReasoningEffort).Canonical()
	if canonical, ok := s.acp.Resolve(request.Provider); ok {
		if request.ReasoningEffort != "" {
			return request, providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindInvalidRequest,
				Message: fmt.Sprintf("ACP provider %q selects reasoning effort through its exact advertised model id; omit reasoningEffort and choose the intended model", canonical),
			}
		}
		request.Provider = canonical
		request.SkipPermissions = request.SkipPermissions && s.acpSupportsPermissionBypass(canonical)
	} else {
		canonical, err := s.catalog.ResolveProviderID(request.Provider)
		if err != nil {
			return request, err
		}
		request.Provider = canonical
		if canonical == providers.IDClaude && request.ReasoningEffort == "minimal" {
			return request, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindInvalidRequest, Message: `Claude does not support reasoning effort "minimal"`}
		}
		if canonical == providers.IDAntigravity && request.ReasoningEffort != "" {
			return request, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindInvalidRequest, Message: "Agy does not support a separate reasoning effort"}
		}
	}
	return request, s.validatePermissionBypass(request.Provider, request.SkipPermissions)
}
