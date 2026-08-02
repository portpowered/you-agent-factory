package service

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

func (s *Service) bindRootCapabilities(
	invoker roles.SessionInvoker,
	activate func(context.Context, string) error,
	activationGateway factorydefinitions.DefinitionActivationGateway,
) {
	if s == nil {
		return
	}
	s.invoker = invoker
	s.activate = activate
	s.activationGateway = activationGateway
}

func (s *Service) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorysessions.InvocationResult, error) {
	if s == nil || s.invoker == nil {
		return factorysessions.InvocationResult{}, fmt.Errorf("Factory Session invocation service is required")
	}
	result, err := s.invoker.InvokeFactorySession(ctx, sessionID, request)
	if err != nil {
		return factorysessions.InvocationResult{}, err
	}
	return factorysessions.InvocationResult{
		RequestID: result.RequestID, TraceID: result.TraceID,
		Status: factorysessions.InvocationTerminalStatus(result.Status),
		PrimaryResult: result.PrimaryResult, ErrorCode: result.ErrorCode,
		Message: result.Message, SessionID: result.SessionID, WorkID: result.WorkID,
		WorkName: result.WorkName, WorkState: result.WorkState,
	}, nil
}

func (s *Service) ActivateNamedFactory(ctx context.Context, name string) error {
	if s == nil || s.activate == nil {
		return fmt.Errorf("Factory Session activation service is required")
	}
	return s.activate(ctx, name)
}

func (s *Service) DefinitionActivationGateway() factorydefinitions.DefinitionActivationGateway {
	if s == nil {
		return nil
	}
	return s.activationGateway
}
