package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func workerSessionEventFrame(observation factoryapi.WorkerSessionObservation, delivery workersessions.ObservationDelivery) workerSessionEventFramePayload {
	frame := workerSessionEventFrameWithIdentity(observation, delivery.Kind, &workerSessionEventRecordPayload{
		Cursor:   workerSessionEventCursor(delivery.Event),
		Position: delivery.Event.Position, SourceType: delivery.Event.SourceType, SourceID: delivery.Event.SourceID,
		SourceSequence: delivery.Event.SourceSequence, SourceEventID: delivery.Event.SourceEventID,
		SchemaID: delivery.Event.SchemaID, Payload: append(json.RawMessage(nil), delivery.Event.Payload...),
	})
	if delivery.Summary != nil {
		frame.ReplaySummary = workerSessionReplaySummaryToAPI(delivery.Summary)
	}
	return frame
}

func workerSessionEventCursor(event workersessions.ObservationEvent) factoryapi.WorkerSessionEventCursor {
	position := event.Cursor.Position
	if position == 0 {
		position = event.Position
	}
	cursor := factoryapi.WorkerSessionEventCursor{Position: int64(position)}
	if event.Cursor.WorkerSessionID != "" {
		workerSessionID := event.Cursor.WorkerSessionID
		cursor.WorkerSessionId = &workerSessionID
	}
	if event.Cursor.StreamGenerationID != "" {
		generation := event.Cursor.StreamGenerationID
		cursor.StreamGenerationId = &generation
	}
	return cursor
}

func workerSessionFailureFrame(observation factoryapi.WorkerSessionObservation, code, message string) workerSessionEventFramePayload {
	return workerSessionEventFrameWithIdentity(observation, workersessions.ObservationDeliverySourceFailure, nil, &code, &message)
}

func workerSessionReplaySummaryFrame(observation factoryapi.WorkerSessionObservation, summary *workersessions.ReplaySummary) workerSessionEventFramePayload {
	if summary == nil {
		return workerSessionFailureFrame(observation, "WORKER_SESSION_STREAM_FAILED", "Worker Session replay summary is unavailable")
	}
	return workerSessionEventFramePayload{
		Delivery:              workersessions.ObservationDeliveryReplaySummary,
		WorkerSessionID:       observation.WorkerSessionId,
		FactorySessionID:      observation.FactorySessionId,
		ProviderSession:       workerSessionProviderSessionRef(observation),
		WorkIDs:               append([]string(nil), observation.WorkIds...),
		RecordingHealth:       eventRecordingHealth(observation.RecordingHealth),
		RecordingHealthReason: observation.RecordingHealthReason,
		ReplaySummary:         workerSessionReplaySummaryToAPI(summary),
	}
}

func workerSessionReplaySummaryToAPI(summary *workersessions.ReplaySummary) *factoryapi.WorkerSessionReplaySummary {
	if summary == nil {
		return nil
	}
	return &factoryapi.WorkerSessionReplaySummary{
		Kind:          "replay-summary",
		Complete:      summary.Complete,
		Reason:        summary.Reason,
		EventsEmitted: int64(summary.EventsEmitted),
	}
}

func workerSessionEventFrameWithIdentity(
	observation factoryapi.WorkerSessionObservation,
	delivery workersessions.ObservationDeliveryKind,
	event *workerSessionEventRecordPayload,
	failure ...*string,
) workerSessionEventFramePayload {
	var errorCode, errorMessage *string
	if len(failure) > 0 {
		errorCode = failure[0]
	}
	if len(failure) > 1 {
		errorMessage = failure[1]
	}
	var providerSession *factoryapi.WorkerSessionProviderSessionRef
	if observation.ProviderSession != nil {
		ref := *observation.ProviderSession
		providerSession = &ref
	}
	return workerSessionEventFramePayload{
		Delivery: delivery, WorkerSessionID: observation.WorkerSessionId,
		FactorySessionID: observation.FactorySessionId, ProviderSession: providerSession,
		WorkIDs: append([]string(nil), observation.WorkIds...), Event: event,
		ErrorCode: errorCode, ErrorMessage: errorMessage,
		RecordingHealth:       eventRecordingHealth(observation.RecordingHealth),
		RecordingHealthReason: observation.RecordingHealthReason,
	}
}

func workerSessionProviderSessionRef(observation factoryapi.WorkerSessionObservation) *factoryapi.WorkerSessionProviderSessionRef {
	if observation.ProviderSession == nil {
		return nil
	}
	ref := *observation.ProviderSession
	return &ref
}

func eventRecordingHealth(value *factoryapi.WorkerSessionObservationRecordingHealth) *factoryapi.WorkerSessionEventRecordingHealth {
	if value == nil {
		return nil
	}
	health := factoryapi.WorkerSessionEventRecordingHealth(*value)
	return &health
}

func writeSSEFrame(w http.ResponseWriter, flusher http.Flusher, frame workerSessionEventFramePayload) error {
	payload, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("encode Worker Session event frame: %w", err)
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func workerSessionStreamFailure(err error) (string, string) {
	switch {
	case errors.Is(err, workersessions.ErrObservationSourceGap):
		return "WORKER_SESSION_STREAM_GAP", "retained Worker Session event history is unavailable"
	case errors.Is(err, workersessions.ErrObservationSourceClosed):
		return "WORKER_SESSION_STREAM_CLOSED", "Worker Session event source closed before terminal"
	case errors.Is(err, workersessions.ErrObservationSourceUnavailable):
		return "WORKER_SESSION_STREAM_UNAVAILABLE", "Worker Session event source is unavailable"
	default:
		return "WORKER_SESSION_STREAM_FAILED", "Worker Session event stream failed"
	}
}

func (h *Handler) writeMappedError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, work.ErrWorkNotFound):
		writeError(w, http.StatusNotFound, "work not found", "NOT_FOUND")
	case errors.Is(err, workersessions.ErrObservationSessionNotFound):
		writeError(w, http.StatusNotFound, "worker session observation not found", "NOT_FOUND")
	case errors.Is(err, workersessions.ErrInvalidObservationScope),
		errors.Is(err, workersessions.ErrInvalidObservationPagination),
		errors.Is(err, workersessions.ErrInvalidState),
		strings.Contains(err.Error(), "worker session id is required"):
		writeError(w, http.StatusBadRequest, "invalid Worker Session observation query", "BAD_REQUEST")
	case errors.Is(err, workersessions.ErrObservationRecordingCorrupt):
		writeError(w, http.StatusInternalServerError, "Worker Session recording history is corrupt", string(factoryapi.ErrorResponseCodeWORKERSESSIONRECORDINGCORRUPT))
	case errors.Is(err, workersessions.ErrObservationRecordingUnavailable):
		writeError(w, http.StatusServiceUnavailable, "Worker Session recording history is unavailable", string(factoryapi.ErrorResponseCodeWORKERSESSIONRECORDINGUNAVAILABLE))
	case errors.Is(err, context.Canceled), errors.Is(err, workersessions.ErrObservationCanceled):
		return
	case strings.Contains(err.Error(), "session id is required"), strings.Contains(err.Error(), "work id is required"):
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
	default:
		writeError(w, http.StatusInternalServerError, "failed to list Worker Sessions", "INTERNAL_ERROR")
	}
}

func (h *Handler) writeMappedStartError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return
	case errors.Is(err, workersessions.ErrInvalidStartRequestID),
		errors.Is(err, workersessions.ErrInvalidSessionID),
		errors.Is(err, workersessions.ErrInvalidExecutionRequest),
		errors.Is(err, errInvalidStartRetry):
		writeError(w, http.StatusBadRequest, "invalid Worker Session start request", "BAD_REQUEST")
	case errors.Is(err, workersessions.ErrStartRequestIDConflict):
		writeError(w, http.StatusConflict, "Worker Session start requestId was reused with different inputs", string(factoryapi.ErrorResponseCodeWORKERSESSIONSTARTREQUESTIDCONFLICT))
	case errors.Is(err, workersessions.ErrSessionNotStartable):
		writeError(w, http.StatusConflict, "Worker Session identity is already in use", string(factoryapi.ErrorResponseCodeWORKERSESSIONNOTSTARTABLE))
	case errors.Is(err, workersessions.ErrEventTopicUnavailable):
		writeError(w, http.StatusServiceUnavailable, "Worker Session event topic is unavailable", string(factoryapi.ErrorResponseCodeWORKERSESSIONEVENTTOPICUNAVAILABLE))
	case errors.Is(err, workersessions.ErrStartOpeningPublication):
		writeError(w, http.StatusServiceUnavailable, "Worker Session opening event is unavailable", string(factoryapi.ErrorResponseCodeWORKERSESSIONSTARTOPENINGFAILED))
	case errors.Is(err, workersessions.ErrStartAdmissionFailed),
		errors.Is(err, workersessions.ErrStartNotAccepted),
		errors.Is(err, workersessions.ErrStartServerStopping):
		writeError(w, http.StatusServiceUnavailable, "Workers could not admit the Worker Session", string(factoryapi.ErrorResponseCodeWORKERSESSIONADMISSIONFAILED))
	default:
		writeError(w, http.StatusInternalServerError, "failed to start Worker Session", "INTERNAL_ERROR")
	}
}

func (h *Handler) writeMappedContinueError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return
	case errors.Is(err, workersessions.ErrInvalidContinuationRequestID),
		errors.Is(err, workersessions.ErrInvalidContinuationLineage),
		errors.Is(err, workersessions.ErrInvalidContinuationInput):
		writeError(w, http.StatusBadRequest, "invalid Worker Session continuation request", "BAD_REQUEST")
	case errors.Is(err, workersessions.ErrContinuationSourceNotFound):
		writeError(w, http.StatusNotFound, "Worker Session continuation source not found", "NOT_FOUND")
	case errors.Is(err, workersessions.ErrContinuationRequestIDConflict):
		writeError(w, http.StatusConflict, "Worker Session continuation requestId was reused with different inputs", string(factoryapi.ErrorResponseCodeWORKERSESSIONCONTINUATIONREQUESTIDCONFLICT))
	case errors.Is(err, workersessions.ErrContinuationSourceActive),
		errors.Is(err, workersessions.ErrContinuationSourceConflict),
		errors.Is(err, workersessions.ErrContinuationSuccessorConflict):
		writeError(w, http.StatusConflict, "Worker Session continuation conflicts with existing state", string(factoryapi.ErrorResponseCodeWORKERSESSIONCONTINUATIONCONFLICT))
	case errors.Is(err, workersessions.ErrContinuationProviderSessionMissing),
		errors.Is(err, workersessions.ErrContinuationProviderSessionInvalid):
		writeError(w, http.StatusConflict, "recorded Provider Session cannot be continued", string(factoryapi.ErrorResponseCodeWORKERSESSIONPROVIDERCONTINUATIONINVALID))
	case errors.Is(err, workersessions.ErrContinuationExecutionUnavailable),
		errors.Is(err, workersessions.ErrContinuationNotAccepted),
		errors.Is(err, workersessions.ErrContinuationServerStopping),
		errors.Is(err, workersessions.ErrEventTopicUnavailable),
		errors.Is(err, workersessions.ErrStartOpeningPublication):
		writeError(w, http.StatusServiceUnavailable, "Workers could not admit the Worker Session continuation", string(factoryapi.ErrorResponseCodeWORKERSESSIONCONTINUATIONADMISSIONFAILED))
	default:
		writeError(w, http.StatusInternalServerError, "failed to continue Worker Session", "INTERNAL_ERROR")
	}
}

func (h *Handler) writeMappedObservationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, workersessions.ErrInvalidSessionID),
		errors.Is(err, workersessions.ErrInvalidObservationIdentity):
		writeError(w, http.StatusBadRequest, "invalid Worker Session identity", "BAD_REQUEST")
	case errors.Is(err, workersessions.ErrObservationSessionNotFound):
		writeError(w, http.StatusNotFound, "worker session observation not found", "NOT_FOUND")
	case errors.Is(err, workersessions.ErrObservationNotDirect):
		writeError(w, http.StatusNotFound, "worker session observation is not direct", "NOT_FOUND")
	case errors.Is(err, workersessions.ErrObservationProjectionUnavailable):
		writeError(w, http.StatusInternalServerError, "worker session observation is unavailable", string(factoryapi.ErrorResponseCodePROJECTIONUNAVAILABLE))
	case errors.Is(err, workersessions.ErrObservationRecordingCorrupt):
		writeError(w, http.StatusInternalServerError, "Worker Session recording history is corrupt", string(factoryapi.ErrorResponseCodeWORKERSESSIONRECORDINGCORRUPT))
	case errors.Is(err, workersessions.ErrObservationRecordingUnavailable):
		writeError(w, http.StatusServiceUnavailable, "Worker Session recording history is unavailable", string(factoryapi.ErrorResponseCodeWORKERSESSIONRECORDINGUNAVAILABLE))
	case errors.Is(err, context.Canceled), errors.Is(err, workersessions.ErrObservationCanceled):
		return
	case strings.Contains(err.Error(), "session id is required"), strings.Contains(err.Error(), "provider, kind, and id are required"):
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
	default:
		writeError(w, http.StatusInternalServerError, "failed to show Worker Session", "INTERNAL_ERROR")
	}
}

func (h *Handler) writeMappedTranscriptError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, workersessions.ErrInvalidSessionID),
		errors.Is(err, workersessions.ErrInvalidObservationIdentity):
		writeError(w, http.StatusBadRequest, "invalid Worker Session identity", "BAD_REQUEST")
	case errors.Is(err, workersessions.ErrObservationSessionNotFound):
		writeError(w, http.StatusNotFound, "worker session transcript not found", "NOT_FOUND")
	case errors.Is(err, workersessions.ErrObservationNotDirect):
		writeError(w, http.StatusNotFound, "worker session transcript is not direct", "NOT_FOUND")
	case errors.Is(err, workersessions.ErrObservationTranscriptActive):
		writeError(w, http.StatusConflict, "worker session is still active; transcript is not final", string(factoryapi.ErrorResponseCodeWORKERSESSIONTRANSCRIPTACTIVE))
	case errors.Is(err, workersessions.ErrObservationTranscriptUnavailable):
		writeError(w, http.StatusInternalServerError, "worker session transcript is unavailable", string(factoryapi.ErrorResponseCodeWORKERSESSIONTRANSCRIPTUNAVAILABLE))
	case errors.Is(err, workersessions.ErrObservationTranscriptProjectionUnavailable):
		writeError(w, http.StatusInternalServerError, "worker session transcript projection is unavailable", string(factoryapi.ErrorResponseCodeWORKERSESSIONTRANSCRIPTPROJECTIONUNAVAILABLE))
	case errors.Is(err, workersessions.ErrObservationRecordingCorrupt):
		writeError(w, http.StatusInternalServerError, "Worker Session recording history is corrupt", string(factoryapi.ErrorResponseCodeWORKERSESSIONRECORDINGCORRUPT))
	case errors.Is(err, workersessions.ErrObservationRecordingUnavailable):
		writeError(w, http.StatusServiceUnavailable, "Worker Session recording history is unavailable", string(factoryapi.ErrorResponseCodeWORKERSESSIONRECORDINGUNAVAILABLE))
	case errors.Is(err, context.Canceled), errors.Is(err, workersessions.ErrObservationCanceled):
		return
	case strings.Contains(err.Error(), "session id is required"), strings.Contains(err.Error(), "provider, kind, and id are required"):
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
	default:
		writeError(w, http.StatusInternalServerError, "failed to read Worker Session transcript", "INTERNAL_ERROR")
	}
}

func (h *Handler) writeMappedStreamError(w http.ResponseWriter, err error) {
	if writeMappedStreamCursorError(w, err) {
		return
	}
	switch {
	case errors.Is(err, workersessions.ErrInvalidSessionID),
		errors.Is(err, workersessions.ErrInvalidObservationIdentity):
		writeError(w, http.StatusBadRequest, "invalid Worker Session identity", "BAD_REQUEST")
	case errors.Is(err, workersessions.ErrObservationSessionNotFound):
		writeError(w, http.StatusNotFound, "worker session observation not found", "NOT_FOUND")
	case errors.Is(err, workersessions.ErrObservationNotDirect):
		writeError(w, http.StatusNotFound, "worker session observation is not direct", "NOT_FOUND")
	case errors.Is(err, workersessions.ErrObservationProjectionUnavailable):
		writeError(w, http.StatusInternalServerError, "worker session observation is unavailable", string(factoryapi.ErrorResponseCodePROJECTIONUNAVAILABLE))
	case errors.Is(err, workersessions.ErrObservationRecordingCorrupt):
		writeError(w, http.StatusInternalServerError, "Worker Session recording history is corrupt", string(factoryapi.ErrorResponseCodeWORKERSESSIONRECORDINGCORRUPT))
	case errors.Is(err, workersessions.ErrObservationRecordingUnavailable):
		writeError(w, http.StatusServiceUnavailable, "Worker Session recording history is unavailable", string(factoryapi.ErrorResponseCodeWORKERSESSIONRECORDINGUNAVAILABLE))
	case errors.Is(err, workersessions.ErrObservationSourceUnavailable), errors.Is(err, workersessions.ErrObservationSourceGap), errors.Is(err, workersessions.ErrObservationSourceClosed):
		writeError(w, http.StatusInternalServerError, "Worker Session event stream is unavailable", "WORKER_SESSION_STREAM_UNAVAILABLE")
	case errors.Is(err, context.Canceled), errors.Is(err, workersessions.ErrObservationCanceled):
		return
	case strings.Contains(err.Error(), "session id is required"), strings.Contains(err.Error(), "provider, kind, and id are required"):
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
	default:
		writeError(w, http.StatusInternalServerError, "failed to stream Worker Session events", "INTERNAL_ERROR")
	}
}

func writeMappedStreamCursorError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, workersessions.ErrInvalidObservationCursor):
		writeError(w, http.StatusBadRequest, "invalid Worker Session event cursor", string(factoryapi.ErrorResponseCodeWORKERSESSIONEVENTCURSORINVALID))
	case errors.Is(err, workersessions.ErrObservationCursorForeign):
		writeError(w, http.StatusBadRequest, "Worker Session event cursor belongs to another Worker Session", string(factoryapi.ErrorResponseCodeWORKERSESSIONEVENTCURSORFOREIGN))
	case errors.Is(err, workersessions.ErrObservationCursorFuture):
		writeError(w, http.StatusBadRequest, "Worker Session event cursor is ahead of available history", string(factoryapi.ErrorResponseCodeWORKERSESSIONEVENTCURSORFUTURE))
	case errors.Is(err, workersessions.ErrObservationCursorStale):
		writeError(w, http.StatusBadRequest, "Worker Session event cursor no longer names retained history", string(factoryapi.ErrorResponseCodeWORKERSESSIONEVENTCURSORSTALE))
	case errors.Is(err, workersessions.ErrObservationCursorUnavailable):
		writeError(w, http.StatusBadRequest, "Worker Session event cursor stream generation is unavailable", string(factoryapi.ErrorResponseCodeWORKERSESSIONEVENTCURSORUNAVAILABLE))
	default:
		return false
	}
	return true
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil && h.logger != nil {
		h.logger.Error("encode Worker Sessions response failed", zap.Error(err))
	}
}

func writeError(w http.ResponseWriter, status int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
		Message: message,
		Family:  errorFamilyForStatus(status),
		Code:    factoryapi.ErrorResponseCode(code),
	})
}

func errorFamilyForStatus(status int) factoryapi.ErrorFamily {
	switch status {
	case http.StatusBadRequest:
		return factoryapi.ErrorFamilyBadRequest
	case http.StatusConflict:
		return factoryapi.ErrorFamilyConflict
	case http.StatusNotFound:
		return factoryapi.ErrorFamilyNotFound
	default:
		return factoryapi.ErrorFamilyInternalServerError
	}
}
