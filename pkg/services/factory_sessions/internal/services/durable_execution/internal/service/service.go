package service

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Service keeps the durable execution implementation private while preserving
// the established execution contract during the package migration.
type Service struct {
	durableexecution.Service
}

// liveChangeCapability is the optional durable-session boundary used by the
// live-change coordinator without widening the root execution contract.
type liveChangeCapability interface {
	ApplyLiveChange(context.Context, string, factorysessions.LiveChangeRequest) (factorysessions.LiveChangeResult, error)
	RecoverLiveChange(context.Context, string, string) (factorysessions.LiveChangeResult, error)
}

type liveChangeForward func(liveChangeCapability) (factorysessions.LiveChangeResult, error)

func (s *Service) forwardLiveChange(forward liveChangeForward) (factorysessions.LiveChangeResult, error) {
	if s == nil || s.Service == nil {
		return factorysessions.LiveChangeResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	capability, ok := s.Service.(liveChangeCapability)
	if !ok {
		return factorysessions.LiveChangeResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	return forward(capability)
}

// BindWorkerInvoker forwards the session's Factory Runtime to the underlying
// JavaScript runtime, which invokes its workflow children as Workers through
// it. An execution backend that runs no Workers of its own -- the fake and
// replay backends -- does not implement the binder, and skipping it is correct
// rather than a missing wire.
func (s *Service) BindWorkerInvoker(resolve func(sessionID string) factoryruntime.Service) {
	if s == nil || s.Service == nil {
		return
	}
	binder, ok := s.Service.(interface {
		BindWorkerInvoker(func(sessionID string) factoryruntime.Service)
	})
	if !ok {
		return
	}
	binder.BindWorkerInvoker(resolve)
}

// SubscribeResponseEvents forwards durable-session response-event subscriptions
// to the underlying JavaScript runtime when that capability is present.
func (s *Service) SubscribeResponseEvents(
	ctx context.Context,
	sessionID string,
	request factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	if s == nil || s.Service == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	subscriber, ok := s.Service.(interface {
		SubscribeResponseEvents(context.Context, string, factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error)
	})
	if !ok {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	return subscriber.SubscribeResponseEvents(ctx, sessionID, request)
}

// ApplyLiveChange forwards the optional durable-session live-change capability
// without widening the durable execution root interface used by other
// backends.
func (s *Service) ApplyLiveChange(
	ctx context.Context,
	sessionID string,
	request factorysessions.LiveChangeRequest,
) (factorysessions.LiveChangeResult, error) {
	return s.forwardLiveChange(func(capability liveChangeCapability) (factorysessions.LiveChangeResult, error) {
		return capability.ApplyLiveChange(ctx, sessionID, request)
	})
}

// RecoverLiveChange forwards durable live-change recovery when the underlying
// execution backend retains canonical change events.
func (s *Service) RecoverLiveChange(
	ctx context.Context,
	sessionID string,
	requestID string,
) (factorysessions.LiveChangeResult, error) {
	return s.forwardLiveChange(func(capability liveChangeCapability) (factorysessions.LiveChangeResult, error) {
		return capability.RecoverLiveChange(ctx, sessionID, requestID)
	})
}

// New constructs an inert durable execution capability around an explicitly
// injected implementation.
func New(execution durableexecution.Service) (*Service, error) {
	if execution == nil {
		return nil, fmt.Errorf("construct durable Factory Sessions execution: execution service is required")
	}
	return &Service{Service: execution}, nil
}

// IsNonLiveReplay preserves the optional replay routing capability without
// exposing the underlying recording-replay implementation.
func (s *Service) IsNonLiveReplay() bool {
	if s == nil || s.Service == nil {
		return false
	}
	replay, ok := s.Service.(interface{ IsNonLiveReplay() bool })
	return ok && replay.IsNonLiveReplay()
}

// RecordPetriTokenMutations preserves the runtime event bridge used by the
// Petri orchestrator while the execution engine remains behind this service.
func (s *Service) RecordPetriTokenMutations(
	sessionID string,
	records []factorydefinitions.TokenMutationRecord,
) error {
	if s == nil || s.Service == nil {
		return factorysessions.ErrExecutionServiceNotConfigured
	}
	recorder, ok := s.Service.(interface {
		RecordPetriTokenMutations(string, []factorydefinitions.TokenMutationRecord) error
	})
	if !ok {
		return fmt.Errorf("durable execution does not record Petri mutations")
	}
	return recorder.RecordPetriTokenMutations(sessionID, records)
}

var _ durableexecution.Service = (*Service)(nil)

// PublishWorkerProgress forwards one Worker's progress to the underlying
// JavaScript runtime, which routes it to the durable session that started that
// Worker. A backend whose children are not Workers does not implement it, and
// its sessions have no such output to record.
func (s *Service) PublishWorkerProgress(fragment workers.ProgressFragment) {
	if s == nil || s.Service == nil {
		return
	}
	observer, ok := s.Service.(interface {
		PublishWorkerProgress(workers.ProgressFragment)
	})
	if !ok {
		return
	}
	observer.PublishWorkerProgress(fragment)
}
