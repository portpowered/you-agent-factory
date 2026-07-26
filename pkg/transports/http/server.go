// Package http provides the REST API server for the agent-factory.
// It exposes endpoints for submitting work, querying token state,
// and listing workflow records.
package http

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	dashboardui "github.com/portpowered/infinite-you/ui"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

const dashboardUIIndexFile = "index.html"

var _ factoryapi.ServerInterface = (*Server)(nil)

// Server is the REST API server for the agent-factory.
type Server struct {
	*factorysessionshttp.Adapter
	modelsHTTP       *modelshttp.Handler
	providerSessions providersessions.Service
	logger           *zap.Logger
	router           *mux.Router
}

// NewServer composes an immutable generated HTTP server from dependencies
// selected by Wire and the opened Factory Session. It performs no dependency
// construction or service lookup.
func NewServer(
	factorySessionsHTTP *factorysessionshttp.Handler,
	modelsHTTP *modelshttp.Handler,
	providerSessions providersessions.Service,
	logger *zap.Logger,
) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}
	srv := &Server{
		Adapter:    factorySessionsHTTP,
		modelsHTTP: modelsHTTP, providerSessions: providerSessions, logger: logger,
	}
	srv.router = srv.buildRouter()
	return srv
}

var noModTime = time.Time{}

// Handler returns the http.Handler for testing and composition.
func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) buildRouter() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/dashboard/ui", s.handleDashboardUI).Methods("GET")
	r.PathPrefix("/dashboard/ui/").HandlerFunc(s.handleDashboardUI).Methods("GET")
	factoryapi.HandlerWithOptions(s, factoryapi.GorillaServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: s.handleGeneratedParameterError,
	})
	return r
}

func (s *Server) handleGeneratedParameterError(w http.ResponseWriter, _ *http.Request, err error) {
	s.logger.Debug("invalid generated API parameter", zap.Error(err))
	s.writeError(w, http.StatusBadRequest, "invalid request parameter", "BAD_REQUEST")
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

func (s *Server) handleDashboardUI(w http.ResponseWriter, r *http.Request) {
	distFS, err := dashboardui.DistFS()
	if err != nil {
		s.logger.Error("open embedded dashboard assets failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to load dashboard ui", "INTERNAL_ERROR")
		return
	}

	requestPath := strings.TrimPrefix(r.URL.Path, dashboardui.BasePath)
	requestPath = strings.TrimPrefix(requestPath, "/")
	requestPath = path.Clean("/" + requestPath)
	requestPath = strings.TrimPrefix(requestPath, "/")

	if requestPath == "." || requestPath == "" {
		s.serveDashboardIndex(w, r, distFS)
		return
	}

	if _, statErr := fs.Stat(distFS, requestPath); statErr == nil {
		http.StripPrefix(dashboardui.BasePath+"/", http.FileServer(http.FS(distFS))).ServeHTTP(w, r)
		return
	}

	s.serveDashboardIndex(w, r, distFS)
}

func (s *Server) serveDashboardIndex(w http.ResponseWriter, r *http.Request, distFS fs.FS) {
	indexFile, err := distFS.Open(dashboardUIIndexFile)
	if err != nil {
		s.logger.Error("open embedded dashboard index failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to load dashboard ui", "INTERNAL_ERROR")
		return
	}
	defer func() {
		if closeErr := indexFile.Close(); closeErr != nil {
			s.logger.Debug("close embedded dashboard index failed", zap.Error(closeErr))
		}
	}()

	readSeeker, ok := indexFile.(io.ReadSeeker)
	if !ok {
		s.logger.Error("embedded dashboard index is not seekable")
		s.writeError(w, http.StatusInternalServerError, "failed to load dashboard ui", "INTERNAL_ERROR")
		return
	}

	http.ServeContent(w, r, dashboardUIIndexFile, noModTime, readSeeker)
}
