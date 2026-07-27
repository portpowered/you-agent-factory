package service

import (
	"context"
	"fmt"
	"path/filepath"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	codexreader "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/codex_reader"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

type reader struct {
	dependencies codexreader.Dependencies
}

var _ codexreader.Service = (*reader)(nil)

// New constructs the parent-private Codex reader from fixed storage effects.
func New(dependencies codexreader.Dependencies) (codexreader.Service, error) {
	if dependencies.Files == nil {
		return nil, fmt.Errorf("provider-session filesystem is required")
	}
	if dependencies.WalkDirectory == nil {
		return nil, fmt.Errorf("provider-session Codex directory walker is required")
	}
	if dependencies.ResolveSymlinks == nil {
		return nil, fmt.Errorf("provider-session Codex symlink resolver is required")
	}
	dependencies.SessionsRoot = filepath.Clean(dependencies.SessionsRoot)
	return &reader{dependencies: dependencies}, nil
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
	return LoadDetails(
		r.dependencies.Files,
		r.dependencies.WalkDirectory,
		r.dependencies.ResolveSymlinks,
		r.dependencies.SessionsRoot,
		session.ID,
	)
}
