package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
	cursorreader "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/cursor_reader"
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/cursor_reader/internal/cursor"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

type reader struct {
	files           providersessionsinternal.FileSystem
	walkDirectory   providersessionsinternal.CursorWalkDirectory
	resolveSymlinks providersessionsinternal.CursorResolveSymlinks
	openDatabase    providersessionsinternal.CursorOpenSQLDatabase
	root            cursor.AgentStorageRoot
}

var _ cursorreader.Service = (*reader)(nil)

// New constructs an inert Cursor reader from the exact storage effects it uses.
func New(
	files providersessionsinternal.FileSystem,
	walkDirectory providersessionsinternal.CursorWalkDirectory,
	resolveSymlinks providersessionsinternal.CursorResolveSymlinks,
	openDatabase providersessionsinternal.CursorOpenSQLDatabase,
	root string,
) (cursorreader.Service, error) {
	switch {
	case files == nil:
		return nil, fmt.Errorf("Cursor reader filesystem is required")
	case walkDirectory == nil:
		return nil, fmt.Errorf("Cursor reader directory walker is required")
	case resolveSymlinks == nil:
		return nil, fmt.Errorf("Cursor reader symlink resolver is required")
	case openDatabase == nil:
		return nil, fmt.Errorf("Cursor reader database opener is required")
	}
	return &reader{
		files:           files,
		walkDirectory:   walkDirectory,
		resolveSymlinks: resolveSymlinks,
		openDatabase:    openDatabase,
		root:            cursor.AgentStorageRoot(root),
	}, nil
}

// DefaultStorageRoot returns the configured platform's Cursor storage root
// without exposing Cursor-native root types to the Provider Sessions root.
func DefaultStorageRoot(
	resolveHome providersessionsinternal.ResolveHomeDirectory,
	files providersessionsinternal.FileSystem,
	operatingSystem providersessionsinternal.OperatingSystem,
) (string, error) {
	root, err := cursor.DefaultAgentStorageRoot(resolveHome, files, operatingSystem)
	return string(root), err
}

// Read validates the canonical Providers reference before performing storage
// discovery, then returns only normalized Provider Sessions detail.
func (r *reader) Read(ctx context.Context, ref providers.SessionRef) (providersessions.Detail, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ref.Provider != providers.IDCursor {
		return providersessions.Detail{}, providersessions.ErrUnsupportedProvider
	}
	if strings.TrimSpace(ref.Kind) != providers.SessionIDKind {
		return providersessions.Detail{}, providersessions.ErrUnsupportedKind
	}
	if strings.TrimSpace(ref.ID) == "" {
		return providersessions.Detail{}, providersessions.ErrInvalidIdentifier
	}

	detail, err := cursor.LoadDetails(
		ctx,
		r.files,
		r.walkDirectory,
		r.resolveSymlinks,
		r.openDatabase,
		r.root,
		ref.ID,
	)
	if err == nil {
		return detail, nil
	}
	switch {
	case errors.Is(err, providersessions.ErrOperationCanceled):
		return providersessions.Detail{}, providersessions.ErrOperationCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return providersessions.Detail{}, context.DeadlineExceeded
	case errors.Is(err, providersessions.ErrResourceLimitExceeded):
		return providersessions.Detail{}, &providersessions.LookupError{
			Provider: providersessions.ProviderCursor,
			Root:     string(r.root),
			Err:      err,
		}
	}
	return providersessions.Detail{}, &providersessions.LookupError{
		Provider: providersessions.ProviderCursor,
		Root:     string(r.root),
		Err:      normalizeDiscoveryError(err),
	}
}

func normalizeDiscoveryError(err error) error {
	switch {
	case errors.Is(err, providersessions.ErrInvalidIdentifier):
		return providersessions.ErrInvalidIdentifier
	case errors.Is(err, providersessions.ErrSessionNotFound):
		return providersessions.ErrSessionNotFound
	case errors.Is(err, providersessions.ErrAmbiguousSessionFile):
		return providersessions.ErrAmbiguousSessionFile
	default:
		return err
	}
}
