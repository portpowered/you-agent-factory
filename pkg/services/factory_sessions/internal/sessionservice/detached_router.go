package service

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// registerDetachedGateway records the runtime gateway that owns one session.
// The process root retains this routing table; it never replaces its service
// slot and never constructs a second runtime-bound service.
func (a *Assembly) registerDetachedGateway(sessionID string, owner factorysessions.DetachedOperationsOwner) {
	if a == nil || owner == nil {
		return
	}
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return
	}
	a.detachedMu.Lock()
	defer a.detachedMu.Unlock()
	if a.detachedGateways == nil {
		a.detachedGateways = make(map[string]factorysessions.DetachedOperationsOwner)
	}
	if _, exists := a.detachedGateways[id]; !exists {
		a.detachedGatewayOrder = append(a.detachedGatewayOrder, id)
	}
	a.detachedGateways[id] = owner
}

func (a *Assembly) detachedOwner(sessionID string) (factorysessions.DetachedOperationsOwner, error) {
	if a == nil {
		return nil, factorysessions.ErrDetachedServiceUnavailable
	}
	id := strings.TrimSpace(sessionID)
	a.detachedMu.RLock()
	owner, ok := a.detachedGateways[id]
	a.detachedMu.RUnlock()
	if !ok || owner == nil {
		return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, id)
	}
	return owner, nil
}

func (a *Assembly) activeDetachedOwner() (factorysessions.DetachedOperationsOwner, error) {
	if a == nil {
		return nil, factorysessions.ErrDetachedServiceUnavailable
	}
	a.detachedMu.RLock()
	defer a.detachedMu.RUnlock()
	for index := len(a.detachedGatewayOrder) - 1; index >= 0; index-- {
		owner := a.detachedGateways[a.detachedGatewayOrder[index]]
		if owner != nil {
			return owner, nil
		}
	}
	return nil, factorysessions.ErrDetachedServiceUnavailable
}

func (a *Assembly) detachedOwners() []factorysessions.DetachedOperationsOwner {
	if a == nil {
		return nil
	}
	a.detachedMu.RLock()
	defer a.detachedMu.RUnlock()
	owners := make([]factorysessions.DetachedOperationsOwner, 0, len(a.detachedGatewayOrder))
	for _, id := range a.detachedGatewayOrder {
		owner := a.detachedGateways[id]
		if owner == nil || containsDetachedOwner(owners, owner) {
			continue
		}
		owners = append(owners, owner)
	}
	return owners
}

func containsDetachedOwner(owners []factorysessions.DetachedOperationsOwner, candidate factorysessions.DetachedOperationsOwner) bool {
	candidateValue := reflect.ValueOf(candidate)
	for _, owner := range owners {
		ownerValue := reflect.ValueOf(owner)
		if !candidateValue.IsValid() || !ownerValue.IsValid() || candidateValue.Type() != ownerValue.Type() {
			continue
		}
		if candidateValue.Type().Comparable() {
			if candidateValue.Interface() == ownerValue.Interface() {
				return true
			}
		}
	}
	return false
}

func (a *Assembly) StartAsync(ctx context.Context, request factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	owner, err := a.activeDetachedOwner()
	if err != nil {
		return factorysessions.AsyncStartResult{}, err
	}
	result, err := owner.StartAsync(ctx, request)
	if err == nil {
		a.registerDetachedGateway(result.SessionID, owner)
	}
	return result, err
}

func (a *Assembly) StartSync(ctx context.Context, request factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
	owner, err := a.activeDetachedOwner()
	if err != nil {
		return factorysessions.SyncStartResult{}, err
	}
	result, err := owner.StartSync(ctx, request)
	if err == nil {
		a.registerDetachedGateway(result.SessionID, owner)
	}
	return result, err
}

func (a *Assembly) ResumeInterruptedSession(ctx context.Context, sessionID string, request factorysessions.ResumeSessionRequest) (factorysessions.AsyncStartResult, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return factorysessions.AsyncStartResult{}, err
	}
	result, err := owner.ResumeInterruptedSession(ctx, sessionID, request)
	if err == nil {
		a.registerDetachedGateway(result.SessionID, owner)
	}
	return result, err
}

func (a *Assembly) OpenFactorySession(ctx context.Context, request factorysessions.OpenRequest) (*factorysessions.OpenResult, error) {
	owner, err := a.activeDetachedOwner()
	if err != nil {
		return nil, err
	}
	result, err := owner.OpenFactorySession(ctx, request)
	if err == nil && result != nil {
		a.registerDetachedGateway(result.SessionID, owner)
	}
	return result, err
}

func (a *Assembly) InvokeFactorySession(ctx context.Context, sessionID string, request factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return factorysessions.InvocationResult{}, err
	}
	return owner.InvokeFactorySession(ctx, sessionID, request)
}

func (a *Assembly) ActivateNamedFactory(ctx context.Context, name string) error {
	owner, err := a.activeDetachedOwner()
	if err != nil {
		return err
	}
	return owner.ActivateNamedFactory(ctx, name)
}

func (a *Assembly) GetFactorySession(ctx context.Context, sessionID string) (factorysessions.SessionProjection, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return factorysessions.SessionProjection{}, err
	}
	return owner.GetFactorySession(ctx, sessionID)
}

func (a *Assembly) GetSession(ctx context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return factorysessions.SessionReadResult{}, err
	}
	return owner.GetSession(ctx, sessionID)
}

func (a *Assembly) ListFactorySessions(ctx context.Context) ([]factorysessions.ReadProjection, error) {
	owners := a.detachedOwners()
	if len(owners) == 0 {
		return nil, factorysessions.ErrDetachedServiceUnavailable
	}
	result := make([]factorysessions.ReadProjection, 0)
	seen := make(map[string]struct{})
	for _, owner := range owners {
		projections, err := owner.ListFactorySessions(ctx)
		if err != nil {
			return nil, err
		}
		for _, projection := range projections {
			id := projection.Context.FactorySessionID
			if id != "" {
				if _, exists := seen[id]; exists {
					continue
				}
				seen[id] = struct{}{}
			}
			result = append(result, projection)
		}
	}
	return result, nil
}

func (a *Assembly) ListSessions(ctx context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	owners := a.detachedOwners()
	if len(owners) == 0 {
		return factorysessions.ListSessionsResult{}, factorysessions.ErrDetachedServiceUnavailable
	}
	result := factorysessions.ListSessionsResult{Scope: request.Scope}
	seenLive := make(map[string]struct{})
	seenDurable := make(map[string]struct{})
	for _, owner := range owners {
		listed, err := owner.ListSessions(ctx, request)
		if err != nil {
			return factorysessions.ListSessionsResult{}, err
		}
		if result.Scope == "" {
			result.Scope = listed.Scope
		}
		for _, session := range listed.LiveSessions {
			if _, exists := seenLive[session.ID]; exists {
				continue
			}
			seenLive[session.ID] = struct{}{}
			result.LiveSessions = append(result.LiveSessions, session)
		}
		for _, session := range listed.DurableSessions {
			if _, exists := seenDurable[session.SessionID]; exists {
				continue
			}
			seenDurable[session.SessionID] = struct{}{}
			result.DurableSessions = append(result.DurableSessions, session)
		}
	}
	return result, nil
}

func (a *Assembly) PauseLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return owner.PauseLiveFactorySession(ctx, sessionID, request)
}

func (a *Assembly) ResumeLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return owner.ResumeLiveFactorySession(ctx, sessionID, request)
}

func (a *Assembly) CloseFactorySession(ctx context.Context, sessionID string) error {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return err
	}
	return owner.CloseFactorySession(ctx, sessionID)
}

func (a *Assembly) Pause(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return a.forwardDurableControl(sessionID, func(owner factorysessions.DetachedOperationsOwner) (factorysessions.LifecycleControlResult, error) {
		return owner.Pause(ctx, sessionID, request)
	})
}

func (a *Assembly) Resume(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return a.forwardDurableControl(sessionID, func(owner factorysessions.DetachedOperationsOwner) (factorysessions.LifecycleControlResult, error) {
		return owner.Resume(ctx, sessionID, request)
	})
}

func (a *Assembly) Cancel(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return a.forwardDurableControl(sessionID, func(owner factorysessions.DetachedOperationsOwner) (factorysessions.LifecycleControlResult, error) {
		return owner.Cancel(ctx, sessionID, request)
	})
}

func (a *Assembly) Terminate(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return a.forwardDurableControl(sessionID, func(owner factorysessions.DetachedOperationsOwner) (factorysessions.LifecycleControlResult, error) {
		return owner.Terminate(ctx, sessionID, request)
	})
}

func (a *Assembly) Approve(ctx context.Context, sessionID string, request factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error) {
	return a.forwardDurableControl(sessionID, func(owner factorysessions.DetachedOperationsOwner) (factorysessions.LifecycleControlResult, error) {
		return owner.Approve(ctx, sessionID, request)
	})
}

func (a *Assembly) RetryDispatch(ctx context.Context, sessionID string, request factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return a.forwardDurableControl(sessionID, func(owner factorysessions.DetachedOperationsOwner) (factorysessions.LifecycleControlResult, error) {
		return owner.RetryDispatch(ctx, sessionID, request)
	})
}

func (a *Assembly) InterruptDispatch(ctx context.Context, sessionID string, request factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return a.forwardDurableControl(sessionID, func(owner factorysessions.DetachedOperationsOwner) (factorysessions.LifecycleControlResult, error) {
		return owner.InterruptDispatch(ctx, sessionID, request)
	})
}

func (a *Assembly) forwardDurableControl(
	sessionID string,
	operation func(factorysessions.DetachedOperationsOwner) (factorysessions.LifecycleControlResult, error),
) (factorysessions.LifecycleControlResult, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return operation(owner)
}

func (a *Assembly) GetResult(ctx context.Context, sessionID string, request factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return factorysessions.ResultReadResult{}, err
	}
	return owner.GetResult(ctx, sessionID, request)
}

func (a *Assembly) GetFactorySessionResult(ctx context.Context, sessionID string) (factoryruntime.LiveSessionResult, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return factoryruntime.LiveSessionResult{}, err
	}
	return owner.GetFactorySessionResult(ctx, sessionID)
}

func (a *Assembly) GetFactorySessionPartialResult(ctx context.Context, sessionID string) (factoryruntime.PartialSessionResult, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return factoryruntime.PartialSessionResult{}, err
	}
	return owner.GetFactorySessionPartialResult(ctx, sessionID)
}

func (a *Assembly) SubscribeFactoryResponseEvents(ctx context.Context, request factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error) {
	owner, err := a.detachedOwner(request.SessionID)
	if err != nil {
		return nil, err
	}
	return owner.SubscribeFactoryResponseEvents(ctx, request)
}

var _ factorysessions.DetachedOperationsOwner = (*Assembly)(nil)
