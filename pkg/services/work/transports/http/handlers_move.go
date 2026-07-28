package http

import (
	"net/http"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
		a.writeRootOrInternalError(w, err, "failed to move work")
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

