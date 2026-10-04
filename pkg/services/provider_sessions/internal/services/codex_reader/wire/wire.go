// Package wire constructs the parent-private Codex Provider Session reader.
package wire

import (
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
	codexreader "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/codex_reader"
	codexreaderservice "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/codex_reader/internal/service"
)

// NewService constructs the inert Codex reader used by Provider Sessions.
func NewService(files providersessionsinternal.FileSystem, walkDirectory providersessionsinternal.CodexWalkDirectory, resolveSymlinks providersessionsinternal.CodexResolveSymlinks, sessionsRoot string) (codexreader.Service, error) {
	return codexreaderservice.New(files, walkDirectory, resolveSymlinks, sessionsRoot)
}

// DefaultSessionsRoot returns the conventional Codex session storage root.
func DefaultSessionsRoot(resolveHome func() (string, error)) (string, error) {
	return codexreaderservice.DefaultSessionsRoot(resolveHome)
}
