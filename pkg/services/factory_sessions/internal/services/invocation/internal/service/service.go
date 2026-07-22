// Package service implements the owner-private Factory Session invocation
// capability.
package service

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	invocationservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation"
	legacyinvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/invocation"
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
	return &Service{owner: legacyinvocation.NewSessionOwner(
		deps.FactoryConfig,
		deps.SubmitWork,
		deps.Observe,
		deps.WaitNext,
		deps.Telemetry,
		deps.SpecialCase,
		deps.Interpolation,
		deps.WorkTypes,
		deps.InputFiles,
	)}, nil
}

// InvokeFactorySession delegates the complete invocation lifecycle to the
// capability-owned engine.
func (s *Service) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorydefinitions.FactoryInvocationResult, error) {
	if s == nil || s.owner == nil {
		return factorydefinitions.FactoryInvocationResult{}, fmt.Errorf("Factory Session invocation service is unavailable")
	}
	return s.owner.InvokeFactorySession(ctx, sessionID, request)
}

// ResolveInvocationInput applies the same normalization policy used by live
// invocations, including structured and compatibility inputs.
func (s *Service) ResolveInvocationInput(
	cfg *factorydefinitions.FactoryConfig,
	request factorysessions.InvocationRequest,
) (factorysessions.ResolvedInvocationInput, error) {
	if s == nil || s.owner == nil {
		return factorysessions.ResolvedInvocationInput{}, fmt.Errorf("Factory Session invocation service is unavailable")
	}
	return s.owner.ResolveInvocationInput(cfg, request)
}
