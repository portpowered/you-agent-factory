// Package service implements the owner-private Factory Session invocation
// capability.
package service

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	legacyinvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	invocationservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation"
)

// Service is the private capability implementation. Keeping the legacy engine
// behind this boundary lets the package migration preserve its established
// validation, wait, and telemetry behavior while consumers depend only on the
// capability contract.
type Service struct {
	owner *legacyinvocation.SessionOwner
}

var _ invocationservice.Service = (*Service)(nil)

// New constructs an inert invocation capability from explicit dependencies.
func New(deps invocationservice.Dependencies) (*Service, error) {
	if deps.FactoryConfig == nil {
		return nil, fmt.Errorf("construct Factory Session invocation: Factory config reader is required")
	}
	if deps.SubmitWork == nil {
		return nil, fmt.Errorf("construct Factory Session invocation: Work submitter is required")
	}
	if deps.Observe == nil {
		return nil, fmt.Errorf("construct Factory Session invocation: result observer is required")
	}
	if deps.Interpolation == nil {
		return nil, fmt.Errorf("construct Factory Session invocation: interpolation service is required")
	}
	if deps.WorkTypes == nil {
		return nil, fmt.Errorf("construct Factory Session invocation: Work Type service is required")
	}
	if deps.InputFiles == nil {
		return nil, fmt.Errorf("construct Factory Session invocation: input file reader is required")
	}
	if deps.Work == nil {
		return nil, fmt.Errorf("construct Factory Session invocation: Work service is required")
	}
	owner := legacyinvocation.NewSessionOwner(
		deps.FactoryConfig,
		deps.SubmitWork,
		deps.Observe,
		deps.WaitNext,
		deps.WaitSession,
		deps.Telemetry,
		deps.SpecialCase,
		deps.Interpolation,
		deps.WorkTypes,
		deps.InputFiles,
		deps.Work,
	)
	owner.BindCancelOnTimeout(deps.CancelOnTimeout)
	return &Service{owner: owner}, nil
}

// Invoke delegates the complete canonical invocation lifecycle to the
// capability-owned engine.
func (s *Service) Invoke(
	ctx context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorydefinitions.FactoryInvocationResult, error) {
	return s.owner.Invoke(ctx, sessionID, request)
}

// InvokeFactorySession preserves the compatibility-shaped capability while
// forwarding one-way into the canonical invocation owner.
func (s *Service) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorydefinitions.FactoryInvocationResult, error) {
	return s.Invoke(ctx, sessionID, request)
}

// ResolveInvocationInput applies the same normalization policy used by live
// invocations, including structured and compatibility inputs.
func (s *Service) ResolveInvocationInput(
	cfg *factorydefinitions.FactoryConfig,
	request factorysessions.InvocationRequest,
) (factorysessions.ResolvedInvocationInput, error) {
	return s.owner.ResolveInvocationInput(cfg, request)
}
