package service

import (
	"context"
	"fmt"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
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

func (s *Service) forwardLiveChange(
	ctx context.Context,
	sessionID string,
	request factorysessions.LiveChangeRequest,
	recoverRequestID string,
) (factorysessions.LiveChangeResult, error) {
	var capability liveChangeCapability
	if s != nil {
		capability, _ = s.Service.(liveChangeCapability)
	}
	if capability == nil {
		return factorysessions.LiveChangeResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	if recoverRequestID != "" {
		return capability.RecoverLiveChange(ctx, sessionID, recoverRequestID)
	}
	return capability.ApplyLiveChange(ctx, sessionID, request)
}

// SetWorkerInvoker forwards the session's opaque Factory Runtime capability to
// the underlying JavaScript runtime, which invokes its workflow children as
// Workers through it. An execution backend that runs no Workers of its own --
// the fake and replay backends -- does not implement the setter, and skipping
// it is correct rather than a missing wire.
func (s *Service) SetWorkerInvoker(runtime factoryruntime.Service) {
	if s == nil || s.Service == nil {
		return
	}
	setter, ok := s.Service.(interface {
		SetWorkerInvoker(factoryruntime.Service)
	})
	if !ok {
		return
	}
	setter.SetWorkerInvoker(runtime)
}

// SetWorkerExecution forwards the narrow Workers Execute capability and the
// Runtime-owned resource lease admission to the JavaScript child projection.
// The underlying durable service may be fake or replay-backed; those services
// do not implement this live-only capability and correctly ignore the bind.
func (s *Service) SetWorkerExecution(
	execution interface {
		Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
	},
	admission factoryruntime.ResourceCapacityLeaseAdmission,
	runtimeID string,
	generationID string,
	providerOverride providers.Service,
	mockWorkers *workers.MockWorkersConfig,
	commandRunnerOverride platformprocess.CommandRunner,
) {
	if s == nil || s.Service == nil {
		return
	}
	setter, ok := s.Service.(interface {
		SetWorkerExecution(
			interface {
				Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
			},
			factoryruntime.ResourceCapacityLeaseAdmission,
			string,
			string,
			providers.Service,
			*workers.MockWorkersConfig,
			platformprocess.CommandRunner,
		)
	})
	if !ok {
		return
	}
	setter.SetWorkerExecution(execution, admission, runtimeID, generationID, providerOverride, mockWorkers, commandRunnerOverride)
}

// SetWorkerProgressPublisher forwards the runtime-owned observation bridge to
// the live JavaScript execution implementation when it supports the optional
// child publication capability.
func (s *Service) SetWorkerProgressPublisher(publisher workers.ProgressPublisher) {
	if s == nil || s.Service == nil {
		return
	}
	setter, ok := s.Service.(interface {
		SetWorkerProgressPublisher(workers.ProgressPublisher)
	})
	if !ok {
		return
	}
	setter.SetWorkerProgressPublisher(publisher)
}

// SetWorkerAttemptStarter forwards the Runtime-owned Worker Session opening
// boundary to the live JavaScript execution implementation when it supports
// the optional direct Execute lifecycle capability.
func (s *Service) SetWorkerAttemptStarter(
	starter func(context.Context, workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) error, error),
) {
	if s == nil || s.Service == nil {
		return
	}
	setter, ok := s.Service.(interface {
		SetWorkerAttemptStarter(
			func(context.Context, workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) error, error),
		)
	})
	if !ok {
		return
	}
	setter.SetWorkerAttemptStarter(starter)
}

// SetDispatchDurability forwards the Recordings completed-flush capability to
// the live execution implementation. Fake and replay backends do not need
// this live-only binding and simply ignore it.
func (s *Service) SetDispatchDurability(
	reader recordings.CompletedFlushWatermarkReader,
	streamGenerationID string,
) {
	if s == nil || s.Service == nil {
		return
	}
	setter, ok := s.Service.(interface {
		SetDispatchDurability(recordings.CompletedFlushWatermarkReader, string)
	})
	if !ok {
		return
	}
	setter.SetDispatchDurability(reader, streamGenerationID)
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
	return s.forwardLiveChange(ctx, sessionID, request, "")
}

// RecoverLiveChange forwards durable live-change recovery when the underlying
// execution backend retains canonical change events.
func (s *Service) RecoverLiveChange(
	ctx context.Context,
	sessionID string,
	requestID string,
) (factorysessions.LiveChangeResult, error) {
	return s.forwardLiveChange(ctx, sessionID, factorysessions.LiveChangeRequest{}, requestID)
}

// New constructs an inert durable execution capability around an explicitly
// injected implementation.
func New(execution durableexecution.Service) (*Service, error) {
	if execution == nil {
		return nil, fmt.Errorf("construct durable Factory Sessions execution: execution service is required")
	}
	return &Service{Service: execution}, nil
}

// Close forwards the optional execution-owner shutdown boundary. The public
// durable execution contract intentionally stays focused on customer
// operations; only implementations that own asynchronous work need to expose
// this private lifecycle capability.
func (s *Service) Close() error {
	if s == nil || s.Service == nil {
		return nil
	}
	closer, ok := s.Service.(interface{ Close() error })
	if !ok {
		return nil
	}
	return closer.Close()
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

// HasRestorableState forwards the optional live-resume eligibility probe
// without widening the durable execution root contract used by other
// backends.
func (s *Service) HasRestorableState(ctx context.Context, sessionID string) (bool, error) {
	if s == nil || s.Service == nil {
		return false, nil
	}
	probe, ok := s.Service.(interface {
		HasRestorableState(context.Context, string) (bool, error)
	})
	if !ok {
		return false, nil
	}
	return probe.HasRestorableState(ctx, sessionID)
}

// HasDurableState forwards the read-only persistence probe used by runtime
// opening to distinguish a first session from a fresh process with prior
// durable state. Unlike HasRestorableState, it does not require an interrupted
// lifecycle or a resume checkpoint.
func (s *Service) HasDurableState(ctx context.Context, sessionID string) (bool, error) {
	if s == nil || s.Service == nil {
		return false, fmt.Errorf("durable execution is unavailable")
	}
	probe, ok := s.Service.(interface {
		HasDurableState(context.Context, string) (bool, error)
	})
	if !ok {
		return false, fmt.Errorf("durable execution does not expose a persistence-backed state probe")
	}
	return probe.HasDurableState(ctx, sessionID)
}

// Resume preserves the ordinary lifecycle-control path for running and paused
// sessions. Interrupted sessions use the same read-only eligibility probe as
// restart-resume before the underlying control operation can mutate state.
func (s *Service) Resume(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	if s == nil || s.Service == nil {
		return factorysessions.LifecycleControlResult{}, factorysessions.ErrExecutionServiceNotConfigured
	}
	if _, ok := s.Service.(interface {
		HasRestorableState(context.Context, string) (bool, error)
	}); !ok {
		return s.Service.Resume(ctx, sessionID, request)
	}
	read, err := s.Service.GetSession(ctx, sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	if read.Status == factorysessions.LifecycleStatusInterrupted {
		available, err := s.HasRestorableState(ctx, sessionID)
		if err != nil {
			return factorysessions.LifecycleControlResult{}, err
		}
		if !available {
			return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
		}
	}
	return s.Service.Resume(ctx, sessionID, request)
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
