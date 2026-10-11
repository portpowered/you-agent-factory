package factorysession

import (
	"context"
	"errors"
	"net/http"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Control represents LIVE owner close using terminate, observation and stopped-
// only DELETE. DELETE performs the owner's join, activation cleanup and retirement;
// neither terminate nor a terminal projection alone means the child is closed.
func (host *hostSessions) Control(ctx context.Context, request factorysessions.SessionControlRequest) (factorysessions.SessionControlResult, error) {
	if request.Mode != factorysessions.SessionOperationModeLive || request.Operation != factorysessions.SessionControlClose || !exactChildID(request.SessionID, nil) {
		return factorysessions.SessionControlResult{}, errors.New("subagent host only closes an exact live child")
	}
	session, err := host.readChild(ctx, request.SessionID)
	if hostMissing(err) {
		return closedChild(request.SessionID), nil
	}
	if err != nil {
		return factorysessions.SessionControlResult{}, err
	}
	if session.IsDefault {
		return factorysessions.SessionControlResult{}, errors.New("subagent host cannot close the default session")
	}
	var terminated factoryapi.FactorySessionLifecycleControlResponse
	err = host.exchange(ctx, http.MethodPost, childPath(request.SessionID)+"/terminate", factoryapi.FactorySessionLifecycleControlRequest{}, nil, &terminated)
	if err != nil && !hostTerminal(err, request.SessionID) && !hostMissing(err) {
		return factorysessions.SessionControlResult{}, err
	}
	if err == nil && terminated.SessionId != request.SessionID {
		return factorysessions.SessionControlResult{}, errors.New("subagent termination returned a different Factory Session")
	}
	if err := host.retireChild(ctx, request.SessionID); err != nil {
		return factorysessions.SessionControlResult{}, err
	}
	return closedChild(request.SessionID), nil
}

func closedChild(id string) factorysessions.SessionControlResult {
	return factorysessions.SessionControlResult{SessionID: id, Mode: factorysessions.SessionOperationModeLive, Operation: factorysessions.SessionControlClose, Closed: true}
}

// Termination is asynchronous at the HTTP boundary. Reconcile only the exact
// child's public stopped state within RUN's existing close context; the safe-
// DELETE owner remains authoritative if the state changes between read/delete.
func (host *hostSessions) retireChild(ctx context.Context, id string) error {
	for {
		session, err := host.readChild(ctx, id)
		if hostMissing(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if session.IsDefault {
			return errors.New("subagent host cannot close the default session")
		}
		if session.Runtime.Status == factoryapi.FactorySessionStatusFINISHED {
			err = host.exchange(ctx, http.MethodDelete, childPath(id), nil, nil, nil)
			if err == nil || hostMissing(err) {
				return nil
			}
			return err
		}
		// There is no public completion cursor for terminate. The bounded status
		// read is required to represent owner close; this is not a provider retry.
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (host *hostSessions) readChild(ctx context.Context, id string) (factoryapi.FactorySession, error) {
	var session factoryapi.FactorySession
	err := host.exchange(ctx, http.MethodGet, childPath(id), nil, nil, &session)
	if err == nil && session.Id != id {
		return factoryapi.FactorySession{}, errors.New("subagent observation returned a different Factory Session")
	}
	return session, err
}

func hostMissing(err error) bool {
	var response *hostError
	return errors.As(err, &response) && response.status == http.StatusNotFound && (response.code == "NOT_FOUND" || response.code == "FACTORY_SESSION_NOT_FOUND")
}

func hostTerminal(err error, id string) bool {
	var response *hostError
	return errors.As(err, &response) && response.status == http.StatusConflict && response.terminal && response.sessionID == id
}
