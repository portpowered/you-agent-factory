// Package http provides the REST API server for the agent-factory.
// It exposes endpoints for submitting work, querying token state,
// and listing workflow records.
package http

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gorilla/mux"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	dashboardui "github.com/portpowered/infinite-you/ui"
	"go.uber.org/zap"
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

// ForwarderHandlers is the generated HTTP surface expressed as owner-handler
// callbacks. Wire supplies these callbacks from owner-local adapters once; the
// top-level transport only forwards the original protocol values.
//
// The callbacks intentionally use generated request parameters rather than
// service request types. This keeps route registration and owner mapping
// separate while preserving the generated contract at the forwarding edge.
type ForwarderHandlers struct {
	PreviewFactory                                                func(http.ResponseWriter, *http.Request)
	ListFactorySessions                                           func(http.ResponseWriter, *http.Request, factoryapi.ListFactorySessionsParams)
	OpenFactorySession                                            func(http.ResponseWriter, *http.Request)
	StartDurableFactorySessionAsync                               func(http.ResponseWriter, *http.Request)
	StartDurableFactorySessionSync                                func(http.ResponseWriter, *http.Request)
	CloseFactorySession                                           func(http.ResponseWriter, *http.Request, string)
	GetFactorySession                                             func(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	ApproveFactorySession                                         func(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	ListFactorySessionArtifacts                                   func(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	GetFactorySessionArtifact                                     func(http.ResponseWriter, *http.Request, factoryapi.SessionID, factoryapi.ArtifactID)
	CancelFactorySession                                          func(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	ListFactorySessionDispatches                                  func(http.ResponseWriter, *http.Request, factoryapi.SessionID, factoryapi.ListFactorySessionDispatchesParams)
	GetFactorySessionDispatch                                     func(http.ResponseWriter, *http.Request, factoryapi.SessionID, factoryapi.DispatchID)
	GetEventsBySessionId                                          func(http.ResponseWriter, *http.Request, factoryapi.SessionID, factoryapi.GetEventsBySessionIdParams)
	GetCurrentFactoryBySessionId                                  func(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	SaveCurrentFactoryBySessionId                                 func(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	GetCurrentFactoryWorkstationPromptTemplateContractBySessionId func(http.ResponseWriter, *http.Request, factoryapi.SessionID, string)
	ValidateCurrentFactoryWorkstationPromptTemplateBySessionId    func(http.ResponseWriter, *http.Request, factoryapi.SessionID, string)
	InterruptFactorySessionDispatch                               func(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	InvokeFactorySessionBySessionId                               func(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	GetFactorySessionPartialResult                                func(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	PauseFactorySession                                           func(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	GetFactoryResponseEventsBySessionId                           func(http.ResponseWriter, *http.Request, factoryapi.SessionID, factoryapi.GetFactoryResponseEventsBySessionIdParams)
	GetFactorySessionResult                                       func(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	GetFactorySessionResults                                      func(http.ResponseWriter, *http.Request, factoryapi.SessionID, factoryapi.GetFactorySessionResultsParams)
	ResumeFactorySession                                          func(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	RetryFactorySessionDispatch                                   func(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	GetStatusBySessionId                                          func(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	GetFactorySessionSyncPreflightBySessionId                     func(http.ResponseWriter, *http.Request, factoryapi.SessionID, factoryapi.GetFactorySessionSyncPreflightBySessionIdParams)
	TerminateFactorySession                                       func(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	ListWorkBySessionId                                           func(http.ResponseWriter, *http.Request, factoryapi.SessionID, factoryapi.ListWorkBySessionIdParams)
	SubmitWorkBySessionId                                         func(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	UpsertWorkRequestBySessionId                                  func(http.ResponseWriter, *http.Request, factoryapi.SessionID, string)
	StageSubmitWorkFileBySessionId                                func(http.ResponseWriter, *http.Request, factoryapi.SessionID)
	GetWorkBySessionId                                            func(http.ResponseWriter, *http.Request, factoryapi.SessionID, factoryapi.WorkOrTokenID)
	MoveWorkBySessionId                                           func(http.ResponseWriter, *http.Request, factoryapi.SessionID, factoryapi.WorkOrTokenID)
	ValidateFactory                                               func(http.ResponseWriter, *http.Request)
	ListModels                                                    func(http.ResponseWriter, *http.Request)
	GetModel                                                      func(http.ResponseWriter, *http.Request, string)
	InvokeModel                                                   func(http.ResponseWriter, *http.Request, string)
	PullModel                                                     func(http.ResponseWriter, *http.Request, string)
	ListPackagedFactories                                         func(http.ResponseWriter, *http.Request)
	GetProviderSessionDetails                                     func(http.ResponseWriter, *http.Request, factoryapi.GetProviderSessionDetailsParams)
	GetStatus                                                     func(http.ResponseWriter, *http.Request)
}

// Forwarder is the process-scoped generated HTTP aggregate. It has no owner
// roots, mapping callbacks, service tables, or request decoding policy.
type Forwarder struct {
	handlers ForwarderHandlers
}

var _ factoryapi.ServerInterface = (*Forwarder)(nil)

// NewForwarder validates and snapshots one complete generated route surface.
// The returned value is safe to reuse for multiple HTTP server instances and
// requests because it retains only function values supplied by Wire.
func NewForwarder(handlers ForwarderHandlers) (*Forwarder, error) {
	for _, required := range []struct {
		name string
		set  bool
	}{
		{"PreviewFactory", handlers.PreviewFactory != nil},
		{"ListFactorySessions", handlers.ListFactorySessions != nil},
		{"OpenFactorySession", handlers.OpenFactorySession != nil},
		{"StartDurableFactorySessionAsync", handlers.StartDurableFactorySessionAsync != nil},
		{"StartDurableFactorySessionSync", handlers.StartDurableFactorySessionSync != nil},
		{"CloseFactorySession", handlers.CloseFactorySession != nil},
		{"GetFactorySession", handlers.GetFactorySession != nil},
		{"ApproveFactorySession", handlers.ApproveFactorySession != nil},
		{"ListFactorySessionArtifacts", handlers.ListFactorySessionArtifacts != nil},
		{"GetFactorySessionArtifact", handlers.GetFactorySessionArtifact != nil},
		{"CancelFactorySession", handlers.CancelFactorySession != nil},
		{"ListFactorySessionDispatches", handlers.ListFactorySessionDispatches != nil},
		{"GetFactorySessionDispatch", handlers.GetFactorySessionDispatch != nil},
		{"GetEventsBySessionId", handlers.GetEventsBySessionId != nil},
		{"GetCurrentFactoryBySessionId", handlers.GetCurrentFactoryBySessionId != nil},
		{"SaveCurrentFactoryBySessionId", handlers.SaveCurrentFactoryBySessionId != nil},
		{"GetCurrentFactoryWorkstationPromptTemplateContractBySessionId", handlers.GetCurrentFactoryWorkstationPromptTemplateContractBySessionId != nil},
		{"ValidateCurrentFactoryWorkstationPromptTemplateBySessionId", handlers.ValidateCurrentFactoryWorkstationPromptTemplateBySessionId != nil},
		{"InterruptFactorySessionDispatch", handlers.InterruptFactorySessionDispatch != nil},
		{"InvokeFactorySessionBySessionId", handlers.InvokeFactorySessionBySessionId != nil},
		{"GetFactorySessionPartialResult", handlers.GetFactorySessionPartialResult != nil},
		{"PauseFactorySession", handlers.PauseFactorySession != nil},
		{"GetFactoryResponseEventsBySessionId", handlers.GetFactoryResponseEventsBySessionId != nil},
		{"GetFactorySessionResult", handlers.GetFactorySessionResult != nil},
		{"GetFactorySessionResults", handlers.GetFactorySessionResults != nil},
		{"ResumeFactorySession", handlers.ResumeFactorySession != nil},
		{"RetryFactorySessionDispatch", handlers.RetryFactorySessionDispatch != nil},
		{"GetStatusBySessionId", handlers.GetStatusBySessionId != nil},
		{"GetFactorySessionSyncPreflightBySessionId", handlers.GetFactorySessionSyncPreflightBySessionId != nil},
		{"TerminateFactorySession", handlers.TerminateFactorySession != nil},
		{"ListWorkBySessionId", handlers.ListWorkBySessionId != nil},
		{"SubmitWorkBySessionId", handlers.SubmitWorkBySessionId != nil},
		{"UpsertWorkRequestBySessionId", handlers.UpsertWorkRequestBySessionId != nil},
		{"StageSubmitWorkFileBySessionId", handlers.StageSubmitWorkFileBySessionId != nil},
		{"GetWorkBySessionId", handlers.GetWorkBySessionId != nil},
		{"MoveWorkBySessionId", handlers.MoveWorkBySessionId != nil},
		{"ValidateFactory", handlers.ValidateFactory != nil},
		{"ListModels", handlers.ListModels != nil},
		{"GetModel", handlers.GetModel != nil},
		{"InvokeModel", handlers.InvokeModel != nil},
		{"PullModel", handlers.PullModel != nil},
		{"ListPackagedFactories", handlers.ListPackagedFactories != nil},
		{"GetProviderSessionDetails", handlers.GetProviderSessionDetails != nil},
		{"GetStatus", handlers.GetStatus != nil},
	} {
		if !required.set {
			return nil, fmt.Errorf("construct HTTP forwarder: %s handler is required", required.name)
		}
	}
	return &Forwarder{handlers: handlers}, nil
}

// NewComposedHandler registers the generated routes around one already-built
// forwarder. It is the protocol-only HTTP composition entry point intended for
// Wire; it does not create an owner adapter or inspect a request body.
func NewComposedHandler(forwarder factoryapi.ServerInterface) (http.Handler, error) {
	if forwarder == nil {
		return nil, fmt.Errorf("construct HTTP router: forwarder is required")
	}
	router := mux.NewRouter()
	router.NotFoundHandler = http.NotFoundHandler()
	router.MethodNotAllowedHandler = http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusMethodNotAllowed)
	})
	factoryapi.HandlerWithOptions(forwarder, factoryapi.GorillaServerOptions{BaseRouter: router})
	return router, nil
}

func (f *Forwarder) PreviewFactory(w http.ResponseWriter, r *http.Request) {
	f.handlers.PreviewFactory(w, r)
}
func (f *Forwarder) ListFactorySessions(w http.ResponseWriter, r *http.Request, p factoryapi.ListFactorySessionsParams) {
	f.handlers.ListFactorySessions(w, r, p)
}
func (f *Forwarder) OpenFactorySession(w http.ResponseWriter, r *http.Request) {
	f.handlers.OpenFactorySession(w, r)
}
func (f *Forwarder) StartDurableFactorySessionAsync(w http.ResponseWriter, r *http.Request) {
	f.handlers.StartDurableFactorySessionAsync(w, r)
}
func (f *Forwarder) StartDurableFactorySessionSync(w http.ResponseWriter, r *http.Request) {
	f.handlers.StartDurableFactorySessionSync(w, r)
}
func (f *Forwarder) CloseFactorySession(w http.ResponseWriter, r *http.Request, id string) {
	f.handlers.CloseFactorySession(w, r, id)
}
func (f *Forwarder) GetFactorySession(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID) {
	f.handlers.GetFactorySession(w, r, id)
}
func (f *Forwarder) ApproveFactorySession(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID) {
	f.handlers.ApproveFactorySession(w, r, id)
}
func (f *Forwarder) ListFactorySessionArtifacts(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID) {
	f.handlers.ListFactorySessionArtifacts(w, r, id)
}
func (f *Forwarder) GetFactorySessionArtifact(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, artifactID factoryapi.ArtifactID) {
	f.handlers.GetFactorySessionArtifact(w, r, sessionID, artifactID)
}
func (f *Forwarder) CancelFactorySession(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID) {
	f.handlers.CancelFactorySession(w, r, id)
}
func (f *Forwarder) ListFactorySessionDispatches(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID, p factoryapi.ListFactorySessionDispatchesParams) {
	f.handlers.ListFactorySessionDispatches(w, r, id, p)
}
func (f *Forwarder) GetFactorySessionDispatch(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, dispatchID factoryapi.DispatchID) {
	f.handlers.GetFactorySessionDispatch(w, r, sessionID, dispatchID)
}
func (f *Forwarder) GetEventsBySessionId(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID, p factoryapi.GetEventsBySessionIdParams) {
	f.handlers.GetEventsBySessionId(w, r, id, p)
}
func (f *Forwarder) GetCurrentFactoryBySessionId(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID) {
	f.handlers.GetCurrentFactoryBySessionId(w, r, id)
}
func (f *Forwarder) SaveCurrentFactoryBySessionId(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID) {
	f.handlers.SaveCurrentFactoryBySessionId(w, r, id)
}
func (f *Forwarder) GetCurrentFactoryWorkstationPromptTemplateContractBySessionId(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID, workstation string) {
	f.handlers.GetCurrentFactoryWorkstationPromptTemplateContractBySessionId(w, r, id, workstation)
}
func (f *Forwarder) ValidateCurrentFactoryWorkstationPromptTemplateBySessionId(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID, workstation string) {
	f.handlers.ValidateCurrentFactoryWorkstationPromptTemplateBySessionId(w, r, id, workstation)
}
func (f *Forwarder) InterruptFactorySessionDispatch(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID) {
	f.handlers.InterruptFactorySessionDispatch(w, r, id)
}
func (f *Forwarder) InvokeFactorySessionBySessionId(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID) {
	f.handlers.InvokeFactorySessionBySessionId(w, r, id)
}
func (f *Forwarder) GetFactorySessionPartialResult(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID) {
	f.handlers.GetFactorySessionPartialResult(w, r, id)
}
func (f *Forwarder) PauseFactorySession(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID) {
	f.handlers.PauseFactorySession(w, r, id)
}
func (f *Forwarder) GetFactoryResponseEventsBySessionId(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID, p factoryapi.GetFactoryResponseEventsBySessionIdParams) {
	f.handlers.GetFactoryResponseEventsBySessionId(w, r, id, p)
}
func (f *Forwarder) GetFactorySessionResult(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID) {
	f.handlers.GetFactorySessionResult(w, r, id)
}
func (f *Forwarder) GetFactorySessionResults(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID, p factoryapi.GetFactorySessionResultsParams) {
	f.handlers.GetFactorySessionResults(w, r, id, p)
}
func (f *Forwarder) ResumeFactorySession(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID) {
	f.handlers.ResumeFactorySession(w, r, id)
}
func (f *Forwarder) RetryFactorySessionDispatch(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID) {
	f.handlers.RetryFactorySessionDispatch(w, r, id)
}
func (f *Forwarder) GetStatusBySessionId(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID) {
	f.handlers.GetStatusBySessionId(w, r, id)
}
func (f *Forwarder) GetFactorySessionSyncPreflightBySessionId(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID, p factoryapi.GetFactorySessionSyncPreflightBySessionIdParams) {
	f.handlers.GetFactorySessionSyncPreflightBySessionId(w, r, id, p)
}
func (f *Forwarder) TerminateFactorySession(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID) {
	f.handlers.TerminateFactorySession(w, r, id)
}
func (f *Forwarder) ListWorkBySessionId(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID, p factoryapi.ListWorkBySessionIdParams) {
	f.handlers.ListWorkBySessionId(w, r, id, p)
}
func (f *Forwarder) SubmitWorkBySessionId(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID) {
	f.handlers.SubmitWorkBySessionId(w, r, id)
}
func (f *Forwarder) UpsertWorkRequestBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, requestID string) {
	f.handlers.UpsertWorkRequestBySessionId(w, r, sessionID, requestID)
}
func (f *Forwarder) StageSubmitWorkFileBySessionId(w http.ResponseWriter, r *http.Request, id factoryapi.SessionID) {
	f.handlers.StageSubmitWorkFileBySessionId(w, r, id)
}
func (f *Forwarder) GetWorkBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, id factoryapi.WorkOrTokenID) {
	f.handlers.GetWorkBySessionId(w, r, sessionID, id)
}
func (f *Forwarder) MoveWorkBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, id factoryapi.WorkOrTokenID) {
	f.handlers.MoveWorkBySessionId(w, r, sessionID, id)
}
func (f *Forwarder) ValidateFactory(w http.ResponseWriter, r *http.Request) {
	f.handlers.ValidateFactory(w, r)
}
func (f *Forwarder) ListModels(w http.ResponseWriter, r *http.Request) {
	f.handlers.ListModels(w, r)
}
func (f *Forwarder) GetModel(w http.ResponseWriter, r *http.Request, name string) {
	f.handlers.GetModel(w, r, name)
}
func (f *Forwarder) InvokeModel(w http.ResponseWriter, r *http.Request, name string) {
	f.handlers.InvokeModel(w, r, name)
}
func (f *Forwarder) PullModel(w http.ResponseWriter, r *http.Request, name string) {
	f.handlers.PullModel(w, r, name)
}
func (f *Forwarder) ListPackagedFactories(w http.ResponseWriter, r *http.Request) {
	f.handlers.ListPackagedFactories(w, r)
}
func (f *Forwarder) GetProviderSessionDetails(w http.ResponseWriter, r *http.Request, p factoryapi.GetProviderSessionDetailsParams) {
	f.handlers.GetProviderSessionDetails(w, r, p)
}
func (f *Forwarder) GetStatus(w http.ResponseWriter, r *http.Request) {
	f.handlers.GetStatus(w, r)
}
