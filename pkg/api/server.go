// Package api provides the REST API server for the agent-factory.
// It exposes endpoints for submitting work, querying token state,
// and listing workflow records.
package api

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	dashboardui "github.com/portpowered/infinite-you/ui"
	"go.uber.org/zap"
)

const dashboardUIIndexFile = "index.html"

var _ factoryapi.ServerInterface = (*Server)(nil)

// Server is the REST API server for the agent-factory.
type Server struct {
	runtime           apisurface.APISurface
	sessionRuntime    apisurface.SessionAPISurface
	logger            *zap.Logger
	router            *mux.Router
	port              int
	codexSessionsRoot string
}

var noModTime = time.Time{}

// NewServer creates a new API server.
func NewServer(runtime apisurface.APISurface, port int, logger *zap.Logger) *Server {
	return NewServerWithOptions(runtime, port, logger, ServerOptions{})
}

// ServerOptions configures optional API server boundaries.
type ServerOptions struct {
	CodexSessionsRoot string
}

// NewServerWithOptions creates a new API server with explicit runtime
// boundaries for tests and embedding processes.
func NewServerWithOptions(runtime apisurface.APISurface, port int, logger *zap.Logger, opts ServerOptions) *Server {
	srv := &Server{
		runtime:           runtime,
		logger:            logger,
		port:              port,
		codexSessionsRoot: normalizeCodexSessionsRoot(opts.CodexSessionsRoot),
	}
	if sessionRuntime, ok := runtime.(apisurface.SessionAPISurface); ok {
		srv.sessionRuntime = sessionRuntime
	}
	srv.router = srv.buildRouter()
	return srv
}

func normalizeCodexSessionsRoot(root string) string {
	if root != "" {
		return filepath.Clean(root)
	}
	return defaultCodexSessionsRoot()
}

// Handler returns the http.Handler for testing and composition.
func (s *Server) Handler() http.Handler {
	return s.router
}

// ListenAndServe starts the HTTP server. Blocks until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, listener)
}

// Serve starts the HTTP server on an already-bound listener. Blocks until ctx
// is cancelled or the server fails.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	httpSrv := &http.Server{
		Handler: s.router,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("API server starting", zap.String("addr", listener.Addr().String()))
		if err := httpSrv.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		return httpSrv.Close()
	case err := <-errCh:
		return err
	}
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

//go:generate go tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=../../api/codegen_config/server.yaml ../../api/openapi.yaml
