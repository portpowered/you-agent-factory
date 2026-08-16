package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workhttp "github.com/portpowered/infinite-you/pkg/services/work/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

const (
	sessionEventStreamBackendScopeHeader      = factorysessionshttp.SessionEventStreamBackendScopeHeader
	sessionEventStreamLogicalSessionKeyHeader = factorysessionshttp.SessionEventStreamLogicalSessionKeyHeader
	sessionEventStreamFactorySessionHeader    = factorysessionshttp.SessionEventStreamFactorySessionHeader
	sessionEventStreamGenerationHeader        = factorysessionshttp.SessionEventStreamGenerationHeader
	submitWorkItemTypeMetadataKey             = workhttp.SubmitWorkItemTypeMetadataKey
	submitWorkFileNameMetadataKey             = workhttp.SubmitWorkFileNameMetadataKey
)

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func reconnectCursorFromParams(
	afterEventID *factoryapi.AfterEventId,
	afterSequence *factoryapi.AfterSequence,
) *interfaces.FactoryEventReconnectCursor {
	return factorysessionshttp.ReconnectCursorFromParams(afterEventID, afterSequence)
}

func workReadModelToGenerated(item work.ReadModel) factoryapi.Work {
	return workhttp.WorkReadModelToGenerated(item)
}

func decodeStrictJSON[T any](body io.Reader) (T, error) {
	return factorysessionshttp.DecodeStrictJSON[T](body)
}

func requestFieldValidationMessage(err error) (string, bool) {
	return factorysessionshttp.RequestFieldValidationMessage(err)
}

func submitWorkResponseFromResult(result work.WorkRequestSubmitResult, sessionID string) factoryapi.SubmitWorkResponse {
	return workhttp.SubmitWorkResponseFromResult(result, sessionID)
}

func (s *Server) getEvents(
	w http.ResponseWriter,
	r *http.Request,
	includeSessionHandshake bool,
	subscribe func(context.Context) (*interfaces.FactoryEventStream, error),
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "streaming unsupported", "INTERNAL_ERROR")
		return
	}
	stream, err := subscribe(r.Context())
	if err != nil {
		status, message, code := http.StatusInternalServerError, "failed to subscribe to factory events", "INTERNAL_ERROR"
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			status, message, code = http.StatusNotFound, "factory session not found", "NOT_FOUND"
		} else if errors.Is(err, apisurface.ErrInvalidEventReconnectCursor) || errors.Is(err, recordings.ErrReconnectCursorNotFound) {
			status, message, code = http.StatusBadRequest, "invalid event reconnect cursor", "BAD_REQUEST"
		}
		s.writeError(w, status, message, code)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set(factorysessionshttp.SessionEventStreamRetainedCountHeader, strconv.Itoa(len(stream.History)))
	if includeSessionHandshake {
		for name, value := range map[string]string{
			sessionEventStreamBackendScopeHeader:      stream.BackendScopeID,
			sessionEventStreamLogicalSessionKeyHeader: stream.LogicalSessionKeyID,
			sessionEventStreamFactorySessionHeader:    stream.FactorySessionID,
			sessionEventStreamGenerationHeader:        stream.StreamGenerationID,
		} {
			if value = strings.TrimSpace(value); value != "" {
				w.Header().Set(name, value)
			}
		}
	}
	write := func(event interfaces.FactoryEvent) error {
		apiEvent, err := apisurface.FactoryEventToAPI(event)
		if err != nil {
			return err
		}
		data, err := json.Marshal(apiEvent)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "data: %s\n\n", data)
		return err
	}
	for _, event := range stream.History {
		if err := write(event); err != nil {
			return
		}
		flusher.Flush()
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-stream.Events:
			if !ok {
				return
			}
			if err := write(event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
