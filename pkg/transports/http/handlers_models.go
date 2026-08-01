package http

import (
	"net/http"
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

// ListPackagedFactories forwards the generated route to the Factory
// Definitions-owned HTTP adapter. Top-level transport composition registers
// the route, but does not own catalog loading or response mapping.
func (s *Server) ListPackagedFactories(w http.ResponseWriter, r *http.Request) {
	if s.factoryDefinitionsHTTP == nil {
		s.writeError(w, http.StatusInternalServerError, "factory definitions API is unavailable", "INTERNAL_ERROR")
		return
	}
	s.factoryDefinitionsHTTP.ListPackagedFactories(w, r)
}
