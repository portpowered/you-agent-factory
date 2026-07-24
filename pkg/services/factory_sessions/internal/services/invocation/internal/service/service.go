// Package service implements the owner-private Factory Session invocation
// capability under the FND-02 nested subservice internal path.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
// capability-owned engine, maps invalid-input failures onto the CTR-SES
// *InvocationValidationError vocabulary, and stamps the bound session identity
// onto observe outcomes (including timed-out and canceled terminals) so
// completed and typed non-completed results stay session-scoped.
func (s *Service) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorysessions.InvocationResult, error) {
	if s == nil || s.owner == nil {
		return factorysessions.InvocationResult{}, fmt.Errorf("Factory Session invocation service is unavailable")
	}
	result, err := s.owner.InvokeFactorySession(ctx, sessionID, request)
	if err != nil {
		return result, asInvocationValidationError(err)
	}
	return withSessionScope(sessionID, result), nil
}

// withSessionScope records the Factory Session ID used for prepare/command/
// observe when the engine outcome omitted it. Callers distinguish completed
// success from typed non-completed outcomes (timeout, cancel, partial-capture
// failure) through Status/ErrorCode/Work context rather than SessionID alone.
func withSessionScope(sessionID string, result factorysessions.InvocationResult) factorysessions.InvocationResult {
	if result.SessionID == "" && sessionID != "" {
		result.SessionID = sessionID
	}
	return result
}

// asInvocationValidationError projects legacy request-validation failures onto
// the CTR-SES published *InvocationValidationError type so invalid input stays
// distinct from TIMED_OUT / CANCELED terminal result codes.
func asInvocationValidationError(err error) error {
	var already *factorysessions.InvocationValidationError
	if errors.As(err, &already) {
		return err
	}
	var requestValidation *factorydefinitions.RequestValidationError
	if errors.As(err, &requestValidation) {
		return &factorysessions.InvocationValidationError{
			Field:   invocationValidationField(requestValidation.Message),
			Message: requestValidation.Message,
		}
	}
	var argumentErr *work.ArgumentError
	if errors.As(err, &argumentErr) {
		field := strings.TrimSpace(argumentErr.Argument)
		if field == "" {
			field = "args"
		}
		return &factorysessions.InvocationValidationError{
			Field:   field,
			Message: argumentErr.Message,
		}
	}
	return err
}

func invocationValidationField(message string) string {
	lower := strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.Contains(lower, "sourcekind"):
		return "sourceKind"
	case strings.Contains(lower, "content"):
		return "content"
	case strings.Contains(lower, "args"):
		return "args"
	default:
		return "request"
	}
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
	resolved, err := s.owner.ResolveInvocationInput(cfg, request)
	if err != nil {
		return factorysessions.ResolvedInvocationInput{}, asInvocationValidationError(err)
	}
	return resolved, nil
}
