package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

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
		yamlDocument, err := yaml.Marshal(document)
		if err != nil {
			s.logger.Error("encode packaged factory YAML artifact", zap.Error(err))
			s.writeError(w, http.StatusInternalServerError, "failed to load packaged factory catalog", "INTERNAL_ERROR")
			return
		}
		response.Factories = append(response.Factories, factoryapi.PackagedFactoryCatalogEntry{
			Name: definition.Name, Project: definition.Project,
			Slug: strings.TrimPrefix(definition.Name, "@you/"), Json: document, Yaml: string(yamlDocument),
		})
	}
	s.writeJSON(w, http.StatusOK, response)
}
