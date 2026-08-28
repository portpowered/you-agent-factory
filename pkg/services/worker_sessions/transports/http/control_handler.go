package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	httpcompat "github.com/portpowered/infinite-you/pkg/transports/http/compat"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ControlWorkerSession maps one exact top-level Worker Session control to the
// corresponding Worker Sessions root operation. The action is selected by
// the route, so callers cannot submit an arbitrary action payload or identity.
func (a *Adapter) ControlWorkerSession(
	ctx context.Context,
	workerSessionID string,
	action workersessions.ControlAction,
) (factoryapi.WorkerSessionControlResponse, error) {
	if a == nil || a.controller == nil {
		return factoryapi.WorkerSessionControlResponse{}, errors.New("Worker Sessions control service is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	request := workersessions.ControlRequest{ID: strings.TrimSpace(workerSessionID)}
	if err := request.Validate(); err != nil {
		return factoryapi.WorkerSessionControlResponse{}, err
	}
	var (
		result workersessions.ControlResult
		err    error
	)
	switch action {
	case workersessions.ControlActionPause:
		result, err = a.controller.Pause(ctx, request)
	case workersessions.ControlActionResume:
		result, err = a.controller.Resume(ctx, request)
	case workersessions.ControlActionCancel:
		result, err = a.controller.Cancel(ctx, request)
	case workersessions.ControlActionTerminate:
		result, err = a.controller.Terminate(ctx, request)
	default:
		return factoryapi.WorkerSessionControlResponse{}, fmt.Errorf("unsupported Worker Session control action %q", action)
	}
	if err != nil {
		return factoryapi.WorkerSessionControlResponse{}, err
	}
	return WorkerSessionControlResponseToAPI(result), nil
}

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
	decoded, err := decodeWorkerSessionInterruptRequestWithDiagnostics(r.Body)
	if err != nil {
		writeInterruptError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid interrupt request payload", workersessions.InterruptPhaseValidation, workersessions.InterruptResult{
			SourceWorkerSessionID: string(sourceWorkerSessionID),
		})
		return
	}
	response, err := h.adapter.InterruptWorkerSession(r.Context(), string(sourceWorkerSessionID), decoded.Value)
	if err != nil {
		h.writeMappedInterruptError(w, err, string(sourceWorkerSessionID), decoded.Value)
		return
	}
	h.writeCompatibilityWarning(w, "interrupt_worker_session", decoded.Diagnostics.Paths())
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

func decodeWorkerSessionStartRequestWithDiagnostics(body io.Reader) (httpcompat.DecodeResult[factoryapi.WorkerSessionStartRequest], error) {
	return httpcompat.Decode[factoryapi.WorkerSessionStartRequest](body)
}

func decodeWorkerSessionContinueRequestWithDiagnostics(body io.Reader) (httpcompat.DecodeResult[factoryapi.WorkerSessionContinueRequest], error) {
	return httpcompat.Decode[factoryapi.WorkerSessionContinueRequest](body)
}

func decodeWorkerSessionInterruptRequestWithDiagnostics(body io.Reader) (httpcompat.DecodeResult[factoryapi.WorkerSessionInterruptRequest], error) {
	return httpcompat.Decode[factoryapi.WorkerSessionInterruptRequest](body)
}

func (h *Handler) writeMappedInterruptError(
	w http.ResponseWriter,
	err error,
	sourceWorkerSessionID string,
	request factoryapi.WorkerSessionInterruptRequest,
) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	result := interruptResultForError(err, sourceWorkerSessionID, request)
	status, code, message := interruptErrorResponse(err)
	writeInterruptError(w, status, code, message, result.Phase, result)
}

func interruptResultForError(
	err error,
	sourceWorkerSessionID string,
	request factoryapi.WorkerSessionInterruptRequest,
) workersessions.InterruptResult {
	result := workersessions.InterruptResult{
		RequestID:                strings.TrimSpace(request.RequestId),
		SourceWorkerSessionID:    strings.TrimSpace(sourceWorkerSessionID),
		SuccessorWorkerSessionID: strings.TrimSpace(request.SuccessorWorkerSessionId),
		Phase:                    workersessions.InterruptPhaseValidation,
	}
	var typed *workersessions.InterruptError
	if errors.As(err, &typed) && typed != nil {
		result = typed.Result.Clone()
		result.Phase = typed.Phase
	} else {
		switch {
		case errors.Is(err, workersessions.ErrInterruptSourceCancellation),
			errors.Is(err, workersessions.ErrInterruptSourceCancellationFailed):
			result.Phase = workersessions.InterruptPhaseSourceCancellation
		case errors.Is(err, workersessions.ErrInterruptSuccessorAdmission),
			errors.Is(err, workersessions.ErrInterruptSuccessorAdmissionFailed):
			result.Phase = workersessions.InterruptPhaseSuccessorAdmission
		}
	}
	return result
}

func interruptErrorResponse(err error) (int, string, string) {
	switch {
	case errors.Is(err, workersessions.ErrInvalidInterruptRequestID),
		errors.Is(err, workersessions.ErrInvalidInterruptLineage),
		errors.Is(err, workersessions.ErrInvalidInterruptMessage),
		errors.Is(err, workersessions.ErrInterruptValidation):
		return http.StatusBadRequest, "BAD_REQUEST", "invalid Worker Session interrupt request"
	case errors.Is(err, workersessions.ErrInterruptSourceNotFound):
		return http.StatusNotFound, "NOT_FOUND", "Worker Session interrupt source not found"
	case errors.Is(err, workersessions.ErrInterruptRequestIDConflict):
		return http.StatusConflict, string(factoryapi.ErrorResponseCodeWORKERSESSIONINTERRUPTREQUESTIDCONFLICT), "Worker Session interrupt requestId was reused with different inputs"
	case errors.Is(err, workersessions.ErrInterruptSourceNotActive),
		errors.Is(err, workersessions.ErrInterruptSourceConflict),
		errors.Is(err, workersessions.ErrInterruptProviderSessionMissing),
		errors.Is(err, workersessions.ErrInterruptProviderSessionInvalid):
		return http.StatusConflict, string(factoryapi.ErrorResponseCodeWORKERSESSIONINTERRUPTCONFLICT), "Worker Session interrupt conflicts with existing state"
	case errors.Is(err, workersessions.ErrInterruptSourceCancellation),
		errors.Is(err, workersessions.ErrInterruptSourceCancellationFailed):
		return http.StatusServiceUnavailable, string(factoryapi.ErrorResponseCodeWORKERSESSIONINTERRUPTSOURCECANCELLATIONFAILED), "Workers could not cancel the Worker Session interrupt source"
	case errors.Is(err, workersessions.ErrInterruptSuccessorAdmission),
		errors.Is(err, workersessions.ErrInterruptSuccessorAdmissionFailed):
		return http.StatusServiceUnavailable, string(factoryapi.ErrorResponseCodeWORKERSESSIONINTERRUPTSUCCESSORADMISSIONFAILED), "Workers could not admit the Worker Session interrupt successor"
	case errors.Is(err, workersessions.ErrInterruptExecutionUnavailable),
		errors.Is(err, workersessions.ErrInterruptServerStopping):
		return http.StatusServiceUnavailable, string(factoryapi.ErrorResponseCodeWORKERSESSIONINTERRUPTADMISSIONFAILED), "Workers could not admit the Worker Session interrupt"
	default:
		return http.StatusInternalServerError, "INTERNAL_ERROR", "failed to interrupt Worker Session"
	}
}

func (h *Handler) writeMappedControlError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return
	case errors.Is(err, workersessions.ErrInvalidSessionID):
		writeError(w, http.StatusBadRequest, "invalid Worker Session control request", "WORKER_SESSION_CONTROL_INVALID")
	case errors.Is(err, workersessions.ErrSessionNotFound):
		writeError(w, http.StatusNotFound, "Worker Session not found", "NOT_FOUND")
	case errors.Is(err, workersessions.ErrInvalidState),
		errors.Is(err, workersessions.ErrProviderSessionAssociationAttemptMismatch),
		errors.Is(err, workersessions.ErrProviderSessionAssociationNotAvailable),
		errors.Is(err, workersessions.ErrProviderSessionAssociationMissing),
		errors.Is(err, workersessions.ErrProviderSessionAssociationConflict),
		errors.Is(err, workersessions.ErrInvalidProviderSessionAssociation):
		writeError(w, http.StatusConflict, "Worker Session control conflicts with current state", "WORKER_SESSION_CONTROL_CONFLICT")
	default:
		writeError(w, http.StatusServiceUnavailable, "Workers could not apply the Worker Session control", "WORKER_SESSION_CONTROL_FAILED")
	}
}

type workerSessionInterruptErrorResponse struct {
	Message                  string                                     `json:"message"`
	Family                   factoryapi.ErrorFamily                     `json:"family"`
	Code                     string                                     `json:"code"`
	Phase                    string                                     `json:"phase"`
	RequestID                *string                                    `json:"requestId,omitempty"`
	SourceWorkerSessionID    *string                                    `json:"sourceWorkerSessionId,omitempty"`
	SuccessorWorkerSessionID *string                                    `json:"successorWorkerSessionId,omitempty"`
	Source                   *factoryapi.WorkerSessionInterruptSnapshot `json:"source,omitempty"`
	Successor                *factoryapi.WorkerSessionInterruptSnapshot `json:"successor,omitempty"`
}

func writeInterruptError(
	w http.ResponseWriter,
	status int,
	code string,
	message string,
	phase workersessions.InterruptPhase,
	result workersessions.InterruptResult,
) {
	payload := workerSessionInterruptErrorResponse{
		Message: message,
		Family:  errorFamilyForStatus(status),
		Code:    code,
		Phase:   string(phase),
	}
	if result.RequestID != "" {
		value := result.RequestID
		payload.RequestID = &value
	}
	if result.SourceWorkerSessionID != "" {
		value := result.SourceWorkerSessionID
		payload.SourceWorkerSessionID = &value
	}
	if result.SuccessorWorkerSessionID != "" {
		value := result.SuccessorWorkerSessionID
		payload.SuccessorWorkerSessionID = &value
	}
	if result.Source.ID != "" {
		snapshot := workerSessionInterruptSnapshotToAPI(result.Source)
		payload.Source = &snapshot
	}
	if result.Successor.ID != "" {
		snapshot := workerSessionInterruptSnapshotToAPI(result.Successor)
		payload.Successor = &snapshot
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
