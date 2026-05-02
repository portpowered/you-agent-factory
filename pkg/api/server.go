// Package api provides the REST API server for the agent-factory.
// It exposes endpoints for submitting work, querying token state,
// and listing workflow records.
package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"go.uber.org/zap"
)

// Server is the REST API server for the agent-factory.
type Server struct {
	runtime apisurface.APISurface
	logger  *zap.Logger
	router  *mux.Router
	port    int
}

var noModTime = time.Time{}

// NewServer creates a new API server.
func NewServer(runtime apisurface.APISurface, port int, logger *zap.Logger) *Server {
	srv := &Server{
		runtime: runtime,
		logger:  logger,
		port:    port,
	}
	srv.router = srv.buildRouter()
	return srv
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
	// Preserve the current tolerant maxResults parsing before generated integer binding.
	r.HandleFunc("/work", s.handleListWorkWithLegacyPagination).Methods("GET")
	factoryapi.HandlerWithOptions(s, factoryapi.GorillaServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: s.handleGeneratedParameterError,
	})
	return r
}

func (s *Server) handleListWorkWithLegacyPagination(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	params := factoryapi.ListWorkParams{}
	if raw := query.Get("maxResults"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			params.MaxResults = &parsed
		}
	}
	if raw := query.Get("nextToken"); raw != "" {
		params.NextToken = &raw
	}
	s.ListWork(w, r, params)
}

func (s *Server) handleGeneratedParameterError(w http.ResponseWriter, _ *http.Request, err error) {
	s.logger.Debug("invalid generated API parameter", zap.Error(err))
	s.writeError(w, http.StatusBadRequest, "invalid request parameter", "BAD_REQUEST")
}
