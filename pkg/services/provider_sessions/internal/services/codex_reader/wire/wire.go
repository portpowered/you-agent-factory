// Package wire constructs the parent-private Codex Provider Session reader.
package wire

import (
	codexreader "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/codex_reader"
	codexreaderservice "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/codex_reader/internal/service"
)

// NewService constructs the inert Codex reader used by Provider Sessions.
func NewService(dependencies codexreader.Dependencies) (codexreader.Service, error) {
	return codexreaderservice.New(dependencies)
}

// DefaultSessionsRoot returns the conventional Codex session storage root.
func DefaultSessionsRoot(resolveHome func() (string, error)) (string, error) {
	return codexreaderservice.DefaultSessionsRoot(resolveHome)
}
