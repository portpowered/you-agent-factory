package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	httpcompat "github.com/portpowered/infinite-you/pkg/transports/http/compat"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

const workerSessionsHTTPBoundary = "worker_sessions.http"

// Handler owns request decoding, Worker Sessions root invocation, error
// mapping, and response encoding. Route registration remains top-level.
type Handler struct {
	adapter *Adapter
	logger  *zap.Logger
}

// NewHandler constructs the Worker Sessions HTTP handler.
func NewHandler(adapter *Adapter, logger *zap.Logger) *Handler {
	if adapter == nil || logger == nil {
		return nil
	}
	return &Handler{adapter: adapter, logger: logger}
}

// StartWorkerSession handles the process-scoped asynchronous Worker Session
// start. Decode and mapping failures are rejected before the Worker Sessions
// root can reserve an identity; a successful call returns at the root's
// admission barrier and never waits for terminal output.
func (h *Handler) StartWorkerSession(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.adapter == nil {
		writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	if r == nil || r.Body == nil {
		writeError(w, http.StatusBadRequest, "request payload is required", "BAD_REQUEST")
		return
	}
	decoded, err := decodeWorkerSessionStartRequestWithDiagnostics(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	response, err := h.adapter.StartWorkerSession(r.Context(), decoded.Value)
	if err != nil {
		h.writeMappedStartError(w, err)
		return
	}
	h.writeCompatibilityWarning(w, "start_worker_session", decoded.Diagnostics.Paths())
	h.writeJSON(w, http.StatusAccepted, response)
}

// ContinueWorkerSession handles one source-addressed asynchronous Worker
// Session continuation. The source identity is supplied by the generated
// route, while the server resolves the exact recorded Provider Session.
func (h *Handler) ContinueWorkerSession(
	w http.ResponseWriter,
	r *http.Request,
	sourceWorkerSessionID factoryapi.WorkerSessionID,
) {
	if h == nil || h.adapter == nil {
		writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	if strings.TrimSpace(string(sourceWorkerSessionID)) == "" {
		writeError(w, http.StatusBadRequest, "worker session id is required", "BAD_REQUEST")
		return
	}
	if r == nil || r.Body == nil {
		writeError(w, http.StatusBadRequest, "request payload is required", "BAD_REQUEST")
		return
	}
	decoded, err := decodeWorkerSessionContinueRequestWithDiagnostics(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid continuation request payload", "BAD_REQUEST")
		return
	}
	response, err := h.adapter.ContinueWorkerSession(r.Context(), string(sourceWorkerSessionID), decoded.Value)
	if err != nil {
		h.writeMappedContinueError(w, err)
		return
	}
	h.writeCompatibilityWarning(w, "continue_worker_session", decoded.Diagnostics.Paths())
	h.writeJSON(w, http.StatusAccepted, response)
}

func (h *Handler) writeCompatibilityWarning(w http.ResponseWriter, operation string, paths []string) {
	httpcompat.ApplyWarning(w, h.logger, workerSessionsHTTPBoundary, operation, paths)
}

// ListWorkerSessions handles the top-level Worker Session observation list.
// Query binding is intentionally translated into the service-owned request so
// scope, lifecycle filters, and cursor validation remain one policy boundary.
func (h *Handler) ListWorkerSessions(
	w http.ResponseWriter,
	r *http.Request,
	params factoryapi.ListWorkerSessionsParams,
) {
	if h == nil || h.adapter == nil {
		writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	if r == nil {
		writeError(w, http.StatusBadRequest, "request is required", "BAD_REQUEST")
		return
	}
	scope := ""
	if params.Scope != nil {
		scope = string(*params.Scope)
	}
	var states []string
	if params.State != nil {
		states = make([]string, 0, len(*params.State))
		for _, state := range *params.State {
			states = append(states, string(state))
		}
	}
	var maxResults *int
	if params.Limit != nil {
		value := int(*params.Limit)
		if value <= 0 {
			h.writeMappedError(w, workersessions.ErrInvalidObservationPagination)
			return
		}
		maxResults = &value
	} else if params.MaxResults != nil {
		value := *params.MaxResults
		maxResults = &value
	}
	var nextToken *string
	if params.NextToken != nil {
		value := string(*params.NextToken)
		nextToken = &value
	}
	response, err := h.adapter.ListTopLevelWorkerSessions(r.Context(), scope, states, maxResults, nextToken)
	if err != nil {
		h.writeMappedError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

// GetWorkerSessionObservationByWorkerSessionId handles the top-level identity
// detail operation. Provider Session tuple fields are deliberately absent.
func (h *Handler) GetWorkerSessionObservationByWorkerSessionId(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
) {
	if h == nil || h.adapter == nil {
		writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	if r == nil {
		writeError(w, http.StatusBadRequest, "request is required", "BAD_REQUEST")
		return
	}
	response, err := h.adapter.GetTopLevelWorkerSessionObservation(r.Context(), string(workerSessionID))
	if err != nil {
		h.writeMappedObservationError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

// ReadWorkerSessionTranscriptByWorkerSessionId handles top-level transcript
// projection by Worker Session identity.
func (h *Handler) ReadWorkerSessionTranscriptByWorkerSessionId(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
) {
	if h == nil || h.adapter == nil {
		writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	if r == nil {
		writeError(w, http.StatusBadRequest, "request is required", "BAD_REQUEST")
		return
	}
	response, err := h.adapter.ReadTopLevelWorkerSessionTranscript(r.Context(), string(workerSessionID))
	if err != nil {
		h.writeMappedTranscriptError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

// ListWorkerSessionsBySessionId handles the session-scoped Worker Sessions
// list operation.
func (h *Handler) ListWorkerSessionsBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.ListWorkerSessionsBySessionIdParams,
) {
	if h == nil || h.adapter == nil {
		writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	if strings.TrimSpace(string(sessionID)) == "" {
		writeError(w, http.StatusBadRequest, "session id is required", "BAD_REQUEST")
		return
	}
	workID := strings.TrimSpace(params.WorkId)
	if workID == "" {
		writeError(w, http.StatusBadRequest, "workId is required", "BAD_REQUEST")
		return
	}
	if r == nil {
		writeError(w, http.StatusBadRequest, "request is required", "BAD_REQUEST")
		return
	}
	response, err := h.adapter.ListWorkerSessions(r.Context(), string(sessionID), workID)
	if err != nil {
		h.writeMappedError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

// GetWorkerSessionObservationBySessionId handles the session-scoped exact
// Provider Session lookup operation.
func (h *Handler) GetWorkerSessionObservationBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.GetWorkerSessionObservationBySessionIdParams,
) {
	if h == nil || h.adapter == nil {
		writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	if strings.TrimSpace(string(sessionID)) == "" {
		writeError(w, http.StatusBadRequest, "session id is required", "BAD_REQUEST")
		return
	}
	if r == nil {
		writeError(w, http.StatusBadRequest, "request is required", "BAD_REQUEST")
		return
	}
	provider, kind, id := string(params.Provider), string(params.Kind), strings.TrimSpace(params.Id)
	if provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required", "BAD_REQUEST")
		return
	}
	if kind == "" {
		writeError(w, http.StatusBadRequest, "kind is required", "BAD_REQUEST")
		return
	}
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required", "BAD_REQUEST")
		return
	}
	if provider != string(providers.IDCodex) && provider != string(providers.IDCursor) {
		writeError(w, http.StatusBadRequest, "unsupported provider", string(factoryapi.ErrorResponseCodePROVIDERUNSUPPORTED))
		return
	}
	if kind != providers.SessionIDKind {
		writeError(w, http.StatusBadRequest, "unsupported session kind", string(factoryapi.ErrorResponseCodeSESSIONKINDUNSUPPORTED))
		return
	}
	response, err := h.adapter.GetWorkerSessionObservation(r.Context(), string(sessionID), provider, kind, id)
	if err != nil {
		h.writeMappedObservationError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

// GetWorkerSessionObservationByFactorySessionAndWorkerSessionId handles the canonical
// session-scoped Worker Session identity lookup. It intentionally has no
// provider query parameters: provider association is optional enrichment.
func (h *Handler) GetWorkerSessionObservationByFactorySessionAndWorkerSessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	workerSessionID factoryapi.WorkerSessionID,
) {
	if h == nil || h.adapter == nil {
		writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	if strings.TrimSpace(string(sessionID)) == "" {
		writeError(w, http.StatusBadRequest, "session id is required", "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(string(workerSessionID)) == "" {
		writeError(w, http.StatusBadRequest, "worker session id is required", "BAD_REQUEST")
		return
	}
	if r == nil {
		writeError(w, http.StatusBadRequest, "request is required", "BAD_REQUEST")
		return
	}
	response, err := h.adapter.GetWorkerSessionObservationByWorkerSessionID(
		r.Context(), string(sessionID), string(workerSessionID),
	)
	if err != nil {
		h.writeMappedObservationError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

// ReadWorkerSessionTranscriptBySessionId handles the session-scoped finished
// Worker Session transcript operation.
func (h *Handler) ReadWorkerSessionTranscriptBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.ReadWorkerSessionTranscriptBySessionIdParams,
) {
	if h == nil || h.adapter == nil {
		writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	if strings.TrimSpace(string(sessionID)) == "" {
		writeError(w, http.StatusBadRequest, "session id is required", "BAD_REQUEST")
		return
	}
	if r == nil {
		writeError(w, http.StatusBadRequest, "request is required", "BAD_REQUEST")
		return
	}
	provider, kind, id := string(params.Provider), string(params.Kind), strings.TrimSpace(params.Id)
	if provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required", "BAD_REQUEST")
		return
	}
	if kind == "" {
		writeError(w, http.StatusBadRequest, "kind is required", "BAD_REQUEST")
		return
	}
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required", "BAD_REQUEST")
		return
	}
	if provider != string(providers.IDCodex) && provider != string(providers.IDCursor) {
		writeError(w, http.StatusBadRequest, "unsupported provider", string(factoryapi.ErrorResponseCodePROVIDERUNSUPPORTED))
		return
	}
	if kind != providers.SessionIDKind {
		writeError(w, http.StatusBadRequest, "unsupported session kind", string(factoryapi.ErrorResponseCodeSESSIONKINDUNSUPPORTED))
		return
	}
	response, err := h.adapter.ReadWorkerSessionTranscript(r.Context(), string(sessionID), provider, kind, id)
	if err != nil {
		h.writeMappedTranscriptError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

// ReadWorkerSessionTranscriptByFactorySessionAndWorkerSessionId handles the canonical
// Worker-ID transcript/history read. Provider-native identity is resolved by
// Worker Sessions when available and is never accepted from the caller here.
func (h *Handler) ReadWorkerSessionTranscriptByFactorySessionAndWorkerSessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	workerSessionID factoryapi.WorkerSessionID,
) {
	if h == nil || h.adapter == nil {
		writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	if strings.TrimSpace(string(sessionID)) == "" {
		writeError(w, http.StatusBadRequest, "session id is required", "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(string(workerSessionID)) == "" {
		writeError(w, http.StatusBadRequest, "worker session id is required", "BAD_REQUEST")
		return
	}
	if r == nil {
		writeError(w, http.StatusBadRequest, "request is required", "BAD_REQUEST")
		return
	}
	response, err := h.adapter.ReadWorkerSessionTranscriptByWorkerSessionID(
		r.Context(), string(sessionID), string(workerSessionID),
	)
	if err != nil {
		h.writeMappedTranscriptError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

// StreamWorkerSessionEventsBySessionId writes one Server-Sent Events data
// frame per retained/live Worker Session event. A terminal frame closes a
// successful stream; source failures remain explicit frames after the HTTP
// response has opened.
func (h *Handler) StreamWorkerSessionEventsBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.StreamWorkerSessionEventsBySessionIdParams,
) {
	provider, kind, id, flusher, ok := h.prepareStreamRequest(w, r, sessionID, params)
	if !ok {
		return
	}

	replayOnly := params.ReplayOnly != nil && *params.ReplayOnly
	cursor, err := WorkerSessionObservationCursorFromAPI(params.AfterPosition, params.AfterSequence, params.StreamGenerationId)
	if err != nil {
		h.writeMappedStreamError(w, err)
		return
	}
	observation, subscription, err := h.adapter.StreamWorkerSessionEventsWithCursor(r.Context(), string(sessionID), provider, kind, id, replayOnly, cursor)
	if err != nil {
		h.writeMappedStreamError(w, err)
		return
	}
	defer subscription.Close()
	h.writeWorkerSessionStream(w, r, flusher, observation, subscription, replayOnly)
}

// StreamWorkerSessionEventsByWorkerSessionId writes a provider-neutral
// retained/live stream addressed by the canonical Worker Session identity.
// This remains usable when the provider emitted no native session reference.
func (h *Handler) StreamWorkerSessionEventsByWorkerSessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	workerSessionID factoryapi.WorkerSessionID,
	params factoryapi.StreamWorkerSessionEventsByWorkerSessionIdParams,
) {
	flusher, ok := h.prepareWorkerSessionIDStreamRequest(w, r, sessionID, workerSessionID)
	if !ok {
		return
	}

	replayOnly := params.ReplayOnly != nil && *params.ReplayOnly
	cursor, err := WorkerSessionObservationCursorFromAPI(params.AfterPosition, params.AfterSequence, params.StreamGenerationId)
	if err != nil {
		h.writeMappedStreamError(w, err)
		return
	}
	observation, subscription, err := h.adapter.StreamWorkerSessionEventsByWorkerSessionIDWithCursor(
		r.Context(), string(sessionID), string(workerSessionID), replayOnly, cursor,
	)
	if err != nil {
		h.writeMappedStreamError(w, err)
		return
	}
	defer subscription.Close()
	h.writeWorkerSessionStream(w, r, flusher, observation, subscription, replayOnly)
}

// StreamWorkerSessionEventsByTopLevelWorkerSessionId writes the top-level
// retained/live stream addressed only by the stable Worker Session identity.
func (h *Handler) StreamWorkerSessionEventsByTopLevelWorkerSessionId(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
	params factoryapi.StreamWorkerSessionEventsByTopLevelWorkerSessionIdParams,
) {
	flusher, ok := h.prepareTopLevelWorkerSessionIDStreamRequest(w, r, workerSessionID)
	if !ok {
		return
	}
	replayOnly := params.ReplayOnly != nil && *params.ReplayOnly
	observation, subscription, err := h.adapter.StreamTopLevelWorkerSessionEvents(r.Context(), string(workerSessionID), replayOnly)
	if err != nil {
		h.writeMappedStreamError(w, err)
		return
	}
	defer subscription.Close()
	h.writeWorkerSessionStream(w, r, flusher, observation, subscription, replayOnly)
}

func (h *Handler) prepareWorkerSessionIDStreamRequest(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	workerSessionID factoryapi.WorkerSessionID,
) (http.Flusher, bool) {
	if h == nil || h.adapter == nil {
		writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	if strings.TrimSpace(string(sessionID)) == "" {
		writeError(w, http.StatusBadRequest, "session id is required", "BAD_REQUEST")
		return nil, false
	}
	if strings.TrimSpace(string(workerSessionID)) == "" {
		writeError(w, http.StatusBadRequest, "worker session id is required", "BAD_REQUEST")
		return nil, false
	}
	if r == nil {
		writeError(w, http.StatusBadRequest, "request is required", "BAD_REQUEST")
		return nil, false
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Worker Session streaming is unavailable", "WORKER_SESSION_STREAM_UNAVAILABLE")
		return nil, false
	}
	return flusher, true
}

func (h *Handler) prepareTopLevelWorkerSessionIDStreamRequest(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
) (http.Flusher, bool) {
	if h == nil || h.adapter == nil {
		writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	if strings.TrimSpace(string(workerSessionID)) == "" {
		writeError(w, http.StatusBadRequest, "worker session id is required", "BAD_REQUEST")
		return nil, false
	}
	if r == nil {
		writeError(w, http.StatusBadRequest, "request is required", "BAD_REQUEST")
		return nil, false
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Worker Session streaming is unavailable", "WORKER_SESSION_STREAM_UNAVAILABLE")
		return nil, false
	}
	return flusher, true
}

func (h *Handler) prepareStreamRequest(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.StreamWorkerSessionEventsBySessionIdParams,
) (string, string, string, http.Flusher, bool) {
	if h == nil || h.adapter == nil {
		writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return "", "", "", nil, false
	}
	if strings.TrimSpace(string(sessionID)) == "" {
		writeError(w, http.StatusBadRequest, "session id is required", "BAD_REQUEST")
		return "", "", "", nil, false
	}
	if r == nil {
		writeError(w, http.StatusBadRequest, "request is required", "BAD_REQUEST")
		return "", "", "", nil, false
	}
	provider, kind, id := string(params.Provider), string(params.Kind), strings.TrimSpace(params.Id)
	if provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required", "BAD_REQUEST")
		return "", "", "", nil, false
	}
	if kind == "" {
		writeError(w, http.StatusBadRequest, "kind is required", "BAD_REQUEST")
		return "", "", "", nil, false
	}
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required", "BAD_REQUEST")
		return "", "", "", nil, false
	}
	if provider != string(providers.IDCodex) && provider != string(providers.IDCursor) {
		writeError(w, http.StatusBadRequest, "unsupported provider", string(factoryapi.ErrorResponseCodePROVIDERUNSUPPORTED))
		return "", "", "", nil, false
	}
	if kind != providers.SessionIDKind {
		writeError(w, http.StatusBadRequest, "unsupported session kind", string(factoryapi.ErrorResponseCodeSESSIONKINDUNSUPPORTED))
		return "", "", "", nil, false
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Worker Session streaming is unavailable", "WORKER_SESSION_STREAM_UNAVAILABLE")
		return "", "", "", nil, false
	}
	return provider, kind, id, flusher, true
}

func (h *Handler) writeWorkerSessionStream(
	w http.ResponseWriter,
	r *http.Request,
	flusher http.Flusher,
	observation factoryapi.WorkerSessionObservation,
	subscription workersessions.ObservationSubscription,
	replayOnly bool,
) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		if h.writeWorkerSessionDelivery(w, flusher, observation, subscription.Next(r.Context()), replayOnly) {
			return
		}
	}
}

func (h *Handler) writeWorkerSessionDelivery(
	w http.ResponseWriter,
	flusher http.Flusher,
	observation factoryapi.WorkerSessionObservation,
	delivery workersessions.ObservationDelivery,
	replayOnly bool,
) bool {
	switch delivery.Kind {
	case workersessions.ObservationDeliveryRecord,
		workersessions.ObservationDeliveryTerminal,
		workersessions.ObservationDeliveryTerminalReplay:
		if !h.writeWorkerSessionFrame(w, flusher, workerSessionEventFrame(observation, delivery), "event stream") {
			return true
		}
		return delivery.Summary != nil || (!replayOnly && (delivery.Kind == workersessions.ObservationDeliveryTerminal || delivery.Kind == workersessions.ObservationDeliveryTerminalReplay))
	case workersessions.ObservationDeliveryReplaySummary:
		h.writeWorkerSessionFrame(w, flusher, workerSessionReplaySummaryFrame(observation, delivery.Summary), "replay summary")
		return true
	case workersessions.ObservationDeliveryCanceled:
		return true
	case workersessions.ObservationDeliverySourceFailure, workersessions.ObservationDeliveryClosed:
		code, message := workerSessionStreamFailure(delivery.Err)
		h.writeWorkerSessionFrame(w, flusher, workerSessionFailureFrame(observation, code, message), "source failure")
		return true
	default:
		code, message := workerSessionStreamFailure(fmt.Errorf("unknown Worker Session delivery kind %q", delivery.Kind))
		h.writeWorkerSessionFrame(w, flusher, workerSessionFailureFrame(observation, code, message), "unknown delivery failure")
		return true
	}
}

func (h *Handler) writeWorkerSessionFrame(
	w http.ResponseWriter,
	flusher http.Flusher,
	frame workerSessionEventFramePayload,
	label string,
) bool {
	if err := writeSSEFrame(w, flusher, frame); err == nil {
		return true
	} else if h.logger != nil {
		h.logger.Debug("write Worker Session "+label+" failed", zap.Error(err))
	}
	return false
}

type workerSessionEventFramePayload struct {
	Delivery              workersessions.ObservationDeliveryKind        `json:"delivery"`
	WorkerSessionID       string                                        `json:"workerSessionId"`
	FactorySessionID      *string                                       `json:"factorySessionId,omitempty"`
	ProviderSession       *factoryapi.WorkerSessionProviderSessionRef   `json:"providerSession"`
	WorkIDs               []string                                      `json:"workIds"`
	Event                 *workerSessionEventRecordPayload              `json:"event"`
	ErrorCode             *string                                       `json:"errorCode"`
	ErrorMessage          *string                                       `json:"errorMessage"`
	ReplaySummary         *factoryapi.WorkerSessionReplaySummary        `json:"replaySummary,omitempty"`
	RecordingHealth       *factoryapi.WorkerSessionEventRecordingHealth `json:"recordingHealth,omitempty"`
	RecordingHealthReason *string                                       `json:"recordingHealthReason,omitempty"`
}

type workerSessionEventRecordPayload struct {
	Cursor         factoryapi.WorkerSessionEventCursor `json:"cursor"`
	Position       uint64                              `json:"position"`
	SourceType     string                              `json:"sourceType"`
	SourceID       string                              `json:"sourceId"`
	SourceSequence uint64                              `json:"sourceSequence"`
	SourceEventID  string                              `json:"sourceEventId"`
	SchemaID       string                              `json:"schemaId"`
	Payload        json.RawMessage                     `json:"payload"`
}
