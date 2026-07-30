package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// Model endpoint methods are generated-server composition shims. Protocol
// behavior lives with the Models service's HTTP adapter.
func (s *Server) ListModels(w http.ResponseWriter, r *http.Request) {
	s.modelsHTTP.ListModels(w, r)
}

func (s *Server) GetModel(w http.ResponseWriter, r *http.Request, modelName string) {
	s.modelsHTTP.GetModel(w, r, modelName)
}

func (s *Server) InvokeModel(w http.ResponseWriter, r *http.Request, modelName string) {
	s.modelsHTTP.InvokeModel(w, r, modelName)
}

func (s *Server) PullModel(w http.ResponseWriter, r *http.Request, modelName string) {
	s.modelsHTTP.PullModel(w, r, modelName)
}

// ListPackagedFactories exposes the backend-owned published catalog for
// dashboard inspection. The dashboard deliberately does not import package
// artifacts at build time.
func (s *Server) ListPackagedFactories(w http.ResponseWriter, _ *http.Request) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		s.logger.Error("load packaged factory catalog", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to load packaged factory catalog", "INTERNAL_ERROR")
		return
	}

	response := factoryapi.PackagedFactoryCatalogResponse{Factories: make([]factoryapi.PackagedFactoryCatalogEntry, 0, len(catalog.All()))}
	for _, definition := range catalog.All() {
		var document map[string]interface{}
		if err := json.Unmarshal(definition.JSON, &document); err != nil {
			s.logger.Error("decode packaged factory catalog artifact", zap.Error(err))
			s.writeError(w, http.StatusInternalServerError, "failed to load packaged factory catalog", "INTERNAL_ERROR")
			return
		}
		var typedDocument factoryapi.Factory
		if err := json.Unmarshal(definition.JSON, &typedDocument); err != nil || typedDocument.Description == nil || typedDocument.Examples == nil || len(*typedDocument.Examples) == 0 {
			s.logger.Error("decode packaged factory discovery metadata", zap.Error(err))
			s.writeError(w, http.StatusInternalServerError, "failed to load packaged factory catalog", "INTERNAL_ERROR")
			return
		}
		yamlDocument, err := yaml.Marshal(document)
		if err != nil {
			s.logger.Error("encode packaged factory YAML artifact", zap.Error(err))
			s.writeError(w, http.StatusInternalServerError, "failed to load packaged factory catalog", "INTERNAL_ERROR")
			return
		}
		response.Factories = append(response.Factories, factoryapi.PackagedFactoryCatalogEntry{
			Name: definition.Name, Project: definition.Project,
			Slug: strings.TrimPrefix(definition.Name, "@you/"), Description: *typedDocument.Description,
			Examples: *typedDocument.Examples, Json: document, Yaml: string(yamlDocument),
		})
	}
	s.writeJSON(w, http.StatusOK, response)
}
