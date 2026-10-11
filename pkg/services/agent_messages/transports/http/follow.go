package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
)

func (h *Handler) follow(w http.ResponseWriter, r *http.Request, request agentmessages.ListRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeError(w, agentmessages.ErrStreamUnavailable)
		return
	}
	started := false
	err := h.service.Follow(r.Context(), request, func(observation agentmessages.Observation) error {
		if err := r.Context().Err(); err != nil {
			return err
		}
		mapped, err := h.mapper.Observation(observation)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(mapped)
		if err != nil {
			return agentmessages.ErrStreamUnavailable
		}
		if !started {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("X-Accel-Buffering", "no")
			w.WriteHeader(http.StatusOK)
			started = true
		}
		if _, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", encoded); err != nil {
			return agentmessages.ErrStreamUnavailable
		}
		flusher.Flush()
		return nil
	})
	if !started {
		if err != nil {
			h.writeError(w, err)
		} else {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			flusher.Flush()
		}
		return
	}
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	_, response := h.mapper.Error(err)
	encoded, encodeErr := json.Marshal(response)
	if encodeErr != nil {
		h.logger.Warn("Agent Message HTTP stream unavailable")
		return
	}
	if _, writeErr := fmt.Fprintf(w, "event: error\ndata: %s\n\n", encoded); writeErr != nil {
		h.logger.Warn("Agent Message HTTP stream unavailable")
		return
	}
	flusher.Flush()
}
