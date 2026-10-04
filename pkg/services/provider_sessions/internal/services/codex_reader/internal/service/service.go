package service

import (
	"context"
	"path/filepath"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
	codexreader "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/codex_reader"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

type reader struct {
	files           providersessionsinternal.FileSystem
	walkDirectory   providersessionsinternal.CodexWalkDirectory
	resolveSymlinks providersessionsinternal.CodexResolveSymlinks
	sessionsRoot    string
}

var _ codexreader.Service = (*reader)(nil)

// New constructs the parent-private Codex reader from fixed storage effects.
// The owning Provider Sessions constructor validates the required effects.
func New(files providersessionsinternal.FileSystem, walkDirectory providersessionsinternal.CodexWalkDirectory, resolveSymlinks providersessionsinternal.CodexResolveSymlinks, sessionsRoot string) (codexreader.Service, error) {
	return &reader{
		files:           files,
		walkDirectory:   walkDirectory,
		resolveSymlinks: resolveSymlinks,
		sessionsRoot:    filepath.Clean(sessionsRoot),
	}, nil
}

func (r *reader) Details(
	ctx context.Context,
	session providers.SessionRef,
) (providersessions.Detail, error) {
	if err := ctx.Err(); err != nil {
		return providersessions.Detail{}, err
	}
	if session.Provider != providers.IDCodex {
		return providersessions.Detail{}, providersessions.ErrUnsupportedProvider
	}
	if session.Kind != providers.SessionIDKind {
		return providersessions.Detail{}, providersessions.ErrUnsupportedKind
	}
	return loadDetails(
		ctx,
		r.files,
		r.walkDirectory,
		r.resolveSymlinks,
		r.sessionsRoot,
		session.ID,
	)
}
