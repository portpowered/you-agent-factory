package api

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	dashboardui "github.com/portpowered/infinite-you/ui"
	"go.uber.org/zap"
)

const dashboardUIIndexFile = "index.html"

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
