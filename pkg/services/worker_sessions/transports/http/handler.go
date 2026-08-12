package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

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
	request, err := decodeWorkerSessionStartRequest(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	response, err := h.adapter.StartWorkerSession(r.Context(), request)
	if err != nil {
		h.writeMappedStartError(w, err)
		return
	}
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
	request, err := decodeWorkerSessionContinueRequest(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid continuation request payload", "BAD_REQUEST")
		return
	}
	response, err := h.adapter.ContinueWorkerSession(r.Context(), string(sourceWorkerSessionID), request)
	if err != nil {
		h.writeMappedContinueError(w, err)
		return
	}
	h.writeJSON(w, http.StatusAccepted, response)
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
	if params.MaxResults != nil {
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

// GetWorkerSessionObservationByWorkerSessionId handles the canonical
// session-scoped Worker Session identity lookup. It intentionally has no
// provider query parameters: provider association is optional enrichment.
func (h *Handler) GetWorkerSessionObservationByWorkerSessionId(
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

// ReadWorkerSessionTranscriptByWorkerSessionId handles the canonical
// Worker-ID transcript/history read. Provider-native identity is resolved by
// Worker Sessions when available and is never accepted from the caller here.
func (h *Handler) ReadWorkerSessionTranscriptByWorkerSessionId(
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
	observation, subscription, err := h.adapter.StreamWorkerSessionEvents(r.Context(), string(sessionID), provider, kind, id, replayOnly)
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
	observation, subscription, err := h.adapter.StreamWorkerSessionEventsByWorkerSessionID(
		r.Context(), string(sessionID), string(workerSessionID), replayOnly,
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
		return !replayOnly && (delivery.Kind == workersessions.ObservationDeliveryTerminal || delivery.Kind == workersessions.ObservationDeliveryTerminalReplay)
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
	ProviderSession       factoryapi.WorkerSessionProviderSessionRef    `json:"providerSession"`
	WorkIDs               []string                                      `json:"workIds"`
	Event                 *workerSessionEventRecordPayload              `json:"event"`
	ErrorCode             *string                                       `json:"errorCode"`
	ErrorMessage          *string                                       `json:"errorMessage"`
	ReplaySummary         *factoryapi.WorkerSessionReplaySummary        `json:"replaySummary,omitempty"`
	RecordingHealth       *factoryapi.WorkerSessionEventRecordingHealth `json:"recordingHealth,omitempty"`
	RecordingHealthReason *string                                       `json:"recordingHealthReason,omitempty"`
}

type workerSessionEventRecordPayload struct {
	Position       uint64          `json:"position"`
	SourceType     string          `json:"sourceType"`
	SourceID       string          `json:"sourceId"`
	SourceSequence uint64          `json:"sourceSequence"`
	SourceEventID  string          `json:"sourceEventId"`
	SchemaID       string          `json:"schemaId"`
	Payload        json.RawMessage `json:"payload"`
}

func workerSessionEventFrame(observation factoryapi.WorkerSessionObservation, delivery workersessions.ObservationDelivery) workerSessionEventFramePayload {
	return workerSessionEventFrameWithIdentity(observation, delivery.Kind, &workerSessionEventRecordPayload{
		Position: delivery.Event.Position, SourceType: delivery.Event.SourceType, SourceID: delivery.Event.SourceID,
		SourceSequence: delivery.Event.SourceSequence, SourceEventID: delivery.Event.SourceEventID,
		SchemaID: delivery.Event.SchemaID, Payload: append(json.RawMessage(nil), delivery.Event.Payload...),
	})
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
		ReplaySummary: &factoryapi.WorkerSessionReplaySummary{
			Kind:          "replay-summary",
			Complete:      summary.Complete,
			Reason:        summary.Reason,
			EventsEmitted: int64(summary.EventsEmitted),
		},
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
	providerSession := factoryapi.WorkerSessionProviderSessionRef{}
	if observation.ProviderSession != nil {
		providerSession = *observation.ProviderSession
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

func workerSessionProviderSessionRef(observation factoryapi.WorkerSessionObservation) factoryapi.WorkerSessionProviderSessionRef {
	if observation.ProviderSession == nil {
		return factoryapi.WorkerSessionProviderSessionRef{}
	}
	return *observation.ProviderSession
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

func (h *Handler) writeMappedInterruptError(
	w http.ResponseWriter,
	err error,
	sourceWorkerSessionID string,
	request factoryapi.WorkerSessionInterruptRequest,
) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
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
	switch {
	case errors.Is(err, workersessions.ErrInvalidInterruptRequestID),
		errors.Is(err, workersessions.ErrInvalidInterruptLineage),
		errors.Is(err, workersessions.ErrInvalidInterruptMessage),
		errors.Is(err, workersessions.ErrInterruptValidation):
		writeInterruptError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid Worker Session interrupt request", result.Phase, result)
	case errors.Is(err, workersessions.ErrInterruptSourceNotFound):
		writeInterruptError(w, http.StatusNotFound, "NOT_FOUND", "Worker Session interrupt source not found", result.Phase, result)
	case errors.Is(err, workersessions.ErrInterruptRequestIDConflict):
		writeInterruptError(w, http.StatusConflict, string(factoryapi.ErrorResponseCodeWORKERSESSIONINTERRUPTREQUESTIDCONFLICT), "Worker Session interrupt requestId was reused with different inputs", result.Phase, result)
	case errors.Is(err, workersessions.ErrInterruptSourceNotActive),
		errors.Is(err, workersessions.ErrInterruptSourceConflict),
		errors.Is(err, workersessions.ErrInterruptProviderSessionMissing),
		errors.Is(err, workersessions.ErrInterruptProviderSessionInvalid):
		writeInterruptError(w, http.StatusConflict, string(factoryapi.ErrorResponseCodeWORKERSESSIONINTERRUPTCONFLICT), "Worker Session interrupt conflicts with existing state", result.Phase, result)
	case errors.Is(err, workersessions.ErrInterruptSourceCancellation),
		errors.Is(err, workersessions.ErrInterruptSourceCancellationFailed):
		writeInterruptError(w, http.StatusServiceUnavailable, string(factoryapi.ErrorResponseCodeWORKERSESSIONINTERRUPTSOURCECANCELLATIONFAILED), "Workers could not cancel the Worker Session interrupt source", result.Phase, result)
	case errors.Is(err, workersessions.ErrInterruptSuccessorAdmission),
		errors.Is(err, workersessions.ErrInterruptSuccessorAdmissionFailed):
		writeInterruptError(w, http.StatusServiceUnavailable, string(factoryapi.ErrorResponseCodeWORKERSESSIONINTERRUPTSUCCESSORADMISSIONFAILED), "Workers could not admit the Worker Session interrupt successor", result.Phase, result)
	case errors.Is(err, workersessions.ErrInterruptExecutionUnavailable),
		errors.Is(err, workersessions.ErrInterruptServerStopping):
		writeInterruptError(w, http.StatusServiceUnavailable, string(factoryapi.ErrorResponseCodeWORKERSESSIONINTERRUPTADMISSIONFAILED), "Workers could not admit the Worker Session interrupt", result.Phase, result)
	default:
		writeInterruptError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to interrupt Worker Session", result.Phase, result)
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
