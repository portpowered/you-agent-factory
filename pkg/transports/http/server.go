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
	factorydefinitionshttp "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/http"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	providersessionshttp "github.com/portpowered/infinite-you/pkg/services/provider_sessions/transports/http"
	workhttp "github.com/portpowered/infinite-you/pkg/services/work/transports/http"
	workersessionshttp "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	dashboardui "github.com/portpowered/infinite-you/ui"
	"go.uber.org/zap"
)

const dashboardUIIndexFile = "index.html"

var _ factoryapi.ServerInterface = (*Server)(nil)

// Server is the REST API server for the agent-factory.
type Server struct {
	*factorySessionsAdapter
	*workAdapter
	factoryDefinitionsHTTP *factorydefinitionshttp.Handler
	modelsHTTP             *modelshttp.Handler
	providerSessionsHTTP   *providersessionshttp.Handler
	workerSessionsHTTP     *workersessionshttp.Handler
	logger                 *zap.Logger
	router                 *mux.Router
}

type factorySessionsAdapter struct{ *factorysessionshttp.Adapter }
type workAdapter struct{ *workhttp.Adapter }

// NewServer composes an immutable generated HTTP server from dependencies
// selected by Wire and the opened Factory Session. It performs no dependency
// construction or service lookup.
func NewServer(
	factorySessionsHTTP *factorysessionshttp.Handler,
	workHTTP *workhttp.Adapter,
	modelsHTTP *modelshttp.Handler,
	providerSessionsHTTP *providersessionshttp.Handler,
	factoryDefinitionsHTTP *factorydefinitionshttp.Handler,
	logger *zap.Logger,
	workerSessions ...*workersessionshttp.Handler,
) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}
	srv := &Server{
		factorySessionsAdapter: &factorySessionsAdapter{Adapter: factorySessionsHTTP},
		workAdapter:            &workAdapter{Adapter: workHTTP},
		factoryDefinitionsHTTP: factoryDefinitionsHTTP,
		modelsHTTP:             modelsHTTP, providerSessionsHTTP: providerSessionsHTTP, logger: logger,
	}
	if len(workerSessions) > 0 {
		srv.workerSessionsHTTP = workerSessions[0]
	}
	srv.router = srv.buildRouter()
	return srv
}

// StartWorkerSession forwards the global asynchronous start operation to the
// Worker Sessions owner handler.
func (s *Server) StartWorkerSession(w http.ResponseWriter, r *http.Request) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.StartWorkerSession(w, r)
}

// ContinueWorkerSession forwards the source-addressed continuation operation
// to the Worker Sessions owner handler.
func (s *Server) ContinueWorkerSession(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.ContinueWorkerSession(w, r, workerSessionID)
}

// InterruptWorkerSession forwards the source-addressed interrupt-and-replace
// operation to the Worker Sessions owner handler.
func (s *Server) InterruptWorkerSession(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.InterruptWorkerSession(w, r, workerSessionID)
}

// PauseWorkerSession forwards the source-addressed pause control to the
// Worker Sessions owner handler.
func (s *Server) PauseWorkerSession(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.PauseWorkerSession(w, r, workerSessionID)
}

// ResumeWorkerSession forwards the source-addressed resume control to the
// Worker Sessions owner handler.
func (s *Server) ResumeWorkerSession(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.ResumeWorkerSession(w, r, workerSessionID)
}

// CancelWorkerSession forwards the source-addressed cancel control to the
// Worker Sessions owner handler.
func (s *Server) CancelWorkerSession(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.CancelWorkerSession(w, r, workerSessionID)
}

// TerminateWorkerSession forwards the source-addressed terminate control to
// the Worker Sessions owner handler.
func (s *Server) TerminateWorkerSession(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.TerminateWorkerSession(w, r, workerSessionID)
}

// ListWorkerSessions forwards the top-level Worker Session observation list.
func (s *Server) ListWorkerSessions(
	w http.ResponseWriter,
	r *http.Request,
	params factoryapi.ListWorkerSessionsParams,
) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.ListWorkerSessions(w, r, params)
}

// GetWorkerSessionObservationByWorkerSessionId forwards the top-level
// identity detail operation.
func (s *Server) GetWorkerSessionObservationByWorkerSessionId(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.GetWorkerSessionObservationByWorkerSessionId(w, r, workerSessionID)
}

// ReadWorkerSessionTranscriptByWorkerSessionId forwards top-level transcript
// projection by stable Worker Session identity.
func (s *Server) ReadWorkerSessionTranscriptByWorkerSessionId(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.ReadWorkerSessionTranscriptByWorkerSessionId(w, r, workerSessionID)
}

// StreamWorkerSessionEventsByTopLevelWorkerSessionId forwards the top-level
// identity event stream.
func (s *Server) StreamWorkerSessionEventsByTopLevelWorkerSessionId(
	w http.ResponseWriter,
	r *http.Request,
	workerSessionID factoryapi.WorkerSessionID,
	params factoryapi.StreamWorkerSessionEventsByTopLevelWorkerSessionIdParams,
) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.StreamWorkerSessionEventsByTopLevelWorkerSessionId(w, r, workerSessionID, params)
}

// ListWorkerSessionsBySessionId forwards the generated operation to the
// Worker Sessions owner handler.
func (s *Server) ListWorkerSessionsBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.ListWorkerSessionsBySessionIdParams,
) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.ListWorkerSessionsBySessionId(w, r, sessionID, params)
}

// GetWorkerSessionObservationBySessionId forwards the generated operation to
// the Worker Sessions owner handler.
func (s *Server) GetWorkerSessionObservationBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.GetWorkerSessionObservationBySessionIdParams,
) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.GetWorkerSessionObservationBySessionId(w, r, sessionID, params)
}

// GetWorkerSessionObservationByFactorySessionAndWorkerSessionId forwards the canonical
// Worker-ID observation operation to the Worker Sessions owner handler.
func (s *Server) GetWorkerSessionObservationByFactorySessionAndWorkerSessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	workerSessionID factoryapi.WorkerSessionID,
) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.GetWorkerSessionObservationByFactorySessionAndWorkerSessionId(w, r, sessionID, workerSessionID)
}

// ReadWorkerSessionTranscriptBySessionId forwards the generated operation to
// the Worker Sessions owner handler.
func (s *Server) ReadWorkerSessionTranscriptBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.ReadWorkerSessionTranscriptBySessionIdParams,
) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.ReadWorkerSessionTranscriptBySessionId(w, r, sessionID, params)
}

// ReadWorkerSessionTranscriptByFactorySessionAndWorkerSessionId forwards the canonical
// Worker-ID transcript operation to the Worker Sessions owner handler.
func (s *Server) ReadWorkerSessionTranscriptByFactorySessionAndWorkerSessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	workerSessionID factoryapi.WorkerSessionID,
) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.ReadWorkerSessionTranscriptByFactorySessionAndWorkerSessionId(w, r, sessionID, workerSessionID)
}

// StreamWorkerSessionEventsBySessionId forwards the generated operation to
// the Worker Sessions owner handler.
func (s *Server) StreamWorkerSessionEventsBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.StreamWorkerSessionEventsBySessionIdParams,
) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.StreamWorkerSessionEventsBySessionId(w, r, sessionID, params)
}

// StreamWorkerSessionEventsByWorkerSessionId forwards the provider-neutral
// Worker Session identity stream to the Worker Sessions owner handler.
func (s *Server) StreamWorkerSessionEventsByWorkerSessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	workerSessionID factoryapi.WorkerSessionID,
	params factoryapi.StreamWorkerSessionEventsByWorkerSessionIdParams,
) {
	if s.workerSessionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	s.workerSessionsHTTP.StreamWorkerSessionEventsByWorkerSessionId(w, r, sessionID, workerSessionID, params)
}

var noModTime = time.Time{}

// Handler returns the http.Handler for testing and composition.
func (s *Server) Handler() http.Handler {
	return s.router
}

// GetProviderSessionDetails forwards the generated operation to the Provider
// Sessions owner handler without changing its request or response values.
func (s *Server) GetProviderSessionDetails(
	w http.ResponseWriter,
	r *http.Request,
	params factoryapi.GetProviderSessionDetailsParams,
) {
	s.providerSessionsHTTP.GetProviderSessionDetails(w, r, params)
}

func (s *Server) buildRouter() *mux.Router {
	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(s.handleUnknownRoute)
	r.MethodNotAllowedHandler = http.HandlerFunc(s.handleDisallowedMethod)
	r.HandleFunc("/dashboard/ui", s.handleDashboardUI).Methods("GET")
	r.PathPrefix("/dashboard/ui/").HandlerFunc(s.handleDashboardUI).Methods("GET")
	factoryapi.HandlerWithOptions(s, factoryapi.GorillaServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: s.handleGeneratedParameterError,
	})
	return r
}

func (s *Server) handleUnknownRoute(w http.ResponseWriter, _ *http.Request) {
	s.writeError(w, http.StatusNotFound, "route not found", "NOT_FOUND")
}

func (s *Server) handleDisallowedMethod(w http.ResponseWriter, _ *http.Request) {
	s.writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
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
