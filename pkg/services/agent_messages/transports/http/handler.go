// Package http owns Agent Message HTTP decoding and response publication.
// Messaging retains authorization, validation and all durable transitions;
// this boundary never reads a journal or chooses a sender from the payload.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/agentmessage"
)

// The envelope accommodates JSON escaping of the 8KiB UTF-8 body while
// bounding decoding independently of the service's unescaped body limit.
const maxSendEnvelopeBytes = 64 * 1024

// The generated anyOf model has a custom UnmarshalJSON that bypasses the
// decoder's unknown-field check. A defined type retains its authored fields
// while allowing the boundary's strict decoder to reject forged extensions.
type strictSendRequest factoryapi.AgentMessageSendRequest

type Handler struct {
	service agentmessages.Service
	logger  logging.Logger
	mapper  agentmessage.Mapper
}

func NewHandler(service agentmessages.Service, logger logging.Logger) *Handler {
	if service == nil || logger == nil {
		return nil
	}
	return &Handler{service: service, logger: logger}
}

func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	caller, err := apisurface.WorkerSessionCallerFromHeaders(r.Header)
	if err != nil {
		h.writeError(w, err)
		return
	}
	var request *strictSendRequest
	if r.Body == nil {
		h.writeError(w, agentmessages.ErrBadRequest)
		return
	}
	payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSendEnvelopeBytes))
	if err != nil || !utf8.Valid(payload) {
		h.writeError(w, agentmessages.ErrBadRequest)
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || request == nil || decoder.Decode(new(any)) != io.EOF {
		h.writeError(w, agentmessages.ErrBadRequest)
		return
	}
	scope, err := singleton(r.URL.Query(), "factorySessionId")
	if err != nil {
		h.writeError(w, err)
		return
	}
	message, err := h.service.Send(r.Context(), h.mapper.SendRequest(factoryapi.AgentMessageSendRequest(*request)), caller, scope)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusAccepted, factoryapi.AgentMessageSendResponse{Message: h.mapper.Message(message)})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request, messageID string) {
	caller, err := apisurface.WorkerSessionCallerFromHeaders(r.Header)
	if err != nil {
		h.writeError(w, err)
		return
	}
	message, err := h.service.Get(r.Context(), agentmessages.GetRequest{MessageID: messageID, Caller: caller})
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.mapper.Message(message))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	caller, err := apisurface.WorkerSessionCallerFromHeaders(r.Header)
	if err != nil {
		h.writeError(w, err)
		return
	}
	request, follow, err := listRequest(r.URL.Query())
	if err != nil {
		h.writeError(w, err)
		return
	}
	request.Caller = caller
	if follow {
		h.follow(w, r, request)
		return
	}
	page, err := h.service.List(r.Context(), request)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.mapper.Page(page))
}

func (h *Handler) writeError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	status, response := h.mapper.Error(err)
	var limit *agentmessages.LimitError
	if errors.As(err, &limit) && limit != nil && limit.RetryAfterSeconds > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(limit.RetryAfterSeconds))
	}
	h.writeJSON(w, status, response)
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if json.NewEncoder(w).Encode(value) != nil {
		// Encoding/write errors can embed response bytes. Diagnose the boundary
		// without attaching an error, message content or caller credentials.
		h.logger.Warn("Agent Message HTTP response unavailable")
	}
}
