// backendsizecheck:ignore-file this legacy API transport surface stays centralized until dedicated handler-splitting work lands.
// pkgmaintcheck:ignore-file-lines legacy API transport handlers still share generated-surface wiring; split by route family in dedicated follow-up work to avoid transport regressions.
package api

import (
	"context"
	"errors"
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/api/workstationprojection"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/apisurface/optional"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
)

var _ factoryapi.ServerInterface = (*Server)(nil)

// BuildFactoryWorldWorkstationRequestProjectionSlice delegates to the
// workstationprojection subpackage while preserving the historical pkg/api entrypoint.
func BuildFactoryWorldWorkstationRequestProjectionSlice(
	state interfaces.FactoryWorldState,
) factoryapi.FactoryWorldWorkstationRequestProjectionSlice {
	return workstationprojection.BuildFactoryWorldWorkstationRequestProjectionSlice(state)
}

func (s *Server) requireSessionRuntime(w http.ResponseWriter) (apisurface.SessionAPISurface, bool) {
	if s.sessionRuntime == nil {
		s.writeError(w, http.StatusInternalServerError, "session-scoped API is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.sessionRuntime, true
}

// GetEvents handles GET /events as a canonical factory event SSE stream.
func (s *Server) GetEvents(w http.ResponseWriter, r *http.Request) {
	s.getEvents(w, r, s.runtime.SubscribeFactoryEvents)
}

func (s *Server) GetEventsBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	s.getEvents(w, r, func(ctx context.Context) (*interfaces.FactoryEventStream, error) {
		return sessionRuntime.SubscribeFactoryEventsForSession(ctx, string(sessionID))
	})
}

func (s *Server) getEvents(
	w http.ResponseWriter,
	r *http.Request,
	subscribe func(context.Context) (*interfaces.FactoryEventStream, error),
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "streaming unsupported", "INTERNAL_ERROR")
		return
	}

	stream, err := subscribe(r.Context())
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		s.logger.Error("subscribe factory events failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to subscribe to factory events", "INTERNAL_ERROR")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	for _, event := range stream.History {
		if err := s.writeSSEDataJSON(w, event); err != nil {
			s.logger.Debug("write historical factory event failed", zap.Error(err))
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
			if err := s.writeSSEDataJSON(w, event); err != nil {
				s.logger.Debug("write live factory event failed", zap.Error(err))
				return
			}
			flusher.Flush()
		}
	}
}

func stringValue(value *string) string {
	return optional.StringValue(value)
}

func generatedWorkStateName(value *factoryapi.WorkState) string {
	if value == nil {
		return ""
	}
	return value.Name
}

func intValue(value *int) int {
	return optional.IntValue(value)
}

func stringSliceValue(values *[]string) []string {
	return optional.StringsValue(values)
}

func stringSlicePtrCopy(values []string) *[]string {
	return optional.CopiedStringsPtr(values)
}

func stringPtrIfNotEmpty(value string) *string {
	return optional.NonEmptyStringPtr(value)
}

func integerMapPtr(values map[string]int) *factoryapi.IntegerMap {
	if len(values) == 0 {
		return nil
	}
	converted := factoryapi.IntegerMap(values)
	return &converted
}

func stringMapPtr(values map[string]string) *factoryapi.StringMap {
	return optional.CopiedStringMapPtr(values)
}

func intPtrIfPositive(value int) *int {
	return optional.PositiveIntPtr(value)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func generatedStringMap(values *factoryapi.StringMap) map[string]string {
	return optional.StringMapValue(values)
}

func generatedSubmitRelations(values *[]factoryapi.SubmitRelation) []interfaces.Relation {
	if values == nil || len(*values) == 0 {
		return nil
	}
	relations := make([]interfaces.Relation, 0, len(*values))
	for _, relation := range *values {
		relations = append(relations, interfaces.Relation{
			Type:          interfaces.RelationType(relation.Type),
			TargetWorkID:  relation.TargetWorkId,
			RequiredState: stringValue(relation.RequiredState),
		})
	}
	return relations
}
