package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"go.uber.org/zap"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// ApplyOwnedControl acts on this session's runtime without resolving a
// per-session gateway. The process root serializes control IDs on the record.
func (a *Assembly) scopedApplyOwnedControl(fs *SessionRuntime, ctx context.Context, sessionID string, operation factorysessions.LifecycleControlKind, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	if fs == nil {
		return factorysessions.LifecycleControlResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	return applyScopedOwnedControl(ctx, a.state, fs.logger, sessionID, operation, request)
}

func applyScopedOwnedControl(ctx context.Context, state runtimebinding.LiveSessionResolver, logger *zap.Logger, sessionID string, operation factorysessions.LifecycleControlKind, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	if err := ctx.Err(); err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	control, err := factorysessionexecution.NormalizeControlRequest(request)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	session, err := runtimebinding.RequireLiveSession(state, sessionID)
	if err != nil {
		runtimebinding.ObserveLifecycleControl(logger, state, sessionID, operation, control, "", "", err)
		return factorysessions.LifecycleControlResult{}, err
	}
	runtime := runtimebinding.ServiceForSession(session)
	if runtime == nil {
		return factorysessions.LifecycleControlResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	observed, err := runtime.Observe(ctx, factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeHealth})
	if err != nil {
		return factorysessions.LifecycleControlResult{}, fmt.Errorf("observe live Factory Session: %w", err)
	}
	status := factorysessions.LifecycleStatusFromFactoryRuntimeState(observed.Observation.Health.FactoryState)
	outcome := factorysessions.EvaluateLifecycleControl(operation, status)
	if outcome == factorysessions.LifecycleControlOutcomeInvalidState || outcome == factorysessions.LifecycleControlOutcomeTerminalSession {
		controlErr := &factorysessions.ControlError{Operation: operation, Outcome: outcome, Status: status, Message: fmt.Sprintf("%s rejected for session %s in status %s", operation, sessionID, status), Links: factorysessions.LiveLifecycleControlLinksForSession(sessionID)}
		runtimebinding.ObserveLifecycleControl(logger, state, sessionID, operation, control, outcome, status, controlErr)
		return factorysessions.LifecycleControlResult{}, controlErr
	}
	if outcome == factorysessions.LifecycleControlOutcomeAccepted {
		status, outcome, err = applyOwnedControl(ctx, runtime, operation, status, control)
		if err != nil {
			runtimebinding.ObserveLifecycleControl(logger, state, sessionID, operation, control, outcome, status, err)
			return factorysessions.LifecycleControlResult{}, err
		}
	}
	result := factorysessions.LifecycleControlResult{SessionID: sessionID, Operation: operation, Outcome: outcome, Status: status, Links: factorysessions.LiveLifecycleControlLinksForSession(sessionID)}
	runtimebinding.ObserveLifecycleControl(logger, state, sessionID, operation, control, outcome, status, nil)
	return result, nil
}

func applyOwnedControl(ctx context.Context, runtime factoryruntime.Service, operation factorysessions.LifecycleControlKind, status factorysessions.LifecycleStatus, control factorysessions.ControlRequest) (factorysessions.LifecycleStatus, factorysessions.LifecycleControlOutcome, error) {
	switch operation {
	case factorysessions.LifecycleControlPause:
		_, err := runtime.ControlPause(ctx, factoryruntime.PauseRequest{TurnID: control.TurnID, ControlID: control.RequestID})
		return factorysessions.LifecycleStatusPaused, factorysessions.LifecycleControlOutcomeAccepted, err
	case factorysessions.LifecycleControlResume:
		_, err := runtime.ControlResume(ctx, factoryruntime.ResumeRequest{TurnID: control.TurnID, ControlID: control.RequestID})
		return factorysessions.LifecycleStatusRunning, factorysessions.LifecycleControlOutcomeAccepted, err
	case factorysessions.LifecycleControlCancel, factorysessions.LifecycleControlTerminate:
		action := factoryruntime.WorkerSessionControlActionCancel
		if operation == factorysessions.LifecycleControlTerminate {
			action = factoryruntime.WorkerSessionControlActionTerminate
		}
		result, err := runtime.ControlTerminate(ctx, factoryruntime.TerminateRequest{Reason: control.Reason, TurnID: control.TurnID, ControlID: control.RequestID, WorkerSessionAction: action})
		if err != nil {
			if errors.Is(err, factoryruntime.ErrAlreadyStopped) {
				return status, factorysessions.LifecycleControlOutcomeTerminalSession, err
			}
			return status, factorysessions.LifecycleControlOutcomeConflict, err
		}
		outcome := factorysessions.LifecycleControlOutcomeAccepted
		if result.Outcome == factoryruntime.ControlOutcomeNoOp {
			outcome = factorysessions.LifecycleControlOutcomeNoOp
		}
		return factorysessions.LifecycleStatusSucceeded, outcome, nil
	default:
		return status, factorysessions.LifecycleControlOutcomeInvalidState, fmt.Errorf("unsupported live lifecycle operation %s", operation)
	}
}

// SessionScopeControl controls an addressed live generation without a Root,
// gateway, invocation engine, or opening-owner dependency.
type SessionScopeControl interface {
	CancelLiveFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	StopLiveSession(context.Context, string) error
	StopLiveGeneration(context.Context, *livesession.LiveSession) error
}

type scopeControl struct {
	state  *sessionruntime.Service
	stop   factoryruntime.RuntimeStopOperation
	logger *zap.Logger
}

// NewScopeControl retains the canonical paired session authority and process
// logger. Stop diagnostics outlive the runtime sink closed by activation/stop.
func NewScopeControl(state *sessionruntime.Service, stop factoryruntime.RuntimeStopOperation, logger *zap.Logger) SessionScopeControl {
	return &scopeControl{state: state, stop: stop, logger: logger}
}

// StopLiveSession stops the selected generation without unregistering its
// record. Retirement and activation cleanup remain separately retryable.
func (c *scopeControl) StopLiveSession(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return &factorysessions.DetachedRequestError{Field: "sessionId", Message: "session id is required"}
	}
	session, err := runtimebinding.RequireLiveSession(c.state, id)
	if err != nil {
		return err
	}
	return c.StopLiveGeneration(ctx, session)
}

// StopLiveGeneration preserves the captured run and fact clock during close.
func (c *scopeControl) StopLiveGeneration(ctx context.Context, session *livesession.LiveSession) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil {
		return factorysessions.ErrRuntimeNotAvailable
	}
	id := session.ID
	c.logger.Info("stopping live Factory Session runtime", zap.String("session_id", id))
	err := c.stop(bound.Handle, bound.Clock)
	if errors.Is(err, context.Canceled) || errors.Is(err, factoryruntime.ErrAlreadyStopped) || errors.Is(err, factoryruntime.ErrNotRunning) {
		err = nil
	}
	if err != nil {
		c.logger.Error("stop live Factory Session runtime failed", zap.String("session_id", id), zap.Error(err))
		return fmt.Errorf("stop live Factory Session runtime: %w", err)
	}
	c.logger.Info("live Factory Session runtime stopped", zap.String("session_id", id))
	return nil
}

func (c *scopeControl) CancelLiveFactorySession(ctx context.Context, sessionID string, control factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return factorysessions.LifecycleControlResult{}, &factorysessions.DetachedRequestError{Field: "sessionId", Message: "session id is required"}
	}
	session := c.state.Resolve(id)
	if session == nil {
		return factorysessions.LifecycleControlResult{}, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, id)
	}
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil {
		return factorysessions.LifecycleControlResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	key := ""
	if control.RequestID != "" {
		key = string(factorysessions.SessionControlCancel) + ":" + control.RequestID + ":" + control.TurnID
	}
	result, err := bound.ApplyControlOnce(key, func() (factorysessions.SessionControlResult, error) {
		applied, err := applyScopedOwnedControl(ctx, addressedControlSession{id: id, session: session}, bound.Logger, id, factorysessions.LifecycleControlCancel, control)
		if err != nil {
			return factorysessions.SessionControlResult{}, err
		}
		return factorysessions.SessionControlResult{
			SessionID: id, Mode: factorysessions.SessionOperationModeLive, Operation: factorysessions.SessionControlCancel,
			Outcome: applied.Outcome, Status: applied.Status, Links: applied.Links,
		}, nil
	})
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return factorysessions.LifecycleControlResult{
		SessionID: result.SessionID, Operation: factorysessions.LifecycleControlCancel,
		Outcome: result.Outcome, Status: result.Status, Links: result.Links,
	}, nil
}

// Keep control and its metrics on the generation selected before the record's
// control fence, even if a replacement is published while Runtime is handling it.
type addressedControlSession struct {
	id      string
	session *livesession.LiveSession
}

func (s addressedControlSession) Resolve(id string) *livesession.LiveSession {
	if id == s.id {
		return s.session
	}
	return nil
}

func (fs *SessionRuntime) ApplyOwnedControl(ctx context.Context, sessionID string, operation factorysessions.LifecycleControlKind, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedApplyOwnedControl(fs, ctx, sessionID, operation, request)
}
