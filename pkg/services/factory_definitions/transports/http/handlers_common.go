package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Error("encode response failed", zap.Error(err))
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, message, code string) {
	s.writeErrorWithTargets(w, status, message, code, nil)
}

func (s *Server) writeErrorWithTargets(
	w http.ResponseWriter,
	status int,
	message, code string,
	targets []factoryapi.FactoryValidationTarget,
) {
	var targetPtr *[]factoryapi.FactoryValidationTarget
	if len(targets) > 0 {
		targetPtr = &targets
	}
	s.writeJSON(w, status, factoryapi.ErrorResponse{
		Message: message,
		Family:  errorFamilyForStatus(status),
		Code:    factoryapi.ErrorResponseCode(code),
		Targets: targetPtr,
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

type requestFieldValidationError struct {
	message string
}

func (e requestFieldValidationError) Error() string {
	return e.message
}

func requestFieldValidationMessage(err error) (string, bool) {
	var validationErr requestFieldValidationError
	if errors.As(err, &validationErr) {
		return validationErr.message, true
	}
	return "", false
}

func ensureSingleJSONObject(dec *json.Decoder) error {
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return requestFieldValidationError{message: "request payload must contain one JSON object"}
		}
		return err
	}
	return nil
}

func decodeStrictJSON[T any](body io.Reader) (T, error) {
	var zero T
	data, err := io.ReadAll(body)
	if err != nil {
		return zero, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var req T
	if err := decoder.Decode(&req); err != nil {
		return zero, err
	}
	if err := ensureSingleJSONObject(decoder); err != nil {
		return zero, err
	}
	return req, nil
}

// ListPackagedFactories handles GET /packaged-factories from the Factory
// Definitions ownership boundary. The list and every definition payload are
// obtained through the injected Definitions root; this adapter never opens the
// embedded publication or authored package tree itself.
func (s *Server) ListPackagedFactories(w http.ResponseWriter, r *http.Request) {
	root, ok := s.requireDefinitionsRoot(w)
	if !ok {
		return
	}
	if s.guardDefinitionsRequestContext(w, r) {
		return
	}

	listed, err := root.ListBuiltInPackagedFactories(
		r.Context(),
		factorydefinitions.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil {
		if s.writeDefinitionsRequestContextOutcome(w, err) {
			return
		}
		s.writeDefinitionsRootErrorOrInternal(w, err, "failed to load packaged factory catalog")
		return
	}

	response := factoryapi.PackagedFactoryCatalogResponse{
		Factories: make([]factoryapi.PackagedFactoryCatalogEntry, 0, len(listed.Entries)),
	}
	for _, entry := range listed.Entries {
		resolved, resolveErr := root.ResolveBuiltInPackagedFactory(
			r.Context(),
			factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: entry.Name},
		)
		if resolveErr != nil {
			if s.writeDefinitionsRequestContextOutcome(w, resolveErr) {
				return
			}
			s.writeDefinitionsRootErrorOrInternal(w, resolveErr, "failed to resolve packaged factory catalog entry")
			return
		}

		mapped, mapErr := packagedFactoryCatalogEntry(entry, resolved.Definition)
		if mapErr != nil {
			s.writeDefinitionsRootErrorOrInternal(w, mapErr, "failed to decode packaged factory catalog entry")
			return
		}
		response.Factories = append(response.Factories, mapped)
	}
	s.writeJSON(w, http.StatusOK, response)
}

func packagedFactoryCatalogEntry(
	listed factorydefinitions.BuiltInPackagedFactoryEntry,
	definition factorydefinitions.PackagedDefinition,
) (factoryapi.PackagedFactoryCatalogEntry, error) {
	var document map[string]any
	if err := json.Unmarshal(definition.JSON, &document); err != nil {
		return factoryapi.PackagedFactoryCatalogEntry{}, err
	}
	var typedDocument factoryapi.Factory
	if err := json.Unmarshal(definition.JSON, &typedDocument); err != nil {
		return factoryapi.PackagedFactoryCatalogEntry{}, err
	}
	if typedDocument.Description == nil || typedDocument.Examples == nil || len(*typedDocument.Examples) == 0 {
		return factoryapi.PackagedFactoryCatalogEntry{},
			&packagedFactoryCatalogMetadataError{name: definition.Name}
	}
	yamlDocument, err := yaml.Marshal(document)
	if err != nil {
		return factoryapi.PackagedFactoryCatalogEntry{}, err
	}

	project := listed.Project
	if project == "" {
		project = definition.Project
	}
	return factoryapi.PackagedFactoryCatalogEntry{
		Name:        definition.Name,
		Project:     project,
		Slug:        strings.TrimPrefix(definition.Name, "@you/"),
		Description: *typedDocument.Description,
		Examples:    *typedDocument.Examples,
		Json:        document,
		Yaml:        string(yamlDocument),
	}, nil
}

type packagedFactoryCatalogMetadataError struct {
	name string
}

func (e *packagedFactoryCatalogMetadataError) Error() string {
	return "packaged Factory " + e.name + " has no customer-facing description/examples"
}
