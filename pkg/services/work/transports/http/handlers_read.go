package http

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

const (
	workListReadOperation = "list_work"
	workShowReadOperation = "show_work"
)

// ListWorkBySessionId decodes one session-scoped list-work request, invokes the
// accepted Work root, and encodes the public HTTP success response from detached
// Work read models.
func (a *Adapter) ListWorkBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.ListWorkBySessionIdParams,
) {
	if strings.TrimSpace(string(sessionID)) == "" {
		a.writeError(w, http.StatusBadRequest, "session id is required", "BAD_REQUEST")
		return
	}
	if err := validateWorkListQuery(r.URL.Query()); err != nil {
		a.writeListDecodeError(w, err)
		return
	}

	options, err := ListOptionsFromAPI(params)
	if err != nil {
		a.writeListDecodeError(w, err)
		return
	}
	if requestContextEnded(r.Context()) {
		return
	}

	result, err := a.invokeListWork(r.Context(), string(sessionID), options)
	if shouldEndOnRequestContext(r.Context(), err) {
		return
	}
	if err != nil {
		a.writeReadOrInternalError(
			w,
			err,
			"failed to list Work",
			workListReadOperation,
			zap.String("session_id", string(sessionID)),
		)
		return
	}
	a.writeJSON(w, http.StatusOK, ListWorkResponseToAPI(result))
}

func validateWorkListQuery(query url.Values) error {
	for parameter := range query {
		switch parameter {
		case "maxResults", "nextToken", "state.name", "state.type", "sortBy", "name", "workTypeName", "traceId", "terminal", "nonTerminal", "counts", "includeSuperseded":
			continue
		default:
			return &work.ValidationError{
				Field:   parameter,
				Message: parameter + " is not supported; use nextToken for Work pagination",
			}
		}
	}
	return nil
}

// GetWorkBySessionId decodes one session-scoped get-work request, invokes the
// accepted Work root, and encodes the public HTTP success response from a
// detached Work read model.
func (a *Adapter) GetWorkBySessionId(
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
	if requestContextEnded(r.Context()) {
		return
	}

	result, err := a.invokeGetWork(r.Context(), string(sessionID), workID)
	if shouldEndOnRequestContext(r.Context(), err) {
		return
	}
	if err != nil {
		a.writeReadOrInternalError(
			w,
			err,
			"failed to get Work",
			workShowReadOperation,
			zap.String("session_id", string(sessionID)),
			zap.String("work_id", workID),
		)
		return
	}
	a.writeJSON(w, http.StatusOK, WorkReadModelToAPI(result))
}

func (a *Adapter) writeListDecodeError(w http.ResponseWriter, err error) {
	var validation *work.ValidationError
	if errors.As(err, &validation) {
		a.writeError(w, http.StatusBadRequest, validation.Message, "BAD_REQUEST")
		return
	}
	a.writeError(w, http.StatusBadRequest, "invalid list-work request", "BAD_REQUEST")
}
