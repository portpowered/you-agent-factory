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
	"go.uber.org/zap"
)

// Service keeps the durable execution implementation private while preserving
// the established execution contract during the package migration.
type Service struct {
	durableexecution.Service
}

var _ durableexecution.CanonicalService = (*Service)(nil)
var _ durableexecution.CanonicalReadService = (*Service)(nil)
var _ durableexecution.CanonicalControlService = (*Service)(nil)
var _ durableexecution.CanonicalResultService = (*Service)(nil)
var _ durableexecution.CanonicalDispatchService = (*Service)(nil)
var _ durableexecution.CanonicalResponseService = (*Service)(nil)

// StartCanonical selects the durable execution wait policy behind the private
// owner seam. The public Factory Sessions canonical operation supplies the
// request value and receives a value-only mode-specific projection.
func (s *Service) StartCanonical(
	ctx context.Context,
	request factorysessions.StartRequest,
	synchronous bool,
) (durableexecution.CanonicalStartResult, error) {
	if s == nil || s.Service == nil {
		return durableexecution.CanonicalStartResult{}, factorysessions.ErrExecutionServiceNotConfigured
	}
	if synchronous {
		started, err := s.Service.StartSync(ctx, request)
		if err != nil {
			return durableexecution.CanonicalStartResult{}, err
		}
		return durableexecution.CanonicalStartResult{Sync: &started}, nil
	}
	started, err := s.Service.StartAsync(ctx, request)
	if err != nil {
		return durableexecution.CanonicalStartResult{}, err
	}
	return durableexecution.CanonicalStartResult{Async: &started}, nil
}

// GetCanonical forwards one durable session read through the private owner
// seam. The outer Sessions service performs the mode-neutral projection.
func (s *Service) GetCanonical(
	ctx context.Context,
	sessionID string,
) (factorysessions.SessionReadResult, error) {
	if s == nil || s.Service == nil {
		return factorysessions.SessionReadResult{}, factorysessions.ErrExecutionServiceNotConfigured
	}
	return s.Service.GetSession(ctx, sessionID)
}

// ListCanonical forwards one durable session inventory read through the
// private owner seam.
func (s *Service) ListCanonical(
	ctx context.Context,
	request factorysessions.ListSessionsRequest,
) (factorysessions.ListSessionsResult, error) {
	if s == nil || s.Service == nil {
		return factorysessions.ListSessionsResult{}, factorysessions.ErrExecutionServiceNotConfigured
	}
	return s.Service.ListSessions(ctx, request)
}

// ControlCanonical maps one mode-neutral durable control request to the
// existing durable owner operation without exposing that operation family to
// the outer canonical Sessions implementation.
func (s *Service) ControlCanonical(
	ctx context.Context,
	request factorysessions.SessionControlRequest,
) (durableexecution.CanonicalControlResult, error) {
	if s == nil || s.Service == nil {
		return durableexecution.CanonicalControlResult{}, factorysessions.ErrExecutionServiceNotConfigured
	}
	control := request.Control
	if control.RequestID == "" {
		control.RequestID = request.Correlation.RequestID
	}
	if control.TurnID == "" {
		control.TurnID = request.Correlation.TurnID
	}
	switch request.Operation {
	case factorysessions.SessionControlPause:
		return s.lifecycleControl(ctx, s.Service.Pause, request.SessionID, control)
	case factorysessions.SessionControlResume:
		return s.lifecycleControl(ctx, s.Service.Resume, request.SessionID, control)
	case factorysessions.SessionControlCancel:
		return s.lifecycleControl(ctx, s.Service.Cancel, request.SessionID, control)
	case factorysessions.SessionControlTerminate:
		return s.lifecycleControl(ctx, s.Service.Terminate, request.SessionID, control)
	case factorysessions.SessionControlRecover:
		return s.recoverCanonicalControl(ctx, request.SessionID, request.Recover, control)
	case factorysessions.SessionControlApprove:
		return s.approveCanonicalControl(ctx, request.SessionID, request.Approve, control)
	case factorysessions.SessionControlRetryDispatch:
		return s.retryCanonicalControl(ctx, request.SessionID, request.Retry, control)
	case factorysessions.SessionControlInterruptDispatch:
		return s.interruptCanonicalControl(ctx, request.SessionID, request.Interrupt, control)
	default:
		return durableexecution.CanonicalControlResult{}, fmt.Errorf("unsupported canonical durable control operation %q", request.Operation)
	}
}

func (s *Service) recoverCanonicalControl(
	ctx context.Context,
	sessionID string,
	request *factorysessions.ResumeSessionRequest,
	control factorysessions.ControlRequest,
) (durableexecution.CanonicalControlResult, error) {
	recovery := factorysessions.ResumeSessionRequest{RequestID: control.RequestID}
	if request != nil {
		recovery = *request
		if recovery.RequestID == "" {
			recovery.RequestID = control.RequestID
		}
	}
	started, err := s.Service.ResumeInterruptedSession(ctx, sessionID, recovery)
	if err != nil {
		return durableexecution.CanonicalControlResult{}, err
	}
	return durableexecution.CanonicalControlResult{Recovery: &started}, nil
}

func (s *Service) approveCanonicalControl(
	ctx context.Context,
	sessionID string,
	request *factorysessions.ApproveRequest,
	control factorysessions.ControlRequest,
) (durableexecution.CanonicalControlResult, error) {
	approve := factorysessions.ApproveRequest{ControlRequest: control}
	if request != nil {
		approve = *request
		if approve.RequestID == "" {
			approve.RequestID = control.RequestID
		}
	}
	return s.lifecycleControl(ctx, func(
		ctx context.Context,
		id string,
		_ factorysessions.ControlRequest,
	) (factorysessions.LifecycleControlResult, error) {
		return s.Service.Approve(ctx, id, approve)
	}, sessionID, control)
}

func (s *Service) retryCanonicalControl(
	ctx context.Context,
	sessionID string,
	request *factorysessions.RetryDispatchRequest,
	control factorysessions.ControlRequest,
) (durableexecution.CanonicalControlResult, error) {
	retry := factorysessions.RetryDispatchRequest{ControlRequest: control}
	if request != nil {
		retry = *request
		if retry.RequestID == "" {
			retry.RequestID = control.RequestID
		}
	}
	return s.lifecycleControl(ctx, func(
		ctx context.Context,
		id string,
		_ factorysessions.ControlRequest,
	) (factorysessions.LifecycleControlResult, error) {
		return s.Service.RetryDispatch(ctx, id, retry)
	}, sessionID, control)
}

func (s *Service) interruptCanonicalControl(
	ctx context.Context,
	sessionID string,
	request *factorysessions.InterruptDispatchRequest,
	control factorysessions.ControlRequest,
) (durableexecution.CanonicalControlResult, error) {
	interrupt := factorysessions.InterruptDispatchRequest{ControlRequest: control}
	if request != nil {
		interrupt = *request
		if interrupt.RequestID == "" {
			interrupt.RequestID = control.RequestID
		}
	}
	return s.lifecycleControl(ctx, func(
		ctx context.Context,
		id string,
		_ factorysessions.ControlRequest,
	) (factorysessions.LifecycleControlResult, error) {
		return s.Service.InterruptDispatch(ctx, id, interrupt)
	}, sessionID, control)
}

type lifecycleControl func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)

func (s *Service) lifecycleControl(
	ctx context.Context,
	operation lifecycleControl,
	sessionID string,
	control factorysessions.ControlRequest,
) (durableexecution.CanonicalControlResult, error) {
	result, err := operation(ctx, sessionID, control)
	if err != nil {
		return durableexecution.CanonicalControlResult{}, err
	}
	return durableexecution.CanonicalControlResult{Lifecycle: &result}, nil
}

// ReadResultCanonical forwards one durable result read through the private
// owner seam.
func (s *Service) ReadResultCanonical(
	ctx context.Context,
	sessionID string,
	request factorysessions.ResultRequest,
) (factorysessions.ResultReadResult, error) {
	if s == nil || s.Service == nil {
		return factorysessions.ResultReadResult{}, factorysessions.ErrExecutionServiceNotConfigured
	}
	return s.Service.GetResult(ctx, sessionID, request)
}

// QueryDispatchesCanonical forwards one filtered dispatch query through the
// private owner seam.
func (s *Service) QueryDispatchesCanonical(
	ctx context.Context,
	request factorysessions.DispatchQueryRequest,
) (factorysessions.ListDispatchesResult, error) {
	if s == nil || s.Service == nil {
		return factorysessions.ListDispatchesResult{}, factorysessions.ErrExecutionServiceNotConfigured
	}
	return s.Service.QueryDispatches(ctx, request)
}

// SubscribeResponsesCanonical forwards one durable response subscription
// through the private owner seam when the underlying runtime supports it.
func (s *Service) SubscribeResponsesCanonical(
	ctx context.Context,
	request factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	if s == nil || s.Service == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	return s.SubscribeResponseEvents(ctx, request.SessionID, request)
}

// SetPersistenceWarningLogger forwards the session-scoped safe diagnostic
// logger to the concrete durable execution owner when it supports persistence
// size warnings.
func (s *Service) SetPersistenceWarningLogger(logger *zap.Logger) {
	if s == nil || s.Service == nil {
		return
	}
	setter, ok := s.Service.(interface {
		SetPersistenceWarningLogger(*zap.Logger)
	})
	if ok {
		setter.SetPersistenceWarningLogger(logger)
	}
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
