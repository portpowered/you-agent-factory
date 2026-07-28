package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// StageSubmitWorkFileBySessionId decodes one stage-submit-work-file request,
// invokes the accepted Work root, and encodes the public HTTP success response.
func (a *Adapter) StageSubmitWorkFileBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	if strings.TrimSpace(string(sessionID)) == "" {
		a.writeError(w, http.StatusBadRequest, "session id is required", "BAD_REQUEST")
		return
	}

	decoded, err := StageSubmitWorkFileRequestFromBody(r.Body)
	if err != nil {
		a.writeAdmissionDecodeError(w, err)
		return
	}

	stageRequest, err := StageContentRequestFromAPI(decoded)
	if err != nil {
		a.writeAdmissionDecodeError(w, err)
		return
	}

	result, err := a.invokeStageContent(r.Context(), stageRequest)
	if err != nil {
		a.writeAdmissionRootError(w, err, "failed to stage submit-work file")
		return
	}
	a.writeJSON(w, http.StatusCreated, StageSubmitWorkFileResponseToAPI(result))
}

// SubmitWorkBySessionId decodes one submit-work request, invokes the accepted
// Work root, and encodes the public HTTP success response.
func (a *Adapter) SubmitWorkBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	if strings.TrimSpace(string(sessionID)) == "" {
		a.writeError(w, http.StatusBadRequest, "session id is required", "BAD_REQUEST")
		return
	}

	decoded, err := SubmitWorkRequestFromBody(r.Body)
	if err != nil {
		a.writeAdmissionDecodeError(w, err)
		return
	}

	workRequest, err := WorkRequestFromSubmitAPI(r.Context(), a.root, decoded.Request)
	if err != nil {
		a.writeAdmissionDecodeError(w, err)
		return
	}

	result, err := a.invokeSubmitWorkRequestForSession(r.Context(), string(sessionID), workRequest)
	if err != nil {
		a.writeAdmissionRootError(w, err, "failed to submit work")
		return
	}
	a.writeJSON(w, http.StatusCreated, SubmitWorkResponseToAPI(result, string(sessionID)))
}

// UpsertWorkRequestBySessionId decodes one upsert-work-request call, invokes the
// accepted Work root, and encodes the public HTTP success response.
func (a *Adapter) UpsertWorkRequestBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	requestID string,
) {
	if strings.TrimSpace(string(sessionID)) == "" {
		a.writeError(w, http.StatusBadRequest, "session id is required", "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(requestID) == "" {
		a.writeError(w, http.StatusBadRequest, "request_id is required", "BAD_REQUEST")
		return
	}

	decoded, err := UpsertWorkRequestFromBody(r.Body)
	if err != nil {
		a.writeAdmissionDecodeError(w, err)
		return
	}
	if decoded.Request.RequestId == "" {
		a.writeError(w, http.StatusBadRequest, "requestId is required", "BAD_REQUEST")
		return
	}
	if decoded.Request.RequestId != requestID {
		a.writeError(w, http.StatusBadRequest, "request_id path and requestId body must match", "BAD_REQUEST")
		return
	}

	workRequest, err := WorkRequestFromUpsertAPI(decoded.Request)
	if err != nil {
		a.writeAdmissionDecodeError(w, err)
		return
	}

	result, err := a.invokeSubmitWorkRequestForSession(r.Context(), string(sessionID), workRequest)
	if err != nil {
		a.writeAdmissionRootError(w, err, "failed to submit work request")
		return
	}
	a.writeJSON(w, http.StatusCreated, UpsertWorkResponseToAPI(result))
}

func (a *Adapter) writeAdmissionDecodeError(w http.ResponseWriter, err error) {
	if message, ok := requestFieldValidationMessage(err); ok {
		a.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
		return
	}
	a.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
}

func (a *Adapter) writeAdmissionRootError(w http.ResponseWriter, err error, fallbackMessage string) {
	if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		a.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
		return
	}
	var stagingErr *work.ContentStagingError
	if errors.As(err, &stagingErr) {
		a.writeError(w, http.StatusBadRequest, stagingErr.Message, "BAD_REQUEST")
		return
	}
	var validation *work.ValidationError
	if errors.As(err, &validation) {
		a.writeError(w, http.StatusBadRequest, validation.Message, "BAD_REQUEST")
		return
	}
	a.writeError(w, http.StatusInternalServerError, fallbackMessage, "INTERNAL_ERROR")
}
