// Package service implements the owner-private Factory Session invocation
// capability under the FND-02 nested subservice internal path.
package service

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	legacyinvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	invocationservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Service is the sole private implementation of prepare, command-Work, and
// observe-completion. Keeping the legacy engine behind this boundary lets the
// package migration preserve established validation, wait, and telemetry
// behavior while outer Sessions composition depends only on the named
// invocation subservice contract.
type Service struct {
	owner *legacyinvocation.SessionOwner
}

var _ invocationservice.Service = (*Service)(nil)

// New constructs an inert invocation capability from explicit dependencies.
func New(deps invocationservice.Dependencies) (*Service, error) {
	if deps.FactoryConfig == nil {
		return nil, fmt.Errorf("construct Factory Session invocation: Factory config reader is required")
	}
	if deps.Work == nil {
		return nil, fmt.Errorf("construct Factory Session invocation: Work peer root is required")
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
		commandWorkThroughPeerRoot(deps.Work),
		deps.Observe,
		deps.WaitNext,
		deps.Telemetry,
		deps.SpecialCase,
		deps.Interpolation,
		deps.WorkTypes,
		deps.InputFiles,
	)}, nil
}

// commandWorkThroughPeerRoot projects one prepared SubmitRequest into the
// CTR-WORK admission vocabulary and issues exactly one peer-root command.
// Admission, staging, and materialization stay on the Work peer; invocation
// only consumes SubmitWorkRequestForSession.
func commandWorkThroughPeerRoot(
	peer invocationservice.WorkAdmission,
) func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
	return func(ctx context.Context, sessionID string, request work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
		return peer.SubmitWorkRequestForSession(
			ctx,
			sessionID,
			work.WorkRequestFromSubmitRequests([]work.SubmitRequest{request}),
		)
	}
}

// InvokeFactorySession delegates the complete invocation lifecycle to the
// capability-owned engine and stamps the bound session identity onto observe
// outcomes so completed and non-completed results stay session-scoped.
func (s *Service) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorydefinitions.FactoryInvocationResult, error) {
	if s == nil || s.owner == nil {
		return factorydefinitions.FactoryInvocationResult{}, fmt.Errorf("Factory Session invocation service is unavailable")
	}
	result, err := s.owner.InvokeFactorySession(ctx, sessionID, request)
	if err != nil {
		return result, err
	}
	return withSessionScope(sessionID, result), nil
}

// withSessionScope records the Factory Session ID used for prepare/command/
// observe when the engine outcome omitted it. Callers distinguish completed
// success from typed non-completed outcomes (including partial-capture
// failures) through Status/ErrorCode/Work context rather than SessionID alone.
func withSessionScope(sessionID string, result factorydefinitions.FactoryInvocationResult) factorydefinitions.FactoryInvocationResult {
	if result.SessionID == "" && sessionID != "" {
		result.SessionID = sessionID
	}
	return result
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
