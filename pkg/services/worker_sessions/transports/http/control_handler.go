package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// InterruptWorkerSession handles one source-addressed asynchronous Worker
// Session interrupt-and-replace operation. The source identity is supplied by
// the generated route and the Worker Sessions root owns cancellation,
// replay, and successor admission.
func (h *Handler) InterruptWorkerSession(
	w http.ResponseWriter,
	r *http.Request,
	sourceWorkerSessionID factoryapi.WorkerSessionID,
) {
	if h == nil || h.adapter == nil {
		writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	if strings.TrimSpace(string(sourceWorkerSessionID)) == "" {
		writeInterruptError(w, http.StatusBadRequest, "BAD_REQUEST", "worker session id is required", workersessions.InterruptPhaseValidation, workersessions.InterruptResult{})
		return
	}
	if r == nil || r.Body == nil {
		writeInterruptError(w, http.StatusBadRequest, "BAD_REQUEST", "request payload is required", workersessions.InterruptPhaseValidation, workersessions.InterruptResult{
			SourceWorkerSessionID: string(sourceWorkerSessionID),
		})
		return
	}
	request, err := decodeWorkerSessionInterruptRequest(r.Body)
	if err != nil {
		writeInterruptError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid interrupt request payload", workersessions.InterruptPhaseValidation, workersessions.InterruptResult{
			SourceWorkerSessionID: string(sourceWorkerSessionID),
		})
		return
	}
	response, err := h.adapter.InterruptWorkerSession(r.Context(), string(sourceWorkerSessionID), request)
	if err != nil {
		h.writeMappedInterruptError(w, err, string(sourceWorkerSessionID), request)
		return
	}
	h.writeJSON(w, http.StatusAccepted, response)
}

// PauseWorkerSession applies the exact source-addressed pause control.
func (h *Handler) PauseWorkerSession(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
) {
	h.controlWorkerSession(w, r, workerSessionID, workersessions.ControlActionPause)
}

// ResumeWorkerSession applies the exact source-addressed resume control.
func (h *Handler) ResumeWorkerSession(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
) {
	h.controlWorkerSession(w, r, workerSessionID, workersessions.ControlActionResume)
}

// CancelWorkerSession applies the exact source-addressed cancel control.
func (h *Handler) CancelWorkerSession(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
) {
	h.controlWorkerSession(w, r, workerSessionID, workersessions.ControlActionCancel)
}

// TerminateWorkerSession applies the exact source-addressed terminate control.
func (h *Handler) TerminateWorkerSession(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
) {
	h.controlWorkerSession(w, r, workerSessionID, workersessions.ControlActionTerminate)
}

func (h *Handler) controlWorkerSession(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
	action workersessions.ControlAction,
) {
	if h == nil || h.adapter == nil {
		writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	if r == nil {
		writeError(w, http.StatusBadRequest, "request is required", "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(string(workerSessionID)) == "" {
		writeError(w, http.StatusBadRequest, "worker session id is required", "WORKER_SESSION_CONTROL_INVALID")
		return
	}
	response, err := h.adapter.ControlWorkerSession(r.Context(), string(workerSessionID), action)
	if err != nil {
		h.writeMappedControlError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

func decodeWorkerSessionStartRequest(body io.Reader) (factoryapi.WorkerSessionStartRequest, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.WorkerSessionStartRequest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var request factoryapi.WorkerSessionStartRequest
	if err := decoder.Decode(&request); err != nil {
		return factoryapi.WorkerSessionStartRequest{}, err
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return factoryapi.WorkerSessionStartRequest{}, errors.New("request payload must contain one JSON object")
		}
		return factoryapi.WorkerSessionStartRequest{}, err
	}
	return request, nil
}

func decodeWorkerSessionContinueRequest(body io.Reader) (factoryapi.WorkerSessionContinueRequest, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.WorkerSessionContinueRequest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var request factoryapi.WorkerSessionContinueRequest
	if err := decoder.Decode(&request); err != nil {
		return factoryapi.WorkerSessionContinueRequest{}, err
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return factoryapi.WorkerSessionContinueRequest{}, errors.New("request payload must contain one JSON object")
		}
		return factoryapi.WorkerSessionContinueRequest{}, err
	}
	return request, nil
}

func decodeWorkerSessionInterruptRequest(body io.Reader) (factoryapi.WorkerSessionInterruptRequest, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.WorkerSessionInterruptRequest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var request factoryapi.WorkerSessionInterruptRequest
	if err := decoder.Decode(&request); err != nil {
		return factoryapi.WorkerSessionInterruptRequest{}, err
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return factoryapi.WorkerSessionInterruptRequest{}, errors.New("request payload must contain one JSON object")
		}
		return factoryapi.WorkerSessionInterruptRequest{}, err
	}
	return request, nil
}
