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

// StartSyncWithEventConsumer forwards the private presentation capability to
// a live JavaScript execution when one is available. The public durable
// StartRequest remains value-only; callers that do not need presentation use
// the ordinary StartSync method above.
func (s *Service) StartSyncWithEventConsumer(
	ctx context.Context,
	request factorysessions.StartRequest,
	consume factorysessions.FactoryEventConsumer,
) (factorysessions.SyncStartResult, error) {
	if s == nil || s.Service == nil {
		return factorysessions.SyncStartResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	observed, ok := s.Service.(interface {
		StartSyncWithEventConsumer(
			context.Context,
			factorysessions.StartRequest,
			factorysessions.FactoryEventConsumer,
		) (factorysessions.SyncStartResult, error)
	})
	if !ok {
		return s.StartSync(ctx, request)
	}
	return observed.StartSyncWithEventConsumer(ctx, request, consume)
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
