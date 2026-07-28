package http

import (
	"errors"
	"net/http"
	"strings"

	state "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/optional"
)

// MoveWorkBySessionId decodes one session-scoped move-work request, invokes the
// accepted Work root, and encodes the public HTTP success response from a
// detached post-move Work read model.
func (a *Adapter) MoveWorkBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	id factoryapi.WorkOrTokenID,
) {
	if strings.TrimSpace(string(sessionID)) == "" {
		a.writeError(w, http.StatusBadRequest, "session id is required", "BAD_REQUEST")
		return
	}
	workID := strings.TrimSpace(string(id))
	if workID == "" {
		a.writeError(w, http.StatusBadRequest, "work id is required", "BAD_REQUEST")
		return
	}

	decoded, err := MoveWorkRequestFromBody(r.Body)
	if err != nil {
		a.writeMoveDecodeError(w, err)
		return
	}
	stateName := strings.TrimSpace(decoded.StateName)
	if stateName == "" {
		a.writeError(w, http.StatusBadRequest, "stateName is required", "BAD_REQUEST")
		return
	}

	result, err := a.invokeMoveWorkAndRead(
		r.Context(),
		string(sessionID),
		workID,
		stateName,
		strings.TrimSpace(optional.StringValue(decoded.RequestId)),
	)
	if err != nil {
		a.writeMoveRootError(w, err, "failed to move work")
		return
	}
	a.writeJSON(w, http.StatusOK, WorkReadModelToAPI(result))
}

func (a *Adapter) writeMoveDecodeError(w http.ResponseWriter, err error) {
	if message, ok := requestFieldValidationMessage(err); ok {
		a.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
		return
	}
	a.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
}

func (a *Adapter) writeMoveRootError(w http.ResponseWriter, err error, fallbackMessage string) {
	switch {
	case errors.Is(err, state.ErrMoveWorkNotFound), errors.Is(err, work.ErrWorkNotFound):
		a.writeError(w, http.StatusNotFound, "work not found", "NOT_FOUND")
	case errors.Is(err, apisurface.ErrFactorySessionNotFound):
		a.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
	case errors.Is(err, state.ErrMoveWorkInvalidState):
		a.writeError(w, http.StatusBadRequest, "invalid target state for work type", "BAD_REQUEST")
	case errors.Is(err, state.ErrMoveWorkInFlightDispatch):
		a.writeError(w, http.StatusBadRequest, "work is in an active dispatch", "BAD_REQUEST")
	case errors.Is(err, state.ErrMoveWorkEngineTerminated):
		a.writeError(w, http.StatusBadRequest, "engine has terminated", "BAD_REQUEST")
	case errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied):
		a.writeError(
			w,
			http.StatusConflict,
			"Operator move request was already applied.",
			"MOVE_WORK_REQUEST_ALREADY_APPLIED",
		)
	default:
		var validation *work.ValidationError
		if errors.As(err, &validation) {
			a.writeError(w, http.StatusBadRequest, validation.Message, "BAD_REQUEST")
			return
		}
		a.writeError(w, http.StatusInternalServerError, fallbackMessage, "INTERNAL_ERROR")
	}
}
