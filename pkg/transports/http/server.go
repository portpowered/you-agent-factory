// Package http provides the REST API server for the agent-factory.
// It exposes endpoints for submitting work, querying token state,
// and listing workflow records.
package http

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gorilla/mux"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	dashboardui "github.com/portpowered/infinite-you/ui"
	"go.uber.org/zap"
)

const dashboardUIIndexFile = "index.html"

var _ factoryapi.ServerInterface = (*Server)(nil)

// Server is the REST API server for the agent-factory.
type Server struct {
	runtime            apisurface.RuntimeAPI
	factoryStatus      apisurface.FactoryStatusAPI
	sessions           apisurface.LiveSessionAPI
	work               apisurface.WorkAPI
	workRead           apisurface.WorkReadAPI
	invocation         apisurface.InvocationAPI
	modelsHTTP         *modelshttp.Handler
	factoryDefinitions apisurface.FactorySaveAPI
	factoryValidation  factorydefinitions.SubmittedDefinitionValidationOperation
	workflowPreview    factoryruntime.WorkflowPreviewOperation
	durableExecution   apisurface.DurableSessionExecutionAPI
	durableLifecycle   apisurface.DurableSessionLifecycleAPI
	durableListing     apisurface.DurableSessionListingAPI
	durableProjection  apisurface.DurableSessionProjectionAPI
	durableLister      DurableExecutionSessionLister
	liveSessionLister  factorysessionexecution.LiveSessionListReader
	providerSessions   providersessions.Service
	workerPrompts      workers.PromptTemplates
	contentStaging     work.ContentStagingService
	requestPreparation work.RequestPreparationService
	sessionRequests    factorysessionexecution.RequestPreparation
	logger             *zap.Logger
	router             *mux.Router
}

// NewServer composes an immutable generated HTTP server from dependencies
// selected by Wire and the opened Factory Session. It performs no dependency
// construction or service lookup.
func NewServer(
	runtime apisurface.RuntimeAPI,
	factoryStatus apisurface.FactoryStatusAPI,
	sessions apisurface.LiveSessionAPI,
	workAPI apisurface.WorkAPI,
	workRead apisurface.WorkReadAPI,
	invocation apisurface.InvocationAPI,
	modelsHTTP *modelshttp.Handler,
	factoryDefinitions apisurface.FactorySaveAPI,
	factoryValidation factorydefinitions.SubmittedDefinitionValidationOperation,
	workflowPreview factoryruntime.WorkflowPreviewOperation,
	durableExecution apisurface.DurableSessionExecutionAPI,
	durableLifecycle apisurface.DurableSessionLifecycleAPI,
	durableListing apisurface.DurableSessionListingAPI,
	durableProjection apisurface.DurableSessionProjectionAPI,
	durableLister DurableExecutionSessionLister,
	liveSessionLister factorysessionexecution.LiveSessionListReader,
	providerSessions providersessions.Service,
	workerPrompts workers.PromptTemplates,
	contentStaging work.ContentStagingService,
	requestPreparation work.RequestPreparationService,
	sessionRequests factorysessionexecution.RequestPreparation,
	logger *zap.Logger,
) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}
	srv := &Server{
		runtime: runtime, factoryStatus: factoryStatus,
		sessions: sessions, work: workAPI, workRead: workRead,
		invocation: invocation, modelsHTTP: modelsHTTP,
		factoryDefinitions: factoryDefinitions, factoryValidation: factoryValidation,
		workflowPreview:  workflowPreview,
		durableExecution: durableExecution, durableLifecycle: durableLifecycle,
		durableListing: durableListing, durableProjection: durableProjection,
		durableLister: durableLister, liveSessionLister: liveSessionLister,
		providerSessions: providerSessions, workerPrompts: workerPrompts,
		contentStaging: contentStaging, requestPreparation: requestPreparation,
		sessionRequests: sessionRequests, logger: logger,
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
