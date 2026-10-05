package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func (a *Adapter) WithLogsService(service workersessions.LogsService) *Adapter {
	if a == nil {
		return nil
	}
	clone := *a
	clone.logs = service
	return &clone
}

func (h *Handler) ReadWorkerSessionLogs(w http.ResponseWriter, r *http.Request, id factoryapi.WorkerSessionID, params factoryapi.ReadWorkerSessionLogsParams) {
	if h == nil || h.adapter == nil || h.adapter.logs == nil {
		writeError(w, http.StatusServiceUnavailable, "Worker Session recording is unavailable", "WORKER_SESSION_RECORDING_UNAVAILABLE")
		return
	}
	if r == nil {
		writeError(w, http.StatusBadRequest, "request is required", "BAD_REQUEST")
		return
	}
	req := workersessions.ReadLogsRequest{WorkerSessionID: string(id)}
	if params.Limit != nil {
		req.Limit = *params.Limit
		if req.Limit < 1 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 1000", "BAD_REQUEST")
			return
		}
	}
	if params.NextToken != nil {
		req.NextToken = *params.NextToken
	}
	page, err := h.adapter.logs.ReadLogs(r.Context(), req)
	if err != nil {
		h.writeLogsError(w, err)
		return
	}
	response, err := workerSessionLogPageToAPI(page)
	if err != nil {
		h.writeLogsError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

func (h *Handler) writeLogsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, workersessions.ErrInvalidSessionID), errors.Is(err, workersessions.ErrInvalidLogsRequest):
		writeError(w, http.StatusBadRequest, "invalid Worker Session logs request", "BAD_REQUEST")
	case errors.Is(err, workersessions.ErrObservationSessionNotFound):
		writeError(w, http.StatusNotFound, "Worker Session was not found", "WORKER_SESSION_NOT_FOUND")
	default:
		writeError(w, http.StatusServiceUnavailable, "Worker Session recording is unavailable", "WORKER_SESSION_RECORDING_UNAVAILABLE")
	}
}

func workerSessionLogPageToAPI(page workersessions.LogPage) (factoryapi.WorkerSessionLogPage, error) {
	result := factoryapi.WorkerSessionLogPage{
		WorkerSessionId: page.WorkerSessionID, RecordingGenerationId: page.RecordingGenerationID,
		CommittedPosition: int64(page.CommittedPosition), Health: factoryapi.WorkerSessionLogPageHealth(page.Health),
		Events: make([]factoryapi.WorkerSessionEvent, 0, len(page.Events)),
	}
	if page.NextToken != "" {
		result.NextToken = &page.NextToken
	}
	for _, event := range page.Events {
		var payload map[string]interface{}
		decoder := json.NewDecoder(bytes.NewReader(event.Payload))
		decoder.UseNumber()
		if decoder.Decode(&payload) != nil {
			return factoryapi.WorkerSessionLogPage{}, workersessions.ErrLogsUnavailable
		}
		frame := factoryapi.WorkerSessionEvent{
			WorkerSessionId: page.WorkerSessionID, WorkIds: append([]string{}, page.WorkIDs...),
			Delivery: factoryapi.WorkerSessionEventDelivery("RECORD"),
			Event: factoryapi.WorkerSessionEventRecord{
				Cursor: workerSessionEventCursor(event), Position: int64(event.Position),
				SourceType: event.SourceType, SourceId: event.SourceID, SourceSequence: int64(event.SourceSequence),
				SourceEventId: event.SourceEventID, SchemaId: event.SchemaID, Payload: payload, CapturedAt: event.CapturedAt,
			},
		}
		if page.FactorySessionID != "" {
			frame.FactorySessionId = &page.FactorySessionID
		}
		if event.Position == page.TerminalPosition {
			frame.Delivery = factoryapi.WorkerSessionEventDelivery("TERMINAL_REPLAY")
		}
		result.Events = append(result.Events, frame)
	}
	return result, nil
}
