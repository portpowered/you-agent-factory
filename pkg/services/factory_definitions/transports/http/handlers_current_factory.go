package http

import (
	"io"
	"net/http"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorydefinition"
	"go.uber.org/zap"
)

// GetCurrentFactoryBySessionId handles GET /factory-sessions/{session_id}/factory by
// mapping the session identifier through the injected Definitions root.
func (s *Server) GetCurrentFactoryBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	root, ok := s.requireDefinitionsRoot(w)
	if !ok {
		return
	}
	if s.guardDefinitionsRequestContext(w, r) {
		return
	}

	factory, err := factorydefinition.GetCurrentFactoryForSession(r.Context(), root, string(sessionID))
	if err != nil {
		s.writeCurrentFactoryError(w, err, "get", zap.String("session_id", string(sessionID)))
		return
	}
	s.writeJSON(w, http.StatusOK, factory)
}

// SaveCurrentFactoryBySessionId handles PUT /factory-sessions/{session_id}/factory by
// decoding the submission payload and invoking the injected Definitions root.
func (s *Server) SaveCurrentFactoryBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	root, ok := s.requireDefinitionsRoot(w)
	if !ok {
		return
	}

	req, err := decodeSaveCurrentFactoryBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeErrorWithTargets(
				w,
				http.StatusBadRequest,
				message,
				"BAD_REQUEST",
				[]factoryapi.FactoryValidationTarget{
					apisurface.FactoryValidationTargetToAPI(interfaces.FormFactoryPayloadValidationTarget()),
				},
			)
			return
		}
		s.writeErrorWithTargets(
			w,
			http.StatusBadRequest,
			"invalid request payload",
			"BAD_REQUEST",
			[]factoryapi.FactoryValidationTarget{
				apisurface.FactoryValidationTargetToAPI(interfaces.FormFactoryPayloadValidationTarget()),
			},
		)
		return
	}

	mode := factoryapi.FactorySaveModeReplaceCurrent
	if req.Mode != nil {
		mode = *req.Mode
	}
	if s.guardDefinitionsRequestContext(w, r) {
		return
	}

	saved, err := factorydefinition.New(root).Save(r.Context(), string(sessionID), mode, req.Factory)
	if err != nil {
		s.writeCurrentFactoryError(w, err, "save", zap.String("session_id", string(sessionID)))
		return
	}
	s.writeJSON(w, http.StatusOK, saved)
}

func (s *Server) writeCurrentFactoryError(
	w http.ResponseWriter,
	err error,
	action string,
	fields ...zap.Field,
) {
	logFields := append([]zap.Field{zap.String("action", action)}, fields...)
	fallbackMessage := "failed to save current factory"
	if action == "get" {
		fallbackMessage = "failed to load current factory"
	}
	s.writeDefinitionsRootErrorOrInternal(w, err, fallbackMessage, logFields...)
}

func decodeSaveCurrentFactoryBody(body io.Reader) (factoryapi.SaveCurrentFactoryBySessionIdJSONRequestBody, error) {
	return decodeStrictJSON[factoryapi.SaveCurrentFactoryBySessionIdJSONRequestBody](body)
}
