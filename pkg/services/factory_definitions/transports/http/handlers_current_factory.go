package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/factorydefinition"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
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

	decoded, err := decodeJSONWithDiagnostics[factoryapi.SaveCurrentFactoryBySessionIdJSONRequestBody](r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeErrorWithTargets(
				w,
				http.StatusBadRequest,
				message,
				"BAD_REQUEST",
				[]factoryapi.FactoryValidationTarget{
					apisurface.FactoryValidationTargetToAPI(factorydefinitions.FormFactoryPayloadValidationTarget()),
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
				apisurface.FactoryValidationTargetToAPI(factorydefinitions.FormFactoryPayloadValidationTarget()),
			},
		)
		return
	}
	req := decoded.Value

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
	s.writeCompatibilityWarning(w, "save_current_factory", decoded.Diagnostics.Paths())
	s.writeJSON(w, http.StatusOK, saved)
}

// ListPackagedFactories handles the read-only built-in Factory catalog used by
// the dashboard. The adapter consumes detached catalog results from the
// Factory Definitions root and never reads publication-package files itself.
func (s *Server) ListPackagedFactories(w http.ResponseWriter, r *http.Request) {
	root, ok := s.requireDefinitionsRoot(w)
	if !ok || s.guardDefinitionsRequestContext(w, r) {
		return
	}

	listed, err := root.ListBuiltInPackagedFactories(
		r.Context(),
		factorydefinitions.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil {
		s.writePackagedFactoryCatalogFailure(w, err, "list")
		return
	}

	entries := make([]factoryapi.PackagedFactoryCatalogEntry, 0, len(listed.Entries))
	for _, listedEntry := range listed.Entries {
		resolved, err := root.ResolveBuiltInPackagedFactory(
			r.Context(),
			factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: listedEntry.Name},
		)
		if err != nil {
			s.writePackagedFactoryCatalogFailure(w, err, "resolve", listedEntry.Name)
			return
		}
		if resolved.Definition.Name != listedEntry.Name || resolved.Definition.Project != listedEntry.Project {
			s.writePackagedFactoryCatalogFailure(
				w,
				fmt.Errorf("catalog identity mismatch"),
				"map",
				listedEntry.Name,
			)
			return
		}

		entry, err := packagedFactoryCatalogEntry(resolved.Definition)
		if err != nil {
			s.writePackagedFactoryCatalogFailure(w, err, "map", listedEntry.Name)
			return
		}
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	s.writeJSON(w, http.StatusOK, factoryapi.PackagedFactoryCatalogResponse{Factories: entries})
}

func packagedFactoryCatalogEntry(
	definition factorydefinitions.PackagedDefinition,
) (factoryapi.PackagedFactoryCatalogEntry, error) {
	if strings.TrimSpace(definition.Name) == "" || strings.TrimSpace(definition.Project) == "" {
		return factoryapi.PackagedFactoryCatalogEntry{}, fmt.Errorf("packaged Factory identity is incomplete")
	}
	if len(definition.JSON) == 0 || len(definition.YAML) == 0 {
		return factoryapi.PackagedFactoryCatalogEntry{}, fmt.Errorf(
			"packaged Factory %q has an incomplete artifact pair",
			definition.Name,
		)
	}

	var document map[string]interface{}
	if err := json.Unmarshal(definition.JSON, &document); err != nil {
		return factoryapi.PackagedFactoryCatalogEntry{}, fmt.Errorf(
			"packaged Factory %q JSON artifact cannot be decoded",
			definition.Name,
		)
	}
	var typed factoryapi.Factory
	if err := json.Unmarshal(definition.JSON, &typed); err != nil || typed.Description == nil || typed.Examples == nil || len(*typed.Examples) == 0 {
		return factoryapi.PackagedFactoryCatalogEntry{}, fmt.Errorf(
			"packaged Factory %q discovery metadata is incomplete",
			definition.Name,
		)
	}

	return factoryapi.PackagedFactoryCatalogEntry{
		Name:        definition.Name,
		Project:     definition.Project,
		Slug:        strings.TrimPrefix(definition.Name, "@you/"),
		Description: *typed.Description,
		Examples:    *typed.Examples,
		Json:        document,
		Yaml:        string(definition.YAML),
	}, nil
}

func (s *Server) writePackagedFactoryCatalogFailure(
	w http.ResponseWriter,
	err error,
	operation string,
	name ...string,
) {
	if s.writeDefinitionsRootError(w, err) {
		return
	}
	fields := []zap.Field{
		zap.String("operation", operation),
		zap.String("error_type", fmt.Sprintf("%T", err)),
		zap.String("next_action", "verify packaged Factory catalog artifacts and regenerate the published catalog"),
	}
	if len(name) > 0 && name[0] != "" {
		fields = append(fields, zap.String("factory", name[0]))
	}
	// Keep the log actionable without serializing the failed Factory payload or
	// the underlying error text into an operator-visible diagnostic.
	s.logger.Error("packaged Factory catalog operation failed", fields...)
	s.writeError(w, http.StatusInternalServerError, "failed to load packaged factory catalog", "INTERNAL_ERROR")
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
